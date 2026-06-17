use gpui::*;
use gpui::prelude::*;
use futures::StreamExt;
use std::sync::Arc;
use yororen_ui::theme::ActiveTheme;

use crate::bounds::TerminalBounds;
use crate::components;
use crate::config;
use crate::hyperlink;
use crate::modes::Modes;
use crate::state::AppState;
use crate::term::{CursorShape, TerminalState};

const DRACULA_BG: u32 = 0x282a36;
const DRACULA_CURSOR: u32 = 0x8be9fd;
const CELL_ASPECT: f32 = 0.6;
const PAD_LEFT: f32 = 8.0;

pub struct NeoVioletApp {
    current_title: String,
    last_window_title: String,
    focus_handle: FocusHandle,
    last_size: Option<Size<Pixels>>,
    show_exit_error: bool,
    entity_id: EntityId,
}

impl NeoVioletApp {
    pub fn new(cx: &mut Context<Self>, launch_args: Vec<String>) -> Self {
        let app_state = cx.global::<AppState>();
        let cfg = app_state.config.clone();

        // Spawn terminal
        let mut terminal = TerminalState::spawn(&cfg, &launch_args)
            .expect("failed to spawn terminal");
        *app_state.pty_sender.lock().unwrap() = Some(terminal.input_sender());
        let mut wake_rx = terminal.take_wake_rx();
        *app_state.terminal.lock().unwrap() = Some(terminal);

        // Event-driven render on PTY output
        cx.spawn(move |_entity, cx: &mut AsyncApp| {
            let cx = cx.clone();
            async move {
                loop {
                    if wake_rx.next().await.is_some() && cx.refresh().is_err() { break; }
                }
            }
        }).detach();

        // Cursor blink timer
        cx.spawn(|_entity, cx: &mut AsyncApp| {
            let cx = cx.clone();
            async move {
                loop {
                    smol::Timer::after(std::time::Duration::from_millis(
                        crate::term::CURSOR_BLINK_MS,
                    )).await;
                    let changed = cx.update(|cx| {
                        let state = cx.global::<AppState>();
                        let term = state.terminal.lock().unwrap();
                        if let Some(ref t) = *term {
                            let should_blink = t.modes().contains(Modes::SHOW_CURSOR)
                                && { let tl = t.inner_term().lock(); tl.cursor_style().blinking };
                            if should_blink { t.toggle_blink(); return true; }
                        }
                        false
                    });
                    if let Ok(true) = changed { let _ = cx.refresh(); }
                }
            }
        }).detach();

        Self {
            current_title: "NeoViolet".into(),
            last_window_title: String::new(),
            focus_handle: cx.focus_handle(),
            last_size: None,
            show_exit_error: false,
            entity_id: cx.entity_id(),
        }
    }
}

