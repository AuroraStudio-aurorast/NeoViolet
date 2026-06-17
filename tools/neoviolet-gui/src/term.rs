use alacritty_terminal::event::WindowSize;
use alacritty_terminal::event_loop::{EventLoop, EventLoopSender, Msg};
use alacritty_terminal::index::{Column, Line, Point};
use alacritty_terminal::sync::FairMutex;
use alacritty_terminal::term::Term;
use alacritty_terminal::term::TermMode;
use alacritty_terminal::term::test::TermSize;
use alacritty_terminal::tty;
use futures::channel::mpsc::{self as fut_mpsc, UnboundedReceiver};
use std::cell::Cell;
use std::sync::Mutex;
use std::sync::mpsc;
use std::sync::Arc;
use std::thread::JoinHandle;

use crate::config::GuiConfig;
use crate::event_listener::{TermEvent, TermEventListener};
use crate::modes::Modes;

type EventLoopState = alacritty_terminal::event_loop::State;

/// Cursor blink interval: 500 ms on, 500 ms off (standard terminal rate).
/// Used by the timer task spawned in NeoVioletApp::new().
pub const CURSOR_BLINK_MS: u64 = 500;

/// A single rendered terminal cell with colors, width flag, and text attributes.
#[derive(Clone)]
pub struct RenderedCell {
    pub ch: char,
    pub fg: Option<[u8; 3]>,
    pub bg: Option<[u8; 3]>,
    /// True for CJK/wide characters occupying 2 columns.
    pub is_wide: bool,
    /// True when the cell has bold/italic/underline/strikethrough styling.
    pub bold: bool,
    pub italic: bool,
    pub underline: bool,
    pub strikethrough: bool,
}

/// Cursor shape as requested by the child process via DECSCUSR.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum CursorShape {
    Block,
    Underline,
    Beam,
    HollowBlock,
}

/// Cursor rendering state extracted from the terminal.
#[derive(Clone, Copy, Debug)]
pub struct CursorStyle {
    pub shape: CursorShape,
    pub visible: bool,
}

impl RenderedCell {
    fn empty() -> Self {
        Self {
            ch: ' ',
            fg: None,
            bg: None,
            is_wide: false,
            bold: false,
            italic: false,
            underline: false,
            strikethrough: false,
        }
    }
}

/// Content snapshot — produced under brief lock, consumed lock-free by render.
/// Follows Zed's pattern: lock → build Content → swap Arc → unlock → render from Arc.
#[derive(Clone)]
pub struct TerminalContent {
    pub lines: Arc<Vec<CachedLine>>,
    pub cols: usize,
    pub cursor: Option<(usize, usize, CursorStyle)>,
}

/// A cached line: pre-processed cells with colors resolved.
#[derive(Clone)]
pub struct CachedLine {
    pub cells: Vec<RenderedCell>,
    /// Hash-like fingerprint: count of non-space cells, for future optimization.
    #[allow(dead_code)]
    content_len: usize,
}

impl CachedLine {
    fn empty(cols: usize) -> Self {
        Self { cells: vec![RenderedCell::empty(); cols], content_len: 0 }
    }
}

/// Holds all terminal state.
pub struct TerminalState {
    term: Arc<FairMutex<Term<TermEventListener>>>,
    event_loop_handle: Option<JoinHandle<(EventLoop<tty::Pty, TermEventListener>, EventLoopState)>>,
    event_sender: EventLoopSender,
    title_rx: mpsc::Receiver<String>,
    child_exit_rx: mpsc::Receiver<Option<i32>>,
    term_event_rx: mpsc::Receiver<TermEvent>,
    pty_write_rx: mpsc::Receiver<Vec<u8>>,
    exit_code: Option<i32>,
    cols: usize,
    rows: usize,
    cell_width: u16,
    cell_height: u16,
    line_cache: Vec<CachedLine>,
    needs_full_redraw: bool,

    // ── Content snapshot (Zed pattern: Arc swap under lock, lock-free read) ──
    snapshot: Arc<Mutex<Arc<TerminalContent>>>,

    // ── Event-driven render: wake channel from PTY event listener ──
    wake_rx: UnboundedReceiver<()>,

    // ── Cursor blink state (timer-driven, interior mutability) ──
    blink_visible: Cell<bool>,

