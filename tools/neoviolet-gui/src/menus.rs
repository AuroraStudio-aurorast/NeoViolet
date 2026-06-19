use gpui::*;
use std::sync::OnceLock;

use crate::components;
use crate::state::AppState;

actions!(
    neoviolet_gui,
    [
        About,
        Preferences,
        QuitApp,
        ZoomIn,
        ZoomOut,
        ZoomReset,
        OpenRepository,
    ]
);

pub static CLI_VER: OnceLock<String> = OnceLock::new();
pub static GUI_VER: &str = env!("CARGO_PKG_VERSION");

pub fn setup(cx: &mut App, neoviolet_path: Option<&str>) {
    // Cache CLI version asynchronously — never block startup.
    let configured_path = neoviolet_path.map(|s| s.to_string());
    let _ = CLI_VER.set(String::new()); // placeholder
    let version_tx = {
        let state = cx.global::<AppState>();
        state.cli_version.clone()
    };
    std::thread::spawn(move || {
        let ver = crate::platform::fetch_cli_version(configured_path.as_deref());
        let _ = CLI_VER.set(ver.clone());
        if let Ok(mut guard) = version_tx.lock() {
            *guard = ver;
        }
    });

    // About
    cx.on_action(move |_: &About, cx: &mut App| {
        components::open_about(cx);
    });

    // Preferences — reveal config directory
    cx.on_action(|_: &Preferences, cx: &mut App| {
        let dir = dirs::config_dir()
            .unwrap_or_else(|| dirs::home_dir().unwrap_or_default().join(".config"))
            .join("neoviolet");
        cx.reveal_path(&dir);
    });

    // Quit — skip confirmation if process already exited
    cx.on_action(|_: &QuitApp, cx: &mut App| {
        let skip_confirm = {
            let state = cx.global::<AppState>();
            *state.show_exit_error.lock().unwrap()
        };
        if skip_confirm {
            cx.quit();
        } else {
            components::open_close(cx);
        }
    });

    // Zoom — updates AppState + live TerminalApp
    cx.on_action(move |_: &ZoomIn, cx: &mut App| {
        let (new_fs, weak_opt, root_eid) = {
            let state = cx.global::<AppState>();
            let mut fs = state.font_size.lock().unwrap();
            *fs = (*fs + 2).min(48);
            let new_fs = *fs as f32;
            let weak = state.terminal_child.lock().unwrap().clone();
            let eid = *state.root_entity_id.lock().unwrap();
            (new_fs, weak, eid)
        };
        if let Some(ref weak) = weak_opt {
            if let Some(terminal) = weak.upgrade() {
                terminal.update(cx, |t, cx| {
                    t.terminal_font_size = new_fs;
                    cx.notify();
                });
            }
        }
        if let Some(eid) = root_eid {
            cx.notify(eid);
        }
    });

    cx.on_action(move |_: &ZoomOut, cx: &mut App| {
        let (new_fs, weak_opt, root_eid) = {
            let state = cx.global::<AppState>();
            let mut fs = state.font_size.lock().unwrap();
            *fs = (*fs).saturating_sub(2).max(8);
            let new_fs = *fs as f32;
            let weak = state.terminal_child.lock().unwrap().clone();
            let eid = *state.root_entity_id.lock().unwrap();
            (new_fs, weak, eid)
        };
        if let Some(ref weak) = weak_opt {
            if let Some(terminal) = weak.upgrade() {
                terminal.update(cx, |t, cx| {
                    t.terminal_font_size = new_fs;
                    cx.notify();
                });
            }
        }
        if let Some(eid) = root_eid {
            cx.notify(eid);
        }
    });

    cx.on_action(move |_: &ZoomReset, cx: &mut App| {
        let (new_fs, weak_opt, root_eid) = {
            let state = cx.global::<AppState>();
            *state.font_size.lock().unwrap() = 14;
            let new_fs = 14.0_f32;
            let weak = state.terminal_child.lock().unwrap().clone();
            let eid = *state.root_entity_id.lock().unwrap();
            (new_fs, weak, eid)
        };
        if let Some(ref weak) = weak_opt {
            if let Some(terminal) = weak.upgrade() {
                terminal.update(cx, |t, cx| {
                    t.terminal_font_size = new_fs;
                    cx.notify();
                });
            }
        }
        if let Some(eid) = root_eid {
            cx.notify(eid);
        }
    });

    cx.on_action(|_: &OpenRepository, _cx: &mut App| {
        let _ = std::process::Command::new("open")
            .arg("https://github.com/AuroraStudio-aurorast/NeoViolet").spawn();
    });

    cx.bind_keys([
        KeyBinding::new("cmd-q", QuitApp, None),
        KeyBinding::new("cmd-,", Preferences, None),
        KeyBinding::new("cmd-+", ZoomIn, None),
        KeyBinding::new("cmd-=", ZoomIn, None),
        KeyBinding::new("cmd--", ZoomOut, None),
        KeyBinding::new("cmd-0", ZoomReset, None),
    ]);

    #[cfg(target_os = "macos")]
    cx.set_menus(vec![
        Menu {
            name: "NeoViolet GUI".into(),
            items: vec![
                MenuItem::action("About NeoViolet GUI", About),
                MenuItem::separator(),
                MenuItem::action("Preferences…", Preferences),
                MenuItem::separator(),
                MenuItem::action("Quit NeoViolet GUI", QuitApp),
            ],
        },
        Menu {
            name: "View".into(),
            items: vec![
                MenuItem::action("Zoom In", ZoomIn),
                MenuItem::action("Zoom Out", ZoomOut),
                MenuItem::action("Actual Size", ZoomReset),
            ],
        },
        Menu {
            name: "Window".into(),
            items: vec![],
        },
        Menu {
            name: "Help".into(),
            items: vec![
                MenuItem::action("GitHub Repository", OpenRepository),
            ],
        },
    ]);
}
