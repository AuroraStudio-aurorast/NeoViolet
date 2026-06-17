use gpui::*;
use std::sync::OnceLock;
use std::time::Duration;

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
        ZoomReset
    ]
);

pub static CLI_VER: OnceLock<String> = OnceLock::new();
pub static GUI_VER: &str = env!("CARGO_PKG_VERSION");

/// Version check timeout — prevents the GUI from hanging at startup if the
/// neoviolet binary is stuck (macOS watchdog kills non-responsive apps).
const VERSION_CHECK_TIMEOUT: Duration = Duration::from_secs(3);

pub fn setup(cx: &mut App, neoviolet_path: Option<&str>) {
    // Cache CLI version asynchronously — never block startup.
    // The version string is available via cached_cli_version() (returns
    // "loading…" until the background thread finishes) and is written
    // into AppState for the About dialog.
    let configured_path = neoviolet_path.map(|s| s.to_string());
    let _ = CLI_VER.set(String::new()); // placeholder
    let version_tx = {
        let state = cx.global::<AppState>();
        state.cli_version.clone()
    };
    std::thread::spawn(move || {
        let ver = cli_version(configured_path.as_deref());
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

    // Zoom — uses AppState for font size
    cx.on_action(move |_: &ZoomIn, cx: &mut App| {
        let eid = {
            let state = cx.global::<AppState>();
            let mut fs = state.font_size.lock().unwrap();
            *fs = (*fs + 2).min(48);
            *state.root_entity_id.lock().unwrap()
        };
        if let Some(eid) = eid {
            cx.notify(eid);
        }
    });

    cx.on_action(move |_: &ZoomOut, cx: &mut App| {
        let eid = {
            let state = cx.global::<AppState>();
            let mut fs = state.font_size.lock().unwrap();
            *fs = (*fs).saturating_sub(2).max(8);
            *state.root_entity_id.lock().unwrap()
        };
        if let Some(eid) = eid {
            cx.notify(eid);
        }
    });

    cx.on_action(move |_: &ZoomReset, cx: &mut App| {
        let eid = {
            let state = cx.global::<AppState>();
            *state.font_size.lock().unwrap() = 14;
            *state.root_entity_id.lock().unwrap()
        };
        if let Some(eid) = eid {
            cx.notify(eid);
        }
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
            items: vec![],
        },
    ]);
}

fn cli_version(configured_path: Option<&str>) -> String {
    let bin = find_neoviolet_binary(configured_path);

    // Run --version with a timeout so a stuck/hanging binary doesn't
    // freeze the GUI at startup (macOS watchdog kills after ~5 s).
    let raw = run_version_command(&bin);

    if raw.is_empty() {
        return "unknown".to_string();
    }

    let version = raw
        .strip_prefix("NeoViolet version ")
        .unwrap_or(&raw)
        .trim()
        .to_string();

    let version = version.strip_prefix('v').unwrap_or(&version).to_string();

    if version.is_empty() {
        return "unknown".to_string();
    }

    // Sanity check: version must be mostly printable ASCII
    if version.chars().any(|c| c.is_control() && c != '\n') {
        return "unknown".to_string();
    }

    // Truncate unreasonably long version strings
    if version.len() > 128 {
        return version[..128].to_string();
    }

    version
}

/// Run `neoviolet --version` in a background thread with a timeout.
/// Returns the captured stdout, or an empty string on timeout / error.
fn run_version_command(bin: &str) -> String {
    let (tx, rx) = std::sync::mpsc::channel();
    let bin = bin.to_string();
    std::thread::spawn(move || {
        let raw = std::process::Command::new(&bin)
            .arg("--version")
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default();
        let _ = tx.send(raw);
    });
    rx.recv_timeout(VERSION_CHECK_TIMEOUT).unwrap_or_default()
}

/// Locate the `neoviolet` binary, respecting the user-configured path first.
fn find_neoviolet_binary(configured_path: Option<&str>) -> String {
    // 1) Explicit path from GUI config
    if let Some(path) = configured_path
        && std::path::Path::new(path).exists()
    {
        return path.to_string();
    }

    // 2) Sibling next to the GUI binary
    if let Ok(exe) = std::env::current_exe()
        && let Some(dir) = exe.parent()
    {
        let c = dir.join("neoviolet");
        if c.exists() {
            return c.to_string_lossy().to_string();
        }
    }

    // 3) Fallback to PATH
    "neoviolet".to_string()
}
