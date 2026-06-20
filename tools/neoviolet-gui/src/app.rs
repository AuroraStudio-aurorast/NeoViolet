use std::{
    ops::Range,
    sync::{mpsc, Arc, Mutex},
};

use alacritty_terminal::{index::Side, selection::SelectionType};
use gpui::{
    App, Bounds, ClipboardItem, Context, EventEmitter, FocusHandle, Focusable,
    InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseDownEvent,
    MouseMoveEvent, MouseUpEvent, ParentElement, Pixels, Point, Render, ScrollDelta,
    ScrollWheelEvent, SharedString, Styled, Window, div, point, px, size,
};
use yororen_ui::theme::ActiveTheme as _;

use crate::backend;
use crate::state::AppState;
use crate::terminal::{
    self, BackendCommand, BackendEvent, TerminalTab,
    element::TerminalElement,
};

// ── Application entity ──

pub struct TerminalApp {
    pub tab: TerminalTab,
    backend_rx: mpsc::Receiver<BackendEvent>,
    pub(crate) terminal_bounds: Bounds<Pixels>,
    terminal_selecting: bool,
    terminal_marked_text: Option<String>,
    pub terminal_font_size: f32,
    pub terminal_font_family: SharedString,
    pub focus_handle: FocusHandle,
    pub(crate) pending_resize: Option<(u16, u16)>,
    /// Buffer for recent PTY output (for bad-args crash diagnostics)
    recent_output: Arc<Mutex<Vec<u8>>>,
}

impl TerminalApp {
    pub fn new(cx: &mut Context<Self>) -> Self {
        let tab_id = "terminal-1".to_string();
        let (events_tx, events_rx) = mpsc::channel();

        // Read config from AppState global
        let (font_size, font_family, launch_args, recent_output, child_pid) = {
            let state = cx.global::<AppState>();
            let fs = *state.font_size.lock().unwrap() as f32;
            let ff = state.config.monospace_font.clone();
            let args = state.launch_args.lock().unwrap().clone();
            let ro = state.recent_output.clone();
            let pid = state.child_pid.clone();
            // Record process start time for bad-args detection
            *state.process_start.lock().unwrap() = Some(std::time::Instant::now());
            (fs, ff, args, ro, pid)
        };

        let backend_tx =
            backend::spawn_neoviolet_terminal(tab_id.clone(), 100, 30, events_tx.clone(), &launch_args, child_pid.clone())
                .expect("failed to spawn neoviolet terminal");

        // Connect IPC client to the TUI's TCP endpoint (retries up to 5 s),
        // then start a reader thread to receive messages from the TUI.
        if let Some(pid) = *child_pid.lock().unwrap() {
            let ipc = cx.global::<AppState>().ipc.clone();
            let incoming = cx.global::<AppState>().ipc_incoming.clone();
            std::thread::spawn(move || {
                if let Err(e) = ipc.connect(pid) {
                    log::warn!("[ipc] connect failed: {}", e);
                    return;
                }
                ipc.start_reader(incoming);
            });
        }

        let tab = TerminalTab::new(tab_id, "neoviolet".into(), backend_tx, events_tx);

        // Background event loop using GPUI's timer
        cx.spawn(async move |this, cx| {
            let mut idle_frames = 0u32;
            loop {
                cx.background_executor()
                    .timer(std::time::Duration::from_millis(16))
                    .await;
                if this
                    .update(cx, |this, cx| {
                        let changed = this.drain_events();
                        if changed {
                            cx.notify();
                            idle_frames = 0;
                        } else {
                            idle_frames += 1;
                            if idle_frames >= 60 {
                                cx.notify();
                                idle_frames = 0;
                            }
                        }
                    })
                    .is_err()
                {
                    break;
                }
            }
        })
        .detach();

        Self {
            tab,
            backend_rx: events_rx,
            terminal_bounds: Bounds::new(point(px(0.), px(0.)), size(px(0.), px(0.))),
            terminal_selecting: false,
            terminal_marked_text: None,
            terminal_font_size: font_size,
            terminal_font_family: font_family.into(),
            focus_handle: cx.focus_handle(),
            pending_resize: None,
            recent_output,
        }
    }

    pub fn current_title(&self) -> &str {
        &self.tab.title
    }

    fn drain_events(&mut self) -> bool {
        let mut changed = false;
        while let Ok(event) = self.backend_rx.try_recv() {
            changed = true;
            match event {
                BackendEvent::Output { bytes, .. } => {
                    self.tab.feed(&bytes);
                    // Buffer recent output for diagnostics (cap at 16 KB)
                    {
                        let mut buf = self.recent_output.lock().unwrap();
                        if buf.len() < 16384 {
                            buf.extend_from_slice(&bytes);
                        }
                    }
                }
                BackendEvent::Status { text, .. } => {
                    self.tab.status = text;
                }
                BackendEvent::Closed { reason, .. } => {
                    log::info!("terminal closed: {reason}");
                    self.tab.status = reason;
                }
                BackendEvent::TerminalTitleChanged { title, .. } => {
                    self.tab.title = title;
                }
            }
        }
        changed
    }

