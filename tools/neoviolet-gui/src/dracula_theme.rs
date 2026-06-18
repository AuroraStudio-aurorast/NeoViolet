use gpui::{hsla, rgb};
use yororen_ui::theme::{
    ActionTheme, ActionVariant, BorderTheme, ContentTheme, StatusTheme, StatusVariant,
    SurfaceTheme, Theme,
};

/// Dracula color palette for the UI chrome.
///
/// Based on the Dracula theme (https://draculatheme.com).
/// Applied to all yororen_ui components via `GlobalTheme`.
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

// ── 256-color palette fallback for terminal cells ──

/// Map an ANSI named-color or 256-color index to an RGB triplet.
/// Used by the terminal grid renderer when alacritty's color table is empty.
pub fn dracula_color(index: usize) -> [u8; 3] {
    use alacritty_terminal::vte::ansi::NamedColor;

    // Standard 16 ANSI + Foreground/Background/Cursor + Dim variants
    match index {
        n if n == NamedColor::Black as usize => [0x21, 0x22, 0x2c],
        n if n == NamedColor::Red as usize => [0xff, 0x55, 0x55],
        n if n == NamedColor::Green as usize => [0x50, 0xfa, 0x7b],
        n if n == NamedColor::Yellow as usize => [0xf1, 0xfa, 0x8c],
        n if n == NamedColor::Blue as usize => [0xbd, 0x93, 0xf9],
        n if n == NamedColor::Magenta as usize => [0xff, 0x79, 0xc6],
        n if n == NamedColor::Cyan as usize => [0x8b, 0xe9, 0xfd],
        n if n == NamedColor::White as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::BrightBlack as usize => [0x62, 0x72, 0xa4],
        n if n == NamedColor::BrightRed as usize => [0xff, 0x6e, 0x6e],
        n if n == NamedColor::BrightGreen as usize => [0x69, 0xff, 0x94],
        n if n == NamedColor::BrightYellow as usize => [0xff, 0xff, 0xa5],
        n if n == NamedColor::BrightBlue as usize => [0xd6, 0xac, 0xff],
        n if n == NamedColor::BrightMagenta as usize => [0xff, 0x92, 0xdf],
        n if n == NamedColor::BrightCyan as usize => [0xa4, 0xff, 0xff],
        n if n == NamedColor::BrightWhite as usize => [0xff, 0xff, 0xff],
        n if n == NamedColor::Foreground as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::Background as usize => [0x28, 0x2a, 0x36],
        n if n == NamedColor::Cursor as usize => [0xf8, 0xf8, 0xf2],
        n if n == NamedColor::DimBlack as usize => [0x21, 0x22, 0x2c],
        n if n == NamedColor::DimRed as usize => [0xff, 0x55, 0x55],
        n if n == NamedColor::DimGreen as usize => [0x50, 0xfa, 0x7b],
        n if n == NamedColor::DimYellow as usize => [0xf1, 0xfa, 0x8c],
        n if n == NamedColor::DimBlue as usize => [0xbd, 0x93, 0xf9],
        n if n == NamedColor::DimMagenta as usize => [0xff, 0x79, 0xc6],
        n if n == NamedColor::DimCyan as usize => [0x8b, 0xe9, 0xfd],
        n if n == NamedColor::DimWhite as usize => [0xf8, 0xf8, 0xf2],

        // 6×6×6 color cube (indices 16-231)
        i if (16..=231).contains(&i) => {
            let v = (i - 16) as u8;
            let (r, g, b) = (v / 36, (v / 6) % 6, v % 6);
            let level = |c: u8| if c == 0 { 0 } else { (c * 40 + 55).min(255) };
            [level(r), level(g), level(b)]
        }

        // Grayscale (indices 232-255)
        i if (232..=255).contains(&i) => {
            let l = ((i - 232) as u8 * 10 + 8).min(255);
            [l, l, l]
        }

        _ => [0xf8, 0xf8, 0xf2],
    }
}
