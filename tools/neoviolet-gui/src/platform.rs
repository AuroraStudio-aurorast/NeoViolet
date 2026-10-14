//! Cross-platform utilities: binary discovery, font defaults, version check,
//! input method detection, URL opening, window appearance.

use std::time::Duration;

#[cfg(target_os = "macos")]
mod macos_tis;

/// Whether a composing input method (Chinese, Japanese, Korean) is selected.
///
/// Only macOS implements the detection; elsewhere this reports `false`, which
/// makes `ImeRouting::Auto` behave like `ImeRouting::Never`. `ImeRouting::Always`
/// is how a caller asks for the other behaviour regardless of platform.
pub fn ime_is_active() -> bool {
    #[cfg(target_os = "macos")]
    {
        macos_tis::ime_is_active()
    }
    #[cfg(not(target_os = "macos"))]
    {
        false
    }
}

/// Which desktop the GUI is running on.
///
/// Callers that care pick their layout by this instead of by build tags, so the
/// unselected layouts stay compilable and testable everywhere.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PlatformKind {
    MacOs,
    Windows,
    Linux,
}

/// The platform this binary was built for.
///
/// Anything that is neither Windows nor macOS is treated as Linux, which is
/// where gpui's supported desktops live.
pub fn current_platform_kind() -> PlatformKind {
    if cfg!(target_os = "macos") {
        PlatformKind::MacOs
    } else if cfg!(target_os = "windows") {
        PlatformKind::Windows
    } else {
        PlatformKind::Linux
    }
}

/// Returns the neoviolet binary name for this platform.
pub fn binary_name() -> &'static str {
    if cfg!(target_os = "windows") {
        "neoviolet.exe"
    } else {
        "neoviolet"
    }
}

/// Locate the `neoviolet` binary.
/// 1. User-configured path from GUI config
/// 2. Sibling next to the GUI binary
/// 3. Fallback to PATH (just the binary name)
pub fn find_neoviolet_binary(configured_path: Option<&str>) -> String {
    if let Some(path) = configured_path
        && std::path::Path::new(path).exists()
    {
        return path.to_string();
    }

    if let Ok(exe) = std::env::current_exe()
        && let Some(dir) = exe.parent()
    {
        let candidate = dir.join(binary_name());
        if candidate.exists() {
            return candidate.to_string_lossy().to_string();
        }
    }

    binary_name().to_string()
}

/// Returns a monospace font available on this platform.
pub fn default_monospace_font() -> &'static str {
    if cfg!(target_os = "macos") {
        "Menlo"
    } else if cfg!(target_os = "windows") {
        "Consolas"
    } else {
        "Monospace"
    }
}

/// The system font database, loaded once.
///
/// Callers sit on the startup path and ask more than once (terminal font, then
/// lyric font), and a scan is not free.
fn system_fonts() -> &'static fontdb::Database {
    static FONTS: std::sync::OnceLock<fontdb::Database> = std::sync::OnceLock::new();
    FONTS.get_or_init(|| {
        let mut database = fontdb::Database::new();
        database.load_system_fonts();
        database
    })
}

/// The generic font family a name stands for, if any.
///
/// Generic names (`Monospace`, `Sans`, …) are resolved by the system's font stack
/// rather than listed in the family database, so a lookup by name cannot find them
/// even though they work. Only these names are exempt from the lookup: exempting
/// every miss would report an uninstalled font as available.
fn generic_family(family: &str) -> Option<fontdb::Family<'static>> {
    match family.trim().to_ascii_lowercase().as_str() {
        "monospace" => Some(fontdb::Family::Monospace),
        "sans" | "sans-serif" | "sans serif" => Some(fontdb::Family::SansSerif),
        "serif" => Some(fontdb::Family::Serif),
        "cursive" => Some(fontdb::Family::Cursive),
        "fantasy" => Some(fontdb::Family::Fantasy),
        _ => None,
    }
}

/// Whether this platform can resolve `family`, including generic aliases.
pub fn is_font_available(family: &str) -> bool {
    family_is_available(system_fonts(), family)
}

