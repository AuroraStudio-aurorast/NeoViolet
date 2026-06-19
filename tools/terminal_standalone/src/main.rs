mod app;
mod backend;
mod terminal;

use app::TerminalApp;
use gpui::{AppContext as _, WindowOptions};

fn main() {
    // Sync macOS launch environment
    #[cfg(target_os = "macos")]
    {
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
                            | "SHELL"
                            | "HOME"
                            | "HOMEBREW_PREFIX"
                            | "HOMEBREW_CELLAR"
                            | "HOMEBREW_REPOSITORY"
                    ) || key.starts_with("LC_");
                    if should_import {
                        unsafe {
                            std::env::set_var(key, value);
                        }
                    }
                }
            }
        }
    }

    // Init logging
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    #[cfg(target_os = "macos")]
    let app = gpui_platform::application().with_quit_mode(gpui::QuitMode::LastWindowClosed);

    #[cfg(not(target_os = "macos"))]
    let app = gpui_platform::application();

    app.run(move |cx| {
        gpui_component::init(cx);

        cx.open_window(
            WindowOptions {
                window_bounds: Some(gpui::WindowBounds::Windowed(gpui::Bounds::new(
                    gpui::point(gpui::px(100.), gpui::px(100.)),
                    gpui::size(gpui::px(900.), gpui::px(550.)),
                ))),
                ..Default::default()
            },
            |window, cx| {
                window.activate_window();
                window.set_window_title("Terminal");

                let view = cx.new(|cx| TerminalApp::new(cx));
                let focus_handle = view.read(cx).focus_handle.clone();
                window.focus(&focus_handle, cx);
                view
            },
        )
        .unwrap();
    });
}
