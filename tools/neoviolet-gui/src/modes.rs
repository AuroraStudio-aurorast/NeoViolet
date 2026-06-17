use std::ops::{BitOr, BitOrAssign};

/// Locally-cached terminal mode flags.
///
/// Mirrors alacritty's `TermMode` flags but lives on our side of the
/// FairMutex boundary so keystroke/mouse handlers can read modes without
/// acquiring the lock. Updated via the event listener on every PTY event.
///
/// Pattern taken from Zed's `crates/terminal/src/terminal.rs`.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Modes(u32);

impl Modes {
    pub const NONE: Self = Self(0);
    pub const APP_CURSOR: Self = Self(1 << 0);
    pub const APP_KEYPAD: Self = Self(1 << 1);
    pub const SHOW_CURSOR: Self = Self(1 << 2);
    pub const LINE_WRAP: Self = Self(1 << 3);
    pub const ORIGIN: Self = Self(1 << 4);
    pub const INSERT: Self = Self(1 << 5);
    pub const LINE_FEED_NEW_LINE: Self = Self(1 << 6);
    pub const FOCUS_IN_OUT: Self = Self(1 << 7);
    pub const ALTERNATE_SCROLL: Self = Self(1 << 8);
    pub const BRACKETED_PASTE: Self = Self(1 << 9);
    pub const SGR_MOUSE: Self = Self(1 << 10);
    pub const UTF8_MOUSE: Self = Self(1 << 11);
    pub const ALT_SCREEN: Self = Self(1 << 12);
    pub const MOUSE_REPORT_CLICK: Self = Self(1 << 13);
    pub const MOUSE_DRAG: Self = Self(1 << 14);
    pub const MOUSE_MOTION: Self = Self(1 << 15);
    pub const MOUSE_MODE: Self = Self(Self::MOUSE_REPORT_CLICK.0 | Self::MOUSE_DRAG.0 | Self::MOUSE_MOTION.0);

    pub const fn empty() -> Self {
        Self::NONE
    }

    pub const fn contains(self, other: Self) -> bool {
        self.0 & other.0 == other.0
    }

    pub const fn intersects(self, other: Self) -> bool {
        self.0 & other.0 != 0
    }

    pub fn insert(&mut self, other: Self) {
        self.0 |= other.0;
    }

    /// Sync from alacritty's `TermMode`. Called after each PTY event.
    pub fn sync_from_alacritty(&mut self, term_mode: alacritty_terminal::term::TermMode) {
        use alacritty_terminal::term::TermMode as T;
        self.0 = 0;
        if term_mode.contains(T::APP_CURSOR)         { self.insert(Self::APP_CURSOR); }
        if term_mode.contains(T::APP_KEYPAD)         { self.insert(Self::APP_KEYPAD); }
        if term_mode.contains(T::SHOW_CURSOR)        { self.insert(Self::SHOW_CURSOR); }
        if term_mode.contains(T::LINE_WRAP)          { self.insert(Self::LINE_WRAP); }
        if term_mode.contains(T::ORIGIN)             { self.insert(Self::ORIGIN); }
        if term_mode.contains(T::INSERT)             { self.insert(Self::INSERT); }
        if term_mode.contains(T::LINE_FEED_NEW_LINE)  { self.insert(Self::LINE_FEED_NEW_LINE); }
        if term_mode.contains(T::FOCUS_IN_OUT)       { self.insert(Self::FOCUS_IN_OUT); }
        if term_mode.contains(T::ALTERNATE_SCROLL)   { self.insert(Self::ALTERNATE_SCROLL); }
        if term_mode.contains(T::BRACKETED_PASTE)    { self.insert(Self::BRACKETED_PASTE); }
        if term_mode.contains(T::SGR_MOUSE)          { self.insert(Self::SGR_MOUSE); }
        if term_mode.contains(T::UTF8_MOUSE)         { self.insert(Self::UTF8_MOUSE); }
        if term_mode.contains(T::ALT_SCREEN)         { self.insert(Self::ALT_SCREEN); }
        if term_mode.contains(T::MOUSE_REPORT_CLICK) { self.insert(Self::MOUSE_REPORT_CLICK); }
        if term_mode.contains(T::MOUSE_DRAG)          { self.insert(Self::MOUSE_DRAG); }
        if term_mode.contains(T::MOUSE_MOTION)        { self.insert(Self::MOUSE_MOTION); }
    }
}

impl BitOr for Modes {
    type Output = Self;

    fn bitor(self, rhs: Self) -> Self::Output {
        Self(self.0 | rhs.0)
    }
}

impl BitOrAssign for Modes {
    fn bitor_assign(&mut self, rhs: Self) {
        self.insert(rhs);
    }
}
