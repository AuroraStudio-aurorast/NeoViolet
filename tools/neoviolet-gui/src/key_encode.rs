use crate::modes::Modes;

/// Encode a gpui keystroke into a terminal escape sequence,
/// respecting the active terminal mode for protocol selection.
///
/// `option_as_meta`: when true (macOS default), Alt+char sends ESC prefix.
/// When false, Alt is encoded as the Alt modifier in CSI sequences.
///
/// Returns the byte sequence to send to the PTY, or `None`
/// if the keystroke should be ignored (unmapped key).
pub fn encode_key_seq(
    key: &str,
    key_char: Option<&str>,
    modifiers: &gpui::Modifiers,
    mode: &Modes,
    option_as_meta: bool,
) -> Option<Vec<u8>> {
    // ── Determine effective modifiers ──
    let ctrl = modifiers.control;
    let alt = modifiers.alt;
    let shift = modifiers.shift;
    let platform = modifiers.platform;

    // Don't forward platform shortcuts (Cmd/Win + key)
    if platform {
        return None;
    }

    // ── Enter / Return ──
    if key == "enter" || key == "return" {
        if shift {
            // Shift+Enter → line feed (LF) — useful in some terminal apps
            return Some(vec![0x0a]);
        }
        if alt {
            return Some(vec![0x1b, 0x0d]);
        }
        return Some(vec![0x0d]); // CR
    }
    if key == "kp_enter" {
        return Some(vec![0x0d]);
    }

    // ── Ctrl + character → control byte (table-driven) ──
    if ctrl && !alt && !shift {
        if let Some(ctrl_byte) = encode_ctrl(key, key_char) {
            return Some(vec![ctrl_byte]);
        }
    }

    // ── Alt prefix (Esc for macOS option-as-meta) ──
    let have_alt = alt && !ctrl;

    // ── xterm modifier parameter ──
    //   2=Shift, 3=Alt, 4=Shift+Alt, 5=Ctrl, 6=Ctrl+Shift,
    //   7=Ctrl+Alt, 8=Ctrl+Shift+Alt
    let mod_param = modifier_param(ctrl, alt, shift);

    // ── Named key encoding ──
    match key {
        "escape" => return Some(vec![0x1b]),

        "backspace" => {
            if ctrl {
                return Some(vec![0x08]); // ^H
            }
            if alt {
                return Some(vec![0x1b, 0x7f]);
            }
            return Some(vec![0x7f]);
        }

        "tab" => {
            if shift {
                return Some(vec![0x1b, b'[', b'Z']); // back-tab
            }
            return Some(vec![0x09]);
        }

        "space" => {
            if ctrl {
                return Some(vec![0x00]); // ^@
            }
            return Some(vec![0x20]);
        }

        "insert" => {
            return Some(encode_csi_modified(b'2', b'~', mod_param, alt, option_as_meta));
        }
        "delete" => {
            return Some(encode_csi_modified(b'3', b'~', mod_param, alt, option_as_meta));
        }

        // Home / End — DECCKM-aware
        "home" => {
            let (prefix, code) = if mode.contains(Modes::APP_CURSOR) {
                (vec![0x1b, b'O'], b'H')
            } else {
                (vec![0x1b, b'['], b'H')
            };
            if let Some(mp) = mod_param {
                return Some(format!("\x1b[1;{}{}", mp, code as char).into_bytes());
            }
            let mut v = prefix;
            v.push(code);
            return Some(v);
        }
        "end" => {
            let (prefix, code) = if mode.contains(Modes::APP_CURSOR) {
                (vec![0x1b, b'O'], b'F')
            } else {
                (vec![0x1b, b'['], b'F')
            };
            if let Some(mp) = mod_param {
                return Some(format!("\x1b[1;{}{}", mp, code as char).into_bytes());
            }
            let mut v = prefix;
            v.push(code);
            return Some(v);
        }

        "pageup" => {
            if let Some(mp) = mod_param {
                return Some(format!("\x1b[5;{}~", mp).into_bytes());
            }
            return Some(vec![0x1b, b'[', b'5', b'~']);
        }
        "pagedown" => {
            if let Some(mp) = mod_param {
                return Some(format!("\x1b[6;{}~", mp).into_bytes());
            }
            return Some(vec![0x1b, b'[', b'6', b'~']);
        }

        // ── Arrow keys (DECCKM-aware, with modifier encoding) ──
        "up" | "down" | "right" | "left" => {
            let code = match key {
                "up" => b'A', "down" => b'B', "right" => b'C', "left" => b'D', _ => unreachable!(),
            };
            let seq = if let Some(mp) = mod_param {
                format!("\x1b[1;{}{}", mp, code as char).into_bytes()
            } else if mode.contains(Modes::APP_CURSOR) {
                vec![0x1b, b'O', code]
            } else {
                vec![0x1b, b'[', code]
            };
            if have_alt && option_as_meta {
                let mut v = vec![0x1b];
                v.extend(seq);
                return Some(v);
            }
            // Alt with no option_as_meta is already encoded in mod_param
            return Some(seq);
        }

        // ── Function keys F1–F20 ──
        k if k.starts_with('f') => {
            if let Ok(n) = k[1..].parse::<u8>() {
                if n >= 1 && n <= 20 {
                    let seq = encode_fkey(n, mod_param);
                    if have_alt && option_as_meta {
                        let mut v = vec![0x1b];
                        v.extend(seq);
                        return Some(v);
                    }
                    return Some(seq);
                }
            }
            return None;
        }

        // ── Keypad keys (APP_KEYPAD / DECKPAM aware) ──
        k if k.starts_with("kp_") => {
            if let Some(seq) = encode_keypad(k, mode) {
                if have_alt && option_as_meta {
                    let mut v = vec![0x1b];
                    v.extend(seq);
                    return Some(v);
                }
                return Some(seq);
            }
            return None;
        }

        // ── Single-character fallback ──
        _ if key.len() == 1 => {
            if have_alt && option_as_meta {
                let ch = key_char.unwrap_or(key);
                return Some(format!("\x1b{}", ch).into_bytes());
            }
            if let Some(ch) = key_char {
                return Some(ch.as_bytes().to_vec());
            }
            return Some(key.as_bytes().to_vec());
        }

        // ── Unmapped ──
        _ => return None,
    }
}

