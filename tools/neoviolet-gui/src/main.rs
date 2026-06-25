mod app;
mod backend;
mod components;
mod config;
mod desktop_lyrics;
mod dracula_theme;
mod ipc;
mod menus;
mod neo_violet_app;
#[cfg(target_os = "macos")]
mod open_files;
mod platform;
mod state;
mod terminal;
mod util;

use gpui::*;
use std::sync::{Arc, Mutex, OnceLock};
use yororen_ui::assets::UiAsset;
use yororen_ui::component;
use yororen_ui::i18n::{I18n, Locale};
use yororen_ui::theme::{GlobalTheme, ThemeSet};

use app::TerminalApp;
use neo_violet_app::NeoVioletApp;
use state::AppState;

fn main() {
    // Sync macOS launch environment (runs at most once, before any threads).
    #[cfg(target_os = "macos")]
    {
        static ENV_SYNC: OnceLock<()> = OnceLock::new();
        ENV_SYNC.get_or_init(|| {
            let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".to_string());
            if let Ok(output) = std::process::Command::new(&shell)
                .args(["-l", "-c", "env -0"])
                .output()
            {
                if output.status.success() {
                    for entry in output.stdout.split(|b| *b == 0) {
                        if entry.is_empty() {
                            continue;
                        }
                        let Some(eq) = entry.iter().position(|b| *b == b'=') else {
                            continue;
                        };
                        let Ok(key) = std::str::from_utf8(&entry[..eq]) else {
                            continue;
                        };
                        let Ok(value) = std::str::from_utf8(&entry[eq + 1..]) else {
                            continue;
                        };
                        let should_import = matches!(
                            key,
                            "PATH"
                                | "LANG"
                                | "LC_ALL"
                                | "LC_CTYPE"
                                | "LC_MESSAGES"
                                | "LC_MONETARY"
                                | "LC_NUMERIC"
                                | "LC_TIME"
                                | "SHELL"
                                | "HOME"
                                | "HOMEBREW_PREFIX"
                                | "HOMEBREW_CELLAR"
                                | "HOMEBREW_REPOSITORY"
                        );
                        if should_import {
                            unsafe {
                                std::env::set_var(key, value);
                            }
                        }
                    }
                }
            }
        });
    }

    env_logger::init();
    let gui_config = config::load_or_create();
    let user_args: Vec<String> = std::env::args().skip(1).collect();

    let app = Application::new().with_assets(UiAsset);

    // ── Shared pending-file-paths store ──
    // On macOS, `on_open_urls` fires before/during `run()` and feeds file
    // paths here. On other platforms this stays empty — files arrive via
    // CLI args (`launch_args`) instead.
    let pending_urls: Arc<Mutex<Vec<String>>> = Arc::new(Mutex::new(Vec::new()));

    // ── macOS: handle files dropped onto Dock icon / opened via Finder ──
    #[cfg(target_os = "macos")]
    {
        let pending = pending_urls.clone();
        app.on_open_urls(move |urls| {
            let paths = open_files::extract_file_paths(&urls);
            if !paths.is_empty() {
                log::info!("[open_urls] received {} file(s)", paths.len());
                if let Ok(mut guard) = pending.lock() {
                    *guard = paths;
                }
            }
        });
    }
    #[cfg(target_os = "macos")]
    app.on_reopen(move |_cx: &mut App| {
        log::info!("[reopen] app re-opened via Dock / Finder");
    });

    app.run(move |cx: &mut App| {
        component::init(cx);

        // Theme (Dracula)
        let dracula = std::sync::Arc::new(dracula_theme::dracula_theme());
        cx.set_global(GlobalTheme::new_with_themes(
            WindowAppearance::Dark,
            ThemeSet::new(dracula.clone()).dark(dracula),
        ));

        // i18n
        cx.set_global(I18n::with_embedded(Locale::new("en").unwrap()));

        // AppState — seed with the shared pending-urls handle so that
        // NeoVioletApp can pick up late-arriving open-file events.
        {
            let mut state = AppState::new(gui_config.clone(), user_args.clone());
            state.pending_file_paths = pending_urls.clone();
            cx.set_global(state);
        }

        // Menu setup
        menus::setup(cx, gui_config.neoviolet_path.as_deref());

        // Open window
        let titlebar_opts = Some(TitlebarOptions {
            title: Some("NeoViolet".into()),
            appears_transparent: cfg!(target_os = "macos"),
            traffic_light_position: None,
        });

        let config_opacity = gui_config.opacity.clamp(0.1, 1.0);
        let window_background = if config_opacity < 1.0 {
            WindowBackgroundAppearance::Transparent
        } else {
            WindowBackgroundAppearance::Opaque
        };

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
                window_background,
                ..Default::default()
            },
            move |window, cx| {
                let _args = user_args.clone();

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

                let terminal_child = cx.new(|cx| TerminalApp::new(cx));
                cx.global::<AppState>()
                    .terminal_child
                    .lock()
                    .unwrap()
                    .replace(terminal_child.downgrade());

                let root_entity = cx.new(|cx| NeoVioletApp::new(terminal_child, config_opacity, cx));
                cx.global::<AppState>()
                    .root_entity_id
                    .lock()
                    .unwrap()
                    .replace(root_entity.entity_id());
                root_entity
            },
        )
        .unwrap();
    });
}
