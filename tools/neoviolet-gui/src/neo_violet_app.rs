//! Root component: terminal (TerminalApp child entity) + dialogs.
//!
//! Wraps the terminal_standalone TerminalApp with menu/dialog support
//! from the old neoviolet-gui project.

use gpui::*;
use gpui::prelude::*;
use yororen_ui::theme::ActiveTheme;

use crate::app::TerminalApp;
use crate::components;
use crate::state::AppState;

pub struct NeoVioletApp {
    pub terminal_child: Entity<TerminalApp>,
    pub current_title: String,
    pub last_window_title: String,
    pub focus_handle: FocusHandle,
    pub show_exit_error: bool,
}

impl NeoVioletApp {
    pub fn new(
        terminal_child: Entity<TerminalApp>,
        cx: &mut Context<Self>,
    ) -> Self {
        // Observe terminal child for title changes
        cx.observe(&terminal_child, |this: &mut NeoVioletApp, child, cx| {
            this.current_title = child.read(cx).current_title().to_string();
            cx.notify();
        })
        .detach();

        // Observe for exit status
        cx.observe(&terminal_child, move |this: &mut NeoVioletApp, child, cx| {
            let status = &child.read(cx).tab.status;
            if status.contains("exited") || status.contains("closed") {
                if !this.show_exit_error {
                    this.show_exit_error = true;
                    *cx.global::<AppState>().show_exit_error.lock().unwrap() = true;
                }
            }
            cx.notify();
        })
        .detach();

        // Set initial title from terminal child
        let initial_title = terminal_child.read(cx).current_title().to_string();

        Self {
            terminal_child,
            current_title: initial_title.clone(),
            last_window_title: initial_title,
            focus_handle: cx.focus_handle(),
            show_exit_error: false,
        }
    }
}

impl Render for NeoVioletApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        // Focus the terminal child so its InputHandler receives keyboard events
        let terminal_focus = self.terminal_child.read(cx).focus_handle.clone();
        if !terminal_focus.is_focused(window) {
            window.focus(&terminal_focus);
        }

        let bounds = window.bounds();
        if bounds.size.width <= px(0.0) || bounds.size.height <= px(0.0) {
            return div().into_any_element();
        }

        // Update window title
        if !self.current_title.is_empty() && self.current_title != self.last_window_title {
            window.set_window_title(&self.current_title);
            self.last_window_title = self.current_title.clone();
        }

        // ── Dialog handlers ──
        let dismiss_about =
            cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
                *cx.global::<AppState>().show_about.lock().unwrap() = false;
                cx.notify();
            });
        let cancel_close =
            cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
                *cx.global::<AppState>().show_close.lock().unwrap() = false;
                cx.notify();
            });
        let do_quit =
            cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());

        let restart_terminal = cx.listener(|this: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            // Drop old child + create new one (font/args read from AppState in TerminalApp::new)
            {
                let state = cx.global::<AppState>();
                *state.terminal_child.lock().unwrap() = None;
            };

            let new_child = cx.new(|cx| TerminalApp::new(cx));

            *cx.global::<AppState>().terminal_child.lock().unwrap() = Some(new_child.downgrade());
            this.terminal_child = new_child;
            this.current_title.clear();
            this.last_window_title.clear();

            {
                let state = cx.global::<AppState>();
                *state.show_exit_error.lock().unwrap() = false;
            }
            this.show_exit_error = false;
            cx.notify();
        });

        let dismiss_exit =
            cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());

        let (show_close, show_about) = {
            let s = cx.global::<AppState>();
            (*s.show_close.lock().unwrap(), *s.show_about.lock().unwrap())
        };

        // ── Build UI ──
        let base = div()
            .id("aria:app:neoviolet-gui")
            .size_full()
            .flex()
            .flex_col()
            .when(
                !window.is_fullscreen() && cfg!(target_os = "macos"),
                |base| {
                    base.child(
                        div()
                            .id("aria:region:titlebar")
                            .w_full()
                            .h(px(28.0))
                            .bg(cx.theme().surface.sunken)
                            .flex()
                            .flex_row()
                            .items_center()
                            .child(
                                div()
                                    .id("aria:titlebar:drag-area")
                                    .window_control_area(WindowControlArea::Drag)
                                    .h_full()
                                    .flex()
                                    .flex_row()
                                    .items_center()
                                    .flex_grow()
                                    .min_w(px(0.0))
                                    .pl_3()
                                    .child(div().flex_grow())
                                    .child(
                                        div()
                                            .id("aria:titlebar:title")
                                            .font_weight(FontWeight::SEMIBOLD)
                                            .text_sm()
                                            .text_color(cx.theme().content.secondary)
                                            .child(self.current_title.clone()),
                                    )
                                    .child(div().flex_grow()),
                            )
                            .on_mouse_down(
                                MouseButton::Left,
                                cx.listener(
                                    move |_t, ev: &MouseDownEvent, w, cx| {
                                        if ev.click_count > 1 {
                                            w.zoom_window();
                                            cx.notify();
                                        }
                                    },
                                ),
                            ),
                    )
                },
            )
            .child(div().flex_1().child(self.terminal_child.clone()));

        let base = if self.show_exit_error {
            base.child(components::render_error_dialog(
                cx,
                "Terminal Exited",
                "The terminal process has exited.\nClose window or restart?",
                "Restart",
                restart_terminal,
                Some(dismiss_exit),
            ))
        } else {
            base
        };

        let base = if show_close {
            base.child(components::render_close_dialog(cx, cancel_close, do_quit))
        } else {
            base
        };

        if show_about {
            base.child(components::render_about_dialog(cx, dismiss_about))
        } else {
            base
        }
        .into_any_element()
    }
}

// ── Focus & IME ──

impl Focusable for NeoVioletApp {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}
