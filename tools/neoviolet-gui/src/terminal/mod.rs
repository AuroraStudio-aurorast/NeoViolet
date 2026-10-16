pub mod custom_blocks;
pub mod element;

use std::sync::mpsc::Sender;

use alacritty_terminal::{
    event::{Event, EventListener},
    grid::{Dimensions, Scroll},
    index::{Column, Point, Side},
    selection::{Selection, SelectionRange, SelectionType},
    term::{Config, Term, TermMode, cell::Cell, point_to_viewport, viewport_to_point},
    vte::ansi::{CursorShape, Processor},
};
use gpui::Keystroke;

#[allow(dead_code)]
#[derive(Debug)]
pub enum BackendCommand {
    Input(Vec<u8>),
    Resize { cols: u16, rows: u16 },
    Close,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub enum BackendEvent {
    Output { tab_id: String, bytes: Vec<u8> },
    Status { tab_id: String, text: String },
    Closed { tab_id: String, reason: String },
    TerminalTitleChanged { tab_id: String, title: String },
}

// ── Paste encoding ──

/// Frame opening a bracketed paste.
pub(crate) const PASTE_START: &[u8] = b"\x1b[200~";
/// Frame closing a bracketed paste.
pub(crate) const PASTE_END: &[u8] = b"\x1b[201~";

/// Returns `(cell_width, line_height)` in logical pixels.
///
/// Measured values use defensive floors; failed measurements preserve the
/// historical font-size ratios and their original minimums.
pub(crate) fn metrics_from_measurement(
    cell_width: Option<f32>,
    ascent: Option<f32>,
    descent: Option<f32>,
    font_size: f32,
) -> (f32, f32) {
    let cell_width = cell_width
        .filter(|width| width.is_finite() && *width > 0.0)
        .map(|width| width.max(4.0))
        .unwrap_or((font_size * 0.646).max(6.0));
    let line_height = match (ascent, descent) {
        (Some(ascent), Some(descent)) => {
            let measured = ascent + descent;
            if ascent.is_finite() && descent.is_finite() && measured.is_finite() && measured > 0.0 {
                measured.max(font_size)
            } else {
                (font_size * 1.385).max(font_size + 2.0)
            }
        }
        _ => (font_size * 1.385).max(font_size + 2.0),
    };
    (cell_width, line_height)
}

/// Encode pasted text for the PTY.
///
/// The text is normalized the way a terminal does for typed input: ESC is
/// dropped (it cannot be escaped, only removed, and a surviving one would let
/// the text close its own bracket early) and both CRLF and a lone LF become a
/// single CR. When the program inside the PTY has enabled bracketed paste, the
/// body is wrapped in the paste frame so the receiving TUI sees one paste
/// instead of its line breaks acting as Enter presses.
pub(crate) fn encode_paste(text: &str, bracketed_paste: bool) -> Vec<u8> {
    let body = text
        .replace('\x1b', "")
        .replace("\r\n", "\r")
        .replace('\n', "\r");

    if !bracketed_paste {
        return body.into_bytes();
    }

    let mut payload = Vec::with_capacity(PASTE_START.len() + body.len() + PASTE_END.len());
    payload.extend_from_slice(PASTE_START);
    payload.extend_from_slice(body.as_bytes());
    payload.extend_from_slice(PASTE_END);
    payload
}

#[allow(dead_code)]
pub struct TerminalTab {
    pub id: String,
    pub title: String,
    pub status: String,
    processor: Processor,
    term: Term<TerminalListener>,
    cols: u16,
    rows: u16,
    pub backend: Sender<BackendCommand>,
    pub scroll_pixel_y: f32,
}

#[derive(Clone, Copy)]
pub struct CursorState {
    pub row: usize,
    pub col: usize,
    pub shape: CursorShape,
}

/// A cell of the visible grid.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct GridCell {
    pub row: usize,
    pub col: usize,
}

/// Maps an alacritty cursor point onto the visible grid.
///
/// The point is an absolute grid line: negative lines are scrollback and
/// `display_offset` says how far the viewport is scrolled up, so the row it is
/// drawn at is `line + display_offset`. Anything outside the grid is clamped,
/// because an approximate position beats no position.
fn viewport_cell(line: i32, col: usize, rows: u16, cols: u16, display_offset: usize) -> GridCell {
    let last_row = rows.saturating_sub(1) as i32;
    GridCell {
        row: (line + display_offset as i32).clamp(0, last_row) as usize,
        col: col.min(cols.saturating_sub(1) as usize),
    }
}

