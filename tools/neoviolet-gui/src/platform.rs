//! Cross-platform utilities: binary discovery, font defaults, URL opening.
//!
//! Code previously scattered across term.rs, menus.rs, config.rs, and hyperlink.rs.

use std::time::Duration;

/// Returns the neoviolet binary name for this platform.
pub fn binary_name() -> &'static str {
    if cfg!(target_os = "windows") { "neoviolet.exe" } else { "neoviolet" }
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
        // "Monospace" is a fontconfig alias present on all major Linux desktops.
        "Monospace"
    }
}

/// Open a URL in the system default browser.
pub fn open_url(url: &str) {
    let result = if cfg!(target_os = "macos") {
        std::process::Command::new("open").arg(url).spawn()
    } else if cfg!(target_os = "linux") {
        std::process::Command::new("xdg-open").arg(url).spawn()
    } else if cfg!(target_os = "windows") {
        std::process::Command::new("cmd").args(["/c", "start", url]).spawn()
    } else {
        return;
    };
    let _ = result;
}

// ── CLI version check (async, non-blocking) ──

const VERSION_CHECK_TIMEOUT: Duration = Duration::from_secs(3);

/// Run `neoviolet --version` with a timeout. Used by menus::setup() in a
/// background thread — never blocks the main thread.
pub fn fetch_cli_version(configured_path: Option<&str>) -> String {
    let bin = find_neoviolet_binary(configured_path);
    let raw = run_version_command(&bin);
    if raw.is_empty() { return "unknown".to_string(); }

    let version = raw
        .strip_prefix("NeoViolet version ")
        .unwrap_or(&raw)
        .trim()
        .strip_prefix('v')
        .unwrap_or(&raw.trim())
        .to_string();

    // Sanity: version must be mostly printable ASCII
    if version.is_empty() || version.chars().any(|c| c.is_control() && c != '\n') {
        return "unknown".to_string();
    }

    // Truncate unreasonably long version strings
    if version.len() > 128 { version[..128].to_string() } else { version }
}

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