    pub(crate) fn apply_pending_resize(&mut self) {
        if let Some((cols, rows)) = self.pending_resize.take() {
            self.tab.resize(cols, rows);
        }
    }

    // ── Metrics ──

    pub fn terminal_cell_width(&self) -> f32 {
        (self.terminal_font_size * 0.646).max(6.0)
    }

    pub fn terminal_line_height(&self) -> f32 {
        (self.terminal_font_size * 1.385).max(self.terminal_font_size + 2.0)
    }

    // ── IME support ──

    pub(crate) fn terminal_accepts_text_input(&self) -> bool {
        true
    }

    pub(crate) fn terminal_marked_text_range(&self) -> Option<Range<usize>> {
        self.terminal_marked_text
            .as_ref()
            .map(|text| 0..text.encode_utf16().count())
    }

    pub(crate) fn set_terminal_marked_text(
        &mut self,
        text: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.terminal_marked_text = if text.is_empty() { None } else { Some(text) };
        window.invalidate_character_coordinates();
        cx.notify();
    }

    pub(crate) fn clear_terminal_marked_text(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.terminal_marked_text.take().is_some() {
            window.invalidate_character_coordinates();
            cx.notify();
        }
    }

    pub(crate) fn commit_terminal_ime_text(
        &mut self,
        text: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.tab.render_snapshot().display_offset > 0 {
            self.tab.scroll_to_bottom();
        }
        self.tab.clear_selection();
        self.terminal_marked_text = None;
        let _ = self
            .tab
            .backend
            .send(BackendCommand::Input(text.as_bytes().to_vec()));
        window.invalidate_character_coordinates();
        cx.notify();
    }

    pub(crate) fn terminal_ime_bounds_for_range(
        &self,
        range_utf16: Range<usize>,
        element_bounds: Bounds<Pixels>,
        cell_width: f32,
        line_height: f32,
    ) -> Option<Bounds<Pixels>> {
        let snapshot = self.tab.render_snapshot();
        let cursor = snapshot.cursor?;
        let x = element_bounds.origin.x
            + px(cell_width) * cursor.col as f32
            + px(cell_width) * range_utf16.start as f32;
        let y = element_bounds.origin.y + px(line_height) * cursor.row as f32;
        Some(Bounds::new(point(x, y), size(px(cell_width), px(line_height))))
    }

    // ── Keyboard input ──

    fn on_terminal_key_down(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // Cmd+C: copy selection
        if event.keystroke.modifiers.secondary()
            && event.keystroke.key.eq_ignore_ascii_case("c")
        {
            if let Some(text) = self.tab.selection_text() {
                cx.write_to_clipboard(ClipboardItem::new_string(text));
                window.prevent_default();
                cx.stop_propagation();
                return;
            }
        }

        // Cmd+V: paste
        if event.keystroke.modifiers.secondary()
            && event.keystroke.key.eq_ignore_ascii_case("v")
        {
            if let Some(clipboard) = cx.read_from_clipboard() {
                if let Some(text) = clipboard.text() {
                    self.paste_into_terminal(&text, window, cx);
                    return;
                }
            }
        }

        // Character input — defer to the IME/InputHandler system.
        // The EntityInputHandler will commit the final text via replace_text_in_range.
        if event.prefer_character_input {
            return;
        }

        if self.tab.render_snapshot().display_offset > 0 {
            self.tab.scroll_to_bottom();
        }
        self.tab.clear_selection();

        if let Some(bytes) =
            terminal::encode_key(&event.keystroke, self.tab.app_cursor_mode(), false)
        {
            let _ = self.tab.backend.send(BackendCommand::Input(bytes));
            window.prevent_default();
            cx.stop_propagation();
            cx.notify();
        }
    }

    fn paste_into_terminal(&mut self, text: &str, window: &mut Window, cx: &mut Context<Self>) {
        if self.tab.render_snapshot().display_offset > 0 {
            self.tab.scroll_to_bottom();
        }
        self.tab.clear_selection();
        self.tab.paste_text(text);
        window.prevent_default();
        cx.stop_propagation();
        cx.notify();
    }

    // ── Mouse input ──