    // ── IME preedit (composition text displayed at cursor) ──
    ime_preedit: Arc<Mutex<Option<String>>>,

    // ── Locally-cached terminal modes (avoids FairMutex lock on read) ──
    modes: Modes,
}

impl TerminalState {
    /// Spawn the `neoviolet` binary in a PTY.
    pub fn spawn(gui_config: &GuiConfig, user_args: &[String]) -> Result<Self, String> {
        tty::setup_env();

        let neoviolet_bin = find_neoviolet_binary(gui_config);

        // Pre-check: verify the binary exists before handing it to the PTY layer.
        // On some platforms a missing shell binary can cause tty::new() to panic
        // rather than return Err.
        if !std::path::Path::new(&neoviolet_bin).exists() {
            return Err(format!(
                "neoviolet binary not found at '{}'",
                neoviolet_bin
            ));
        }

        let mut cmd_args: Vec<String> = Vec::new();
        #[cfg(target_os = "linux")]
        cmd_args.push("--xdg-config".to_string());
        cmd_args.extend_from_slice(user_args);

        let shell = tty::Shell::new(neoviolet_bin, cmd_args);

        let font_size = gui_config.font_size;
        let cell_width = (font_size * 2) as u16;
        let cell_height = font_size as u16;

        let cols: u16 = ((gui_config.window_width as u16) / cell_width).max(80);
        let rows: u16 = ((gui_config.window_height as u16) / cell_height).max(24);

        let window_size = WindowSize {
            num_lines: rows,
            num_cols: cols,
            cell_width,
            cell_height,
        };

        let pty = tty::new(
            &tty::Options {
                shell: Some(shell),
                drain_on_exit: true,
                ..Default::default()
            },
            window_size,
            0,
        )
        .map_err(|e| format!("Failed to create PTY: {}", e))?;

        let (title_tx, title_rx) = mpsc::channel();
        let (child_exit_tx, child_exit_rx) = mpsc::channel();
        let (term_event_tx, term_event_rx) = mpsc::channel();
        let (pty_write_tx, pty_write_rx) = mpsc::channel();
        let (wake_tx, wake_rx) = fut_mpsc::unbounded();
        let _ = title_tx.send("NeoViolet".to_string());

        let mut config = alacritty_terminal::term::Config::default();
        config.kitty_keyboard = true;
        config.osc52 = alacritty_terminal::term::Osc52::CopyPaste; // full OSC 52
        let dims = TermSize::new(cols as usize, rows as usize);
        let term = Term::new(config, &dims, TermEventListener {
            title_tx: title_tx.clone(),
            child_exit_tx: child_exit_tx.clone(),
            term_event_tx: term_event_tx.clone(),
            pty_write_tx: pty_write_tx.clone(),
            wake_tx: wake_tx.clone(),
        });

        let term = Arc::new(FairMutex::new(term));

        let event_loop = EventLoop::new(
            term.clone(),
            TermEventListener {
                title_tx,
                child_exit_tx,
                term_event_tx,
                pty_write_tx,
                wake_tx,
            },
            pty,
            true,
            false,
        )
        .map_err(|e| format!("Failed to create event loop: {}", e))?;

        let event_sender = event_loop.channel();
        let handle = event_loop.spawn();

        // Init empty cache
        let line_cache = (0..rows as usize)
            .map(|_| CachedLine::empty(cols as usize))
            .collect();

        Ok(Self {
            term,
            event_loop_handle: Some(handle),
            event_sender,
            title_rx,
            child_exit_rx,
            term_event_rx,
            pty_write_rx,
            exit_code: None,
            cols: cols as usize,
            rows: rows as usize,
            cell_width,
            cell_height,
            line_cache,
            needs_full_redraw: true,
            snapshot: Arc::new(Mutex::new(Arc::new(TerminalContent {
                lines: Arc::new(vec![]),
                cols: 0,
                cursor: None,
            }))),
            wake_rx,
            blink_visible: Cell::new(true),
            ime_preedit: Arc::new(Mutex::new(None)),
            modes: Modes::empty(),
        })
    }

    /// Get a clone of the event sender for external key forwarding.
    pub fn input_sender(&self) -> EventLoopSender {
        self.event_sender.clone()
    }

