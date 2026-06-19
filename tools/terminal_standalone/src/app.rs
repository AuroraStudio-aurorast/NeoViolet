use std::{
    ops::Range,
    sync::mpsc,
};

use alacritty_terminal::{index::Side, selection::SelectionType};
use gpui::{
    App, Bounds, ClipboardItem, Context, Element, Entity, EventEmitter, FocusHandle, Focusable,
    Hsla, InteractiveElement, IntoElement, KeyDownEvent, MouseButton, MouseDownEvent,
    MouseMoveEvent, MouseUpEvent, ParentElement, Pixels, Point, Render, ScrollDelta,
    ScrollWheelEvent, SharedString, Styled, Window, div, fill, point, px, size,
};
use gpui_component::ActiveTheme as _;

use crate::backend;
use crate::terminal::{
    self, BackendCommand, BackendEvent, RenderSnapshot, TerminalTab,
    custom_blocks::{is_custom_block_supported, paint_custom_block},
};

// ── Application entity ──

pub struct TerminalApp {
    tab: TerminalTab,
    backend_rx: mpsc::Receiver<BackendEvent>,
    terminal_bounds: Bounds<Pixels>,
    terminal_selecting: bool,
    terminal_marked_text: Option<String>,
    terminal_font_size: f32,
    terminal_font_family: SharedString,
    pub focus_handle: FocusHandle,
    pending_resize: Option<(u16, u16)>,
}