// ── Ctrl character encoding (table-driven, Zed pattern) ──

/// Convert a key+key_char under Ctrl to its control byte.
/// Table-driven approach that maps ASCII letters, symbols and special chars.
fn encode_ctrl(key: &str, key_char: Option<&str>) -> Option<u8> {
    // First check key_char (IME-aware) for single characters
    if let Some(ch) = key_char {
        if let Some(byte) = char_to_control_byte(ch) {
            return Some(byte);
        }
    }
    // Fall back to the key name itself
    if key.len() == 1 {
        return char_to_control_byte(key);
    }
    None
}

/// Map a single-character string to its control equivalent.
fn char_to_control_byte(s: &str) -> Option<u8> {
    let byte = s.as_bytes().first()?;
    Some(match byte {
        // Letters: Ctrl+A..Z → 1..26
        b'a'..=b'z' => byte - b'a' + 1,
        b'A'..=b'Z' => byte - b'A' + 1,
        // Special characters
        b'@' | b'`' | b' ' => 0x00,  // NUL (^@, ^`, ^Space)
        b'[' => 0x1b,                   // ESC (^[)
        b'\\' => 0x1c,                  // FS  (^\)
        b']' => 0x1d,                   // GS  (^])
        b'^' => 0x1e,                   // RS  (^^)
        b'_' => 0x1f,                   // US  (^_)
        b'?' => 0x7f,                   // DEL (^?)
        _ => return None,
    })
}

// ── xterm modifier parameter ──

/// Compute xterm-style modifier parameter for CSI sequences.
/// Returns None for no modifiers, or Some(2..8) per xterm spec.
fn modifier_param(ctrl: bool, alt: bool, shift: bool) -> Option<u8> {
    match (ctrl, alt, shift) {
        (false, false, false) => None,
        (false, false, true)  => Some(2),
        (false, true,  false) => Some(3),
        (false, true,  true)  => Some(4),
        (true,  false, false) => Some(5),
        (true,  false, true)  => Some(6),
        (true,  true,  false) => Some(7),
        (true,  true,  true)  => Some(8),
    }
}

