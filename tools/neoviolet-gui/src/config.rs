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
    /// Keep the lyrics window above other windows.
    #[serde(default = "default_desktop_lyrics_always_on_top")]
    pub always_on_top: bool,
    /// Last saved window X position (None = auto-center).
    #[serde(default)]
    pub position_x: Option<i32>,
    /// Last saved window Y position (None = auto-center).
    #[serde(default)]
    pub position_y: Option<i32>,
}

// ── Default value helpers for serde ──

fn default_desktop_lyrics_enabled() -> bool { false }
fn default_desktop_lyrics_font_family() -> String {
    if cfg!(target_os = "macos") {
        "Helvetica Neue".into()
    } else if cfg!(target_os = "windows") {
        "Segoe UI".into()
    } else {
        "Sans".into()
    }
}
fn default_desktop_lyrics_font_size() -> u32 { 18 }
fn default_desktop_lyrics_window_width() -> u32 { 600 }
fn default_desktop_lyrics_window_height() -> u32 { 80 }
fn default_desktop_lyrics_opacity() -> f32 { 0.85 }
fn default_desktop_lyrics_show_song_info() -> bool { true }
fn default_desktop_lyrics_num_lines() -> u32 { 1 }
fn default_desktop_lyrics_text_color() -> String { "#FFFFFF".into() }
fn default_desktop_lyrics_highlight_color() -> String { "#FFD700".into() }
fn default_desktop_lyrics_always_on_top() -> bool { true }

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
            always_on_top: true,
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
    if pre_existed {
        if let Ok(toml_str) = toml::to_string_pretty(&cfg) {
            // Atomic write: write to a temporary file, then rename
            let tmp_path = config_path.with_extension("tmp");
            if std::fs::write(&tmp_path, &toml_str).is_ok() {
                let _ = std::fs::rename(&tmp_path, &config_path);
            }
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
            dirs::config_dir().unwrap_or_else(|| {
                dirs::home_dir()
                    .unwrap_or_default()
                    .join(".config")
            })
        })
        .join("neoviolet")
}