    /// Resize the terminal grid. Triggers a full redraw.
    pub fn resize(&mut self, cols: usize, rows: usize, cell_width: u16, cell_height: u16) {
        self.cols = cols;
        self.rows = rows;
        self.cell_width = cell_width;
        self.cell_height = cell_height;
        self.line_cache = (0..rows).map(|_| CachedLine::empty(cols)).collect();
        self.needs_full_redraw = true;
        self.term.lock().resize(TermSize::new(cols, rows));
        let _ = self.event_sender.send(Msg::Resize(WindowSize {
            num_lines: rows as u16,
            num_cols: cols as u16,
            cell_width,
            cell_height,
        }));
    }

    /// Check whether the underlying PTY event loop is still running.
    pub fn is_alive(&self) -> bool {
        self.event_loop_handle
            .as_ref()
            .is_none_or(|h| !h.is_finished())
    }

    // ── Incremental rendering with damage tracking ──

    /// Update the line cache and swap in a new content snapshot.
    /// Following Zed's pattern: lock grid → build content → swap Arc → unlock.
    /// Render path reads snapshot via `content_snapshot()` without holding locks.
    ///
    /// Also syncs the local Modes cache from alacritty's TermMode under the
    /// same lock to keep key/mouse encoding accurate without extra locking.
    pub fn update_and_swap_snapshot(&mut self) {
        let cols = self.cols;
        let rows = self.rows;

        let mut term_lock = self.term.lock();

        // ── Sync modes to local cache (avoids FairMutex on every keystroke) ──
        self.modes.sync_from_alacritty(*term_lock.mode());

        if self.needs_full_redraw {
            let grid = term_lock.grid();
            let colors = term_lock.colors();
            for line_idx in 0..rows {
                self.line_cache[line_idx] = Self::build_line(grid, colors, line_idx, cols);
            }
            term_lock.reset_damage();
            self.needs_full_redraw = false;
        } else {
            let damaged_indices: Vec<usize> = {
                let damaged = term_lock.damage();
                match damaged {
                    alacritty_terminal::term::TermDamage::Full => (0..rows).collect(),
                    alacritty_terminal::term::TermDamage::Partial(damage_iter) => {
                        damage_iter.map(|b| b.line).filter(|&l| l < rows).collect()
                    }
                }
            };
            let grid = term_lock.grid();
            let colors = term_lock.colors();
            for line_idx in damaged_indices {
                self.line_cache[line_idx] = Self::build_line(grid, colors, line_idx, cols);
            }
            term_lock.reset_damage();
        }

        // Cursor info under the same lock (avoids separate lock+unlock cycle)
        let cursor = if term_lock.mode().contains(TermMode::SHOW_CURSOR) {
            let point = term_lock.grid().cursor.point;
            let col = (point.column.0 as usize).min(cols.saturating_sub(1));
            let row = (point.line.0 as usize).min(rows.saturating_sub(1));
            let vte_style = term_lock.cursor_style();
            let shape = match vte_style.shape {
                alacritty_terminal::vte::ansi::CursorShape::Block => CursorShape::Block,
                alacritty_terminal::vte::ansi::CursorShape::Underline => CursorShape::Underline,
                alacritty_terminal::vte::ansi::CursorShape::Beam => CursorShape::Beam,
                alacritty_terminal::vte::ansi::CursorShape::HollowBlock => CursorShape::HollowBlock,
                _ => CursorShape::Block,
            };
            if vte_style.blinking && !self.blink_visible.get() {
                None
            } else {
                Some((col, row, CursorStyle { shape, visible: true }))
            }
        } else {
            None
        };
        drop(term_lock);

        // Atomic snapshot swap — Arc clone for render path is just a refcount bump
        *self.snapshot.lock().unwrap() = Arc::new(TerminalContent {
            lines: Arc::new(self.line_cache.clone()),
            cols,
            cursor,
        });
    }

    /// Lock-free read of the latest terminal content snapshot.
    pub fn content_snapshot(&self) -> Arc<TerminalContent> {
        self.snapshot.lock().unwrap().clone()
    }

