//! Root component: titlebar + terminal + dialogs.
//!
//! Delegates terminal rendering/maintenance to `terminal_view`, mouse/key
//! encoding to `mouse`/`key_encode`, and cross-platform logic to `platform`.

use gpui::*;
use gpui::prelude::*;
use futures::StreamExt;
use yororen_ui::theme::ActiveTheme;

use crate::components;
use crate::config;
use crate::state::AppState;
use crate::term::TerminalState;
use crate::terminal_view::{self, CELL_ASPECT};

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
                            let should_blink = t.modes().contains(crate::modes::Modes::SHOW_CURSOR)
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

        // ── Terminal maintenance (single lock) ──
        let (title_change, process_exited, lines, cols, cursor, ime_text) =
            terminal_view::maintain_terminal(cx, bounds, cell_w, cell_h, &mut self.last_size);

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
        let ime_overlay = terminal_view::render_ime_overlay(&ime_text, cursor, cell_w, cell_h, font_size);
        let cursor_overlay = terminal_view::render_cursor_overlay(&lines, cols, cursor, cell_w, cell_h, font_size, default_fg);

        let terminal_element = move || {
            if lines.is_empty() {
                return div().id("aria:terminal:empty").flex_1().bg(gpui::rgb(terminal_view::DRACULA_BG))
                    .flex().items_center().justify_center()
                    .child(div().text_color(gpui::rgb(0xf8f8f2)).text_sm().child("No terminal"))
                    .into_any_element();
            }
            div().id("aria:region:terminal").flex_1().overflow_hidden().px_2().pb_1()
                .font_family(font_family).text_size(px(font_size)).text_color(default_fg)
                .bg(gpui::rgb(terminal_view::DRACULA_BG))
                .child(div().flex().flex_col().children(lines.iter().map(|line| {
                    terminal_view::build_line(line, cell_w, cell_h, default_fg)
                })))
                .when(ime_overlay.is_some(), |b| b.child(ime_overlay.unwrap()))
                .when(cursor_overlay.is_some(), |b| b.child(cursor_overlay.unwrap()))
                .on_mouse_down(MouseButton::Left, terminal_view::mk_mouse_down(cell_w, cell_h, MouseButton::Left))
                .on_mouse_down(MouseButton::Right, terminal_view::mk_mouse_down(cell_w, cell_h, MouseButton::Right))
                .on_mouse_down(MouseButton::Middle, terminal_view::mk_mouse_down(cell_w, cell_h, MouseButton::Middle))
                .on_mouse_up(MouseButton::Left, terminal_view::mk_mouse_up(cell_w, cell_h, MouseButton::Left))
                .on_mouse_up(MouseButton::Right, terminal_view::mk_mouse_up(cell_w, cell_h, MouseButton::Right))
                .on_mouse_up(MouseButton::Middle, terminal_view::mk_mouse_up(cell_w, cell_h, MouseButton::Middle))
                .on_mouse_move(terminal_view::mk_mouse_move(cell_w, cell_h))
                .on_scroll_wheel(terminal_view::mk_scroll(cell_w, cell_h))
                .into_any_element()
        };

        let base = div().id("aria:app:neoviolet-gui").bg(cx.theme().surface.canvas).size_full().flex().flex_col()
            .when(!window.is_fullscreen() && cfg!(target_os = "macos"), |base| {
                base.child(div().id("aria:region:titlebar").w_full().h(px(28.0))
                    .bg(cx.theme().surface.base).text_color(cx.theme().content.primary)
                    .flex().flex_row().items_center()
                    .child(div().id("aria:titlebar:drag-area")
                        .window_control_area(WindowControlArea::Drag)
                        .h_full().flex().flex_row().items_center().flex_grow().min_w(px(0.0)).pl_3()
                        .child(div().flex_grow())
                        .child(div().id("aria:titlebar:title").font_weight(FontWeight::SEMIBOLD).text_sm()
                            .child(self.current_title.clone()))
                        .child(div().flex_grow()))
                    .on_mouse_down(MouseButton::Left, cx.listener(move |_t, ev: &MouseDownEvent, w, cx| {
                        if ev.click_count > 1 { w.zoom_window(); cx.notify(); }
                    })))
            })
            .child(terminal_element());

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
