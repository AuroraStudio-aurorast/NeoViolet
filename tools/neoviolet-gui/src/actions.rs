#[derive(Debug, Clone, Copy)]
pub enum Action {
    Open,
    OpenOnlineAudio,
    Preferences,
    About,
    Quit,
    Copy,
    Paste,
    ZoomIn,
    ZoomOut,
    ZoomReset,
    Minimize,
    ZoomWindow,
    Fullscreen,
    Repository,
    DesktopLyrics,
    LyricsSettings,
    UnlockLyrics,
    ResetLyricsPosition,
}

impl Action {
    /// These commands change player content or depend on its ready state.
    pub fn requires_player(self) -> bool {
        matches!(
            self,
            Self::Open
                | Self::OpenOnlineAudio
                | Self::DesktopLyrics
                | Self::LyricsSettings
                | Self::UnlockLyrics
                | Self::ResetLyricsPosition
        )
    }

    pub fn shortcut(
        key: &iced::keyboard::Key,
        modifiers: iced::keyboard::Modifiers,
    ) -> Option<Self> {
        use iced::keyboard::Key;
        if !modifiers.command() {
            return None;
        }
        let Key::Character(key) = key else {
            return None;
        };
        if key.eq_ignore_ascii_case("l")
            && modifiers == (iced::keyboard::Modifiers::COMMAND | iced::keyboard::Modifiers::SHIFT)
        {
            return Some(Self::DesktopLyrics);
        }
        Some(match key.as_str() {
            "o" => Self::Open,
            "c" => Self::Copy,
            "v" => Self::Paste,
            "," => Self::Preferences,
            "q" => Self::Quit,
            "+" | "=" => Self::ZoomIn,
            "-" => Self::ZoomOut,
            "0" => Self::ZoomReset,
            _ => return None,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use iced::keyboard::{Key, Modifiers};

    #[test]
    fn shifted_lyrics_shortcut_accepts_both_key_cases_and_requires_exact_modifiers() {
        for key in ["l", "L"] {
            assert!(matches!(
                Action::shortcut(
                    &Key::Character(key.into()),
                    Modifiers::COMMAND | Modifiers::SHIFT
                ),
                Some(Action::DesktopLyrics)
            ));
        }
        assert!(Action::shortcut(&Key::Character("l".into()), Modifiers::COMMAND).is_none());
        assert!(Action::shortcut(&Key::Character("L".into()), Modifiers::SHIFT).is_none());
        assert!(
            Action::shortcut(
                &Key::Character("L".into()),
                Modifiers::COMMAND | Modifiers::SHIFT | Modifiers::ALT
            )
            .is_none()
        );
    }

    #[test]
    fn zoom_in_accepts_both_plus_and_equals_key_events() {
        for (key, modifiers) in [
            ("+", Modifiers::COMMAND | Modifiers::SHIFT),
            ("=", Modifiers::COMMAND),
        ] {
            assert!(matches!(
                Action::shortcut(&Key::Character(key.into()), modifiers),
                Some(Action::ZoomIn)
            ));
        }
    }
}
