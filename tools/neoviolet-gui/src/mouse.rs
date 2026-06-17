use crate::modes::Modes;

// ── Mouse button codes (Zed pattern) ──

#[derive(Debug, Clone, Copy)]
pub(crate) enum MouseButtonCode {
    LeftButton = 0,
    MiddleButton = 1,
    RightButton = 2,
    LeftMove = 32,
    MiddleMove = 33,
    RightMove = 34,
    NoneMove = 35,
    ScrollUp = 64,
    ScrollDown = 65,
}

impl MouseButtonCode {
    pub fn from_button(button: gpui::MouseButton) -> Self {
        match button {
            gpui::MouseButton::Left => Self::LeftButton,
            gpui::MouseButton::Middle => Self::MiddleButton,
            gpui::MouseButton::Right => Self::RightButton,
            gpui::MouseButton::Navigate(_) => Self::NoneMove,
        }
    }

    pub fn from_move_button(button: Option<gpui::MouseButton>) -> Self {
        match button {
            Some(gpui::MouseButton::Left) => Self::LeftMove,
            Some(gpui::MouseButton::Middle) => Self::MiddleMove,
            Some(gpui::MouseButton::Right) => Self::RightMove,
            Some(gpui::MouseButton::Navigate(_)) => Self::NoneMove,
            None => Self::NoneMove,
        }
    }

    pub fn from_scroll_down(is_down: bool) -> Self {
        if is_down {
            Self::ScrollDown
        } else {
            Self::ScrollUp
        }
    }
}

// ── Mouse format (protocol selection) ──

#[derive(Clone, Copy)]
enum MouseFormat {
    Sgr,
    Normal(bool), // bool = utf8
}

impl MouseFormat {
    fn from_mode(mode: &Modes) -> Self {
        if mode.contains(Modes::SGR_MOUSE) {
            MouseFormat::Sgr
        } else if mode.contains(Modes::UTF8_MOUSE) {
            MouseFormat::Normal(true)
        } else {
            MouseFormat::Normal(false)
        }
    }
}

// ── Mouse modifiers ──

#[derive(Clone, Copy, Default)]
pub struct MouseModifiers {
    pub shift: bool,
    pub alt: bool,
    pub ctrl: bool,
}

impl MouseModifiers {
    /// Encode modifiers into the upper bits of a button byte.
    /// Bit 2 = Shift (value 4), Bit 3 = Alt (value 8), Bit 4 = Ctrl (value 16)
    fn apply(&self, btn: u8) -> u8 {
        let mut b = btn;
        if self.shift { b |= 4; }
        if self.alt { b |= 8; }
        if self.ctrl { b |= 16; }
        b
    }
}

impl From<&gpui::Modifiers> for MouseModifiers {
    fn from(m: &gpui::Modifiers) -> Self {
        Self { shift: m.shift, alt: m.alt, ctrl: m.control }
    }
}

// ── Public API: format and send mouse events ──

/// Format a mouse button press/release event.
pub fn mouse_button_report(
    mode: &Modes,
    button: gpui::MouseButton,
    modifiers: &gpui::Modifiers,
    pressed: bool,
    col: u16,
    row: u16,
) -> Option<Vec<u8>> {
    let code = MouseButtonCode::from_button(button);
    let mods = MouseModifiers::from(modifiers);
    if mode.intersects(Modes::MOUSE_MODE) {
        mouse_report(code, pressed, col, row, &mods, MouseFormat::from_mode(mode))
    } else {
        None
    }
}

/// Format a mouse move event.
pub fn mouse_moved_report(
    mode: &Modes,
    button: Option<gpui::MouseButton>,
    modifiers: &gpui::Modifiers,
    col: u16,
    row: u16,
) -> Option<Vec<u8>> {
    let code = MouseButtonCode::from_move_button(button);
    let mods = MouseModifiers::from(modifiers);
    if mode.intersects(Modes::MOUSE_MOTION | Modes::MOUSE_DRAG) {
        // In drag-only mode, suppress None-move events
        if mode.contains(Modes::MOUSE_DRAG) && matches!(code, MouseButtonCode::NoneMove) {
            return None;
        }
        mouse_report(code, true, col, row, &mods, MouseFormat::from_mode(mode))
    } else {
        None
    }
}

/// Format a scroll event. Returns an iterator of repeated reports for
/// multi-line scroll events.
pub fn scroll_report(
    mode: &Modes,
    is_down: bool,
    modifiers: &gpui::Modifiers,
    col: u16,
    row: u16,
    _scroll_lines: u32,
) -> Option<Vec<u8>> {
    let code = MouseButtonCode::from_scroll_down(is_down);
    // For scroll, we report once per scroll event
    // (Zed repeats for multi-line, but typical usage sends one report per scroll event)
    let mods = MouseModifiers::from(modifiers);
    if mode.intersects(Modes::MOUSE_MODE) {
        mouse_report(code, true, col, row, &mods, MouseFormat::from_mode(mode))
    } else {
        None
    }
}

// ── Internal: generic mouse report formatting ──

fn mouse_report(
    button: MouseButtonCode,
    pressed: bool,
    col: u16,
    row: u16,
    mods: &MouseModifiers,
    format: MouseFormat,
) -> Option<Vec<u8>> {
    let code = mods.apply(button as u8);
    match format {
        MouseFormat::Sgr => Some(sgr_mouse_report(code, col, row, pressed).into_bytes()),
        MouseFormat::Normal(utf8) => {
            let btn = if pressed { code } else { 3 + mods.apply(0) };
            normal_mouse_report(btn, col, row, utf8)
        }
    }
}

fn normal_mouse_report(button: u8, col: u16, row: u16, utf8: bool) -> Option<Vec<u8>> {
    let max_coord: u16 = if utf8 { 2015 } else { 223 };

    if col >= max_coord || row >= max_coord {
        return None;
    }

    let mut msg = vec![b'\x1b', b'[', b'M', 32 + button];

    if utf8 && col >= 95 {
        msg.extend(utf8_encode_coord(col as u32 + 33));
    } else {
        msg.push((32 + 1 + col) as u8);
    }

    if utf8 && row >= 95 {
        msg.extend(utf8_encode_coord(row as u32 + 33));
    } else {
        msg.push((32 + 1 + row) as u8);
    }

    Some(msg)
}

fn sgr_mouse_report(button: u8, col: u16, row: u16, pressed: bool) -> String {
    let action = if pressed { 'M' } else { 'm' };
    format!("\x1b[<{};{};{}{}", button, col + 1, row + 1, action)
}

fn utf8_encode_coord(value: u32) -> Vec<u8> {
    if value <= 0x7f {
        vec![value as u8]
    } else if value <= 0x7ff {
        vec![
            0xc0 | ((value >> 6) & 0x1f) as u8,
            0x80 | (value & 0x3f) as u8,
        ]
    } else {
        vec![0x7f] // out of range indicator
    }
}
