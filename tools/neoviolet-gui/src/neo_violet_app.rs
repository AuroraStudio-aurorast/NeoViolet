//! Root component: terminal (TerminalApp child entity) + dialogs.
//!
//! Handles focus management: when dialogs are visible, focus stays on this
//! component so that ESC can dismiss About/Close dialogs. The exit-error
//! dialog is intentionally NOT ESC-dismissable — it requires an explicit
//! button click (Restart or Close).

use gpui::prelude::*;
use gpui::*;
use yororen_ui::headless::modal::ModalState;

use crate::app::TerminalApp;
use crate::components;
use crate::drop_paste;
use crate::ipc::IpcMessage;
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
    /// Window opacity (0.0–1.0), applied to the root element.
    pub opacity: f32,
    /// Dialog state entities. yororen-ui 0.3's modal renderer draws nothing
    /// until its `ModalState` is open, and the flags that decide this are set
    /// from threads that cannot touch a gpui entity (the IPC path raises the
    /// quit dialog from the socket thread), so the flags stay the source of
    /// truth and `render` mirrors them into these entities. Titles are not set
    /// here: this renderer never reads them, our own shell draws the visible
    /// heading.
    about_modal: Entity<ModalState>,
    close_modal: Entity<ModalState>,
    error_modal: Entity<ModalState>,
    bad_args_modal: Entity<ModalState>,
}