    /// Build a single cached line from the grid by index.
    fn build_line(
        grid: &alacritty_terminal::Grid<alacritty_terminal::term::cell::Cell>,
        colors: &alacritty_terminal::term::color::Colors,
        line_idx: usize,
        cols: usize,
    ) -> CachedLine {
        let resolve = |c: alacritty_terminal::vte::ansi::Color| -> Option<[u8; 3]> {
            use alacritty_terminal::vte::ansi::Color;
            match c {
                Color::Named(n) => colors[n]
                    .map(|r| [r.r, r.g, r.b])
                    .or_else(|| Some(dracula_fallback(n as usize))),
                Color::Indexed(i) => colors[i as usize]
                    .map(|r| [r.r, r.g, r.b])
                    .or_else(|| Some(dracula_fallback(i as usize))),
                Color::Spec(r) => Some([r.r, r.g, r.b]),
            }
        };

        let mut cells = Vec::with_capacity(cols);
        let mut content_len = 0usize;
        let mut skip_spacer = false;

        for col in 0..cols {
            if skip_spacer {
                // Previous cell was a WIDE_CHAR — skip this spacer column
                skip_spacer = false;
                continue;
            }

            let cell = &grid[Point::new(Line(line_idx as i32), Column(col))];
            let flags = cell.flags;
            use alacritty_terminal::term::cell::Flags;

            // Skip WIDE_CHAR_SPACER / LEADING_WIDE_CHAR_SPACER cells
            if flags.intersects(Flags::WIDE_CHAR_SPACER | Flags::LEADING_WIDE_CHAR_SPACER) {
                continue;
            }

            let ch = cell.c;
            let fg = resolve(cell.fg);
            let bg = resolve(cell.bg);
            let is_wide = flags.contains(Flags::WIDE_CHAR);
            let bold = flags.contains(Flags::BOLD);
            let italic = flags.contains(Flags::ITALIC);
            let underline = flags.intersects(Flags::ALL_UNDERLINES);
            let strikethrough = flags.contains(Flags::STRIKEOUT);

            if ch != ' ' {
                content_len += 1;
            }
            if is_wide {
                skip_spacer = true;
            }
            cells.push(RenderedCell {
                ch,
                fg,
                bg,
                is_wide,
                bold,
                italic,
                underline,
                strikethrough,
            });
        }

        CachedLine { cells, content_len }
    }

    /// Direct access to the inner alacritty Term — used for reading modes.
    pub fn inner_term(&self) -> &Arc<FairMutex<Term<TermEventListener>>> {
        &self.term
    }

    /// Return the locally-cached terminal modes (no lock needed).
    pub fn modes(&self) -> Modes {
        self.modes
    }

    // ── Event-driven render & blink ──

    /// Take ownership of the wake receiver of the wake receiver for the event-driven render loop.
    /// Each PTY output event sends a `()` on this channel.
    pub fn take_wake_rx(&mut self) -> UnboundedReceiver<()> {
        let (_tx, rx) = fut_mpsc::unbounded();
        std::mem::replace(&mut self.wake_rx, rx)
    }

    /// Toggle cursor blink visibility. Called by a timer in the render loop.
    pub fn toggle_blink(&self) {
        self.blink_visible.set(!self.blink_visible.get());
    }

    // ── IME preedit ──

    /// Return the current IME composition text, if any.
    pub fn ime_preedit(&self) -> Option<String> {
        self.ime_preedit.lock().unwrap().clone()
    }

    /// Set the IME composition text (preedit). Pass `None` to clear.
    ///
    /// Called via AppState::set_ime_preedit → wired from platform InputHandler.
    /// The render path reads preedit via `ime_preedit()` and displays it
    /// at the cursor position with an underline.
    pub fn set_ime_preedit(&self, text: Option<String>) {
        *self.ime_preedit.lock().unwrap() = text;
    }

    // ── Title / exit ──

    pub fn poll_title(&self) -> Option<String> {
        self.title_rx.try_recv().ok()
    }

    pub fn poll_exit_code(&mut self) {
        if self.exit_code.is_none() {
            if let Ok(code) = self.child_exit_rx.try_recv() {
                self.exit_code = code;
            }
        }
    }

    pub fn exit_code(&self) -> Option<i32> {
        self.exit_code
    }

    /// Drain pending OSC events (clipboard, color queries) from alacritty.
    pub fn drain_term_events(&self) -> Vec<TermEvent> {
        self.term_event_rx.try_iter().collect()
    }

