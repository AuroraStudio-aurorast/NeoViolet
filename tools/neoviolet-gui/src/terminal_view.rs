//! Terminal rendering helpers: cell grid, cursor overlay, IME preedit, mouse bridge.
//!
//! Extracted from neo_violet_app.rs to keep the app shell focused on orchestration.

use gpui::*;
use gpui::prelude::*;
use std::sync::Arc;

use crate::bounds::TerminalBounds;
use crate::hyperlink;
use crate::modes::Modes;
use crate::state::AppState;
use crate::term::CursorShape;
use crate::platform;

/// Dracula terminal background.
pub const DRACULA_BG: u32 = 0x282a36;
/// Cursor color — Dracula cyan.
pub const DRACULA_CURSOR: u32 = 0x8be9fd;
/// Monospace cell aspect: width ≈ height × 0.6.
pub const CELL_ASPECT: f32 = 0.6;
/// Left padding matching px_2().
pub const PAD_LEFT: f32 = 8.0;

// ── Terminal maintenance (called once per render frame) ──

/// Perform resize + snapshot + mode sync + OSC event processing.
/// Returns (title_change, process_exited, lines, cols, cursor, ime_text).
pub fn maintain_terminal(
    cx: &mut App,
    bounds: Bounds<Pixels>,
    cell_w: f32, cell_h: f32,
    last_size: &mut Option<Size<Pixels>>,
) -> (Option<String>, bool, Arc<Vec<crate::term::CachedLine>>, usize, Option<(usize, usize, crate::term::CursorStyle)>, Option<String>)
{
    let titlebar_h = if cfg!(target_os = "macos") { 28.0 } else { 0.0 };
    let wf = bounds.size.width / px(1.0) - 16.0;
    let hf = bounds.size.height / px(1.0) - titlebar_h - 4.0;
    let ncols = (wf / cell_w).max(80.0) as usize;
    let nrows = (hf / cell_h).max(24.0) as usize;

    let state = cx.global::<AppState>();
    let mut term = state.terminal.lock().unwrap();

    let (title, exited, lines, cols, cursor, ime) = if let Some(ref mut t) = *term {
        if *last_size != Some(bounds.size) {
            t.resize(ncols, nrows, cell_w as u16, cell_h as u16);
            *last_size = Some(bounds.size);
        }
        t.update_and_swap_snapshot();
        *state.modes.lock().unwrap() = t.modes();

        let title = t.poll_title();
        t.poll_exit_code();
        let exited = !t.is_alive();

        // OSC events
        for data in t.drain_pty_writes() {
            let _ = t.input_sender().send(alacritty_terminal::event_loop::Msg::Input(
                std::borrow::Cow::Owned(data)));
        }
        for event in t.drain_term_events() {
            use crate::event_listener::TermEvent;
            match event {
                TermEvent::ClipboardStore(text) => {
                    cx.write_to_clipboard(ClipboardItem::new_string(text));
                }
                TermEvent::ClipboardLoad(fmt) => {
                    if let Some(item) = cx.read_from_clipboard() {
                        if let Some(txt) = item.text().as_deref() {
                            let _ = t.input_sender().send(alacritty_terminal::event_loop::Msg::Input(
                                std::borrow::Cow::Owned(fmt(txt).into_bytes())));
                        }
                    }
                }
                TermEvent::ColorQuery(idx, fmt) => {
                    let c = crate::dracula_theme::dracula_color(idx);
                    let rgb = alacritty_terminal::vte::ansi::Rgb { r: c[0], g: c[1], b: c[2] };
                    let _ = t.input_sender().send(alacritty_terminal::event_loop::Msg::Input(
                        std::borrow::Cow::Owned(fmt(rgb).into_bytes())));
                }
            }
        }

        let snap = t.content_snapshot();
        let ime = t.ime_preedit();
        (title, exited, snap.lines.clone(), snap.cols, snap.cursor, ime)
    } else {
        (None, false, Arc::new(vec![]), 0, None, None::<String>)
    };
    (title, exited, lines, cols, cursor, ime)
}

