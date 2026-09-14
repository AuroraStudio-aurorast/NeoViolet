//! The app's Dracula palette, expressed as overrides on the bundled dark theme.
//!
//! yororen-ui 0.3 dropped the typed `Theme` struct this file used to build.
//! A theme is now a JSON document read by dot path, and any path a theme does
//! not set falls back to the renderer's own default. That is a nicer model for
//! us — the bundled theme already fills every colour path the renderers read —
//! but it fails silently: `Theme::set` creates a missing path rather than
//! rejecting it, so a misspelled key here would add an unused entry and quietly
//! leave the base colour in place. The tests below exist for exactly that
//! reason, and they already earned it twice while this file was written: they
//! caught a bare `status.danger` that would have flattened the `status.danger.bg`
//! / `.fg` object the renderers actually read.

use serde_json::Value;
use yororen_ui::renderer::themes;
use yororen_ui::theme::Theme;

/// Dracula palette, applied over `themes::system_dark()`.
///
/// The paths mirror the bundled theme's own shape. Status colours are nested
/// (`status.success.bg` / `.fg`) because that is how the renderers build them,
/// and the old theme's warning/info colours are kept even though today's
/// renderers only read success/error/danger: they are part of our palette and
/// cost nothing to carry.
const DRACULA: &[(&str, &str)] = &[
    // Surfaces.
    ("surface.base", "#21222c"),
    ("surface.canvas", "#282a36"),
    ("surface.raised", "#383a4a"),
    ("surface.sunken", "#1d1e26"),
    ("surface.hover", "#44475a"),
    ("surface.popover", "#383a4a"),
    // Scrim behind modals and overlays: Dracula's background, mostly opaque.
    // The bundled theme leaves this to the renderer's fallback.
    ("surface.scrim", "#1d1e26cc"),
    // Foreground content.
    ("content.primary", "#f8f8f2"),
    ("content.secondary", "#cfcfc2"),
    ("content.tertiary", "#6272a4"),
    ("content.disabled", "#525263"),
    ("content.on_primary", "#0b0b0d"),
    ("content.on_status", "#0b0b0d"),
    ("content.error", "#ff5555"),
    // Borders.
    ("border.default", "#44475a"),
    ("border.muted", "#383a4a"),
    ("border.focus", "#bd93f9"),
    ("border.divider", "#383a4a"),
    // Neutral actions (secondary buttons).
    ("action.neutral.bg", "#383a4a"),
    ("action.neutral.hover_bg", "#44475a"),
    ("action.neutral.active_bg", "#525270"),
    ("action.neutral.fg", "#f8f8f2"),
    ("action.neutral.disabled_bg", "#2a2a35"),
    ("action.neutral.disabled_fg", "#525263"),
    // Primary actions (purple).
    ("action.primary.bg", "#bd93f9"),
    ("action.primary.hover_bg", "#caa3fa"),
    ("action.primary.active_bg", "#ad7ff7"),
    ("action.primary.fg", "#0b0b0d"),
    ("action.primary.disabled_bg", "#5a4580"),
    ("action.primary.disabled_fg", "#525263"),
    // Destructive actions (red).
    ("action.danger.bg", "#ff5555"),
    ("action.danger.hover_bg", "#ff6e6e"),
    ("action.danger.active_bg", "#ff4040"),
    ("action.danger.fg", "#f8f8f2"),
    ("action.danger.disabled_bg", "#663333"),
    ("action.danger.disabled_fg", "#525263"),
    // Status colours, nested the way the renderers read them.
    ("status.neutral.bg", "#44475a"),
    ("status.neutral.fg", "#f8f8f2"),
    ("status.success.bg", "#50fa7b"),
    ("status.success.fg", "#0b0b0d"),
    ("status.warning.bg", "#ffb86c"),
    ("status.warning.fg", "#0b0b0d"),
    ("status.danger.bg", "#ff5555"),
    ("status.danger.fg", "#0b0b0d"),
    ("status.error.bg", "#ff5555"),
    ("status.error.fg", "#0b0b0d"),
    ("status.info.bg", "#8be9fd"),
    ("status.info.fg", "#0b0b0d"),
    // Shadows. 0.3 keeps them in the theme, but note the bundled theme writes
    // them as `rgba(0,0,0,0.30)` while `value_to_hsla` only decodes `#rrggbb`
    // and `#rrggbbaa`, so those strings resolve to nothing. Ours use hex with
    // the same alphas as the typed theme this replaced.
    ("shadow.elevation_1", "#0000002e"),
    ("shadow.elevation_2", "#0000004d"),
];

