//! Payload construction and delivery gating for files dropped into the window.
//!
//! A dropped file is handed to the running PTY as a *bracketed paste* — the
//! same bytes a terminal emulator sends when a file is dropped onto a terminal
//! — so the TUI's own drop handling decides what happens: in normal mode the
//! first valid audio file is loaded, in command mode the paths are inserted on
//! the command line. The GUI does not restart the process for a drop.

use gpui::{App, Entity};

use crate::app::TerminalApp;
use crate::backend::PTY_READY_STATUS;
use crate::state::AppState;
use crate::terminal::BackendCommand;

/// Frame opening a bracketed paste.
const PASTE_START: &[u8] = b"\x1b[200~";
/// Frame closing a bracketed paste.
const PASTE_END: &[u8] = b"\x1b[201~";

/// Drop every ESC from a path. The escape set cannot cover ESC itself (it
/// cannot be escaped, only removed) and a surviving ESC would let a path close
/// the paste early.
fn strip_esc(path: &str) -> String {
    path.chars().filter(|c| *c != '\u{1b}').collect()
}

/// Backslash-escape the characters the receiving program's splitter treats as
/// syntax.
///
/// Cross-program invariant: the set is exactly what the TUI's paste
/// normalization restores — Go's `unescapeBackslashes` in `internal/ui/drop.go`
/// drops the backslash before precisely these runes, and the symmetric set is
/// what makes a GUI-escaped payload round-trip to the original path even when
/// the file no longer exists on disk. The same round trip is pinned by a
/// contract test on the Go side (`drop_test.go`, mirroring this function), so
/// a one-sided change to either set fails a test on at least one side. All
/// whitespace (the same set as Go's `unicode.IsSpace`), backslash, and the six
/// quote characters the splitter recognizes qualify; nothing else may be
/// added — in either program, without the other.
fn escape_for_drop(path: &str) -> String {
    let mut escaped = String::with_capacity(path.len());
    for c in path.chars() {
        if c.is_whitespace()
            || c == '\\'
            || matches!(
                c,
                '\'' | '"' | '\u{201c}' | '\u{201d}' | '\u{2018}' | '\u{2019}'
            )
        {
            escaped.push('\\');
        }
        escaped.push(c);
    }
    escaped
}

/// Drop repeated paths, keeping the order of the remaining ones.
fn dedupe_preserving_order(paths: &[String]) -> Vec<String> {
    let mut seen = std::collections::HashSet::with_capacity(paths.len());
    let mut unique = Vec::with_capacity(paths.len());
    for path in paths {
        if seen.insert(path.as_str()) {
            unique.push(path.clone());
        }
    }
    unique
}

/// Build the byte payload written to the PTY for `paths`.
///
/// Shape (unconditional): the paste start frame, the paths escaped and joined
/// by one space, the paste end frame.
pub fn bracketed_paste_payload(paths: &[String]) -> Vec<u8> {
    let unique = dedupe_preserving_order(paths);
    if unique.is_empty() {
        return Vec::new();
    }
    let body = unique
        .iter()
        .map(|path| escape_for_drop(&strip_esc(path)))
        .collect::<Vec<String>>()
        .join(" ");

    let mut payload = Vec::with_capacity(PASTE_START.len() + body.len() + PASTE_END.len());
    payload.extend_from_slice(PASTE_START);
    payload.extend_from_slice(body.as_bytes());
    payload.extend_from_slice(PASTE_END);
    payload
}

/// Whether a drop may be written to the PTY.
///
/// `status` is `TerminalTab::status`. `PTY_READY_STATUS` (see `backend.rs`, its
/// only write point) means the PTY has been spawned and has not been closed
/// and it is *not* a statement that the TUI inside it is ready to read input.
/// The three flags are the GUI dialogs that own the keyboard.
pub fn should_paste(
    status: &str,
    show_exit_error: bool,
    show_close: bool,
    show_about: bool,
) -> bool {
    status == PTY_READY_STATUS && !show_exit_error && !show_close && !show_about
}

/// The arguments a cold start appends to the GUI's own CLI args for `paths`.
///
/// Shape: `--` then the first path. `--` keeps a path that looks like a flag
/// from being read as one, which is why the option terminator is part of the
/// shape rather than the caller's business. Only the first path is used: the
/// TUI treats the argv files as "the file to open", so a second one would make
/// the first run end in a usage error. Any further paths from the same
/// cold-start event are dropped: the TUI takes a single positional file, and
/// only paths that arrive after the spawn reach it through a paste. No paths
/// means nothing to append.
pub fn launch_args_for(paths: &[String]) -> Vec<String> {
    let Some(first) = paths.first() else {
        return Vec::new();
    };
    vec!["--".to_string(), first.clone()]
}