impl Render for NeoVioletApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.focus_handle.is_focused(window) { window.focus(&self.focus_handle); }

        let bounds = window.bounds();
        if bounds.size.width <= px(0.0) || bounds.size.height <= px(0.0) {
            return div().into_any_element();
        }

        let font_size = { let s = cx.global::<AppState>(); *s.font_size.lock().unwrap() as f32 };
        let font_family = { let s = cx.global::<AppState>(); s.font_family.lock().unwrap().clone() };
        let cell_w = (font_size * CELL_ASPECT).max(6.0);
        let cell_h = font_size * 1.2;

        // ── Terminal maintenance + snapshot (single lock) ──
        let (title_change, process_exited, lines, cols, cursor, ime_text) = {
            let titlebar_h = if !window.is_fullscreen() && cfg!(target_os = "macos") { 28.0 } else { 0.0 };
            let wf = bounds.size.width / px(1.0) - 16.0;
            let hf = bounds.size.height / px(1.0) - titlebar_h - 4.0;
            let ncols = (wf / cell_w).max(80.0) as usize;
            let nrows = (hf / cell_h).max(24.0) as usize;

            let state = cx.global::<AppState>();
            let mut term = state.terminal.lock().unwrap();

            let (title, exited, ls, cs, cur, ime) = if let Some(ref mut t) = *term {
                if self.last_size != Some(bounds.size) {
                    t.resize(ncols, nrows, cell_w as u16, cell_h as u16);
                    self.last_size = Some(bounds.size);
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
                            cx.write_to_clipboard(gpui::ClipboardItem::new_string(text));
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
                            let [r, g, b] = crate::term::dracula_fallback(idx);
                            let rgb = alacritty_terminal::vte::ansi::Rgb { r, g, b };
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
            (title, exited, ls, cs, cur, ime)
        };

        // ── React to terminal events ──
        if let Some(ref title) = title_change {
            self.current_title = title.clone();
        }
        if process_exited && !self.show_exit_error {
            self.show_exit_error = true;
        }

        if self.current_title != self.last_window_title {
            window.set_window_title(&self.current_title);
            self.last_window_title = self.current_title.clone();
        }

        // ── Dialog handlers ──
        let dismiss_about = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            *cx.global::<AppState>().show_about.lock().unwrap() = false; cx.notify();
        });
        let cancel_close = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            *cx.global::<AppState>().show_close.lock().unwrap() = false; cx.notify();
        });
        let do_quit = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());
        let restart_terminal = cx.listener(|this: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            let state = cx.global::<AppState>();
            let args = match state.terminal.lock().unwrap().as_ref().map(|t| t.exit_code()) {
                Some(Some(0)) => vec![],
                _ => state.launch_args.clone(),
            };
            let cfg = config::load_or_create();
            if let Ok(t) = TerminalState::spawn(&cfg, &args) {
                *state.pty_sender.lock().unwrap() = Some(t.input_sender());
                *state.terminal.lock().unwrap() = Some(t);
            }
            this.show_exit_error = false;
            *state.show_exit_error.lock().unwrap() = false;
            cx.notify();
        });
        let dismiss_exit = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());

        let (show_close, show_about) = {
            let s = cx.global::<AppState>();
            (*s.show_close.lock().unwrap(), *s.show_about.lock().unwrap())
        };

        // ── Build UI ──
        let default_fg: Hsla = gpui::rgb(0xf8f8f2).into();

        let ime_overlay: Option<AnyElement> = match (&ime_text, cursor) {
            (Some(text), Some((col, row, _))) => {
                let left = col as f32 * cell_w + PAD_LEFT;
                let top = row as f32 * cell_h;
                Some(div().absolute().left(px(left)).top(px(top)).h(px(cell_h))
                    .text_size(px(font_size)).text_color(gpui::rgb(DRACULA_CURSOR))
                    .underline().child(text.clone()).into_any_element())
            }
            _ => None,
        };

        let cursor_overlay: Option<AnyElement> = cursor.and_then(|(col, row, style)| {
            if !style.visible { return None; }
            let ch = lines.get(row).and_then(|l| l.cells.get(col)).map(|c| c.ch).unwrap_or(' ');
            let left = col as f32 * cell_w + PAD_LEFT;
            let top = row as f32 * cell_h;
            let wide = col < cols.saturating_sub(1)
                && lines.get(row).and_then(|l| l.cells.get(col)).is_some_and(|c| c.is_wide);
            let w = if wide { cell_w * 2.0 } else { cell_w };
            Some(match style.shape {
                CursorShape::Block => div().absolute().left(px(left)).top(px(top)).min_w(px(w)).h(px(cell_h))
                    .bg(gpui::rgb(DRACULA_CURSOR)).text_color(gpui::rgb(DRACULA_BG)).text_size(px(font_size))
                    .flex().items_center().justify_center().child(ch.to_string()).into_any_element(),
                CursorShape::HollowBlock => div().absolute().left(px(left)).top(px(top)).min_w(px(w)).h(px(cell_h))
                    .border_1().border_color(gpui::rgb(DRACULA_CURSOR)).text_size(px(font_size)).text_color(default_fg)
                    .flex().items_center().justify_center().child(ch.to_string()).into_any_element(),
                CursorShape::Underline => div().absolute().left(px(left)).top(px(top + cell_h - 2.0))
                    .min_w(px(w)).h(px(2.0)).bg(gpui::rgb(DRACULA_CURSOR)).into_any_element(),
                CursorShape::Beam => div().absolute().left(px(left)).top(px(top))
                    .w(px(2.0)).h(px(cell_h)).bg(gpui::rgb(DRACULA_CURSOR)).into_any_element(),
            })
        });

        let terminal_view = move || {
            if lines.is_empty() {
                return div().flex_1().bg(gpui::rgb(DRACULA_BG))
                    .flex().items_center().justify_center()
                    .child(div().text_color(gpui::rgb(0xf8f8f2)).text_sm().child("No terminal"))
                    .into_any_element();
            }
            div().id("terminal-view").flex_1().overflow_hidden().px_2().pb_1()
                .font_family(font_family).text_size(px(font_size)).text_color(default_fg)
                .bg(gpui::rgb(DRACULA_BG))
                .child(div().flex().flex_col().children(lines.iter().map(|line| {
                    let cw = cell_w; let ch = cell_h; let dfg = default_fg;
                    build_line(line, cw, ch, dfg)
                })))
                .when(ime_overlay.is_some(), |b| b.child(ime_overlay.unwrap()))
                .when(cursor_overlay.is_some(), |b| b.child(cursor_overlay.unwrap()))
                .on_mouse_down(MouseButton::Left, mk_mouse(cell_w, cell_h, MouseButton::Left))
                .on_mouse_down(MouseButton::Right, mk_mouse(cell_w, cell_h, MouseButton::Right))
                .on_mouse_down(MouseButton::Middle, mk_mouse(cell_w, cell_h, MouseButton::Middle))
                .on_mouse_up(MouseButton::Left, mk_mouse_up(cell_w, cell_h, MouseButton::Left))
                .on_mouse_up(MouseButton::Right, mk_mouse_up(cell_w, cell_h, MouseButton::Right))
                .on_mouse_up(MouseButton::Middle, mk_mouse_up(cell_w, cell_h, MouseButton::Middle))
                .on_mouse_move(mk_mouse_move(cell_w, cell_h))
                .on_scroll_wheel(mk_scroll(cell_w, cell_h))
                .into_any_element()
        };

        let base = div().bg(cx.theme().surface.canvas).size_full().flex().flex_col()
            .when(!window.is_fullscreen() && cfg!(target_os = "macos"), |base| {
                base.child(div().id("titlebar").w_full().h(px(28.0))
                    .bg(cx.theme().surface.base).text_color(cx.theme().content.primary)
                    .flex().flex_row().items_center()
                    .child(div().id("titlebar:drag-area")
                        .window_control_area(WindowControlArea::Drag)
                        .h_full().flex().flex_row().items_center().flex_grow().min_w(px(0.0)).pl_3()
                        .child(div().flex_grow())
                        .child(div().id("titlebar:title").font_weight(FontWeight::SEMIBOLD).text_sm()
                            .child(self.current_title.clone()))
                        .child(div().flex_grow()))
                    .on_mouse_down(MouseButton::Left, cx.listener(move |_t, ev: &MouseDownEvent, w, cx| {
                        if ev.click_count > 1 { w.zoom_window(); cx.notify(); }
                    })))
            })
            .child(terminal_view());

        let base = if self.show_exit_error {
            let exit_msg = {
                let state = cx.global::<AppState>();
                let term = state.terminal.lock().unwrap();
                term.as_ref().map(|t| match t.exit_code() {
                    Some(0) => "NeoViolet has exited.\nClose GUI or restart?".to_string(),
                    Some(c) => format!("NeoViolet crashed with exit code {c}."),
                    None => "NeoViolet has ended.".to_string(),
                }).unwrap_or_else(|| "NeoViolet has ended.".to_string())
            };
            base.child(components::render_error_dialog(cx, "NeoViolet Exited", &exit_msg,
                "Restart", restart_terminal, Some(dismiss_exit)))
        } else { base };

        let base = if show_close {
            base.child(components::render_close_dialog(cx, cancel_close, do_quit))
        } else { base };

        if show_about { base.child(components::render_about_dialog(cx, dismiss_about)) } else { base }.into_any_element()
    }
}

