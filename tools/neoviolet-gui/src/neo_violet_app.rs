//! Root component: terminal (TerminalApp child entity) + dialogs.
//!
//! Handles focus management: when dialogs are visible, focus stays on this
//! component so that ESC can dismiss About/Close dialogs. The exit-error
//! dialog is intentionally NOT ESC-dismissable — it requires an explicit
//! button click (Restart or Close).

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
    pub exit_reason: String,
    /// True when the CLI exited with a non-zero code within 3 s — likely
    /// invalid arguments. The dialog then shows captured help output.
    pub exit_is_bad_args: bool,
    pub exit_output: String,
}

impl NeoVioletApp {
    pub fn new(
        terminal_child: Entity<TerminalApp>,
        cx: &mut Context<Self>,
    ) -> Self {
        Self::attach_terminal_observer(&terminal_child, cx);

        let initial_title = terminal_child.read(cx).current_title().to_string();

        Self {
            terminal_child,
            current_title: initial_title.clone(),
            last_window_title: initial_title,
            focus_handle: cx.focus_handle(),
            show_exit_error: false,
            exit_reason: String::new(),
            exit_is_bad_args: false,
            exit_output: String::new(),
        }
    }

    /// Attach a lightweight observer that simply notifies NeoVioletApp
    /// whenever TerminalApp changes. The actual status inspection happens
    /// in `render()`, which runs at most once per frame and is naturally
    /// guarded against double-triggering.
    fn attach_terminal_observer(
        child: &Entity<TerminalApp>,
        cx: &mut Context<Self>,
    ) {
        cx.observe(child, |_: &mut NeoVioletApp, _child, cx| {
            cx.notify();
        })
        .detach();
    }

    /// Create a brand-new TerminalApp, swap it in, and re-attach observe.
    fn restart_terminal(&mut self, cx: &mut Context<Self>) {
        // Drop old reference in global state and clear launch args
        // so the restarted process starts with a clean slate.
        {
            let state = cx.global::<AppState>();
            *state.terminal_child.lock().unwrap() = None;
            state.launch_args.lock().unwrap().clear();
        }

        let new_child = cx.new(|cx| TerminalApp::new(cx));
        cx.global::<AppState>()
            .terminal_child
            .lock()
            .unwrap()
            .replace(new_child.downgrade());

        // Re-attach observer so the *new* child's exit is detected
        Self::attach_terminal_observer(&new_child, cx);

        self.terminal_child = new_child;
        self.current_title.clear();
        self.last_window_title.clear();
        self.show_exit_error = false;
        *cx.global::<AppState>().show_exit_error.lock().unwrap() = false;
        self.exit_reason.clear();
        self.exit_is_bad_args = false;
        self.exit_output.clear();
        // Reset diagnostics buffer and timer for the new process
        cx.global::<AppState>().recent_output.lock().unwrap().clear();
        *cx.global::<AppState>().process_start.lock().unwrap() = None;
        cx.notify();
    }
}