// ── Cell grid rendering ──

/// Build a single terminal line as a flex row of cell divs.
pub fn build_line(line: &crate::term::CachedLine, cw: f32, ch: f32, dfg: Hsla) -> impl IntoElement {
    let cells = &line.cells;
    if cells.is_empty() { return div().flex().h(px(ch)).into_any_element(); }
    let end = cells.iter().rposition(|c| c.ch != ' ').map_or(0, |i| i + 1);
    if end == 0 { return div().flex().h(px(ch)).into_any_element(); }
    div().flex().children(cells[..end].iter().map(|cell| {
        let c = if cell.ch == ' ' { '\u{00A0}' } else { cell.ch };
        let mut dc = div().min_w(px(if cell.is_wide { cw * 2.0 } else { cw })).h(px(ch))
            .flex().items_center().justify_center().child(c.to_string());
        if let Some([r, g, b]) = cell.fg {
            dc = dc.text_color(rgb((r as u32) << 16 | (g as u32) << 8 | b as u32));
        } else { dc = dc.text_color(dfg); }
        if let Some([r, g, b]) = cell.bg {
            dc = dc.bg(rgb((r as u32) << 16 | (g as u32) << 8 | b as u32));
        }
        if cell.bold { dc = dc.font_weight(FontWeight::BOLD); }
        if cell.italic { dc = dc.italic(); }
        if cell.underline { dc = dc.underline(); }
        if cell.strikethrough { dc = dc.line_through(); }
        dc
    })).into_any_element()
}

// ── Overlays ──

pub fn render_ime_overlay(ime_text: &Option<String>, cursor: Option<(usize, usize, crate::term::CursorStyle)>, cw: f32, ch: f32, fs: f32) -> Option<AnyElement> {
    let (col, row) = match (ime_text, cursor) {
        (Some(text), Some((col, row, _))) if !text.is_empty() => (col, row),
        _ => return None,
    };
    let left = col as f32 * cw + PAD_LEFT;
    let top = row as f32 * ch;
    Some(div().absolute().left(px(left)).top(px(top)).h(px(ch))
        .text_size(px(fs)).text_color(rgb(DRACULA_CURSOR)).underline()
        .child(ime_text.clone().unwrap()).into_any_element())
}

pub fn render_cursor_overlay(
    lines: &[crate::term::CachedLine], cols: usize,
    cursor: Option<(usize, usize, crate::term::CursorStyle)>,
    cw: f32, ch: f32, fs: f32, dfg: Hsla,
) -> Option<AnyElement> {
    let (col, row, style) = cursor?;
    if !style.visible { return None; }
    let char_under = lines.get(row).and_then(|l| l.cells.get(col)).map(|c| c.ch).unwrap_or(' ');
    let left = col as f32 * cw + PAD_LEFT;
    let top = row as f32 * ch;
    let wide = col < cols.saturating_sub(1)
        && lines.get(row).and_then(|l| l.cells.get(col)).is_some_and(|c| c.is_wide);
    let w = if wide { cw * 2.0 } else { cw };
    let h = ch;
    Some(match style.shape {
        CursorShape::Block => div().absolute().left(px(left)).top(px(top)).min_w(px(w)).h(px(h))
            .bg(rgb(DRACULA_CURSOR)).text_color(rgb(DRACULA_BG)).text_size(px(fs))
            .flex().items_center().justify_center().child(char_under.to_string()).into_any_element(),
        CursorShape::HollowBlock => div().absolute().left(px(left)).top(px(top)).min_w(px(w)).h(px(h))
            .border_1().border_color(rgb(DRACULA_CURSOR)).text_size(px(fs)).text_color(dfg)
            .flex().items_center().justify_center().child(char_under.to_string()).into_any_element(),
        CursorShape::Underline => div().absolute().left(px(left)).top(px(top + h - 2.0))
            .min_w(px(w)).h(px(2.0)).bg(rgb(DRACULA_CURSOR)).into_any_element(),
        CursorShape::Beam => div().absolute().left(px(left)).top(px(top))
            .w(px(2.0)).h(px(h)).bg(rgb(DRACULA_CURSOR)).into_any_element(),
    })
}

