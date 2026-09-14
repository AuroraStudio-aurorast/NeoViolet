use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DesktopLyricsConfig {
    /// Automatically open desktop lyrics window on startup.
    #[serde(default = "default_desktop_lyrics_enabled")]
    pub enabled: bool,
    /// Font family for lyric text.
    #[serde(default = "default_desktop_lyrics_font_family")]
    pub font_family: String,
    /// Font size in pixels.
    #[serde(default = "default_desktop_lyrics_font_size")]
    pub font_size: u32,
    /// Window width in pixels.
    #[serde(default = "default_desktop_lyrics_window_width")]
    pub window_width: u32,
    /// Window height in pixels.
    #[serde(default = "default_desktop_lyrics_window_height")]
    pub window_height: u32,
    /// Window opacity (0.0–1.0).
    #[serde(default = "default_desktop_lyrics_opacity")]
    pub opacity: f32,
    /// Show song title and artist above the lyrics.
    #[serde(default = "default_desktop_lyrics_show_song_info")]
    pub show_song_info: bool,
    /// Number of time-slot lines to show (1, 3, or 5).
    #[serde(default = "default_desktop_lyrics_num_lines")]
    pub num_lines: u32,
    /// Hex color for non-highlighted lyric text.
    #[serde(default = "default_desktop_lyrics_text_color")]
    pub text_color: String,
    /// Hex color for the current (active) lyric line.
    #[serde(default = "default_desktop_lyrics_highlight_color")]
    pub highlight_color: String,
    /// Last saved window X position (None = auto-center).
    #[serde(default)]
    pub position_x: Option<i32>,
    /// Last saved window Y position (None = auto-center).
    #[serde(default)]
    pub position_y: Option<i32>,
}

// ── Default value helpers for serde ──

fn default_zoom_via_scroll() -> bool {
    false
}
fn default_desktop_lyrics_enabled() -> bool {
    false
}
fn default_desktop_lyrics_font_family() -> String {
    if cfg!(target_os = "macos") {
        "Helvetica Neue".into()
    } else if cfg!(target_os = "windows") {
        "Segoe UI".into()
    } else {
        "Sans".into()
    }
}
fn default_desktop_lyrics_font_size() -> u32 {
    18
}
fn default_desktop_lyrics_window_width() -> u32 {
    600
}
fn default_desktop_lyrics_window_height() -> u32 {
    80
}
fn default_desktop_lyrics_opacity() -> f32 {
    0.85
}
fn default_desktop_lyrics_show_song_info() -> bool {
    true
}
fn default_desktop_lyrics_num_lines() -> u32 {
    1
}
fn default_desktop_lyrics_text_color() -> String {
    "#FFFFFF".into()
}
fn default_desktop_lyrics_highlight_color() -> String {
    "#FFD700".into()
}
impl Default for DesktopLyricsConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            font_family: default_desktop_lyrics_font_family(),
            font_size: 18,
            window_width: 600,
            window_height: 80,
            opacity: 0.85,
            show_song_info: true,
            num_lines: 1,
            text_color: "#FFFFFF".into(),
            highlight_color: "#FFD700".into(),
            position_x: None,
            position_y: None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GuiConfig {
    pub monospace_font: String,
    pub font_size: u32,
    pub window_width: u32,
    pub window_height: u32,
    pub opacity: f32,
    pub neoviolet_path: Option<String>,
    /// Enable zoom via Ctrl+scroll (non-macOS) or Cmd+scroll (macOS).
    #[serde(default = "default_zoom_via_scroll")]
    pub zoom_via_scroll: bool,
    #[serde(default)]
    pub desktop_lyrics: DesktopLyricsConfig,
}

impl Default for GuiConfig {
    fn default() -> Self {
        Self {
            monospace_font: crate::platform::default_monospace_font().into(),
            font_size: 14,
            window_width: 800,
            window_height: 600,
            opacity: 1.0,
            neoviolet_path: None,
            zoom_via_scroll: false,
            desktop_lyrics: DesktopLyricsConfig::default(),
        }
    }
}

pub fn load_or_create() -> GuiConfig {
    let config_dir = config_dir_path();
    let config_path = config_dir.join("neoviolet_gui.toml");

    let pre_existed = config_path.exists();

    let mut cfg = if let Ok(content) = std::fs::read_to_string(&config_path)
        && let Ok(parsed) = toml::from_str::<GuiConfig>(&content)
    {
        parsed
    } else {
        let defaults = GuiConfig::default();
        if let Ok(toml_str) = toml::to_string_pretty(&defaults) {
            let _ = std::fs::create_dir_all(&config_dir);
            let _ = std::fs::write(&config_path, toml_str);
        }
        defaults
    };

    // Validate the configured font family — if it doesn't exist on this
    // system, fall back to the platform default monospace font.
    if !crate::platform::is_font_available(&cfg.monospace_font) {
        let fallback = crate::platform::default_monospace_font();
        log::warn!(
            "Configured font '{}' is not installed; falling back to '{}'",
            cfg.monospace_font,
            fallback,
        );
        cfg.monospace_font = fallback.to_string();
    }

    // Validate desktop-lyrics font — if not installed, warn but keep the user's choice.
    // Unlike the terminal font, desktop lyrics can use any font family.
    if !crate::platform::is_font_available(&cfg.desktop_lyrics.font_family) {
        log::warn!(
            "Configured desktop-lyrics font '{}' may not be installed",
            cfg.desktop_lyrics.font_family,
        );
    }

    // If the config file pre-existed, re-serialize to capture any new
    // fields that were filled in by Default during deserialization.
    if pre_existed && let Ok(toml_str) = toml::to_string_pretty(&cfg) {
        // Atomic write: write to a temporary file, then rename
        let tmp_path = config_path.with_extension("tmp");
        if std::fs::write(&tmp_path, &toml_str).is_ok() {
            let _ = std::fs::rename(&tmp_path, &config_path);
        }
    }

    cfg
}

