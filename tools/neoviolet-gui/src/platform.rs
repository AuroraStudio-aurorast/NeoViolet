//! Cross-platform utilities: binary discovery, font defaults, version check.

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
        "Monospace"
    }
}

/// Check whether a font family is installed on the system.
/// Uses `fontdb` under the hood — queries the system font database.
pub fn is_font_available(family: &str) -> bool {
    let mut db = fontdb::Database::new();
    db.load_system_fonts();
    db.query(&fontdb::Query {
        families: &[fontdb::Family::Name(family)],
        ..Default::default()
    })
    .is_some()
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
    if s.len() > max { s[..max].to_string() } else { s }
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