/// Paths the renderers do read but the bundled theme deliberately leaves unset,
/// so the renderer's own fallback covers them. Keeping this list short and
/// explicit is what lets the guard test stay strict for everything else.
#[cfg(test)]
const OPTIONAL_PATHS: &[&str] = &[
    // Scrim: `TokenOverlayRenderer` falls back to a 50% black.
    "surface.scrim",
    // Form-field error text: falls back to `status.danger`, which the bundled
    // theme stores as an object and therefore cannot resolve as a colour.
    "content.error",
];

/// Build the Dracula theme: the bundled dark theme with the palette above
/// layered on top of it.
pub fn dracula_theme() -> Theme {
    let mut theme = themes::system_dark();
    for (path, hex) in DRACULA {
        theme.set(path, Value::String((*hex).to_string()));
    }
    theme
}

#[cfg(test)]
mod tests {
    use super::*;
    use gpui::Hsla;

    /// Resolve a colour the same way the renderer would, so expected values
    /// never have to be hand-derived.
    fn color_of(spec: &str) -> Hsla {
        let json = format!("{{\"probe\":\"{spec}\"}}");
        Theme::from_json(&json)
            .expect("probe theme parses")
            .get_color("probe")
            .expect("probe colour parses")
    }

    /// `Theme::set` happily creates paths that no renderer reads and never
    /// complains about a misspelling, so a typo would leave the base theme's
    /// colour in place while every other test still passed. Requiring each path
    /// to pre-exist in the bundled theme is what makes a typo fail loudly.
    #[test]
    fn every_override_path_is_one_the_bundled_theme_defines() {
        let base = themes::system_dark();
        for (path, _) in DRACULA {
            assert!(
                base.get(path).is_some() || OPTIONAL_PATHS.contains(path),
                "neither the bundled dark theme nor the optional list knows {path}"
            );
        }
    }

    #[test]
    fn overrides_replace_the_bundled_palette() {
        let theme = dracula_theme();
        for (path, spec) in DRACULA {
            assert_eq!(
                theme.get_color(path),
                Some(color_of(spec)),
                "override did not take effect for {path}"
            );
        }
    }

    /// Setting a parent path as a string would flatten the nested object the
    /// renderers read from, which is the mistake this file already made once
    /// with `status.danger`.
    #[test]
    fn nested_status_colours_survive_as_an_object() {
        let theme = dracula_theme();
        assert!(theme.get("status.success").is_some_and(|v| v.is_object()));
        assert_eq!(
            theme.get_color("status.success.bg"),
            Some(color_of("#50fa7b"))
        );
        assert_eq!(
            theme.get_color("status.success.fg"),
            Some(color_of("#0b0b0d"))
        );
    }

    /// The palette is only visible if nothing re-installs a theme, so guard a
    /// few colours the app's own chrome and the dialogs draw with.
    #[test]
    fn chrome_colours_are_dracula_not_the_base_theme() {
        let theme = dracula_theme();
        let base = themes::system_dark();
        for path in [
            "surface.canvas",
            "surface.raised",
            "surface.sunken",
            "content.primary",
            "content.secondary",
        ] {
            assert_ne!(
                theme.get_color(path),
                base.get_color(path),
                "{path} was left at the bundled value"
            );
        }
    }
}