impl Render for NeoVioletApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let bounds = window.bounds();
        if bounds.size.width <= px(0.0) || bounds.size.height <= px(0.0) {
            return div().into_any_element();
        }

        // ── Sync title from terminal child ──
        let new_title = self.terminal_child.read(cx).current_title().to_string();
        if new_title != self.current_title {
            self.current_title = new_title;
        }

        // ── Exit detection (once per frame — no double-fire) ──
        if !self.show_exit_error {
            let status = &self.terminal_child.read(cx).tab.status;
            if status.contains("exited") || status.contains("closed") {
                self.exit_reason = status.clone();
                self.show_exit_error = true;
                *cx.global::<AppState>().show_exit_error.lock().unwrap() = true;

                // Detect bad-args / invalid-flags scenario:
                //   - non-zero exit code (not "exit code 0")
                //   - process lived less than 3 seconds
                //   - the PTY has captured output (likely --help text)
                let is_non_zero = !status.contains("exit code 0");
                let is_quick = {
                    let ps = cx.global::<AppState>().process_start.lock().unwrap();
                    ps.map(|t| t.elapsed() < std::time::Duration::from_secs(3))
                        .unwrap_or(false)
                };
                if is_non_zero && is_quick {
                    let raw = cx.global::<AppState>()
                        .recent_output.lock().unwrap()
                        .clone();
                    let cleaned = crate::util::strip_ansi_escapes(&raw);
                    if !cleaned.trim().is_empty() {
                        self.exit_is_bad_args = true;
                        self.exit_output = cleaned;
                    }
                }
            }
        }

        // ── Focus management ──
        // When any dialog is visible, keep focus on NeoVioletApp so that
        // ESC can dismiss About/Close dialogs. When all dialogs are gone,
        // focus the terminal so keystrokes reach the PTY.
        let (show_close, show_about) = {
            let s = cx.global::<AppState>();
            (*s.show_close.lock().unwrap(), *s.show_about.lock().unwrap())
        };
        let any_dialog = self.show_exit_error || show_close || show_about;

        if any_dialog {
            if !self.focus_handle.is_focused(window) {
                window.focus(&self.focus_handle);
            }
        } else {
            let terminal_focus = self.terminal_child.read(cx).focus_handle.clone();
            if !terminal_focus.is_focused(window) {
                window.focus(&terminal_focus);
            }
        }

        // ── Window title ──
        if !self.current_title.is_empty() && self.current_title != self.last_window_title {
            window.set_window_title(&self.current_title);
            self.last_window_title = self.current_title.clone();
        }

        // ── Dialog button handlers ──
        let dismiss_about = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            *cx.global::<AppState>().show_about.lock().unwrap() = false;
            cx.notify();
        });
        let cancel_close = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            *cx.global::<AppState>().show_close.lock().unwrap() = false;
            cx.notify();
        });
        let do_quit = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());
        let restart_terminal =
            cx.listener(|this: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
                this.restart_terminal(cx);
            });
        let dismiss_exit =
            cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());

        // ── ESC key handler ──
        // Dismisses About and Close dialogs. The exit-error dialog is
        // intentionally NOT ESC-dismissable: the user must explicitly click
        // Restart or Close.
        let on_key_down = cx.listener(
            |_this: &mut NeoVioletApp, event: &KeyDownEvent, window, cx| {
                if event.keystroke.key.eq_ignore_ascii_case("escape") {
                    let (sc, sa) = {
                        let s = cx.global::<AppState>();
                        (*s.show_close.lock().unwrap(), *s.show_about.lock().unwrap())
                    };
                    if sa {
                        *cx.global::<AppState>().show_about.lock().unwrap() = false;
                        window.prevent_default();
                        cx.stop_propagation();
                        cx.notify();
                    } else if sc {
                        *cx.global::<AppState>().show_close.lock().unwrap() = false;
                        window.prevent_default();
                        cx.stop_propagation();
                        cx.notify();
                    }
                    // Exit-error dialog: ESC is intentionally ignored here.
                }
            },
        );

        // ── Exit message ──
        let exit_msg = if self.exit_reason.contains("exit code 0")
            || self.exit_reason.contains("exited")
        {
            "NeoViolet has exited.\nClose GUI or restart?".to_string()
        } else if self.exit_reason.contains("exit code")
            || self.exit_reason.contains("killed")
        {
            format!("NeoViolet crashed with {}.", self.exit_reason)
        } else if self.exit_reason.is_empty() {
            "NeoViolet has ended.".to_string()
        } else {
            format!("NeoViolet has ended.\nReason: {}", self.exit_reason)
        };

        // ── Build UI ──
        let base = div()
            .id("aria:app:neoviolet-gui")
            .size_full()
            .flex()
            .flex_col()
            .track_focus(&self.focus_handle)
            .on_key_down(on_key_down)
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

        let base = if self.exit_is_bad_args {
            base.child(components::render_bad_args_dialog(
                cx,
                &self.exit_output,
                dismiss_exit,
            ))
        } else if self.show_exit_error {
            base.child(components::render_error_dialog(
                cx,
                "NeoViolet Exited",
                &exit_msg,
                "Restart",
                restart_terminal,
                Some(dismiss_exit),
            ))
        } else {
            base
        };

        let base = if show_close {
            base.child(components::render_close_dialog(
                cx,
                cancel_close,
                do_quit,
            ))
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

impl Focusable for NeoVioletApp {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}