impl NeoVioletApp {
    pub fn new(terminal_child: Entity<TerminalApp>, opacity: f32, cx: &mut Context<Self>) -> Self {
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
            opacity,
            about_modal: ModalState::new(cx),
            close_modal: ModalState::new(cx),
            error_modal: ModalState::new(cx),
            bad_args_modal: ModalState::new(cx),
        }
    }

    /// Match a dialog's state entity to the flag that drives it. Guarded
    /// because `update` notifies observers and `render` runs every frame.
    fn sync_modal(state: &Entity<ModalState>, open: bool, cx: &mut Context<Self>) {
        if state.read(cx).is_open() != open {
            state.update(cx, |s, _| if open { s.open() } else { s.close() });
        }
    }

    /// Attach a lightweight observer that simply notifies NeoVioletApp
    /// whenever TerminalApp changes. The actual status inspection happens
    /// in `render()`, which runs at most once per frame and is naturally
    /// guarded against double-triggering.
    fn attach_terminal_observer(child: &Entity<TerminalApp>, cx: &mut Context<Self>) {
        cx.observe(child, |_: &mut NeoVioletApp, _child, cx| {
            cx.notify();
        })
        .detach();
    }

    /// Create a brand-new TerminalApp, swap it in, and re-attach observe.
    fn restart_terminal(&mut self, cx: &mut Context<Self>) {
        // Drop old reference in global state
        {
            let state = cx.global::<AppState>();
            *state.terminal_child.lock().unwrap() = None;
        }

        let new_child = cx.new(TerminalApp::new);
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
        cx.global::<AppState>()
            .recent_output
            .lock()
            .unwrap()
            .clear();
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

        // ── Pending file paths from Dock-icon drop / open-file event ──
        // macOS delivers these through `on_open_urls`, which can fire long
        // after the cold start has consumed its share. There is no new spawn
        // to hand them to, so they are pasted into the running PTY like a
        // window drop — the process is never restarted for a file.
        let late_paths: Vec<String> = {
            let pending = cx.global::<AppState>().pending_file_paths.clone();
            if let Ok(mut guard) = pending.lock() {
                guard.drain(..).collect()
            } else {
                Vec::new()
            }
        };
        if !late_paths.is_empty() {
            log::debug!(
                "[drag-drop] handing {} file(s) from an open-file event to the PTY",
                late_paths.len()
            );
            let outcome = drop_paste::send_paths(cx, &self.terminal_child, &late_paths);
            drop_paste::defer_pending_paths(cx, outcome, late_paths);
        }

        // ── IPC messages from TUI ──
        {
            let incoming = cx.global::<AppState>().ipc_incoming.clone();
            if let Ok(mut guard) = incoming.lock() {
                let msgs: Vec<String> = guard.drain(..).collect();
                for raw in msgs {
                    match serde_json::from_str::<IpcMessage>(&raw) {
                        Ok(msg) if msg.msg_type == "quit" => {
                            // dialog: true = show dialog (:q/:quit)
                            // dialog: false = quit immediately (:wq)
                            if msg.dialog == Some(true) {
                                log::info!("[ipc] :quit — showing close dialog");
                                *cx.global::<AppState>().show_close.lock().unwrap() = true;
                            } else {
                                log::info!("[ipc] :wq — exiting immediately");
                                *cx.global::<AppState>().show_exit_error.lock().unwrap() = true;
                                cx.quit();
                                return div().into_any_element();
                            }
                        }
                        Ok(msg) if msg.msg_type == "lyrics" => {
                            let mut state = cx.global::<AppState>().lyrics_state.lock().unwrap();
                            // Always update lines — clear when None (e.g. lyrics unloaded).
                            state.lines = msg.lines.unwrap_or_default();
                            state.elapsed = msg.elapsed.unwrap_or(0.0);
                            state.title = msg.title.unwrap_or_default();
                            state.artist = msg.artist.unwrap_or_default();
                            state.dirty = true;
                            log::trace!(
                                "[lyrics] updated: title={:?} artist={:?} elapsed={:.1} lines={}",
                                state.title,
                                state.artist,
                                state.elapsed,
                                state.lines.len(),
                            );
                        }
                        Ok(msg) if msg.msg_type == "desktop_lyrics" => {
                            if let Some(enable) = msg.enable {
                                let mut guard = cx
                                    .global::<AppState>()
                                    .desktop_lyrics_enabled
                                    .lock()
                                    .unwrap();
                                let was_enabled = *guard;
                                if enable == was_enabled {
                                    continue;
                                }
                                *guard = enable;
                                drop(guard);

                                if enable {
                                    log::info!("[ipc] desktop_lyrics enabled from CLI");
                                    let state = cx.global::<AppState>();
                                    let lyrics_cfg = state.config.desktop_lyrics.clone();
                                    let handle_slot = state.lyrics_window_handle.clone();
                                    let window_opts =
                                        components::lyrics_window_options(&lyrics_cfg);
                                    // Defer window creation to avoid crashing during render.
                                    cx.spawn(async move |_, cx| {
                                        cx.background_executor()
                                            .timer(std::time::Duration::from_millis(0))
                                            .await;
                                        let _ = cx.update(|cx| {
                                            cx.open_window(window_opts, move |window, cx| {
                                                let root = cx.new(
                                                    crate::desktop_lyrics::DesktopLyricsView::new,
                                                );
                                                *handle_slot.lock().unwrap() =
                                                    Some(window.window_handle());
                                                root
                                            })
                                        });
                                    })
                                    .detach();
                                } else {
                                    log::info!("[ipc] desktop_lyrics disabled from CLI");
                                }
                            }
                        }
                        Ok(other) => {
                            log::debug!("[ipc] unhandled message: {:?}", other);
                        }
                        Err(e) => {
                            log::warn!("[ipc] invalid JSON: {} — {}", raw, e);
                        }
                    }
                }
            }
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
                    let raw = cx
                        .global::<AppState>()
                        .recent_output
                        .lock()
                        .unwrap()
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

        Self::sync_modal(&self.about_modal, show_about, cx);
        Self::sync_modal(&self.close_modal, show_close, cx);
        Self::sync_modal(
            &self.error_modal,
            self.show_exit_error && !self.exit_is_bad_args,
            cx,
        );
        Self::sync_modal(&self.bad_args_modal, self.exit_is_bad_args, cx);

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
        let restart_terminal = cx.listener(|this: &mut NeoVioletApp, _: &ClickEvent, _w, cx| {
            // Clear launch args for a clean restart
            cx.global::<AppState>().launch_args.lock().unwrap().clear();
            this.restart_terminal(cx);
        });
        let dismiss_exit = cx.listener(|_: &mut NeoVioletApp, _: &ClickEvent, _w, cx| cx.quit());

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
        } else if self.exit_reason.contains("exit code") || self.exit_reason.contains("killed") {
            format!("NeoViolet crashed with {}.", self.exit_reason)
        } else if self.exit_reason.is_empty() {
            "NeoViolet has ended.".to_string()
        } else {
            format!("NeoViolet has ended.\nReason: {}", self.exit_reason)
        };

        // ── Build UI ──
        // ── In-window drag-and-drop intake ──
        // GPUI does not deliver `FileDropEvent::Entered`/`Submit` to app
        // listeners: the platform layer turns them into a MouseMove and a
        // MouseUp, carrying the dropped paths in `active_drag`. Only `Exited`
        // is dispatched as a FileDropEvent. The paths are therefore picked up
        // by an element-level `on_drop`, which GPUI fires on the MouseUp of a
        // drop for the element under the cursor — hence on this root element,
        // which covers the whole window.
        let drop_child = self.terminal_child.clone();
        let base = div()
            .id("aria:app:neoviolet-gui")
            .size_full()
            .flex()
            .flex_col()
            .opacity(self.opacity)
            .track_focus(&self.focus_handle)
            .on_key_down(on_key_down)
            .on_drop(move |paths: &ExternalPaths, _window, cx| {
                let dropped: Vec<String> = paths
                    .paths()
                    .iter()
                    .map(|p| p.to_string_lossy().to_string())
                    .collect();
                if dropped.is_empty() {
                    return;
                }
                log::info!("[drag-drop] dropped {} file(s)", dropped.len());
                let outcome = drop_paste::send_paths(cx, &drop_child, &dropped);
                drop_paste::defer_pending_paths(cx, outcome, dropped);
            })
            .when(
                !window.is_fullscreen() && cfg!(target_os = "macos"),
                |base| {
                    base.child(
                        div()
                            .id("aria:region:titlebar")
                            .w_full()
                            .h(px(28.0))
                            .bg(crate::theme_colors::sunken(cx))
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
                                            .text_color(crate::theme_colors::secondary(cx))
                                            .child(self.current_title.clone()),
                                    )
                                    .child(div().flex_grow()),
                            )
                            .on_mouse_down(
                                MouseButton::Left,
                                cx.listener(move |_t, ev: &MouseDownEvent, w, cx| {
                                    if ev.click_count > 1 {
                                        w.zoom_window();
                                        cx.notify();
                                    }
                                }),
                            ),
                    )
                },
            )
            .child(div().flex_1().child(self.terminal_child.clone()));

        let base = if self.exit_is_bad_args {
            base.child(components::render_bad_args_dialog(
                cx,
                &self.bad_args_modal,
                &self.exit_output,
                dismiss_exit,
            ))
        } else if self.show_exit_error {
            base.child(components::render_error_dialog(
                cx,
                &self.error_modal,
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
                &self.close_modal,
                cancel_close,
                do_quit,
            ))
        } else {
            base
        };

        if show_about {
            base.child(components::render_about_dialog(
                cx,
                &self.about_modal,
                dismiss_about,
            ))
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
