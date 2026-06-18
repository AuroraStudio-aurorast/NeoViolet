use gpui::*;
use yororen_ui::assets::UiAsset;
use yororen_ui::component;
use yororen_ui::i18n::{I18n, Locale};
use yororen_ui::theme::{GlobalTheme, ThemeSet};

mod components;
mod config;
mod bounds;
mod dracula_theme;
mod event_listener;
mod hyperlink;
mod key_encode;
mod menus;
mod modes;
mod mouse;
mod neo_violet_app;
mod platform;
mod state;
mod term;
mod terminal_view;

use key_encode::encode_key_seq;
use neo_violet_app::NeoVioletApp;
use modes::Modes;
use state::AppState;

fn main() {
    env_logger::init();
    let gui_config = config::load_or_create();
    let user_args: Vec<String> = std::env::args().skip(1).collect();

    // ── 1) Create app with Yororen UI assets ──
    let app = Application::new().with_assets(UiAsset);

    app.run(move |cx: &mut App| {
        // ── 2) Init Yororen UI components ──
        component::init(cx);

        // ── 3) Theme (Dracula) ──
        let dracula = std::sync::Arc::new(dracula_theme::dracula_theme());
        cx.set_global(GlobalTheme::new_with_themes(
            WindowAppearance::Dark,
            ThemeSet::new(dracula.clone()).dark(dracula),
        ));

        // ── 4) i18n ──
        cx.set_global(I18n::with_embedded(Locale::new("en").unwrap()));

        // ── 5) AppState (no mpsc channel — keys go direct to PTY) ──
        cx.set_global(AppState::new(gui_config.clone(), user_args.clone()));

        // ── 6) Menu setup (CLI version check is now non-blocking) ──
        menus::setup(cx, gui_config.neoviolet_path.as_deref());

        // ── 7) Keystroke observer — sends directly to PTY, zero buffering ──
        cx.observe_keystrokes(move |e, _w, cx| {
            let modifiers = e.keystroke.modifiers;

            // ── Bracketed paste: Cmd/Ctrl+V ──
            if modifiers.platform
                && (e.keystroke.key.as_str() == "v" || e.keystroke.key.as_str() == "V")
            {
                if let Some(item) = cx.read_from_clipboard() {
                    if let Some(text) = item.text().as_deref() {
                        let paste = bracketed_paste_wrap(cx, text);
                        let state = cx.global::<AppState>();
                        state.send_to_pty(paste.into_bytes());
                    }
                }
                return;
            }

            // Don't forward other app shortcuts (Cmd/Win + key)
            if modifiers.platform {
                return;
            }

            // ── Escape: dismiss dialogs (not exit_error — user must choose) ──
            if e.keystroke.key.as_str() == "escape"
                && !modifiers.control && !modifiers.alt && !modifiers.shift
            {
                let (dismissed, notify_eid) = {
                    let state = cx.global::<AppState>();
                    let about = *state.show_about.lock().unwrap();
                    let close = *state.show_close.lock().unwrap();
                    if about { *state.show_about.lock().unwrap() = false; }
                    if close { *state.show_close.lock().unwrap() = false; }
                    (about || close, *state.root_entity_id.lock().unwrap())
                };
                if dismissed {
                    if let Some(eid) = notify_eid {
                        cx.notify(eid);
                    }
                    return;
                }
            }

            // Read locally-cached terminal modes (no FairMutex lock).
            let mode = cx.global::<AppState>().cached_modes();

            if let Some(bytes) = encode_key_seq(
                e.keystroke.key.as_str(),
                e.keystroke.key_char.as_deref(),
                &modifiers,
                &mode,
                true,
            ) {
                cx.global::<AppState>().send_to_pty(bytes);
            }
        })
        .detach();

        // ── 8) Open window ──
        // macOS: custom transparent titlebar with traffic-light buttons.
        // Linux/Windows: explicit native system titlebar (appears_transparent: false).
        let titlebar_opts = Some(TitlebarOptions {
            title: Some("NeoViolet".into()),
            appears_transparent: cfg!(target_os = "macos"),
            traffic_light_position: None,
        });

        cx.open_window(
            WindowOptions {
                window_bounds: Some(WindowBounds::Windowed(Bounds::new(
                    point(px(0.0), px(0.0)),
                    size(
                        px(gui_config.window_width as f32),
                        px(gui_config.window_height as f32),
                    ),
                ))),
                titlebar: titlebar_opts,
                window_background: WindowBackgroundAppearance::Opaque,
                ..Default::default()
            },
            move |window, cx| {
                let args = user_args.clone();
                // Window close → skip confirmation if process already exited
                window.on_window_should_close(cx, move |w, cx| {
                    let already_exited = {
                        let state = cx.global::<AppState>();
                        *state.show_exit_error.lock().unwrap()
                    };
                    if already_exited {
                        return true;
                    }
                    let eid = {
                        let state = cx.global::<AppState>();
                        *state.show_close.lock().unwrap() = true;
                        *state.root_entity_id.lock().unwrap()
                    };
                    if let Some(eid) = eid {
                        cx.notify(eid);
                    }
                    w.refresh();
                    false
                });

                let root_entity = cx.new(|cx| NeoVioletApp::new(cx, args));
                // Store root entity ID for menu/dialog notifications
                cx.global::<AppState>().root_entity_id.lock().unwrap().replace(root_entity.entity_id());
                root_entity
            },
        )
        .unwrap();
    });
}

fn bracketed_paste_wrap(cx: &mut App, text: &str) -> String {
    let state = cx.global::<AppState>();
    let use_bracketed = state.cached_modes().contains(Modes::BRACKETED_PASTE);
    if use_bracketed {
        format!("\x1b[200~{}\x1b[201~", text)
    } else {
        text.to_string()
    }
}