#[derive(Clone)]
pub struct RenderCell {
    pub row: i32,
    pub col: i32,
    pub cell: Cell,
}

#[allow(dead_code)]
#[derive(Clone)]
pub struct RenderSnapshot {
    pub cells: Vec<RenderCell>,
    pub cursor: Option<CursorState>,
    /// Where the input method anchors, even when the cursor is not painted.
    pub ime_anchor: GridCell,
    pub selection: Option<ViewportSelection>,
    pub display_offset: usize,
    pub history_size: usize,
    pub rows: usize,
    pub cols: usize,
}

#[derive(Clone, Copy)]
pub struct ViewportSelection {
    pub start_row: usize,
    pub start_col: usize,
    pub end_row: usize,
    pub end_col: usize,
    pub is_block: bool,
}

impl TerminalTab {
    pub fn new(
        id: String,
        title: String,
        backend: Sender<BackendCommand>,
        events: std::sync::mpsc::Sender<BackendEvent>,
    ) -> Self {
        Self {
            id: id.clone(),
            title,
            status: "local shell".into(),
            processor: Processor::new(),
            term: new_term(100, 30, backend.clone(), id, events),
            cols: 100,
            rows: 30,
            backend,
            scroll_pixel_y: 0.0,
        }
    }

    pub fn feed(&mut self, bytes: &[u8]) {
        self.processor.advance(&mut self.term, bytes);
    }

    pub fn resize(&mut self, cols: u16, rows: u16) {
        let new_cols = cols.max(1);
        let new_rows = rows.max(1);
        if self.cols != new_cols || self.rows != new_rows {
            self.cols = new_cols;
            self.rows = new_rows;
            log::info!(
                "[ui] terminal resized to {}x{} (cols x rows)",
                self.cols,
                self.rows
            );
            self.term.resize(TerminalSize::new(self.cols, self.rows));
            let _ = self.backend.send(BackendCommand::Resize { cols, rows });
        }
    }

    pub fn cursor_state(&self) -> Option<CursorState> {
        let content = self.term.renderable_content();
        if matches!(content.cursor.shape, CursorShape::Hidden) || content.display_offset > 0 {
            return None;
        }
        let row = content.cursor.point.line.0;
        if row < 0 {
            return None;
        }
        let row = row as usize;
        if row >= self.rows as usize {
            return None;
        }

        Some(CursorState {
            row,
            col: content.cursor.point.column.0,
            shape: content.cursor.shape,
        })
    }

    /// The cell an input method should anchor its candidate window to.
    ///
    /// Unlike [`TerminalTab::cursor_state`] this ignores cursor visibility and
    /// scrollback. A terminal program hides the cursor whenever it draws no
    /// caret of its own — Bubble Tea does, and only shows one inside a focused
    /// text input — while a composition can run regardless. Reporting no
    /// rectangle in that state makes the platform fall back to the screen
    /// origin, which puts the candidate window in the corner of the display, so
    /// this always names a cell.
    pub fn ime_anchor(&self) -> GridCell {
        let content = self.term.renderable_content();
        viewport_cell(
            content.cursor.point.line.0,
            content.cursor.point.column.0,
            self.rows,
            self.cols,
            content.display_offset,
        )
    }

    pub fn app_cursor_mode(&self) -> bool {
        self.term.mode().contains(TermMode::APP_CURSOR)
    }

    pub fn render_snapshot(&self) -> RenderSnapshot {
        let rows = self.rows;
        let cols = self.cols;
        let content = self.term.renderable_content();
        let display_offset = content.display_offset as i32;
        let mut cells = Vec::with_capacity((rows as usize) * (cols as usize));

        for indexed in content.display_iter {
            let line = indexed.point.line.0;
            let row = line + display_offset;
            if row < 0 {
                continue;
            }
            if row >= rows as i32 {
                continue;
            }

            let col = indexed.point.column.0 as i32;
            if col >= cols as i32 {
                continue;
            }

            cells.push(RenderCell {
                row,
                col,
                cell: indexed.cell.clone(),
            });
        }

        RenderSnapshot {
            cells,
            cursor: self.cursor_state(),
            ime_anchor: self.ime_anchor(),
            selection: viewport_selection_from_range(content.display_offset, &content.selection),
            display_offset: content.display_offset,
            history_size: self.term.grid().history_size(),
            rows: self.rows as usize,
            cols: self.cols as usize,
        }
    }