    /// Drain pending PTY write-backs from alacritty's Notify (e.g. OSC 52 responses).
    pub fn drain_pty_writes(&self) -> Vec<Vec<u8>> {
        self.pty_write_rx.try_iter().collect()
    }
}

impl Drop for TerminalState {
    fn drop(&mut self) {
        if let Some(handle) = self.event_loop_handle.take() {
            let _ = handle.join();
        }
    }
}

/// Dracula color palette fallback for ANSI named colors and 256-color palette.
///
/// Used when alacritty's `Colors` table has `None` for a given index.
pub fn dracula_fallback(index: usize) -> [u8; 3] {
    use alacritty_terminal::vte::ansi::NamedColor;

    // Standard 16 ANSI + Foreground/Background/Cursor + Dim variants
    match index {
        n if n == NamedColor::Black as usize => [0x21, 0x22, 0x2c],
        n if n == NamedColor::Red as usize => [0xff, 0x55, 0x55],
        n if n == NamedColor::Green as usize => [0x50, 0xfa, 0x7b],
        n if n == NamedColor::Yellow as usize => [0xf1, 0xfa, 0x8c],
        n if n == NamedColor::Blue as usize => [0xbd, 0x93, 0xf9],
        n if n == NamedColor::Magenta as usize => [0xff, 0x79, 0xc6],
        n if n == NamedColor::Cyan as usize => [0x8b, 0xe9, 0xfd],
        n if n == NamedColor::White as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::BrightBlack as usize => [0x62, 0x72, 0xa4],
        n if n == NamedColor::BrightRed as usize => [0xff, 0x6e, 0x6e],
        n if n == NamedColor::BrightGreen as usize => [0x69, 0xff, 0x94],
        n if n == NamedColor::BrightYellow as usize => [0xff, 0xff, 0xa5],
        n if n == NamedColor::BrightBlue as usize => [0xd6, 0xac, 0xff],
        n if n == NamedColor::BrightMagenta as usize => [0xff, 0x92, 0xdf],
        n if n == NamedColor::BrightCyan as usize => [0xa4, 0xff, 0xff],
        n if n == NamedColor::BrightWhite as usize => [0xff, 0xff, 0xff],
        n if n == NamedColor::Foreground as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::Background as usize => [0x28, 0x2a, 0x36],
        n if n == NamedColor::Cursor as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::DimBlack as usize => [0x21, 0x22, 0x2c],
        n if n == NamedColor::DimRed as usize => [0xff, 0x55, 0x55],
        n if n == NamedColor::DimGreen as usize => [0x50, 0xfa, 0x7b],
        n if n == NamedColor::DimYellow as usize => [0xf1, 0xfa, 0x8c],
        n if n == NamedColor::DimBlue as usize => [0xbd, 0x93, 0xf9],
        n if n == NamedColor::DimMagenta as usize => [0xff, 0x79, 0xc6],
        n if n == NamedColor::DimCyan as usize => [0x8b, 0xe9, 0xfd],
        n if n == NamedColor::DimWhite as usize => [0xf8, 0xf8, 0xf2],

        // Standard xterm 6×6×6 color cube (indices 16-231)
        i if (16..=231).contains(&i) => {
            let v = (i - 16) as u8;
            let (r, g, b) = (v / 36, (v / 6) % 6, v % 6);
            let level = |c: u8| -> u8 {
                if c == 0 { 0 } else { (c * 40 + 55).min(255) }
            };
            [level(r), level(g), level(b)]
        }

        // Standard xterm grayscale (indices 232-255)
        i if (232..=255).contains(&i) => {
            let level = ((i - 232) as u8 * 10 + 8).min(255);
            [level, level, level]
        }

        _ => [0xf8, 0xf8, 0xf2], // default foreground
    }
}

/// Locate the `neoviolet` binary.
fn find_neoviolet_binary(gui_config: &GuiConfig) -> String {
    if let Some(ref path) = gui_config.neoviolet_path
        && std::path::Path::new(path).exists()
    {
        return path.clone();
    }

    if let Ok(exe) = std::env::current_exe()
        && let Some(dir) = exe.parent()
    {
        let candidate = dir.join("neoviolet");
        if candidate.exists() {
            return candidate.to_string_lossy().to_string();
        }
    }

    "neoviolet".to_string()
}
