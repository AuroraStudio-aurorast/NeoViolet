use gpui::{hsla, rgb};
use yororen_ui::theme::{
    ActionTheme, ActionVariant, BorderTheme, ContentTheme, StatusTheme, StatusVariant,
    SurfaceTheme, Theme,
};

/// Dracula color palette for the UI chrome.
pub fn dracula_theme() -> Theme {
    let content = ContentTheme {
        primary: rgb(0xf8f8f2).into(),
        secondary: rgb(0xcfcfc2).into(),
        tertiary: rgb(0x6272a4).into(),
        disabled: rgb(0x525263).into(),
        on_primary: rgb(0x0b0b0d).into(),
        on_status: rgb(0x0b0b0d).into(),
    };

    Theme {
        surface: SurfaceTheme {
            canvas: rgb(0x282a36).into(),
            base: rgb(0x21222c).into(),
            raised: rgb(0x383a4a).into(),
            sunken: rgb(0x1d1e26).into(),
            hover: rgb(0x44475a).into(),
        },
        content: content.clone(),
        border: BorderTheme {
            default: rgb(0x44475a).into(),
            muted: rgb(0x383a4a).into(),
            focus: rgb(0xbd93f9).into(),
            divider: rgb(0x383a4a).into(),
        },
        action: ActionTheme {
            neutral: ActionVariant {
                bg: rgb(0x383a4a).into(),
                hover_bg: rgb(0x44475a).into(),
                active_bg: rgb(0x525270).into(),
                fg: content.primary,
                disabled_bg: rgb(0x2a2a35).into(),
                disabled_fg: content.disabled,
            },
            primary: ActionVariant {
                bg: rgb(0xbd93f9).into(),
                hover_bg: rgb(0xcaa3fa).into(),
                active_bg: rgb(0xad7ff7).into(),
                fg: content.on_primary,
                disabled_bg: rgb(0x5a4580).into(),
                disabled_fg: content.disabled,
            },
            danger: ActionVariant {
                bg: rgb(0xff5555).into(),
                hover_bg: rgb(0xff6e6e).into(),
                active_bg: rgb(0xff4040).into(),
                fg: content.primary,
                disabled_bg: rgb(0x663333).into(),
                disabled_fg: content.disabled,
            },
        },
        status: StatusTheme {
            success: StatusVariant {
                bg: rgb(0x50fa7b).into(),
                fg: content.on_status,
            },
            warning: StatusVariant {
                bg: rgb(0xffb86c).into(),
                fg: content.on_status,
            },
            error: StatusVariant {
                bg: rgb(0xff5555).into(),
                fg: content.on_status,
            },
            info: StatusVariant {
                bg: rgb(0x8be9fd).into(),
                fg: content.on_status,
            },
        },
        shadow: yororen_ui::theme::ShadowTheme {
            elevation_1: hsla(0.0, 0.0, 0.0, 0.18),
            elevation_2: hsla(0.0, 0.0, 0.0, 0.30),
        },
        text_direction: Default::default(),
    }
}