/// What one paste attempt did with the paths.
pub(crate) enum PasteOutcome {
    /// Written into the PTY by the same entity update that read its status.
    Sent,
    /// A dialog owns the keyboard. The paths are kept; the caller must offer
    /// them again once the dialog is gone (the next render is enough: the
    /// dialog state itself repaints when it closes).
    Deferred,
    /// There was nothing to paste into; the paths are gone (a warning log
    /// records why).
    Unavailable,
}

/// Hand `paths` to the running PTY as one bracketed paste.
///
/// The status read and the payload write happen inside one entity update: what
/// is decided is decided against the state the PTY actually has at write time,
/// so a close arriving between "read status" and "write bytes" cannot slip
/// through. A refusal by an open dialog does not discard the paths — the
/// caller decides from `PasteOutcome::Deferred` what a retry looks like; a PTY
/// that never came up is reported as `Unavailable` instead.
pub(crate) fn send_paths(
    cx: &mut App,
    child: &Entity<TerminalApp>,
    paths: &[String],
) -> PasteOutcome {
    if paths.is_empty() {
        return PasteOutcome::Unavailable;
    }
    let payload = bracketed_paste_payload(paths);
    if payload.is_empty() {
        return PasteOutcome::Unavailable;
    }

    child.update(cx, |tab, cx| {
        let status = tab.tab.status.clone();
        let (show_exit_error, show_close, show_about) = {
            let state = cx.global::<AppState>();
            (
                *state.show_exit_error.lock().unwrap(),
                *state.show_close.lock().unwrap(),
                *state.show_about.lock().unwrap(),
            )
        };
        if !should_paste(&status, show_exit_error, show_close, show_about) {
            if show_exit_error || show_close || show_about {
                // Debug rather than warn: a deferral is expected, temporary
                // state while a dialog is up — and repeated attempts are
                // normal, one per render until the dialog closes.
                log::debug!(
                    "[drag-drop] deferring {} file(s): a dialog owns the keyboard (status: {status})",
                    paths.len()
                );
                return PasteOutcome::Deferred;
            }
            log::warn!(
                "[drag-drop] dropping {} file(s): no PTY to paste into (status: {status:?})",
                paths.len()
            );
            return PasteOutcome::Unavailable;
        }
        log::info!(
            "[drag-drop] pasting {} file(s) into the terminal",
            paths.len()
        );
        let _ = tab.tab.backend.send(BackendCommand::Input(payload));
        PasteOutcome::Sent
    })
}