/// Whether `database` can resolve `family`.
///
/// A generic alias is resolved by the system's font stack rather than by an installed
/// family, so no font database answers for it: an empty one and a full one give the
/// same result, and asking anyway made the answer depend on which fonts the machine
/// happens to have. Aliases are therefore taken as available; only real family names
/// are looked up, which is what keeps a misspelled or uninstalled family reported as
/// missing.
fn family_is_available(database: &fontdb::Database, family: &str) -> bool {
    if generic_family(family).is_some() {
        return true;
    }

    let query = fontdb::Query {
        families: &[fontdb::Family::Name(family)],
        ..Default::default()
    };
    database.query(&query).is_some()
}

// ── CLI version check (async, non-blocking) ──

const VERSION_CHECK_TIMEOUT: Duration = Duration::from_secs(3);

/// Run `neoviolet --version` with a timeout. Used by menus::setup() in a
/// background thread — never blocks the main thread.
pub fn fetch_cli_version(configured_path: Option<&str>) -> String {
    let bin = find_neoviolet_binary(configured_path);
    let raw = run_version_command(&bin);
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return "unknown".to_string();
    }

    // 1) Try to extract a clean version number from known output formats:
    //    --version:  "NeoViolet version 1.2.3"
    //    version:    "neoviolet 1.2.3"
    if let Some(ver) = try_extract_version(trimmed) {
        return truncate(ver, 128);
    }

    // 2) Parsing failed — return the raw output as-is (truncated).
    truncate(trimmed.to_string(), 128)
}

/// Attempt to pull a human-readable version string from the CLI output.
/// Returns `None` when the output doesn't match any expected pattern.
fn try_extract_version(raw: &str) -> Option<String> {
    // Known prefixes emitted by `--version` / `version` subcommand
    let body = raw
        .strip_prefix("NeoViolet version ")
        .or_else(|| raw.strip_prefix("neoviolet "))
        .unwrap_or(raw)
        .trim();

    if body.is_empty() {
        return None;
    }

    // Strip optional leading 'v' (e.g. "v1.2.3" → "1.2.3")
    let version = body.strip_prefix('v').unwrap_or(body);

    // Reject strings containing ASCII control chars (except space)
    if version.chars().any(|c| c.is_ascii_control() && c != ' ') {
        return None;
    }

    Some(version.to_string())
}

fn truncate(s: String, max: usize) -> String {
    if s.len() > max {
        s[..max].to_string()
    } else {
        s
    }
}

/// Windows: keep a console program from opening a console window of its own.
#[cfg(target_os = "windows")]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

fn run_version_command(bin: &str) -> String {
    let (tx, rx) = std::sync::mpsc::channel();
    let bin = bin.to_string();
    std::thread::spawn(move || {
        let mut command = std::process::Command::new(&bin);
        command.arg("--version");
        // This child is a console program and the GUI has no console of its own,
        // so Windows would give it a console window to flash at startup.
        #[cfg(target_os = "windows")]
        {
            use std::os::windows::process::CommandExt;
            command.creation_flags(CREATE_NO_WINDOW);
        }
        let raw = command
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default();
        let _ = tx.send(raw);
    });
    rx.recv_timeout(VERSION_CHECK_TIMEOUT).unwrap_or_default()
}

// ── Opening URLs in the desktop's browser ──

/// How this platform hands a URL to the desktop.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum UrlOpener {
    /// macOS: the `open` command.
    Open,
    /// Linux and the BSDs: `xdg-open` through the desktop environment.
    XdgOpen,
    /// Windows: the Explorer shell.
    Explorer,
}

fn url_opener() -> UrlOpener {
    if cfg!(target_os = "macos") {
        UrlOpener::Open
    } else if cfg!(target_os = "windows") {
        UrlOpener::Explorer
    } else {
        UrlOpener::XdgOpen
    }
}