impl TerminalApp {
    pub fn new(cx: &mut Context<Self>) -> Self {
        let tab_id = "terminal-1".to_string();
        let (events_tx, events_rx) = mpsc::channel();

        let backend_tx =
            backend::spawn_local_terminal(tab_id.clone(), 100, 30, events_tx.clone())
                .expect("failed to spawn local terminal");

        let tab = TerminalTab::new(tab_id, "local".into(), backend_tx, events_tx);

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
            terminal_font_size: 14.0,
            terminal_font_family: ".SystemUIFont".into(),
            focus_handle: cx.focus_handle(),
            pending_resize: None,
        }
    }

    fn drain_events(&mut self) -> bool {
        let mut changed = false;
        while let Ok(event) = self.backend_rx.try_recv() {
            changed = true;
            match event {
                BackendEvent::Output { bytes, .. } => {
                    self.tab.feed(&bytes);
                }
                BackendEvent::Status { text, .. } => {
                    self.tab.status = text;
                }
                BackendEvent::Closed { reason, .. } => {
                    tracing::info!("terminal closed: {reason}");
                }
                BackendEvent::TerminalTitleChanged { title, .. } => {
                    self.tab.title = title;
                }
            }
        }
        changed
    }

    fn apply_pending_resize(&mut self) {
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

    fn terminal_accepts_text_input(&self) -> bool {
        true
    }

    fn terminal_marked_text_range(&self) -> Option<Range<usize>> {
        self.terminal_marked_text
            .as_ref()
            .map(|text| 0..text.encode_utf16().count())
    }

    fn set_terminal_marked_text(
        &mut self,
        text: String,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        self.terminal_marked_text = if text.is_empty() { None } else { Some(text) };
        window.invalidate_character_coordinates();
        cx.notify();
    }

    fn clear_terminal_marked_text(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.terminal_marked_text.take().is_some() {
            window.invalidate_character_coordinates();
            cx.notify();
        }
    }

    fn commit_terminal_ime_text(
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

    fn terminal_ime_bounds_for_range(
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

        // Character input via IME
        if event.prefer_character_input {
            if let Some(text) = event.keystroke.key_char.as_deref() {
                if !text.is_empty()
                    && !event.keystroke.modifiers.control
                    && !event.keystroke.modifiers.function
                    && !event.keystroke.modifiers.platform
                {
                    self.send_terminal_input(text.as_bytes().to_vec(), window, cx);
                }
            }
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

    fn send_terminal_input(
        &mut self,
        bytes: Vec<u8>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if self.tab.render_snapshot().display_offset > 0 {
            self.tab.scroll_to_bottom();
        }
        self.tab.clear_selection();
        let _ = self.tab.backend.send(BackendCommand::Input(bytes));
        window.prevent_default();
        cx.stop_propagation();
        cx.notify();
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

    fn on_terminal_right_click(
        &mut self,
        _event: &MouseDownEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        // Right-click: copy selection or paste
        let mut handled = false;
        if let Some(text) = self.tab.selection_text() {
            if !text.is_empty() {
                cx.write_to_clipboard(ClipboardItem::new_string(text));
                self.tab.clear_selection();
                cx.notify();
                handled = true;
            }
        }
        if !handled {
            if let Some(clipboard_item) = cx.read_from_clipboard() {
                if let Some(text) = clipboard_item.text() {
                    if !text.is_empty() {
                        self.paste_into_terminal(&text, window, cx);
                    }
                }
            }
        }
    }

    fn begin_terminal_selection(&mut self, event: &MouseDownEvent, cx: &mut Context<Self>) {
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
        _event: &MouseUpEvent,
        _window: &mut Window,
        cx: &mut Context<Self>,
    ) {
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
                self.tab.scroll_pixel_y += point.y.as_f32();
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
        let cell_offset_x = px(local_x.as_f32() % cell_width.as_f32());
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
            .bg(cx.theme().background)
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(
                |this, event: &KeyDownEvent, window, cx| {
                    this.on_terminal_key_down(event, window, cx);
                },
            ))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(|this, event: &MouseDownEvent, _window, cx| {
                    this.begin_terminal_selection(event, cx);
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

// ── Terminal GPUI Element ──

#[derive(Clone, Copy)]
struct TerminalMetrics {
    cell_width: Pixels,
    line_height: Pixels,
}

#[derive(Clone)]
struct LayoutRect {
    row: i32,
    col: i32,
    cells: usize,
    color: Hsla,
}

impl LayoutRect {
    fn paint(&self, origin: Point<Pixels>, metrics: TerminalMetrics, window: &mut Window) {
        let position = point(
            origin.x + metrics.cell_width * self.col as f32,
            origin.y + metrics.line_height * self.row as f32,
        );
        let size = gpui::size(metrics.cell_width * self.cells as f32, metrics.line_height);
        window.paint_quad(fill(Bounds::new(position, size), self.color));
    }
}

#[derive(Clone)]
struct BatchedTextRun {
    row: i32,
    col: i32,
    cell_count: usize,
    text: String,
    style: gpui::TextRun,
    font_size: Pixels,
}

impl BatchedTextRun {
    fn new(row: i32, col: i32, ch: char, style: gpui::TextRun, font_size: Pixels) -> Self {
        Self {
            row,
            col,
            cell_count: 1,
            text: ch.to_string(),
            style,
            font_size,
        }
    }

    fn can_append(&self, other: &gpui::TextRun, row: i32, col: i32) -> bool {
        self.row == row
            && self.col + self.cell_count as i32 == col
            && self.style.font == other.font
            && self.style.color == other.color
            && self.style.background_color == other.background_color
            && self.style.underline == other.underline
            && self.style.strikethrough == other.strikethrough
    }

    fn append(&mut self, ch: char, zerowidth: Option<&[char]>) {
        self.text.push(ch);
        self.cell_count += 1;
        self.style.len += ch.len_utf8();
        if let Some(chars) = zerowidth {
            for c in chars {
                self.text.push(*c);
                self.style.len += c.len_utf8();
            }
        }
    }

    fn paint(
        &self,
        origin: Point<Pixels>,
        metrics: TerminalMetrics,
        window: &mut Window,
        cx: &mut App,
    ) {
        let pos = point(
            origin.x + metrics.cell_width * self.col as f32,
            origin.y + metrics.line_height * self.row as f32,
        );

        window
            .text_system()
            .shape_line(
                self.text.clone().into(),
                self.font_size,
                std::slice::from_ref(&self.style),
                Some(metrics.cell_width),
            )
            .paint(
                pos,
                metrics.line_height,
                gpui::TextAlign::Left,
                None,
                window,
                cx,
            )
            .ok();
    }
}

#[derive(Clone, Copy)]
struct CursorLayout {
    row: usize,
    col: usize,
    shape: alacritty_terminal::vte::ansi::CursorShape,
    color: Hsla,
}

pub struct TerminalElement {
    snapshot: RenderSnapshot,
    marked_text: Option<String>,
    font_family: SharedString,
    font_size: Pixels,
    line_height: Pixels,
    cell_width: Pixels,
    view: Entity<TerminalApp>,
    focus_handle: FocusHandle,
}

pub struct PrepaintState {
    bounds: Bounds<Pixels>,
    metrics: TerminalMetrics,
    rects: Vec<LayoutRect>,
    runs: Vec<BatchedTextRun>,
    custom_blocks: Vec<LayoutCustomBlock>,
    cursor: Option<CursorLayout>,
}

#[derive(Clone)]
struct LayoutCustomBlock {
    c: char,
    row: i32,
    col: i32,
    cells: usize,
    color: Hsla,
}

struct TerminalInputHandler {
    view: Entity<TerminalApp>,
    element_bounds: Bounds<Pixels>,
    cell_width: f32,
    line_height: f32,
}

impl gpui::InputHandler for TerminalInputHandler {
    fn selected_text_range(
        &mut self,
        _ignore_disabled_input: bool,
        _window: &mut Window,
        cx: &mut App,
    ) -> Option<gpui::UTF16Selection> {
        self.view
            .read(cx)
            .terminal_accepts_text_input()
            .then_some(gpui::UTF16Selection {
                range: 0..0,
                reversed: false,
            })
    }

    fn marked_text_range(
        &mut self,
        _window: &mut Window,
        cx: &mut App,
    ) -> Option<std::ops::Range<usize>> {
        self.view.read(cx).terminal_marked_text_range()
    }

    fn text_for_range(
        &mut self,
        _range_utf16: std::ops::Range<usize>,
        _adjusted_range: &mut Option<std::ops::Range<usize>>,
        _window: &mut Window,
        _cx: &mut App,
    ) -> Option<String> {
        None
    }

    fn replace_text_in_range(
        &mut self,
        _replacement_range: Option<std::ops::Range<usize>>,
        text: &str,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.view.update(cx, |view, cx| {
            view.commit_terminal_ime_text(text, window, cx);
        });
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        _range_utf16: Option<std::ops::Range<usize>>,
        new_text: &str,
        _new_selected_range: Option<std::ops::Range<usize>>,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.view.update(cx, |view, cx| {
            view.set_terminal_marked_text(new_text.to_string(), window, cx);
        });
    }

    fn unmark_text(&mut self, window: &mut Window, cx: &mut App) {
        self.view.update(cx, |view, cx| {
            view.clear_terminal_marked_text(window, cx);
        });
    }

    fn bounds_for_range(
        &mut self,
        range_utf16: std::ops::Range<usize>,
        _window: &mut Window,
        cx: &mut App,
    ) -> Option<Bounds<Pixels>> {
        self.view.read(cx).terminal_ime_bounds_for_range(
            range_utf16,
            self.element_bounds,
            self.cell_width,
            self.line_height,
        )
    }

    fn character_index_for_point(
        &mut self,
        _point: Point<Pixels>,
        _window: &mut Window,
        _cx: &mut App,
    ) -> Option<usize> {
        None
    }

    fn accepts_text_input(&mut self, _window: &mut Window, cx: &mut App) -> bool {
        self.view.read(cx).terminal_accepts_text_input()
    }

    fn apple_press_and_hold_enabled(&mut self) -> bool {
        false
    }

    fn prefers_ime_for_printable_keys(&mut self, _window: &mut Window, cx: &mut App) -> bool {
        self.view.read(cx).terminal_accepts_text_input()
    }
}

impl TerminalElement {
    pub fn new(
        snapshot: RenderSnapshot,
        marked_text: Option<String>,
        font_family: SharedString,
        font_size: Pixels,
        line_height: Pixels,
        cell_width: Pixels,
        view: Entity<TerminalApp>,
        focus_handle: FocusHandle,
    ) -> Self {
        Self {
            snapshot,
            marked_text,
            font_family,
            font_size,
            line_height,
            cell_width,
            view,
            focus_handle,
        }
    }

    fn base_text_style(&self, cx: &App) -> gpui::TextStyle {
        gpui::TextStyle {
            color: cx.theme().foreground,
            font_family: self.font_family.clone(),
            font_size: self.font_size.into(),
            line_height: self.line_height.into(),
            ..Default::default()
        }
    }

    fn cell_run_style(
        &self,
        cell: &alacritty_terminal::term::cell::Cell,
        cx: &App,
    ) -> gpui::TextRun {
        use alacritty_terminal::term::cell::Flags;
        use gpui::{Font, FontStyle, FontWeight, StrikethroughStyle, UnderlineStyle};

        let mut fg = color_to_hsla(cell.fg, true, cx);
        let mut bg = color_to_hsla(cell.bg, false, cx);
        if cell.flags.contains(Flags::INVERSE) {
            std::mem::swap(&mut fg, &mut bg);
        }
        if cell.flags.contains(Flags::DIM) {
            fg.a *= 0.7;
        }

        let underline = cell
            .flags
            .intersects(Flags::ALL_UNDERLINES)
            .then(|| UnderlineStyle {
                color: Some(fg),
                thickness: px(1.0),
                wavy: cell.flags.contains(Flags::UNDERCURL),
            });
        let strikethrough = cell
            .flags
            .contains(Flags::STRIKEOUT)
            .then(|| StrikethroughStyle {
                color: Some(fg),
                thickness: px(1.0),
            });

        let weight = if cell.flags.intersects(Flags::BOLD | Flags::DIM_BOLD) {
            FontWeight::BOLD
        } else {
            FontWeight::NORMAL
        };
        let style = if cell.flags.intersects(Flags::ITALIC | Flags::BOLD_ITALIC) {
            FontStyle::Italic
        } else {
            FontStyle::Normal
        };

        gpui::TextRun {
            len: cell.c.len_utf8(),
            color: fg,
            background_color: None,
            font: Font {
                family: self.font_family.clone(),
                weight,
                style,
                ..Font::default()
            },
            underline,
            strikethrough,
        }
    }

    fn layout_grid(
        &self,
        cx: &App,
    ) -> (
        Vec<LayoutRect>,
        Vec<BatchedTextRun>,
        Vec<LayoutCustomBlock>,
    ) {
        use alacritty_terminal::term::cell::Flags;

        let mut rects = Vec::new();
        let mut runs = Vec::new();
        let mut custom_blocks = Vec::new();
        let mut current_run: Option<BatchedTextRun> = None;

        for render_cell in &self.snapshot.cells {
            let cell = &render_cell.cell;
            if cell.flags.intersects(
                Flags::HIDDEN | Flags::WIDE_CHAR_SPACER | Flags::LEADING_WIDE_CHAR_SPACER,
            ) {
                continue;
            }

            let selected = self.snapshot.selection.is_some_and(|selection| {
                selection_contains(selection, render_cell.row, render_cell.col)
            });
            let bg = color_to_hsla(cell.bg, false, cx);
            if selected || !is_default_bg(cell.bg) {
                rects.push(LayoutRect {
                    row: render_cell.row,
                    col: render_cell.col,
                    cells: if cell.flags.contains(Flags::WIDE_CHAR) {
                        2
                    } else {
                        1
                    },
                    color: if selected {
                        cx.theme().selection
                    } else if cell.flags.contains(Flags::INVERSE) {
                        color_to_hsla(cell.fg, true, cx)
                    } else {
                        bg
                    },
                });
            }

            if is_blank(cell) {
                if let Some(run) = current_run.take() {
                    runs.push(run);
                }
                continue;
            }

            let style = self.cell_run_style(cell, cx);

            let is_custom_block = is_custom_block_supported(cell.c);

            if is_custom_block {
                if let Some(run) = current_run.take() {
                    runs.push(run);
                }
                custom_blocks.push(LayoutCustomBlock {
                    c: cell.c,
                    row: render_cell.row,
                    col: render_cell.col,
                    cells: if cell.flags.contains(Flags::WIDE_CHAR) {
                        2
                    } else {
                        1
                    },
                    color: style.color,
                });
                continue;
            }

            if let Some(run) = current_run.as_mut() {
                if run.can_append(&style, render_cell.row, render_cell.col) {
                    run.append(cell.c, cell.zerowidth());
                    continue;
                }
            }

            if let Some(run) = current_run.take() {
                runs.push(run);
            }

            let mut run = BatchedTextRun::new(
                render_cell.row,
                render_cell.col,
                cell.c,
                style,
                self.font_size,
            );
            if let Some(chars) = cell.zerowidth() {
                for ch in chars {
                    run.text.push(*ch);
                    run.style.len += ch.len_utf8();
                }
            }
            current_run = Some(run);
        }

        if let Some(run) = current_run {
            runs.push(run);
        }

        (merge_rects(rects), runs, custom_blocks)
    }

    fn cursor_layout(&self, cx: &App) -> Option<CursorLayout> {
        self.snapshot.cursor.map(|cursor| CursorLayout {
            row: cursor.row,
            col: cursor.col,
            shape: cursor.shape,
            color: cx.theme().primary,
        })
    }
}

impl IntoElement for TerminalElement {
    type Element = Self;

    fn into_element(self) -> Self::Element {
        self
    }
}

impl Element for TerminalElement {
    type RequestLayoutState = ();
    type PrepaintState = PrepaintState;

    fn id(&self) -> Option<gpui::ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _id: Option<&gpui::GlobalElementId>,
        _inspector_id: Option<&gpui::InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (gpui::LayoutId, Self::RequestLayoutState) {
        let mut style = gpui::Style::default();
        style.size.width = gpui::relative(1.).into();
        style.size.height = gpui::relative(1.).into();
        (window.request_layout(style, None, cx), ())
    }

    fn prepaint(
        &mut self,
        _id: Option<&gpui::GlobalElementId>,
        _inspector_id: Option<&gpui::InspectorElementId>,
        bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        _window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let _ = self.base_text_style(cx);
        let (rects, runs, custom_blocks) = self.layout_grid(cx);

        // Notify view of bounds and trigger resize if needed
        self.view.update(cx, |view, _cx| {
            view.terminal_bounds = bounds;

            // Calculate terminal size from bounds
            let cell_width = view.terminal_cell_width();
            let line_height = view.terminal_line_height();
            let cols = ((bounds.size.width.as_f32() / cell_width).floor() as u16).max(1);
            let rows = ((bounds.size.height.as_f32() / line_height).floor() as u16).max(1);
            view.pending_resize = Some((cols, rows));
            view.apply_pending_resize();
        });

        // Refresh snapshot after potential resize
        self.view.update(cx, |view, _cx| {
            self.snapshot = view.tab.render_snapshot();
        });

        PrepaintState {
            bounds,
            metrics: TerminalMetrics {
                cell_width: self.cell_width,
                line_height: self.line_height,
            },
            rects,
            runs,
            custom_blocks,
            cursor: self.cursor_layout(cx),
        }
    }

    fn paint(
        &mut self,
        _id: Option<&gpui::GlobalElementId>,
        _inspector_id: Option<&gpui::InspectorElementId>,
        _bounds: Bounds<Pixels>,
        _request_layout: &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        for rect in &prepaint.rects {
            rect.paint(prepaint.bounds.origin, prepaint.metrics, window);
        }

        for run in &prepaint.runs {
            run.paint(prepaint.bounds.origin, prepaint.metrics, window, cx);
        }

        for block in &prepaint.custom_blocks {
            let x = prepaint.bounds.origin.x.as_f32()
                + block.col as f32 * prepaint.metrics.cell_width.as_f32();
            let y = prepaint.bounds.origin.y.as_f32()
                + block.row as f32 * prepaint.metrics.line_height.as_f32();
            paint_custom_block(
                window,
                block.c,
                x,
                y,
                prepaint.metrics.cell_width.as_f32() * block.cells as f32,
                prepaint.metrics.line_height.as_f32(),
                block.color,
            );
        }

        window.handle_input(
            &self.focus_handle,
            TerminalInputHandler {
                view: self.view.clone(),
                element_bounds: prepaint.bounds,
                cell_width: prepaint.metrics.cell_width.as_f32(),
                line_height: prepaint.metrics.line_height.as_f32(),
            },
            cx,
        );

        if let Some(marked_text) = self.marked_text.as_ref().filter(|text| !text.is_empty()) {
            if let Some(cursor) = prepaint.cursor {
                let pos = point(
                    prepaint.bounds.origin.x + prepaint.metrics.cell_width * cursor.col as f32,
                    prepaint.bounds.origin.y + prepaint.metrics.line_height * cursor.row as f32,
                );
                let mut base_style = self.base_text_style(cx);
                base_style.underline = Some(gpui::UnderlineStyle {
                    color: Some(base_style.color),
                    thickness: px(1.0),
                    wavy: false,
                });
                let shaped = window.text_system().shape_line(
                    marked_text.clone().into(),
                    self.font_size,
                    &[gpui::TextRun {
                        len: marked_text.len(),
                        font: gpui::Font {
                            family: self.font_family.clone(),
                            ..gpui::Font::default()
                        },
                        color: base_style.color,
                        underline: base_style.underline,
                        ..Default::default()
                    }],
                    None,
                );
                let bg_bounds =
                    Bounds::new(pos, gpui::size(shaped.width, prepaint.metrics.line_height));
                window.paint_quad(fill(bg_bounds, cx.theme().background));
                shaped
                    .paint(
                        pos,
                        prepaint.metrics.line_height,
                        gpui::TextAlign::Left,
                        None,
                        window,
                        cx,
                    )
                    .ok();
            }
        }

        if let Some(cursor) = prepaint.cursor {
            if self
                .marked_text
                .as_ref()
                .is_some_and(|text| !text.is_empty())
            {
                return;
            }
            let x = prepaint.bounds.origin.x + prepaint.metrics.cell_width * cursor.col as f32;
            let y = prepaint.bounds.origin.y + prepaint.metrics.line_height * cursor.row as f32;
            match cursor.shape {
                alacritty_terminal::vte::ansi::CursorShape::Hidden => {}
                alacritty_terminal::vte::ansi::CursorShape::Beam => {
                    window.paint_quad(fill(
                        Bounds::new(
                            point(x, y),
                            gpui::size(px(2.), prepaint.metrics.line_height),
                        ),
                        cursor.color,
                    ));
                }
                alacritty_terminal::vte::ansi::CursorShape::Underline => {
                    window.paint_quad(fill(
                        Bounds::new(
                            point(x, y + prepaint.metrics.line_height - px(2.)),
                            gpui::size(prepaint.metrics.cell_width, px(2.)),
                        ),
                        cursor.color,
                    ));
                }
                alacritty_terminal::vte::ansi::CursorShape::Block
                | alacritty_terminal::vte::ansi::CursorShape::HollowBlock => {
                    let alpha = if matches!(
                        cursor.shape,
                        alacritty_terminal::vte::ansi::CursorShape::HollowBlock
                    ) {
                        0.18
                    } else {
                        0.32
                    };
                    window.paint_quad(fill(
                        Bounds::new(
                            point(x, y),
                            gpui::size(
                                prepaint.metrics.cell_width,
                                prepaint.metrics.line_height,
                            ),
                        ),
                        cursor.color.opacity(alpha),
                    ));
                }
            }
        }
    }
}

// ── Helper functions ──

fn merge_rects(mut rects: Vec<LayoutRect>) -> Vec<LayoutRect> {
    rects.sort_by_key(|rect| (rect.row, rect.col));
    let mut merged: Vec<LayoutRect> = Vec::with_capacity(rects.len());

    for rect in rects {
        if let Some(last) = merged.last_mut() {
            if last.row == rect.row
                && last.color == rect.color
                && last.col + last.cells as i32 == rect.col
            {
                last.cells += rect.cells;
                continue;
            }
        }
        merged.push(rect);
    }

    merged
}

fn selection_contains(
    selection: terminal::ViewportSelection,
    row: i32,
    col: i32,
) -> bool {
    let row = row.max(0) as usize;
    let col = col.max(0) as usize;

    if row < selection.start_row || row > selection.end_row {
        return false;
    }

    if selection.is_block {
        return col >= selection.start_col && col <= selection.end_col;
    }

    let after_start = row > selection.start_row || col >= selection.start_col;
    let before_end = row < selection.end_row || col <= selection.end_col;
    after_start && before_end
}

fn is_blank(cell: &alacritty_terminal::term::cell::Cell) -> bool {
    use alacritty_terminal::term::cell::Flags;

    cell.c == ' '
        && cell.zerowidth().is_none()
        && !cell
            .flags
            .intersects(Flags::ALL_UNDERLINES | Flags::STRIKEOUT)
}

fn is_default_bg(color: alacritty_terminal::vte::ansi::Color) -> bool {
    matches!(
        color,
        alacritty_terminal::vte::ansi::Color::Named(
            alacritty_terminal::vte::ansi::NamedColor::Background
        )
    )
}

fn color_to_hsla(
    color: alacritty_terminal::vte::ansi::Color,
    foreground: bool,
    cx: &App,
) -> Hsla {
    use alacritty_terminal::vte::ansi::Color as AnsiColor;

    match color {
        AnsiColor::Spec(rgb) => Hsla::from(gpui::Rgba {
            r: rgb.r as f32 / 255.0,
            g: rgb.g as f32 / 255.0,
            b: rgb.b as f32 / 255.0,
            a: 1.0,
        }),
        AnsiColor::Indexed(index) => ansi_index_color(index, cx),
        AnsiColor::Named(named) => named_color(named, foreground, cx),
    }
}

fn ansi_index_color(index: u8, _cx: &App) -> Hsla {
    use gpui::rgb;

    const ANSI_16: [u32; 16] = [
        0x1f2430, 0xff5c57, 0x5af78e, 0xf3f99d, 0x57c7ff, 0xff6ac1, 0x9aedfe, 0xf1f1f0, 0x686868,
        0xff5c57, 0x5af78e, 0xf3f99d, 0x57c7ff, 0xff6ac1, 0x9aedfe, 0xffffff,
    ];

    if (index as usize) < ANSI_16.len() {
        return Hsla::from(rgb(ANSI_16[index as usize]));
    }

    if index >= 232 {
        let gray = 8 + (index - 232) * 10;
        return Hsla::from(gpui::Rgba {
            r: gray as f32 / 255.0,
            g: gray as f32 / 255.0,
            b: gray as f32 / 255.0,
            a: 1.0,
        });
    }

    let i = index - 16;
    let r = i / 36;
    let g = (i % 36) / 6;
    let b = i % 6;
    let conv = |v: u8| if v == 0 { 0 } else { 55 + v * 40 };
    Hsla::from(gpui::Rgba {
        r: conv(r) as f32 / 255.0,
        g: conv(g) as f32 / 255.0,
        b: conv(b) as f32 / 255.0,
        a: 1.0,
    })
}

fn named_color(named: alacritty_terminal::vte::ansi::NamedColor, _foreground: bool, cx: &App) -> Hsla {
    use alacritty_terminal::vte::ansi::NamedColor;
    use gpui::rgb;

    match named {
        NamedColor::Foreground => cx.theme().foreground,
        NamedColor::Background => cx.theme().background,
        NamedColor::Black => Hsla::from(rgb(0x1f2430)),
        NamedColor::Red => Hsla::from(rgb(0xff5c57)),
        NamedColor::Green => Hsla::from(rgb(0x5af78e)),
        NamedColor::Yellow => Hsla::from(rgb(0xf3f99d)),
        NamedColor::Blue => Hsla::from(rgb(0x57c7ff)),
        NamedColor::Magenta => Hsla::from(rgb(0xff6ac1)),
        NamedColor::Cyan => Hsla::from(rgb(0x9aedfe)),
        NamedColor::White => Hsla::from(rgb(0xf1f1f0)),
        NamedColor::BrightBlack => Hsla::from(rgb(0x686868)),
        NamedColor::BrightRed => Hsla::from(rgb(0xff5c57)),
        NamedColor::BrightGreen => Hsla::from(rgb(0x5af78e)),
        NamedColor::BrightYellow => Hsla::from(rgb(0xf3f99d)),
        NamedColor::BrightBlue => Hsla::from(rgb(0x57c7ff)),
        NamedColor::BrightMagenta => Hsla::from(rgb(0xff6ac1)),
        NamedColor::BrightCyan => Hsla::from(rgb(0x9aedfe)),
        NamedColor::BrightWhite => Hsla::from(rgb(0xffffff)),
        NamedColor::Cursor => cx.theme().primary,
        NamedColor::DimForeground => cx.theme().muted_foreground,
        NamedColor::BrightForeground => cx.theme().foreground,
        NamedColor::DimBlack => Hsla::from(rgb(0x3b4252)),
        NamedColor::DimRed => Hsla::from(rgb(0xbf616a)),
        NamedColor::DimGreen => Hsla::from(rgb(0xa3be8c)),
        NamedColor::DimYellow => Hsla::from(rgb(0xebcb8b)),
        NamedColor::DimBlue => Hsla::from(rgb(0x81a1c1)),
        NamedColor::DimMagenta => Hsla::from(rgb(0xb48ead)),
        NamedColor::DimCyan => Hsla::from(rgb(0x88c0d0)),
        NamedColor::DimWhite => Hsla::from(rgb(0xe5e9f0)),
    }
}
