#[derive(Debug, Clone, Copy)]
pub enum Action {
    Open,
    Preferences,
    About,
    Quit,
    Copy,
    Paste,
    ZoomIn,
    ZoomOut,
    ZoomReset,
    Theme,
    Repository,
}

impl Action {
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
        Some(match key.as_str() {
            "o" => Self::Open,
            "," => Self::Preferences,
            "q" => Self::Quit,
            "+" | "=" => Self::ZoomIn,
            "-" => Self::ZoomOut,
            "0" => Self::ZoomReset,
            _ => return None,
        })
    }
}
