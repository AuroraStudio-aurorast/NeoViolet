//! Miscellaneous utilities for the GUI wrapper.

use gpui::{Hsla, rgb};

/// Strip common ANSI escape sequences from a byte slice, returning a plain
/// UTF-8 `String`. Handles SGR (`\x1b[...m`), erase (`\x1b[...J`/`K`),
/// cursor movement (`\x1b[...A`-`H`), and a handful of other common CSI
/// sequences. Unknown sequences are passed through as-is.
pub fn strip_ansi_escapes(bytes: &[u8]) -> String {
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    let len = bytes.len();

    while i < len {
        if bytes[i] == 0x1b && i + 1 < len && bytes[i + 1] == b'[' {
            // CSI sequence: ESC [ ... <final byte>
            i += 2; // skip ESC [
            // Skip parameter bytes (0x30-0x3F) and intermediate bytes (0x20-0x2F)
            while i < len && ((0x30..=0x3F).contains(&bytes[i]) || (0x20..=0x2F).contains(&bytes[i])) {
                i += 1;
            }
            // Skip final byte (0x40-0x7E)
            if i < len && (0x40..=0x7E).contains(&bytes[i]) {
                i += 1;
            }
        } else if bytes[i] == 0x1b && i + 1 < len && bytes[i + 1] == b']' {
            // OSC sequence: ESC ] ... BEL or ST (ESC \)
            i += 2; // skip ESC ]
            while i < len && bytes[i] != 0x07 && !(bytes[i] == 0x1b && i + 1 < len && bytes[i + 1] == b'\\') {
                i += 1;
            }
            // Skip terminating BEL or ST
            if i < len && bytes[i] == 0x07 {
                i += 1;
            } else if i + 1 < len && bytes[i] == 0x1b && bytes[i + 1] == b'\\' {
                i += 2;
            }
        } else if bytes[i] == 0x1b {
            // Lone ESC or simple two-char sequence (e.g. ESC c, ESC D, etc.)
            i += 1; // skip ESC
            if i < len && bytes[i].is_ascii_alphabetic() {
                i += 1;
            }
        } else if bytes[i] == b'\r' {
            // Keep line-feeds but skip carriage returns
            i += 1;
            if i < len && bytes[i] == b'\n' {
                // CRLF → just the LF
            }
        } else {
            out.push(bytes[i]);
            i += 1;
        }
    }

    String::from_utf8_lossy(&out).into_owned()
}

/// Parse a hex color string ("#RRGGBB" or "RRGGBB") into a gpui Hsla.
/// Returns opaque white on parse failure.
pub fn hex_to_hsla(hex: &str) -> Hsla {
    let hex = hex.trim_start_matches('#');
    let val = u32::from_str_radix(hex, 16).unwrap_or(0xFFFFFF);
    rgb(val).into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strip_sgr() {
        let input = b"\x1b[1;31mHello\x1b[0m World";
        assert_eq!(strip_ansi_escapes(input), "Hello World");
    }

    #[test]
    fn strip_cursor_and_erase() {
        let input = b"\x1b[2J\x1b[H\x1b[?25lPrompt>";
        assert_eq!(strip_ansi_escapes(input), "Prompt>");
    }

    #[test]
    fn strip_crlf() {
        let input = b"line1\r\nline2\r\n";
        assert_eq!(strip_ansi_escapes(input), "line1\nline2\n");
    }

    #[test]
    fn plain_text_passthrough() {
        let input = b"usage: neoviolet [flags] [file]";
        assert_eq!(strip_ansi_escapes(input), "usage: neoviolet [flags] [file]");
    }

    #[test]
    fn strip_osc_title() {
        let input = b"\x1b]0;neoviolet --help\x07Output";
        assert_eq!(strip_ansi_escapes(input), "Output");
    }
}