/// Store the paths of a deferred attempt back into the shared pending store,
/// so the next render offers them again (closing a dialog repaints, which is
/// the natural retry point). Both callers — the render-time open-file drain
/// and the element-level `on_drop` — go through here; an attempt that was
/// sent or had nothing to paste into is a no-op.
pub(crate) fn defer_pending_paths(cx: &mut App, outcome: PasteOutcome, paths: Vec<String>) {
    if let PasteOutcome::Deferred = outcome {
        let pending = cx.global::<AppState>().pending_file_paths.clone();
        if let Ok(mut guard) = pending.lock() {
            guard.extend(paths);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn payload_frames_a_single_path() {
        let payload = bracketed_paste_payload(&["/music/song.mp3".to_string()]);
        assert_eq!(payload, b"\x1b[200~/music/song.mp3\x1b[201~".to_vec());
    }

    #[test]
    fn payload_escapes_spaces() {
        let payload = bracketed_paste_payload(&["/music/my song.mp3".to_string()]);
        assert_eq!(payload, b"\x1b[200~/music/my\\ song.mp3\x1b[201~".to_vec());
    }

    #[test]
    fn payload_escapes_tabs() {
        let payload = bracketed_paste_payload(&["/music/my\tsong.mp3".to_string()]);
        assert_eq!(payload, b"\x1b[200~/music/my\\\tsong.mp3\x1b[201~".to_vec());
    }

    #[test]
    fn payload_escapes_newlines() {
        let payload = bracketed_paste_payload(&["/music/my\nsong.mp3".to_string()]);
        assert_eq!(payload, b"\x1b[200~/music/my\\\nsong.mp3\x1b[201~".to_vec());
    }

    #[test]
    fn payload_escapes_the_rest_of_the_whitespace_set() {
        // Everything `char::is_whitespace()` reports must be escaped, not just
        // ASCII spaces and tabs.
        for ws in ['\u{0b}', '\u{0c}', '\u{85}', '\u{a0}', '\u{3000}'] {
            let path = format!("/music/left{ws}right.mp3");
            let payload = bracketed_paste_payload(&[path]);
            let expected = format!("\x1b[200~/music/left\\{ws}right.mp3\x1b[201~");
            assert_eq!(payload, expected.as_bytes().to_vec(), "U+{:04X}", ws as u32);
        }
    }

    #[test]
    fn payload_escapes_every_quote_character() {
        for quote in ['\'', '"', '\u{201c}', '\u{201d}', '\u{2018}', '\u{2019}'] {
            let path = format!("/music/{quote}song.mp3");
            let payload = bracketed_paste_payload(&[path]);
            let expected = format!("\x1b[200~/music/\\{quote}song.mp3\x1b[201~");
            assert_eq!(payload, expected.as_bytes().to_vec(), "quote {quote}");
        }
    }

    #[test]
    fn payload_doubles_backslashes() {
        let payload = bracketed_paste_payload(&["/music/a\\b.mp3".to_string()]);
        assert_eq!(payload, b"\x1b[200~/music/a\\\\b.mp3\x1b[201~".to_vec());
    }

    #[test]
    fn payload_joins_paths_with_one_space_in_order() {
        let payload = bracketed_paste_payload(&[
            "/music/\u{6b4c} one.mp3".to_string(),
            "/music/b.mp3".to_string(),
        ]);
        let expected = "\x1b[200~/music/\u{6b4c}\\ one.mp3 /music/b.mp3\x1b[201~";
        assert_eq!(payload, expected.as_bytes().to_vec());
    }

    #[test]
    fn payload_drops_duplicate_paths() {
        let once = bracketed_paste_payload(&["/music/a.mp3".to_string()]);
        let twice =
            bracketed_paste_payload(&["/music/a.mp3".to_string(), "/music/a.mp3".to_string()]);
        assert_eq!(twice, once);
    }

    #[test]
    fn payload_of_no_paths_is_empty() {
        assert_eq!(bracketed_paste_payload(&[]), Vec::<u8>::new());
    }

    #[test]
    fn payload_has_no_bare_esc_outside_the_frames() {
        // A surviving ESC would let a path terminate the paste early.
        let payload = bracketed_paste_payload(&["/music/\x1b[31mred\x1b[0m song.mp3".to_string()]);
        assert_eq!(
            payload,
            b"\x1b[200~/music/[31mred[0m\\ song.mp3\x1b[201~".to_vec()
        );
        let body = &payload[6..payload.len() - 6];
        assert!(
            !body.contains(&0x1b),
            "bare ESC left in the paste body: {:?}",
            String::from_utf8_lossy(body)
        );
    }

    #[test]
    fn launch_args_of_no_paths_are_empty() {
        assert_eq!(launch_args_for(&[]), Vec::<String>::new());
    }

    #[test]
    fn launch_args_of_one_path_terminate_options_then_name_it() {
        assert_eq!(
            launch_args_for(&["/music/song.mp3".to_string()]),
            vec!["--".to_string(), "/music/song.mp3".to_string()]
        );
    }

    #[test]
    fn launch_args_of_many_paths_keep_only_the_first() {
        assert_eq!(
            launch_args_for(&["/music/a.mp3".to_string(), "/music/b.mp3".to_string(),]),
            vec!["--".to_string(), "/music/a.mp3".to_string()]
        );
    }

    #[test]
    fn launch_args_of_a_path_that_looks_like_a_flag_are_still_terminated() {
        // Without `--` the TUI's flag parser would read this as an option.
        assert_eq!(
            launch_args_for(&["-h.mp3".to_string()]),
            vec!["--".to_string(), "-h.mp3".to_string()]
        );
    }

    #[test]
    fn should_paste_accepts_the_ready_status() {
        assert!(should_paste("neoviolet ready", false, false, false));
    }

    #[test]
    fn should_paste_rejects_every_non_ready_status() {
        for status in [
            "neoviolet closed",
            "read error: EIO",
            "write error: EPIPE",
            "neoviolet exited: exit status: 1",
            "closed: failed to start neoviolet: No such file or directory",
            "local shell",
            "ready",
            "neoviolet ready (restarted)",
        ] {
            assert!(
                !should_paste(status, false, false, false),
                "status {status:?} must not be treated as pasteable"
            );
        }
    }

    #[test]
    fn should_paste_rejects_an_open_dialog() {
        assert!(!should_paste("neoviolet ready", true, false, false));
        assert!(!should_paste("neoviolet ready", false, true, false));
        assert!(!should_paste("neoviolet ready", false, false, true));
    }
}