// ── Mouse bridge ──

fn grid_coords(pos: Point<Pixels>, cw: f32, ch: f32) -> (usize, usize) {
    TerminalBounds { line_height: px(ch), cell_width: px(cw),
        bounds: Bounds { origin: point(px(0.0), px(0.0)), size: size(px(2000.0), px(2000.0)) }
    }.grid_point(pos)
}

pub fn mk_mouse_down(cw: f32, ch: f32, btn: MouseButton) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) {
    move |ev: &MouseDownEvent, _w, cx| {
        if btn == MouseButton::Left && ev.modifiers.control {
            if try_open_hyperlink(cx, ev.position, cw, ch) { return; }
        }
        let (col, row) = grid_coords(ev.position, cw, ch);
        let m = cx.global::<AppState>().cached_modes();
        if let Some(s) = crate::mouse::mouse_button_report(&m, btn, &ev.modifiers, true, col as u16, row as u16) {
            cx.global::<AppState>().send_to_pty(s);
        }
    }
}

pub fn mk_mouse_up(cw: f32, ch: f32, btn: MouseButton) -> impl Fn(&MouseUpEvent, &mut Window, &mut App) {
    move |ev: &MouseUpEvent, _w, cx| {
        let (col, row) = grid_coords(ev.position, cw, ch);
        let m = cx.global::<AppState>().cached_modes();
        if let Some(s) = crate::mouse::mouse_button_report(&m, btn, &ev.modifiers, false, col as u16, row as u16) {
            cx.global::<AppState>().send_to_pty(s);
        }
    }
}

pub fn mk_mouse_move(cw: f32, ch: f32) -> impl Fn(&MouseMoveEvent, &mut Window, &mut App) {
    move |ev: &MouseMoveEvent, _w, cx| {
        let (col, row) = grid_coords(ev.position, cw, ch);
        let m = cx.global::<AppState>().cached_modes();
        if let Some(s) = crate::mouse::mouse_moved_report(&m, ev.pressed_button, &ev.modifiers, col as u16, row as u16) {
            cx.global::<AppState>().send_to_pty(s);
        }
    }
}

pub fn mk_scroll(cw: f32, ch: f32) -> impl Fn(&ScrollWheelEvent, &mut Window, &mut App) {
    move |ev: &ScrollWheelEvent, _w, cx| {
        let (col, row) = grid_coords(ev.position, cw, ch);
        let is_down = match ev.delta { ScrollDelta::Lines(p) => p.y > 0.0, ScrollDelta::Pixels(p) => f32::from(p.y) > 0.0 };
        let m = cx.global::<AppState>().cached_modes();
        if m.intersects(Modes::MOUSE_MODE) {
            if let Some(s) = crate::mouse::scroll_report(&m, is_down, &ev.modifiers, col as u16, row as u16, 1) {
                cx.global::<AppState>().send_to_pty(s);
            }
        } else if m.contains(Modes::ALTERNATE_SCROLL) {
            cx.global::<AppState>().send_to_pty(if is_down { vec![0x1b, b'O', b'B'] } else { vec![0x1b, b'O', b'A'] });
        }
    }
}

fn try_open_hyperlink(cx: &mut App, pos: Point<Pixels>, cw: f32, ch: f32) -> bool {
    let (col, row) = grid_coords(pos, cw, ch);
    let s = cx.global::<AppState>();
    let tg = s.terminal.lock().unwrap();
    if let Some(ref t) = *tg {
        let snap = t.content_snapshot();
        if let Some(line) = snap.lines.get(row) {
            let text: String = line.cells.iter().map(|c| c.ch).collect();
            if let Some(url) = hyperlink::hyperlink_at_column(&text, col) {
                platform::open_url(&url); return true;
            }
        }
    }
    false
}
