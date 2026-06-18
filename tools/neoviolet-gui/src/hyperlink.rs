use regex::Regex;
use std::sync::LazyLock;

/// Compiled regex for detecting URLs/hyperlinks in terminal text.
/// Matches http/https URLs and common schemes.
static URL_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    // Match URLs: http/https/ftp/file/mailto followed by non-whitespace, non-control chars.
    // Excludes common delimiters: < > " ' { } | \ ^ `
    Regex::new(r#"(?:https?://|ftp://|file://|mailto:)[^\x00-\x1f\x7f\s<>{}|\\^`'"]+"#)
        .expect("invalid URL regex")
});

/// Find the hyperlink at a specific column in a line's text.
/// Returns the URL if the column falls within a hyperlink.
pub fn hyperlink_at_column(line_text: &str, col: usize) -> Option<String> {
    for m in URL_REGEX.find_iter(line_text) {
        if m.range().contains(&col) {
            return Some(m.as_str().to_string());
        }
    }
    None
}