/// The program and arguments that open `url` here.
fn open_command(opener: UrlOpener, url: &str) -> (&'static str, Vec<String>) {
    match opener {
        UrlOpener::Open => ("open", vec![url.to_owned()]),
        UrlOpener::XdgOpen => ("xdg-open", vec![url.to_owned()]),
        UrlOpener::Explorer => ("explorer", vec![url.to_owned()]),
    }
}

/// Opens `url` with the desktop's own handler.
///
/// Spawned without a command-line interpreter, so the URL is passed directly
/// to the platform's desktop handler, and deliberately not waited on: the
/// browser outlives this call.
pub fn open_url(url: &str) -> std::io::Result<()> {
    let (program, args) = open_command(url_opener(), url);
    std::process::Command::new(program)
        .args(args)
        .spawn()
        .map(|_| ())
}

// ── Window appearance ──

/// What the title bar area has to draw on one platform.
///
/// One decision for both callers of [`chrome_plan`]: window creation and the
/// per-frame render. They differ only in what `client_decorations` means.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ChromePlan {
    /// Draw the app's title bar content; this can sit under native titlebar controls.
    pub titlebar: bool,
    /// Draw the app's frame treatment where native window decorations omit it.
    pub frame: bool,
    /// Draw the minimize/maximize/close buttons.
    pub window_buttons: bool,
    /// Draw the menu button, for platforms whose menus we render ourselves.
    pub menu_button: bool,
    /// Drag through `WindowControlArea::Drag` rather than an explicit move.
    pub drag_via_control_area: bool,
}

/// What `kind` draws, given the client decorations in play.
///
/// `client_decorations` is what the caller asked for at window creation and what
/// the window actually got at render time: on Linux the compositor may grant
/// server decorations anyway, in which case the desktop already draws a title
/// bar and drawing a second one would double it. `fullscreen` leaves the drag
/// area at its platform value so callers need not special-case it.
pub fn chrome_plan(kind: PlatformKind, client_decorations: bool, fullscreen: bool) -> ChromePlan {
    let mut plan = match kind {
        // The native frame and traffic lights stay; only the area under the
        // transparent title bar becomes ours.
        PlatformKind::MacOs => ChromePlan {
            titlebar: true,
            frame: false,
            window_buttons: false,
            menu_button: false,
            drag_via_control_area: true,
        },
        // No caption, but the frame keeps resizing and snapping, and the caption
        // controls are ours to draw and route back through the control areas.
        PlatformKind::Windows => ChromePlan {
            titlebar: true,
            frame: true,
            window_buttons: true,
            menu_button: true,
            drag_via_control_area: true,
        },
        // Client decorations are what make the title bar the window's own;
        // dragging goes through the compositor instead of a control area, which
        // is empty on Linux.
        PlatformKind::Linux if client_decorations => ChromePlan {
            titlebar: true,
            frame: true,
            window_buttons: true,
            menu_button: true,
            drag_via_control_area: false,
        },
        PlatformKind::Linux => ChromePlan {
            titlebar: false,
            frame: false,
            window_buttons: false,
            menu_button: false,
            drag_via_control_area: false,
        },
    };

    // A fullscreen window has no title bar anywhere.
    if fullscreen {
        plan.titlebar = false;
        plan.frame = false;
        plan.window_buttons = false;
        plan.menu_button = false;
    }

    plan
}

/// Whether the client can start an edge resize on this platform.
///
/// gpui implements `start_window_resize` on X11 and Wayland only; macOS and
/// Windows resize through their own native paths, where the call does nothing.
/// Mounting a self-drawn grab strip there would swallow the click without
/// resizing anything.
pub fn client_side_resize_is_honoured() -> bool {
    client_side_resize_is_honoured_on(current_platform_kind())
}

/// Whether `kind` resizes through the client, which is the same platforms that
/// drag through the compositor rather than through a window control area.
fn client_side_resize_is_honoured_on(kind: PlatformKind) -> bool {
    matches!(kind, PlatformKind::Linux)
}

/// Whether a window at `opacity` needs a transparent background.
///
/// Whether the request is honoured is up to the platform backend; the caller
/// keeps the mapping onto the appearance enum.
pub fn background_should_be_transparent(opacity: f32) -> bool {
    opacity < 1.0
}

