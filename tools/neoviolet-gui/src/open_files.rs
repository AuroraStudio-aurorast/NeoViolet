//! Handles macOS app-level "open files" events (Dock icon drop / Finder "Open With").
//!
//! App-level open events arrive as URLs through `Application::on_open_urls()`, which
//! GPUI dispatches on macOS. In-window drag-and-drop does **not** go through this
//! module: GPUI delivers it to the root element's `on_drop` handler, which pastes
//! the paths into the running PTY.
//!
//! This module extracts paths from those URLs. They are queued in
//! `AppState::pending_file_paths`: the cold start consumes the queue as its argv
//! file, and anything arriving later is pasted into the running PTY.

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
    let without_scheme = trimmed.strip_prefix("file://")?;

    // Strip optional "localhost" authority
    let path_part = without_scheme
        .strip_prefix("localhost")
        .unwrap_or(without_scheme);

    let decoded = percent_decode(path_part);
    let path = PathBuf::from(&decoded);

    Some(path.to_string_lossy().to_string())
}

/// Decode percent-encoded characters (e.g. `%20` → ` `).
/// Percent escapes that do not form valid UTF-8 are replaced lossily.
fn percent_decode(input: &str) -> String {
    let mut bytes: Vec<u8> = Vec::with_capacity(input.len());
    let mut chars = input.chars();
    while let Some(c) = chars.next() {
        if c == '%' {
            let hex: String = chars.by_ref().take(2).collect();
            if hex.len() == 2
                && let Ok(byte) = u8::from_str_radix(&hex, 16)
            {
                bytes.push(byte);
                continue;
            }
            // Invalid escape — keep literal
            bytes.push(b'%');
            bytes.extend_from_slice(hex.as_bytes());
        } else {
            let mut buf = [0u8; 4];
            bytes.extend_from_slice(c.encode_utf8(&mut buf).as_bytes());
        }
    }
    match String::from_utf8(bytes) {
        Ok(decoded) => decoded,
        Err(err) => {
            log::warn!("[open-files] decoded path is not valid UTF-8: {err}");
            String::from_utf8_lossy(err.as_bytes()).into_owned()
        }
    }
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
    fn test_url_to_path_cjk_is_decoded_as_utf8() {
        assert_eq!(
            url_to_file_path("file:///Users/test/%E6%AD%8C.mp3"),
            Some("/Users/test/歌.mp3".into())
        );
    }

    #[test]
    fn test_url_to_path_literal_non_ascii_stays_intact() {
        assert_eq!(
            url_to_file_path("file:///Users/test/歌.mp3"),
            Some("/Users/test/歌.mp3".into())
        );
    }

    #[test]
    fn test_url_to_path_invalid_utf8_is_lossy() {
        assert_eq!(
            url_to_file_path("file:///Users/test/%FF.mp3"),
            Some("/Users/test/\u{FFFD}.mp3".into())
        );
    }

    #[test]
    fn test_url_to_path_invalid_escape_stays_literal() {
        assert_eq!(
            url_to_file_path("file:///Users/test/%zz.mp3"),
            Some("/Users/test/%zz.mp3".into())
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