// ── Focus & IME ──

impl Focusable for NeoVioletApp {
    fn focus_handle(&self, _cx: &App) -> FocusHandle { self.focus_handle.clone() }
}

impl InputHandler for NeoVioletApp {
    fn selected_text_range(&mut self, _: bool, _: &mut Window, _: &mut App) -> Option<UTF16Selection> { None }
    fn marked_text_range(&mut self, _: &mut Window, cx: &mut App) -> Option<std::ops::Range<usize>> {
        let t = cx.global::<AppState>().terminal.lock().unwrap();
        t.as_ref().and_then(|t| t.ime_preedit()).map(|p| { let l = p.encode_utf16().count(); 0..l })
    }
    fn text_for_range(&mut self, r: std::ops::Range<usize>, _adj: &mut Option<std::ops::Range<usize>>, _: &mut Window, cx: &mut App) -> Option<String> {
        let t = cx.global::<AppState>().terminal.lock().unwrap();
        t.as_ref().and_then(|t| t.ime_preedit()).and_then(|p| {
            let u: Vec<u16> = p.encode_utf16().collect();
            (r.start < u.len() && r.end <= u.len()).then(|| String::from_utf16_lossy(&u[r]))
        })
    }
    fn replace_text_in_range(&mut self, _: Option<std::ops::Range<usize>>, text: &str, _: &mut Window, cx: &mut App) {
        cx.global::<AppState>().set_ime_preedit(None);
        cx.global::<AppState>().send_to_pty(text.as_bytes().to_vec());
        cx.notify(self.entity_id);
    }
    fn replace_and_mark_text_in_range(&mut self, _: Option<std::ops::Range<usize>>, text: &str, _: Option<std::ops::Range<usize>>, _: &mut Window, cx: &mut App) {
        cx.global::<AppState>().set_ime_preedit(if text.is_empty() { None } else { Some(text.into()) });
        cx.notify(self.entity_id);
    }
    fn unmark_text(&mut self, _: &mut Window, cx: &mut App) {
        cx.global::<AppState>().set_ime_preedit(None);
        cx.notify(self.entity_id);
    }
    fn bounds_for_range(&mut self, _: std::ops::Range<usize>, _: &mut Window, _: &mut App) -> Option<Bounds<Pixels>> { None }
    fn character_index_for_point(&mut self, _: Point<Pixels>, _: &mut Window, _: &mut App) -> Option<usize> { None }
}