/// Returns the config directory path (same as the CLI's --xdg-config path).
///
/// Resolution order:
/// 1. `$XDG_CONFIG_HOME/neoviolet/`
/// 2. `~/.config/neoviolet/`
/// 3. OS config directory /neoviolet/ (fallback)
/// 4. `~/.config/neoviolet/` (ultimate fallback)
pub fn config_dir_path() -> std::path::PathBuf {
    std::env::var("XDG_CONFIG_HOME")
        .ok()
        .filter(|p| !p.is_empty())
        .map(std::path::PathBuf::from)
        .or_else(|| dirs::home_dir().map(|home| home.join(".config")))
        .unwrap_or_else(|| {
            dirs::config_dir()
                .unwrap_or_else(|| dirs::home_dir().unwrap_or_default().join(".config"))
        })
        .join("neoviolet")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn defaults_are_sane() {
        let c = GuiConfig::default();
        assert!(!c.desktop_lyrics.enabled);
        assert!(c.desktop_lyrics.font_size > 0);
        assert!(c.desktop_lyrics.opacity > 0.0 && c.desktop_lyrics.opacity <= 1.0);
    }

    #[test]
    fn malformed_toml_falls_back_to_defaults() {
        let cfg = toml::from_str::<GuiConfig>("not valid toml [[[").unwrap_or_default();
        assert!(cfg.desktop_lyrics.font_size > 0);
    }

    /// `load_or_create` rewrites neoviolet_gui.toml on every launch (to capture
    /// fields that Default filled in), so the text form has to be a fixed point:
    /// what we write must parse back to the same values and re-serialize to the
    /// same bytes. This is what was worth checking in the toml 0.8 -> 1.x bump,
    /// where float formatting changed.
    #[test]
    fn config_toml_round_trip_is_a_fixed_point() {
        let original = GuiConfig::default();
        let first = toml::to_string_pretty(&original).expect("serialize defaults");
        let parsed: GuiConfig = toml::from_str(&first).expect("parse what we just wrote");
        let second = toml::to_string_pretty(&parsed).expect("re-serialize");

        assert_eq!(
            first, second,
            "rewriting the config file changed its bytes, so every launch would dirty it",
        );
        assert_eq!(parsed.monospace_font, original.monospace_font);
        assert_eq!(parsed.font_size, original.font_size);
        assert_eq!(parsed.opacity, original.opacity);
        assert_eq!(parsed.zoom_via_scroll, original.zoom_via_scroll);
        assert_eq!(
            parsed.desktop_lyrics.opacity,
            original.desktop_lyrics.opacity
        );
        assert_eq!(
            parsed.desktop_lyrics.num_lines,
            original.desktop_lyrics.num_lines
        );
        assert_eq!(
            parsed.desktop_lyrics.text_color,
            original.desktop_lyrics.text_color
        );
    }

    /// Both opacity fields are f32, and toml prints the shortest string that
    /// round-trips. Reading that string back must reproduce the same bits rather
    /// than a merely close value: the old serializer emitted the f32 widened to
    /// f64 (0.8500000238418579), which was ugly but lossless, and a serializer
    /// that instead rounded would silently shift every user's saved opacity.
    #[test]
    fn f32_opacity_survives_the_text_form_exactly() {
        for value in [0.0f32, 0.1, 0.33, 0.5, 0.85, 1.0, 0.123_456_79] {
            // Built in one initializer on purpose: clippy's
            // field_reassign_with_default rejects assigning into a struct that
            // Default::default() just produced.
            let cfg = GuiConfig {
                opacity: value,
                desktop_lyrics: DesktopLyricsConfig {
                    opacity: value,
                    ..Default::default()
                },
                ..Default::default()
            };

            let text = toml::to_string_pretty(&cfg).expect("serialize");
            let back: GuiConfig = toml::from_str(&text).expect("parse");

            assert_eq!(
                back.opacity.to_bits(),
                value.to_bits(),
                "window opacity {value} lost precision, file was: {text}",
            );
            assert_eq!(
                back.desktop_lyrics.opacity.to_bits(),
                value.to_bits(),
                "desktop lyrics opacity {value} lost precision, file was: {text}",
            );
        }
    }
}
