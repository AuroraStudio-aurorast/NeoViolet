//! Platform menu labels, commands and shortcuts.
use crate::actions::Action;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
pub(super) enum Platform {
    MacOs,
    Windows,
    Linux,
}
impl Platform {
    pub(super) fn current() -> Self {
        #[cfg(target_os = "macos")]
        {
            Self::MacOs
        }
        #[cfg(target_os = "windows")]
        {
            Self::Windows
        }
        #[cfg(not(any(target_os = "macos", target_os = "windows")))]
        {
            Self::Linux
        }
    }
}

pub(super) struct MenuItem {
    pub(super) label: &'static str,
    pub(super) shortcut: Option<&'static str>,
    pub(super) action: Action,
    pub(super) checked: bool,
}

impl MenuItem {
    pub(super) fn enabled(&self, lyrics: bool, available: bool) -> bool {
        (available || !self.action.requires_player())
            && (!matches!(self.action, Action::UnlockLyrics) || lyrics)
    }
}

pub(super) struct MenuGroup {
    pub(super) name: &'static str,
    pub(super) items: Vec<MenuItem>,
}

impl MenuGroup {
    pub(super) fn all(platform: Platform, lyrics_enabled: bool, maximized: bool) -> Vec<Self> {
        let lyrics_shortcut = match platform {
            Platform::MacOs => "⌘⇧L",
            Platform::Windows | Platform::Linux => "Ctrl+Shift+L",
        };
        vec![
            Self {
                name: "File",
                items: vec![
                    MenuItem {
                        label: "Open File…",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘O"
                        } else {
                            "Ctrl+O"
                        }),
                        action: Action::Open,
                        checked: false,
                    },
                    MenuItem {
                        label: if platform == Platform::Windows {
                            "Exit"
                        } else {
                            "Quit"
                        },
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘Q"
                        } else {
                            "Ctrl+Q"
                        }),
                        action: Action::Quit,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Edit",
                items: vec![
                    MenuItem {
                        label: "Copy",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘C"
                        } else {
                            "Ctrl+C"
                        }),
                        action: Action::Copy,
                        checked: false,
                    },
                    MenuItem {
                        label: "Paste",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘V"
                        } else {
                            "Ctrl+V"
                        }),
                        action: Action::Paste,
                        checked: false,
                    },
                    MenuItem {
                        label: "Preferences…",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘,"
                        } else {
                            "Ctrl+,"
                        }),
                        action: Action::Preferences,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "View",
                items: vec![
                    MenuItem {
                        label: "Zoom In",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘+"
                        } else {
                            "Ctrl++"
                        }),
                        action: Action::ZoomIn,
                        checked: false,
                    },
                    MenuItem {
                        label: "Zoom Out",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘−"
                        } else {
                            "Ctrl+−"
                        }),
                        action: Action::ZoomOut,
                        checked: false,
                    },
                    MenuItem {
                        label: "Actual Size",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘0"
                        } else {
                            "Ctrl+0"
                        }),
                        action: Action::ZoomReset,
                        checked: false,
                    },
                    MenuItem {
                        label: "Desktop Lyrics",
                        shortcut: Some(lyrics_shortcut),
                        action: Action::DesktopLyrics,
                        checked: lyrics_enabled,
                    },
                    MenuItem {
                        label: "Desktop Lyrics Settings…",
                        shortcut: None,
                        action: Action::LyricsSettings,
                        checked: false,
                    },
                    MenuItem {
                        label: "Unlock Desktop Lyrics",
                        shortcut: None,
                        action: Action::UnlockLyrics,
                        checked: false,
                    },
                    MenuItem {
                        label: "Reset Lyrics Position",
                        shortcut: None,
                        action: Action::ResetLyricsPosition,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Window",
                items: vec![
                    MenuItem {
                        label: "Minimize",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘M"
                        } else {
                            ""
                        }),
                        action: Action::Minimize,
                        checked: false,
                    },
                    MenuItem {
                        label: if maximized {
                            "Restore"
                        } else if platform == Platform::MacOs {
                            "Zoom"
                        } else {
                            "Maximize"
                        },
                        shortcut: None,
                        action: Action::ZoomWindow,
                        checked: false,
                    },
                    MenuItem {
                        label: "Toggle Full Screen",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌃⌘F"
                        } else {
                            "F11"
                        }),
                        action: Action::Fullscreen,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Help",
                items: vec![
                    MenuItem {
                        label: "Project Repository",
                        shortcut: None,
                        action: Action::Repository,
                        checked: false,
                    },
                    MenuItem {
                        label: "About NeoViolet",
                        shortcut: None,
                        action: Action::About,
                        checked: false,
                    },
                ],
            },
        ]
    }
}