// ── Cell rendering ──

fn build_line(line: &crate::term::CachedLine, cw: f32, ch: f32, dfg: Hsla) -> impl IntoElement {
    let cells = &line.cells;
    if cells.is_empty() { return div().flex().h(px(ch)).into_any_element(); }
    let end = cells.iter().rposition(|c| c.ch != ' ').map_or(0, |i| i + 1);
    if end == 0 { return div().flex().h(px(ch)).into_any_element(); }
    div().flex().children(cells[..end].iter().map(|cell| {
        let c = if cell.ch == ' ' { '\u{00A0}' } else { cell.ch };
        let mut dc = div().min_w(px(if cell.is_wide { cw * 2.0 } else { cw })).h(px(ch))
            .flex().items_center().justify_center().child(c.to_string());
        if let Some([r, g, b]) = cell.fg {
            dc = dc.text_color(gpui::rgb((r as u32) << 16 | (g as u32) << 8 | b as u32));
        } else { dc = dc.text_color(dfg); }
        if let Some([r, g, b]) = cell.bg {
            dc = dc.bg(gpui::rgb((r as u32) << 16 | (g as u32) << 8 | b as u32));
        }
        if cell.bold { dc = dc.font_weight(FontWeight::BOLD); }
        if cell.italic { dc = dc.italic(); }
        if cell.underline { dc = dc.underline(); }
        if cell.strikethrough { dc = dc.line_through(); }
        dc
    })).into_any_element()
}

// ── Mouse event helpers ──

fn grid_coords(pos: Point<Pixels>, cw: f32, ch: f32) -> (usize, usize) {
    TerminalBounds { line_height: px(ch), cell_width: px(cw),
        bounds: Bounds { origin: point(px(0.0), px(0.0)), size: size(px(2000.0), px(2000.0)) }
    }.grid_point(pos)
}

fn mk_mouse(cw: f32, ch: f32, btn: MouseButton) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) {
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

fn mk_mouse_up(cw: f32, ch: f32, btn: MouseButton) -> impl Fn(&MouseUpEvent, &mut Window, &mut App) {
    move |ev: &MouseUpEvent, _w, cx| {
        let (col, row) = grid_coords(ev.position, cw, ch);
        let m = cx.global::<AppState>().cached_modes();
        if let Some(s) = crate::mouse::mouse_button_report(&m, btn, &ev.modifiers, false, col as u16, row as u16) {
            cx.global::<AppState>().send_to_pty(s);
        }
    }
}

fn mk_mouse_move(cw: f32, ch: f32) -> impl Fn(&MouseMoveEvent, &mut Window, &mut App) {
    move |ev: &MouseMoveEvent, _w, cx| {
        let (col, row) = grid_coords(ev.position, cw, ch);
        let m = cx.global::<AppState>().cached_modes();
        if let Some(s) = crate::mouse::mouse_moved_report(&m, ev.pressed_button, &ev.modifiers, col as u16, row as u16) {
            cx.global::<AppState>().send_to_pty(s);
        }
    }
}

fn mk_scroll(cw: f32, ch: f32) -> impl Fn(&ScrollWheelEvent, &mut Window, &mut App) {
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
                hyperlink::open_url(&url); return true;
            }
        }
    }
    false
}