    pub fn scroll_history(&mut self, delta: i32) {
        if delta != 0 {
            self.term.scroll_display(Scroll::Delta(delta));
        }
    }

    pub fn scroll_to_bottom(&mut self) {
        self.term.scroll_display(Scroll::Bottom);
    }

    pub fn clear_selection(&mut self) {
        self.term.selection = None;
    }

    pub fn selection_text(&self) -> Option<String> {
        self.term
            .selection_to_string()
            .filter(|text| !text.is_empty())
    }

    pub fn begin_selection(
        &mut self,
        row: usize,
        col: usize,
        side: Side,
        selection_type: SelectionType,
    ) {
        let point = viewport_to_point(
            self.term.grid().display_offset(),
            Point::new(row, Column(col)),
        );
        self.term.selection = Some(Selection::new(selection_type, point, side));
    }

    pub fn update_selection(&mut self, row: usize, col: usize, side: Side) {
        let point = viewport_to_point(
            self.term.grid().display_offset(),
            Point::new(row, Column(col)),
        );
        if let Some(selection) = self.term.selection.as_mut() {
            selection.update(point, side);
        }
    }

    pub fn paste_text(&mut self, text: &str) {
        // Empty clipboard text has nothing to paste; avoid sending an empty bracketed frame.
        if text.is_empty() {
            return;
        }
        let bracketed = self.term.mode().contains(TermMode::BRACKETED_PASTE);
        let _ = self
            .backend
            .send(BackendCommand::Input(encode_paste(text, bracketed)));
    }

    pub fn term_mode(&self) -> TermMode {
        *self.term.mode()
    }
}

fn viewport_selection_from_range(
    display_offset: usize,
    selection: &Option<SelectionRange>,
) -> Option<ViewportSelection> {
    let SelectionRange {
        start,
        end,
        is_block,
    } = selection.as_ref().copied()?;
    let start = point_to_viewport(display_offset, start)?;
    let end = point_to_viewport(display_offset, end)?;

    Some(ViewportSelection {
        start_row: start.line,
        start_col: start.column.0,
        end_row: end.line,
        end_col: end.column.0,
        is_block,
    })
}

#[derive(Clone)]
struct TerminalListener {
    tab_id: String,
    backend: Sender<BackendCommand>,
    events: std::sync::mpsc::Sender<BackendEvent>,
}

impl EventListener for TerminalListener {
    fn send_event(&self, event: Event) {
        match event {
            Event::PtyWrite(output) => {
                let _ = self
                    .backend
                    .send(BackendCommand::Input(output.into_bytes()));
            }
            Event::TextAreaSizeRequest(format) => {
                let size = alacritty_terminal::event::WindowSize {
                    num_lines: 30,
                    num_cols: 100,
                    cell_width: 8,
                    cell_height: 16,
                };
                let _ = self
                    .backend
                    .send(BackendCommand::Input(format(size).into_bytes()));
            }
            Event::Title(title) => {
                let _ = self.events.send(BackendEvent::TerminalTitleChanged {
                    tab_id: self.tab_id.clone(),
                    title,
                });
            }
            _ => {}
        }
    }
}

fn new_term(
    cols: u16,
    rows: u16,
    backend: Sender<BackendCommand>,
    tab_id: String,
    events: std::sync::mpsc::Sender<BackendEvent>,
) -> Term<TerminalListener> {
    Term::new(
        Config {
            scrolling_history: 2000,
            ..Config::default()
        },
        &TerminalSize::new(cols, rows),
        TerminalListener {
            tab_id,
            backend,
            events,
        },
    )
}

struct TerminalSize {
    cols: usize,
    rows: usize,
}

impl TerminalSize {
    fn new(cols: u16, rows: u16) -> Self {
        Self {
            cols: cols.max(1) as usize,
            rows: rows.max(1) as usize,
        }
    }
}