    /// Send a mouse-report escape sequence to the PTY if the terminal has
    /// mouse tracking enabled. Returns `true` when the event was forwarded
    /// (and `prevent_default` + `stop_propagation` have been called).
    fn send_mouse_report(
        &mut self,
        button: u8, // 0=left, 1=middle, 2=right; add 0x80 for SGR release
        position: Point<Pixels>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> bool {
        use alacritty_terminal::term::TermMode;

        let mode = self.tab.term_mode();
        let is_mouse_tracking = mode.intersects(
            TermMode::MOUSE_REPORT_CLICK
                | TermMode::MOUSE_MOTION
                | TermMode::MOUSE_DRAG,
        );
        if !is_mouse_tracking {
            return false;
        }

        if let Some((row, col, _)) = self.terminal_grid_point_and_side(position) {
            let sgr = mode.contains(TermMode::SGR_MOUSE);
            let mut bytes: Vec<u8> = Vec::new();
            if sgr {
                // SGR extended mouse: \x1b[<b;col;rowM (press) or m (release)
                let release_char = if button >= 0x80 { 'm' } else { 'M' };
                let btn = button & 0x7F;
                bytes.extend_from_slice(
                    format!("\x1b[<{};{};{}{}", btn, col + 1, row + 1, release_char)
                        .as_bytes(),
                );
            } else {
                // Normal mouse tracking (X10 encoding):
                // \x1b[M cb cx cy where cb = button + 32, cx/cy = col/row + 33
                if col < 223 && row < 223 {
                    bytes.extend_from_slice(b"\x1b[M");
                    bytes.push(button.wrapping_add(32));
                    bytes.push(col as u8 + 33);
                    bytes.push(row as u8 + 33);
                }
            }
            if !bytes.is_empty() {
                let _ = self.tab.backend.send(BackendCommand::Input(bytes));
            }
            window.prevent_default();
            cx.stop_propagation();
        }
        true
    }

    fn on_terminal_right_click(
        &mut self,
        event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // If the terminal program has enabled mouse tracking, forward the
        // right-click as a mouse-report escape sequence instead of treating
        // it as a copy/paste gesture.
        if self.send_mouse_report(2, event.position, window, cx) {
            return;
        }

        // Right-click: copy selection to clipboard (no paste).
        if let Some(text) = self.tab.selection_text() {
            if !text.is_empty() {
                cx.write_to_clipboard(ClipboardItem::new_string(text));
                self.tab.clear_selection();
                cx.notify();
            }
        }
    }

    fn begin_terminal_selection(
        &mut self,
        event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // If the terminal program has enabled mouse tracking, forward the
        // left-click as a mouse-report escape sequence.
        if self.send_mouse_report(0, event.position, window, cx) {
            return;
        }

        let click_count = event.click_count.max(1);
        let selection_type = match click_count {
            1 => SelectionType::Simple,
            2 => SelectionType::Semantic,
            3 => SelectionType::Lines,
            _ => SelectionType::Simple,
        };
        let Some((row, col, side)) = self.terminal_grid_point_and_side(event.position) else {
            return;
        };
        self.tab.begin_selection(row, col, side, selection_type);
        self.terminal_selecting = true;
        cx.notify();
    }

    fn on_terminal_mouse_move(
        &mut self,
        event: &MouseMoveEvent,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if !self.terminal_selecting || event.pressed_button != Some(MouseButton::Left) {
            return;
        }
        let Some((row, col, side)) = self.terminal_grid_point_and_side(event.position) else {
            return;
        };
        self.tab.update_selection(row, col, side);
        cx.notify();
    }

    fn on_terminal_mouse_up(
        &mut self,
        event: &MouseUpEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // If the terminal has mouse tracking, report the release.
        // In SGR mode bit 7 signals release; in normal mode button 3 = release.
        self.send_mouse_report(0 | 0x80, event.position, window, cx);

        self.terminal_selecting = false;
        cx.notify();
    }

    fn on_terminal_scroll(
        &mut self,
        event: &ScrollWheelEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let grid_point = self.terminal_grid_point_and_side(event.position);
        let line_height = self.terminal_line_height();

        let delta_lines = match event.delta {
            ScrollDelta::Lines(point) => point.y.round() as i32,
            ScrollDelta::Pixels(point) => {
                self.tab.scroll_pixel_y += f32::from(point.y);
                let lines = (self.tab.scroll_pixel_y / line_height).trunc() as i32;
                self.tab.scroll_pixel_y -= (lines as f32) * line_height;
                lines
            }
        };

        if delta_lines == 0 {
            return;
        }

        let mode = self.tab.term_mode();

        let is_mouse_tracking = mode.intersects(
            alacritty_terminal::term::TermMode::MOUSE_REPORT_CLICK
                | alacritty_terminal::term::TermMode::MOUSE_MOTION
                | alacritty_terminal::term::TermMode::MOUSE_DRAG,
        );

        let is_alternate_scroll = mode.contains(
            alacritty_terminal::term::TermMode::ALT_SCREEN
                | alacritty_terminal::term::TermMode::ALTERNATE_SCROLL,
        );

        if is_mouse_tracking {
            if let Some((row, col, _)) = grid_point {
                let sgr = mode.contains(alacritty_terminal::term::TermMode::SGR_MOUSE);
                let button = if delta_lines > 0 { 64 } else { 65 };
                let times = delta_lines.abs();
                let mut bytes = Vec::new();
                for _ in 0..times {
                    if sgr {
                        bytes.extend_from_slice(
                            format!("\x1b[<{};{};{}M", button, col + 1, row + 1).as_bytes(),
                        );
                    } else {
                        if col < 223 && row < 223 {
                            bytes.extend_from_slice(b"\x1b[M");
                            bytes.push(button as u8 + 32);
                            bytes.push(col as u8 + 33);
                            bytes.push(row as u8 + 33);
                        }
                    }
                }
                if !bytes.is_empty() {
                    let _ = self.tab.backend.send(BackendCommand::Input(bytes));
                }
            }
            window.prevent_default();
            cx.stop_propagation();
            return;
        } else if is_alternate_scroll {
            let times = delta_lines.abs();
            let code = if delta_lines > 0 { b'A' } else { b'B' };
            let mut bytes = Vec::with_capacity((times * 3) as usize);
            for _ in 0..times {
                bytes.extend_from_slice(&[b'\x1b', b'O', code]);
            }
            if !bytes.is_empty() {
                let _ = self.tab.backend.send(BackendCommand::Input(bytes));
            }
            window.prevent_default();
            cx.stop_propagation();
            return;
        }

        self.tab.scroll_history(delta_lines);
        window.prevent_default();
        cx.stop_propagation();
        cx.notify();
    }

    fn terminal_grid_point_and_side(
        &self,
        position: Point<Pixels>,
    ) -> Option<(usize, usize, Side)> {
        if !self.terminal_bounds.contains(&position) {
            return None;
        }
        let local_x = (position.x - self.terminal_bounds.origin.x).max(px(0.));
        let local_y = (position.y - self.terminal_bounds.origin.y).max(px(0.));
        let cell_width = px(self.terminal_cell_width());
        let line_height = px(self.terminal_line_height());
        let snapshot = self.tab.render_snapshot();
        let max_col = snapshot.cols.saturating_sub(1);
        let max_row = snapshot.rows.saturating_sub(1);
        let col = ((local_x / cell_width).floor() as usize).min(max_col);
        let row = ((local_y / line_height).floor() as usize).min(max_row);
        let cell_offset_x = px(f32::from(local_x) % f32::from(cell_width));
        let side = if cell_offset_x >= (cell_width / 2.) {
            Side::Right
        } else {
            Side::Left
        };
        Some((row, col, side))
    }
}

// ── GPUI Entity implementation ──

impl EventEmitter<()> for TerminalApp {}

impl Focusable for TerminalApp {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for TerminalApp {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        // Process pending events
        self.drain_events();

