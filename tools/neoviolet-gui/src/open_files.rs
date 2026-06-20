//! Handles macOS "open files" events and in-window drag-and-drop.
//!
//! Two paths for receiving files:
//! 1. **App-level open events** (Dock icon drop / Finder "Open With"):
//!    GPUI translates these into `Application::on_open_urls()` callbacks.
//! 2. **In-window drag-and-drop**: GPUI dispatches `FileDropEvent` variants
//!    through the window event system.
//!
//! Both paths converge here: URLs/paths are extracted and stored as pending
//! file arguments, which `NeoVioletApp::render()` picks up and feeds to a
//! restarted PTY process.

use std::path::PathBuf;

/// Extract file paths from a list of URL strings (as delivered by
/// `Application::on_open_urls`). Handles `file://` URLs, percent-encoded
/// paths, and bare filesystem paths.
pub fn extract_file_paths(urls: &[String]) -> Vec<String> {
    urls.iter()
        .filter_map(|url| url_to_file_path(url))
        .filter(|p| !p.is_empty())
        .collect()
}

/// Convert a URL string (possibly `file://...`) to a filesystem path.
///
/// Handles:
/// - `file:///absolute/path` → `/absolute/path`
/// - `file://localhost/absolute/path` → `/absolute/path`
/// - Bare paths (no scheme) → returned as-is
fn url_to_file_path(raw: &str) -> Option<String> {
    // Strip surrounding whitespace / quotes that some platforms attach
    let trimmed = raw.trim().trim_matches('"');

    // If it doesn't look like a URL, treat it as a plain path
    if !trimmed.starts_with("file://") && !trimmed.contains("://") {
        // It might still be percent-encoded by the platform
        let decoded = percent_decode(trimmed);
        let path = PathBuf::from(&decoded);
        return Some(path.to_string_lossy().to_string());
    }

    // Reject non-file URL schemes
    let without_scheme = if let Some(rest) = trimmed.strip_prefix("file://") {
        rest
    } else {
        // Unknown scheme — not a file
        return None;
    };

    // Strip optional "localhost" authority
    let path_part = without_scheme
        .strip_prefix("localhost")
        .unwrap_or(without_scheme);

    let decoded = percent_decode(path_part);
    let path = PathBuf::from(&decoded);

    Some(path.to_string_lossy().to_string())
}

/// Decode percent-encoded characters (e.g. `%20` → ` `).
fn percent_decode(input: &str) -> String {
    let mut result = String::with_capacity(input.len());
    let mut chars = input.chars();
    while let Some(c) = chars.next() {
        if c == '%' {
            let hex: String = chars.by_ref().take(2).collect();
            if hex.len() == 2 {
                if let Ok(byte) = u8::from_str_radix(&hex, 16) {
                    result.push(byte as char);
                    continue;
                }
            }
            // Invalid escape — keep literal
            result.push('%');
            result.push_str(&hex);
        } else {
            result.push(c);
        }
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_url_to_path_file_scheme() {
        assert_eq!(
            url_to_file_path("file:///Users/test/music.mp3"),
            Some("/Users/test/music.mp3".into())
        );
    }

    #[test]
    fn test_url_to_path_localhost() {
        assert_eq!(
            url_to_file_path("file://localhost/Users/test/song.flac"),
            Some("/Users/test/song.flac".into())
        );
    }

    #[test]
    fn test_url_to_path_percent_encoded() {
        assert_eq!(
            url_to_file_path("file:///Users/test/My%20Music/song.mp3"),
            Some("/Users/test/My Music/song.mp3".into())
        );
    }

    #[test]
    fn test_url_to_path_bare_path() {
        assert_eq!(
            url_to_file_path("/home/user/music.ogg"),
            Some("/home/user/music.ogg".into())
        );
    }

    #[test]
    fn test_extract_file_paths_mixed() {
        let urls = vec![
            "file:///a/b.mp3".to_string(),
            "not-a-url".to_string(),
            "/bare/path.flac".to_string(),
        ];
        let paths = extract_file_paths(&urls);
        assert_eq!(paths.len(), 3);
        assert!(paths.contains(&"/a/b.mp3".to_string()));
        assert!(paths.contains(&"not-a-url".to_string()));
        assert!(paths.contains(&"/bare/path.flac".to_string()));
    }

}