impl Dimensions for TerminalSize {
    fn total_lines(&self) -> usize {
        self.rows
    }

    fn screen_lines(&self) -> usize {
        self.rows
    }

    fn columns(&self) -> usize {
        self.cols
    }
}

// ── Key encoding ──

pub fn encode_key(
    keystroke: &Keystroke,
    app_cursor_mode: bool,
    option_as_meta: bool,
) -> Option<Vec<u8>> {
    zed_like_to_esc_str(keystroke, app_cursor_mode, option_as_meta)
        .map(|text| text.into_owned().into_bytes())
}

/// Whether a keystroke is a candidate for an input method to compose with.
///
/// A composing input method only ever receives a keystroke the application did
/// not consume, so a character key encoded for the terminal can never start a
/// composition: typing Chinese or Japanese would insert raw Latin letters. The
/// caller combines this with `ImeRouting` to decide when to leave the key alone.
///
/// Modifier combinations and control codes are deliberately excluded. They are
/// commands rather than composition input: `Ctrl`+letter has to reach the PTY as
/// a control byte, `Alt`/`Option` has to stay available to `option_as_meta`, and
/// `Tab`/`Enter` arrive carrying `"\t"`/`"\n"`. Keys without a `key_char`
/// (arrows, escape, backspace, function keys) need no special case because the
/// platform already offers them to the input method before asking the
/// application at all.
pub(crate) fn is_composable_key(keystroke: &Keystroke) -> bool {
    keystroke
        .key_char
        .as_deref()
        .is_some_and(is_composable_character)
        && !keystroke.modifiers.control
        && !keystroke.modifiers.alt
        && !keystroke.modifiers.platform
        && !keystroke.modifiers.function
}

/// Whether a typed character is composition input rather than a control code.
///
/// Space is composition input because it picks an input method's candidate, but
/// a control code is a keystroke for the terminal: an input method that received
/// `Enter` mid-composition would commit its candidate instead of the terminal
/// ever seeing the key.
fn is_composable_character(key_char: &str) -> bool {
    !key_char.is_empty() && key_char.chars().all(|c| !c.is_control())
}

#[derive(Debug, PartialEq, Eq)]
enum TerminalModifiers {
    None,
    Alt,
    Ctrl,
    Shift,
    CtrlShift,
    Other,
}

impl TerminalModifiers {
    fn new(ks: &Keystroke) -> Self {
        match (
            ks.modifiers.alt,
            ks.modifiers.control,
            ks.modifiers.shift,
            ks.modifiers.platform,
        ) {
            (false, false, false, false) => Self::None,
            (true, false, false, false) => Self::Alt,
            (false, true, false, false) => Self::Ctrl,
            (false, false, true, false) => Self::Shift,
            (false, true, true, false) => Self::CtrlShift,
            _ => Self::Other,
        }
    }

    fn any(&self) -> bool {
        !matches!(self, Self::None)
    }
}

