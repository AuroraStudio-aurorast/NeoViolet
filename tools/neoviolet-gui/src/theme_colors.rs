//! Theme colour accessors for the colours the app paints itself.
//!
//! yororen-ui 0.3 themes are JSON documents: a path either exists or it does
//! not, and `Theme::get_color` returns an `Option`. Component renderers bring
//! their own fallbacks, but the few colours this app draws directly — the
//! terminal's default foreground/background/cursor, dialog body text — need
//! one too, and repeating `.unwrap_or(..)` at two dozen call sites would bury
//! the intent. Hence this module.
//!
//! The literals are Dracula values and exist only as a guard: the app installs
//! a theme that defines every path below, so a fallback being used means the
//! theme is wrong, and returning something readable beats painting transparent
//! black.

use gpui::{Hsla, rgb};
use yororen_ui::theme::{ActiveTheme, Theme};

/// Resolve `path`, falling back to the packed RGB literal when the theme does
/// not define it. Split out from the `cx` wrappers so it can be tested without
/// building a gpui `App`.
fn resolve(theme: &Theme, path: &str, fallback: u32) -> Hsla {
    theme
        .get_color(path)
        .unwrap_or_else(|| rgb(fallback).into())
}

/// Window and terminal background.
pub fn canvas(cx: &impl ActiveTheme) -> Hsla {
    resolve(cx.theme(), "surface.canvas", 0x282a36)
}

/// Recessed surfaces, such as the captured-help box in the bad-args dialog.
pub fn sunken(cx: &impl ActiveTheme) -> Hsla {
    resolve(cx.theme(), "surface.sunken", 0x1d1e26)
}

/// Primary foreground: terminal default text, dialog headings.
pub fn primary(cx: &impl ActiveTheme) -> Hsla {
    resolve(cx.theme(), "content.primary", 0xf8f8f2)
}

/// Secondary foreground: dialog body text.
pub fn secondary(cx: &impl ActiveTheme) -> Hsla {
    resolve(cx.theme(), "content.secondary", 0xcfcfc2)
}

/// Tertiary foreground: dimmed hints and notes.
pub fn tertiary(cx: &impl ActiveTheme) -> Hsla {
    resolve(cx.theme(), "content.tertiary", 0x6272a4)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn defined_paths_win_over_the_fallback() {
        let theme = Theme::from_json(r##"{"content":{"primary":"#00ff00"}}"##).expect("parse");
        let got = resolve(&theme, "content.primary", 0xf8f8f2);
        let want = Theme::from_json(r##"{"x":"#00ff00"}"##)
            .expect("parse")
            .get_color("x")
            .expect("colour");
        assert_eq!(got, want);
    }

    #[test]
    fn missing_paths_fall_back() {
        let theme = Theme::from_json(r##"{"content":{}}"##).expect("parse");
        let got = resolve(&theme, "content.primary", 0xf8f8f2);
        assert_eq!(got, rgb(0xf8f8f2).into());
        // The fallback must stay opaque: a transparent foreground would make
        // the terminal look broken rather than merely off-palette.
        assert_eq!(got.a, 1.0);
    }
}