        let snapshot = self.tab.render_snapshot();
        let font_family = self.terminal_font_family.clone();
        let font_size = px(self.terminal_font_size);
        let line_height = px(self.terminal_line_height());
        let cell_width = px(self.terminal_cell_width());
        let marked_text = self.terminal_marked_text.clone();
        let focus_handle = self.focus_handle.clone();

        // Calculate desired terminal size based on available space
        // This will be handled by the layout system

        div()
            .size_full()
            .bg(cx.theme().surface.canvas)
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(
                |this, event: &KeyDownEvent, window, cx| {
                    this.on_terminal_key_down(event, window, cx);
                },
            ))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(|this, event: &MouseDownEvent, window, cx| {
                    this.begin_terminal_selection(event, window, cx);
                }),
            )
            .on_mouse_down(
                MouseButton::Right,
                cx.listener(
                    |this, event: &MouseDownEvent, window, cx| {
                        this.on_terminal_right_click(event, window, cx);
                    },
                ),
            )
            .on_mouse_move(cx.listener(
                |this, event: &MouseMoveEvent, window, cx| {
                    this.on_terminal_mouse_move(event, window, cx);
                },
            ))
            .on_mouse_up(
                MouseButton::Left,
                cx.listener(
                    |this, event: &MouseUpEvent, window, cx| {
                        this.on_terminal_mouse_up(event, window, cx);
                    },
                ),
            )
            .on_scroll_wheel(cx.listener(
                |this, event: &ScrollWheelEvent, window, cx| {
                    this.on_terminal_scroll(event, window, cx);
                },
            ))
            .child(
                TerminalElement::new(
                    snapshot,
                    marked_text,
                    font_family,
                    font_size,
                    line_height,
                    cell_width,
                    cx.entity().clone(),
                    focus_handle,
                ),
            )
    }
}