fn zed_like_to_esc_str(
    keystroke: &Keystroke,
    app_cursor_mode: bool,
    option_as_meta: bool,
) -> Option<std::borrow::Cow<'static, str>> {
    let modifiers = TerminalModifiers::new(keystroke);
    let key = keystroke.key.to_ascii_lowercase();

    let manual_esc_str = match (key.as_str(), &modifiers) {
        ("tab", TerminalModifiers::None) => Some("\x09"),
        ("tab", TerminalModifiers::Shift) => Some("\x1b[Z"),
        ("escape", TerminalModifiers::None) => Some("\x1b"),
        ("enter", TerminalModifiers::None) => Some("\x0d"),
        ("enter", TerminalModifiers::Shift) => Some("\x0a"),
        ("enter", TerminalModifiers::Alt) => Some("\x1b\x0d"),
        ("backspace", TerminalModifiers::None) => Some("\x7f"),
        ("backspace", TerminalModifiers::Ctrl) => Some("\x08"),
        ("backspace", TerminalModifiers::Alt) => Some("\x1b[27;3;127~"),
        ("backspace", TerminalModifiers::Shift) => Some("\x7f"),
        ("space", TerminalModifiers::Ctrl) => Some("\x00"),
        ("home", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOH"),
        ("home", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[H"),
        ("end", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOF"),
        ("end", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[F"),
        ("up", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOA"),
        ("up", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[A"),
        ("down", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOB"),
        ("down", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[B"),
        ("right", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOC"),
        ("right", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[C"),
        ("left", TerminalModifiers::None) if app_cursor_mode => Some("\x1bOD"),
        ("left", TerminalModifiers::None) if !app_cursor_mode => Some("\x1b[D"),
        ("insert", TerminalModifiers::None) => Some("\x1b[2~"),
        ("delete", TerminalModifiers::None) => Some("\x1b[3~"),
        ("delete", TerminalModifiers::Alt) => Some("\x1b[3;3~"),
        ("pageup", TerminalModifiers::None) => Some("\x1b[5~"),
        ("pagedown", TerminalModifiers::None) => Some("\x1b[6~"),
        ("a", TerminalModifiers::Ctrl) | ("A", TerminalModifiers::CtrlShift) => Some("\x01"),
        ("b", TerminalModifiers::Ctrl) | ("B", TerminalModifiers::CtrlShift) => Some("\x02"),
        ("c", TerminalModifiers::Ctrl) | ("C", TerminalModifiers::CtrlShift) => Some("\x03"),
        ("d", TerminalModifiers::Ctrl) | ("D", TerminalModifiers::CtrlShift) => Some("\x04"),
        ("e", TerminalModifiers::Ctrl) | ("E", TerminalModifiers::CtrlShift) => Some("\x05"),
        ("f", TerminalModifiers::Ctrl) | ("F", TerminalModifiers::CtrlShift) => Some("\x06"),
        ("g", TerminalModifiers::Ctrl) | ("G", TerminalModifiers::CtrlShift) => Some("\x07"),
        ("h", TerminalModifiers::Ctrl) | ("H", TerminalModifiers::CtrlShift) => Some("\x08"),
        ("i", TerminalModifiers::Ctrl) | ("I", TerminalModifiers::CtrlShift) => Some("\x09"),
        ("j", TerminalModifiers::Ctrl) | ("J", TerminalModifiers::CtrlShift) => Some("\x0a"),
        ("k", TerminalModifiers::Ctrl) | ("K", TerminalModifiers::CtrlShift) => Some("\x0b"),
        ("l", TerminalModifiers::Ctrl) | ("L", TerminalModifiers::CtrlShift) => Some("\x0c"),
        ("m", TerminalModifiers::Ctrl) | ("M", TerminalModifiers::CtrlShift) => Some("\x0d"),
        ("n", TerminalModifiers::Ctrl) | ("N", TerminalModifiers::CtrlShift) => Some("\x0e"),
        ("o", TerminalModifiers::Ctrl) | ("O", TerminalModifiers::CtrlShift) => Some("\x0f"),
        ("p", TerminalModifiers::Ctrl) | ("P", TerminalModifiers::CtrlShift) => Some("\x10"),
        ("q", TerminalModifiers::Ctrl) | ("Q", TerminalModifiers::CtrlShift) => Some("\x11"),
        ("r", TerminalModifiers::Ctrl) | ("R", TerminalModifiers::CtrlShift) => Some("\x12"),
        ("s", TerminalModifiers::Ctrl) | ("S", TerminalModifiers::CtrlShift) => Some("\x13"),
        ("t", TerminalModifiers::Ctrl) | ("T", TerminalModifiers::CtrlShift) => Some("\x14"),
        ("u", TerminalModifiers::Ctrl) | ("U", TerminalModifiers::CtrlShift) => Some("\x15"),
        ("v", TerminalModifiers::Ctrl) | ("V", TerminalModifiers::CtrlShift) => Some("\x16"),
        ("w", TerminalModifiers::Ctrl) | ("W", TerminalModifiers::CtrlShift) => Some("\x17"),
        ("x", TerminalModifiers::Ctrl) | ("X", TerminalModifiers::CtrlShift) => Some("\x18"),
        ("y", TerminalModifiers::Ctrl) | ("Y", TerminalModifiers::CtrlShift) => Some("\x19"),
        ("z", TerminalModifiers::Ctrl) | ("Z", TerminalModifiers::CtrlShift) => Some("\x1a"),
        ("@", TerminalModifiers::Ctrl) => Some("\x00"),
        ("[", TerminalModifiers::Ctrl) => Some("\x1b"),
        ("\\", TerminalModifiers::Ctrl) => Some("\x1c"),
        ("]", TerminalModifiers::Ctrl) => Some("\x1d"),
        ("^", TerminalModifiers::Ctrl) => Some("\x1e"),
        ("_", TerminalModifiers::Ctrl) => Some("\x1f"),
        ("?", TerminalModifiers::Ctrl) => Some("\x7f"),
        ("delete", TerminalModifiers::Ctrl) => Some("\x1b[3;5~"),
        _ => None,
    };
    if let Some(esc) = manual_esc_str {
        return Some(esc.into());
    }

    if modifiers.any() {
        let modifier_code = modifier_code(keystroke);
        let modified = match key.as_str() {
            "up" => Some(format!("\x1b[1;{}A", modifier_code)),
            "down" => Some(format!("\x1b[1;{}B", modifier_code)),
            "right" => Some(format!("\x1b[1;{}C", modifier_code)),
            "left" => Some(format!("\x1b[1;{}D", modifier_code)),
            "insert" => Some(format!("\x1b[2;{}~", modifier_code)),
            "pageup" => Some(format!("\x1b[5;{}~", modifier_code)),
            "pagedown" => Some(format!("\x1b[6;{}~", modifier_code)),
            "end" => Some(format!("\x1b[1;{}F", modifier_code)),
            "home" => Some(format!("\x1b[1;{}H", modifier_code)),
            _ => None,
        };
        if let Some(esc) = modified {
            return Some(esc.into());
        }
    }

    if !cfg!(target_os = "macos") || option_as_meta {
        let is_alt_lowercase_ascii = modifiers == TerminalModifiers::Alt
            && keystroke.key.is_ascii()
            && keystroke.key.len() == 1;
        let is_alt_uppercase_ascii = keystroke.modifiers.alt
            && keystroke.modifiers.shift
            && keystroke.key.is_ascii()
            && keystroke.key.len() == 1;
        if is_alt_lowercase_ascii || is_alt_uppercase_ascii {
            let key = if is_alt_uppercase_ascii {
                keystroke.key.to_ascii_uppercase()
            } else {
                keystroke.key.clone()
            };
            return Some(format!("\x1b{}", key).into());
        }
    }

    if let Some(text) = &keystroke.key_char {
        return Some(text.clone().into());
    }

    if keystroke.key.len() == 1 {
        return Some(keystroke.key.clone().into());
    }

    None
}

fn modifier_code(keystroke: &Keystroke) -> u32 {
    let mut modifier_code = 0;
    if keystroke.modifiers.shift {
        modifier_code |= 1;
    }
    if keystroke.modifiers.alt {
        modifier_code |= 1 << 1;
    }
    if keystroke.modifiers.control {
        modifier_code |= 1 << 2;
    }
    modifier_code + 1
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ctrl_delete_encodes_as_delete_word_forward() {
        let keystroke = Keystroke::parse("ctrl-delete").expect("parse ctrl-delete");
        assert_eq!(
            encode_key(&keystroke, false, false),
            Some(b"\x1b[3;5~".to_vec())
        );
    }

    #[test]
    fn alt_delete_encodes_as_delete_word_forward() {
        let keystroke = Keystroke::parse("alt-delete").expect("parse alt-delete");
        assert_eq!(
            encode_key(&keystroke, false, false),
            Some(b"\x1b[3;3~".to_vec())
        );
    }

    #[test]
    fn alt_backspace_encodes_as_delete_word_backward() {
        let keystroke = Keystroke::parse("alt-backspace").expect("parse alt-backspace");
        assert_eq!(
            encode_key(&keystroke, false, false),
            Some(b"\x1b[27;3;127~".to_vec())
        );
    }

    #[test]
    fn alt_backspace_does_not_encode_the_key_name_as_meta_text() {
        let keystroke = Keystroke::parse("alt-backspace").expect("parse alt-backspace");
        assert_ne!(
            encode_key(&keystroke, false, false),
            Some(b"\x1bbackspace".to_vec())
        );
    }

    #[test]
    fn encode_paste_frames_when_the_tui_asked_for_bracketed_paste() {
        assert_eq!(
            encode_paste("a\nb", true),
            b"\x1b[200~a\rb\x1b[201~".to_vec()
        );
    }

    #[test]
    fn encode_paste_sends_the_text_bare_when_bracketed_paste_is_off() {
        assert_eq!(encode_paste("a\nb", false), b"a\rb".to_vec());
    }

    #[test]
    fn encode_paste_strips_esc_and_collapses_crlf() {
        assert_eq!(
            encode_paste("\x1b[31mred\r\n", false),
            b"[31mred\r".to_vec()
        );
    }

    #[test]
    fn encode_paste_cannot_be_closed_early_by_its_own_terminator() {
        // The ESC is stripped, so a pasted "\x1b[201~" reaches the TUI as the
        // literal text "[201~" instead of ending the bracket early.
        assert_eq!(
            encode_paste("\x1b[201~", true),
            b"\x1b[200~[201~\x1b[201~".to_vec()
        );
    }

    #[test]
    fn alt_tab_does_not_encode_the_key_name_as_meta_text() {
        let keystroke = Keystroke::parse("alt-tab").expect("parse alt-tab");
        // Named ASCII keys must not pass the single-character Alt length guard.
        assert_eq!(encode_key(&keystroke, false, true), None);
    }

    #[test]
    fn alt_shift_tab_does_not_encode_the_key_name_as_meta_text() {
        let keystroke = Keystroke::parse("alt-shift-tab").expect("parse alt-shift-tab");
        assert_ne!(
            encode_key(&keystroke, false, true),
            Some(b"\x1bTAB".to_vec())
        );
    }

    #[test]
    fn measured_metrics_win_over_the_ratio() {
        let (cell_width, line_height) =
            metrics_from_measurement(Some(8.43), Some(12.0), Some(3.4), 14.0);
        assert_eq!(cell_width, 8.43);
        assert!(
            (line_height - 15.4).abs() < 1e-3,
            "line height was {line_height}"
        );
    }

    #[test]
    fn metrics_fall_back_to_the_historical_ratio_when_measurement_fails() {
        let (cell_width, line_height) = metrics_from_measurement(None, None, None, 14.0);
        assert_eq!(cell_width, (14.0f32 * 0.646).max(6.0));
        assert_eq!(line_height, (14.0f32 * 1.385).max(14.0 + 2.0));
    }

    #[test]
    fn metrics_reject_non_finite_measurements() {
        let (cell_width, line_height) =
            metrics_from_measurement(Some(f32::NAN), Some(f32::INFINITY), Some(0.0), 14.0);
        assert!(
            cell_width.is_finite() && cell_width > 0.0,
            "cell width was {cell_width}"
        );
        assert!(
            line_height.is_finite() && line_height > 0.0,
            "line height was {line_height}"
        );
    }

    #[test]
    fn metrics_clamp_absurdly_small_measurements() {
        let (cell_width, line_height) =
            metrics_from_measurement(Some(0.1), Some(0.1), Some(0.0), 14.0);
        assert!(cell_width >= 4.0, "cell width was {cell_width}");
        assert!(line_height >= 14.0, "line height was {line_height}");
    }

    #[test]
    fn metrics_follow_the_font_size() {
        let at14 = metrics_from_measurement(None, None, None, 14.0);
        let at28 = metrics_from_measurement(None, None, None, 28.0);
        assert!(
            at28.0 > at14.0 && at28.1 > at14.1,
            "metrics did not follow the font size"
        );
    }

    // ── Input method routing ──

    fn typed(key: &str, key_char: Option<&str>) -> Keystroke {
        Keystroke {
            key: key.into(),
            key_char: key_char.map(Into::into),
            ..Default::default()
        }
    }

    #[test]
    fn printable_keys_are_composable() {
        // A composition can only start from a key the app did not consume.
        assert!(is_composable_key(&typed("a", Some("a"))));
        assert!(is_composable_key(&typed("space", Some(" "))), "space");
        assert!(is_composable_key(&typed("s", Some("ß"))), "option-s");
        assert!(is_composable_key(&typed("a", Some("中"))), "composed text");
        assert!(!is_composable_key(&typed("a", Some(""))), "empty character");
    }

    #[test]
    fn shifted_letters_are_composable() {
        // Shift is part of ordinary typing, not a command.
        let mut keystroke = typed("A", Some("A"));
        keystroke.modifiers.shift = true;
        assert!(is_composable_key(&keystroke));
    }

    #[test]
    fn modifier_combos_are_not_composable() {
        // Ctrl+letter must reach the PTY as a control byte, and Option must stay
        // available to `option_as_meta`; neither is composition input.
        let with = |modifiers: gpui::Modifiers| Keystroke {
            modifiers,
            ..typed("a", Some("a"))
        };
        let control = gpui::Modifiers {
            control: true,
            ..Default::default()
        };
        let alt = gpui::Modifiers {
            alt: true,
            ..Default::default()
        };
        let platform = gpui::Modifiers {
            platform: true,
            ..Default::default()
        };
        let function = gpui::Modifiers {
            function: true,
            ..Default::default()
        };

        assert!(!is_composable_key(&with(control)), "ctrl+a");
        assert!(!is_composable_key(&with(alt)), "alt+a");
        assert!(!is_composable_key(&with(platform)), "cmd+a");
        assert!(!is_composable_key(&with(function)), "fn+a");
    }

    #[test]
    fn control_codes_are_not_composable() {
        // Tab and Enter carry a character on macOS, but they are keystrokes for
        // the terminal: giving them to an input method lets it swallow the key.
        assert!(!is_composable_key(&typed("tab", Some("\t"))), "tab");
        assert!(!is_composable_key(&typed("enter", Some("\n"))), "enter");
        assert!(
            !is_composable_key(&typed("x", Some("\r"))),
            "carriage return"
        );
        assert!(
            !is_composable_key(&typed("x", Some("\u{8}"))),
            "control byte"
        );
    }

    #[test]
    fn keys_without_a_character_are_not_composable() {
        // Named keys keep their escape encoding; gpui already offers them to the
        // input method before the app is asked.
        assert!(!is_composable_key(&typed("escape", None)));
        assert!(!is_composable_key(&typed("left", None)));
        assert!(!is_composable_key(&typed("backspace", None)));
    }

    // ── Input method anchor ──

    fn new_tab() -> TerminalTab {
        let (backend, _backend_rx) = std::sync::mpsc::channel();
        let (events, _events_rx) = std::sync::mpsc::channel();
        TerminalTab::new("test".into(), "Test".into(), backend, events)
    }

    #[test]
    fn ime_anchor_survives_a_hidden_cursor() {
        // A terminal program hides the cursor whenever it draws no caret of its
        // own — Bubble Tea does, and only shows one inside a focused text input.
        // A composition can run anyway, and a handler that reports no rectangle
        // makes the platform place the candidate window at the screen origin.
        let mut tab = new_tab();
        tab.feed(b"\x1b[5;7H");
        tab.feed(b"\x1b[?25l");

        assert!(
            tab.cursor_state().is_none(),
            "a hidden cursor is not painted"
        );
        assert_eq!(tab.ime_anchor(), GridCell { row: 4, col: 6 });
    }

    #[test]
    fn ime_anchor_follows_the_visible_cursor() {
        let mut tab = new_tab();
        tab.feed(b"\x1b[2;3H");

        assert_eq!(tab.ime_anchor(), GridCell { row: 1, col: 2 });
        assert_eq!(
            tab.cursor_state().map(|cursor| (cursor.row, cursor.col)),
            Some((1, 2))
        );
    }

    #[test]
    fn viewport_cell_offsets_scrolled_rows() {
        // The cursor point is an absolute grid line, so a viewport scrolled up by
        // three lines shows that line three rows lower.
        assert_eq!(viewport_cell(5, 0, 30, 100, 3), GridCell { row: 8, col: 0 });
    }

    #[test]
    fn viewport_cell_clamps_to_the_visible_grid() {
        assert_eq!(
            viewport_cell(-7, 0, 30, 100, 0),
            GridCell { row: 0, col: 0 }
        );
        assert_eq!(
            viewport_cell(29, 0, 30, 100, 40),
            GridCell { row: 29, col: 0 }
        );
        assert_eq!(
            viewport_cell(0, 500, 30, 100, 0),
            GridCell { row: 0, col: 99 }
        );
    }
}