// ── CSI encoding helpers ──

/// Encode a CSI sequence with optional modifier: `\x1b[{param}{code}` or
/// `\x1b[{param};{mod}{code}` with modifier.
fn encode_csi_modified(
    leading: u8,
    trailing: u8,
    mod_param: Option<u8>,
    alt: bool,
    option_as_meta: bool,
) -> Vec<u8> {
    let seq = if let Some(mp) = mod_param {
        format!("\x1b[{};{}{}", leading, mp, trailing as char).into_bytes()
    } else {
        vec![0x1b, b'[', leading, trailing]
    };
    if alt && option_as_meta {
        let mut v = vec![0x1b];
        v.extend(seq);
        v
    } else {
        seq
    }
}

// ── Function keys ──

/// Encode function key F1-F20 with optional modifier parameter.
fn encode_fkey(n: u8, mod_param: Option<u8>) -> Vec<u8> {
    match n {
        1 => fkey_ss3(b'P', mod_param),
        2 => fkey_ss3(b'Q', mod_param),
        3 => fkey_ss3(b'R', mod_param),
        4 => fkey_ss3(b'S', mod_param),
        // F5-F20 use CSI ~ encoding
        5  => fkey_csi(15, mod_param),
        6  => fkey_csi(17, mod_param),
        7  => fkey_csi(18, mod_param),
        8  => fkey_csi(19, mod_param),
        9  => fkey_csi(20, mod_param),
        10 => fkey_csi(21, mod_param),
        11 => fkey_csi(23, mod_param),
        12 => fkey_csi(24, mod_param),
        13 => fkey_csi(25, mod_param),
        14 => fkey_csi(26, mod_param),
        15 => fkey_csi(28, mod_param),
        16 => fkey_csi(29, mod_param),
        17 => fkey_csi(31, mod_param),
        18 => fkey_csi(32, mod_param),
        19 => fkey_csi(33, mod_param),
        20 => fkey_csi(34, mod_param),
        _ => vec![], // unreachable
    }
}

/// SS3-based function key (F1-F4): `\x1bO{code}` or `\x1b[1;{mod}{code}`
fn fkey_ss3(code: u8, mod_param: Option<u8>) -> Vec<u8> {
    if let Some(mp) = mod_param {
        format!("\x1b[1;{}{}", mp, code as char).into_bytes()
    } else {
        vec![0x1b, b'O', code]
    }
}

/// CSI-based function key (F5-F20): `\x1b[{num}~` or `\x1b[{num};{mod}~`
fn fkey_csi(num: u8, mod_param: Option<u8>) -> Vec<u8> {
    if let Some(mp) = mod_param {
        format!("\x1b[{num};{mp}~").into_bytes()
    } else {
        format!("\x1b[{num}~").into_bytes()
    }
}

// ── Keypad ──

/// Encode a keypad key based on APP_KEYPAD mode.
fn encode_keypad(key: &str, mode: &Modes) -> Option<Vec<u8>> {
    if mode.contains(Modes::APP_KEYPAD) {
        let code = match key {
            "kp_0" => b'p', "kp_1" => b'q', "kp_2" => b'r', "kp_3" => b's',
            "kp_4" => b't', "kp_5" => b'u', "kp_6" => b'v', "kp_7" => b'w',
            "kp_8" => b'x', "kp_9" => b'y',
            "kp_decimal" => b'n',
            "kp_add" => b'l',
            "kp_subtract" => b'm',
            "kp_multiply" => b'j',
            "kp_divide" => b'o',
            "kp_equals" => b'X',
            _ => return None,
        };
        Some(vec![0x1b, b'O', code])
    } else {
        let ch = match key {
            "kp_0" => "0", "kp_1" => "1", "kp_2" => "2", "kp_3" => "3",
            "kp_4" => "4", "kp_5" => "5", "kp_6" => "6", "kp_7" => "7",
            "kp_8" => "8", "kp_9" => "9",
            "kp_decimal" => ".",
            "kp_add" => "+",
            "kp_subtract" => "-",
            "kp_multiply" => "*",
            "kp_divide" => "/",
            "kp_equals" => "=",
            _ => return None,
        };
        Some(ch.as_bytes().to_vec())
    }
}