// ── Console output ──

#[cfg(target_os = "windows")]
mod windows_console;

/// Give this process somewhere to write its log when it was started from a console.
///
/// Windows builds are GUI programs (see `main.rs`), so Windows never gives them a
/// console: started from a terminal, the process would otherwise have nowhere to
/// write. This attaches to that terminal's console. Started from Explorer there is
/// no console to attach to, and none appears — which is the point of the subsystem.
pub fn attach_parent_console() {
    #[cfg(target_os = "windows")]
    windows_console::attach_parent_console();
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn main_declares_the_windows_subsystem() {
        // Without it the binary links as a console program, and Windows opens a
        // console window beside the window it was asked for — the reason
        // `attach_parent_console` has to reattach to a terminal's console by hand.
        assert!(
            include_str!("main.rs").contains("windows_subsystem = \"windows\""),
            "main.rs must declare the Windows subsystem"
        );
    }

    #[test]
    fn the_macos_opener_uses_open() {
        assert_eq!(
            open_command(UrlOpener::Open, "https://example.com"),
            ("open", vec!["https://example.com".to_string()])
        );
    }

    #[test]
    fn the_linux_opener_uses_xdg_open() {
        assert_eq!(
            open_command(UrlOpener::XdgOpen, "https://example.com"),
            ("xdg-open", vec!["https://example.com".to_string()])
        );
    }

    #[test]
    fn the_windows_opener_uses_explorer() {
        assert_eq!(
            open_command(UrlOpener::Explorer, "https://example.com"),
            ("explorer", vec!["https://example.com".to_string()])
        );
    }

    #[test]
    fn the_url_is_always_a_single_argument() {
        // The command is spawned without a shell, so shell metacharacters in the
        // URL must stay inside one argument instead of being split or expanded.
        let url = "https://example.com/?a=1&b=2 file";
        for opener in [UrlOpener::Open, UrlOpener::XdgOpen, UrlOpener::Explorer] {
            let (_, args) = open_command(opener, url);
            assert_eq!(args.iter().filter(|arg| arg.as_str() == url).count(), 1);
        }
    }

    #[test]
    fn opener_programs_are_not_command_interpreters() {
        let interpreters = [
            "sh",
            "bash",
            "cmd",
            "cmd.exe",
            "powershell",
            "powershell.exe",
        ];
        for opener in [UrlOpener::Open, UrlOpener::XdgOpen, UrlOpener::Explorer] {
            let (program, _) = open_command(opener, "https://example.com");
            assert!(
                !interpreters
                    .iter()
                    .any(|interpreter| program.eq_ignore_ascii_case(interpreter)),
                "{program} is a command interpreter"
            );
        }
    }

    #[test]
    fn generic_family_names_are_recognised() {
        // The system font stack resolves these itself, so they are absent from the
        // family database while still naming a usable font.
        assert_eq!(generic_family("Monospace"), Some(fontdb::Family::Monospace));
        assert_eq!(
            generic_family("sans-serif"),
            Some(fontdb::Family::SansSerif)
        );
        assert_eq!(generic_family("SERIF"), Some(fontdb::Family::Serif));
        assert_eq!(generic_family("Comic Sans MS"), None);
    }

    #[test]
    fn a_generic_family_is_available_without_any_font_database() {
        // An empty database knows no fonts, and no database can answer for a generic
        // name anyway. Consulting one made this answer depend on the machine's installed
        // fonts, which is how a check on "Monospace" failed on a minimal font set.
        assert!(family_is_available(&fontdb::Database::new(), "Monospace"));
        assert!(family_is_available(&fontdb::Database::new(), "Sans"));
    }

    #[test]
    fn an_exact_family_name_is_looked_up_in_the_database() {
        // The other half of the rule: a real family name still needs the database, so a
        // misspelled or uninstalled family is reported instead of silently substituted.
        assert!(!family_is_available(&fontdb::Database::new(), "Menlo"));
    }

    #[test]
    fn the_linux_default_font_is_available() {
        // The Linux default is the generic "Monospace"; treating its absence from the
        // family database as "not installed" warned on every start and fell back to the
        // very same name. This has to hold on a machine with no fonts installed.
        assert!(is_font_available("Monospace"));
    }

    #[test]
    fn a_missing_font_family_is_unavailable() {
        assert!(!is_font_available("Definitely-Not-Installed Font 9000"));
    }

    #[test]
    fn blank_font_families_are_unavailable() {
        assert!(!is_font_available(""));
        assert!(!is_font_available("   "));
    }

    #[test]
    fn macos_keeps_its_native_frame_and_traffic_lights() {
        let plan = chrome_plan(
            PlatformKind::MacOs,
            /* client_decorations */ false,
            /* fullscreen */ false,
        );
        assert_eq!(
            plan,
            ChromePlan {
                titlebar: true,
                frame: false,
                window_buttons: false,
                menu_button: false,
                drag_via_control_area: true
            }
        );
    }

    #[test]
    fn windows_draws_its_own_caption_controls() {
        let plan = chrome_plan(PlatformKind::Windows, false, false);
        assert_eq!(
            plan,
            ChromePlan {
                titlebar: true,
                frame: true,
                window_buttons: true,
                menu_button: true,
                drag_via_control_area: true
            }
        );
    }

    #[test]
    fn linux_draws_everything_only_with_client_decorations() {
        let client = chrome_plan(PlatformKind::Linux, true, false);
        assert_eq!(
            client,
            ChromePlan {
                titlebar: true,
                frame: true,
                window_buttons: true,
                menu_button: true,
                drag_via_control_area: false
            }
        );
    }

    #[test]
    fn linux_with_server_decorations_draws_nothing() {
        let server = chrome_plan(PlatformKind::Linux, false, false);
        assert!(
            !server.titlebar && !server.frame && !server.window_buttons && !server.menu_button,
            "服务端装饰下必须什么都不画，否则会出现双标题栏"
        );
    }

    #[test]
    fn fullscreen_never_draws_the_titlebar() {
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            for client in [true, false] {
                let plan = chrome_plan(kind, client, true);
                assert!(!plan.titlebar, "{kind:?} 全屏时不得绘制标题栏");
                assert!(!plan.frame, "{kind:?} 全屏时不得绘制窗口边框");
                assert!(!plan.window_buttons, "{kind:?} 全屏时不得绘制窗口按钮");
                assert!(!plan.menu_button, "{kind:?} 全屏时不得绘制菜单按钮");
                assert_eq!(
                    plan.drag_via_control_area,
                    kind != PlatformKind::Linux,
                    "{kind:?} 全屏仍保留平台拖拽策略"
                );
            }
        }
    }

    #[test]
    fn no_platform_needs_its_own_window_buttons_off_the_client_side() {
        assert!(!chrome_plan(PlatformKind::MacOs, true, false).window_buttons);
    }

    #[test]
    fn only_linux_honours_a_client_side_resize() {
        // gpui implements `start_window_resize` on X11 and Wayland only, so the
        // self-drawn grab strips would swallow clicks without resizing anywhere else.
        assert_eq!(
            client_side_resize_is_honoured(),
            matches!(current_platform_kind(), PlatformKind::Linux)
        );
        // The platforms whose drag goes through the compositor instead of a control
        // area are exactly the ones that own the resize path as well.
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            let plan = chrome_plan(
                kind, /* client_decorations */ true, /* fullscreen */ false,
            );
            assert_eq!(
                client_side_resize_is_honoured_on(kind),
                !plan.drag_via_control_area,
                "{kind:?} 的自绘缩放与自绘拖拽必须走同一条路径"
            );
        }
    }

    #[test]
    fn opacity_below_one_asks_for_a_transparent_background() {
        assert!(background_should_be_transparent(0.5));
        assert!(background_should_be_transparent(0.99));
        assert!(!background_should_be_transparent(1.0));
    }
}
