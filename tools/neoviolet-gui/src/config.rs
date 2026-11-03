use std::collections::BTreeMap;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(default)]
pub struct Config {
    pub monospace_font: String,
    pub font_size: u32,
    pub window_width: u32,
    pub window_height: u32,
    pub opacity: f32,
    pub neoviolet_path: Option<String>,
    pub zoom_via_scroll: bool,
    pub theme: String,
    pub desktop_lyrics: crate::lyrics::Preferences,
    #[serde(flatten)]
    pub extra: BTreeMap<String, toml::Value>,
}

impl Default for Config {
    fn default() -> Self {
        #[cfg(target_os = "macos")]
        let font = "Menlo";
        #[cfg(target_os = "windows")]
        let font = "Consolas";
        #[cfg(not(any(target_os = "macos", target_os = "windows")))]
        let font = "monospace";
        Self {
            monospace_font: font.into(),
            font_size: 14,
            window_width: 1000,
            window_height: 700,
            opacity: 1.0,
            neoviolet_path: None,
            zoom_via_scroll: false,
            theme: "dark".into(),
            desktop_lyrics: crate::lyrics::Preferences::default(),
            extra: BTreeMap::new(),
        }
    }
}

pub fn directory() -> PathBuf {
    #[cfg(windows)]
    let home = std::env::var_os("USERPROFILE");
    #[cfg(not(windows))]
    let home = std::env::var_os("HOME");
    std::env::var_os("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .filter(|path| path.is_absolute())
        .or_else(|| home.map(PathBuf::from).map(|home| home.join(".config")))
        .unwrap_or_else(std::env::temp_dir)
        .join("neoviolet")
}

impl Config {
    pub fn load() -> (Self, Option<String>) {
        let path = directory().join("neoviolet_gui.toml");
        match std::fs::read_to_string(&path) {
            Ok(contents) => match toml::from_str::<Self>(&contents) {
                Ok(mut config) => {
                    config.normalize();
                    (config, None)
                }
                Err(error) => (
                    Self::default(),
                    Some(format!("Could not read GUI settings: {error}")),
                ),
            },
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                let config = Self::default();
                let warning = config.save().err();
                (config, warning)
            }
            Err(error) => (
                Self::default(),
                Some(format!("Could not open GUI settings: {error}")),
            ),
        }
    }

    fn normalize(&mut self) {
        self.desktop_lyrics.normalize();
        self.font_size = self.font_size.clamp(8, 48);
        self.window_width = self.window_width.clamp(480, 7680);
        self.window_height = self.window_height.clamp(320, 4320);
        self.opacity = if self.opacity.is_finite() {
            self.opacity.clamp(0.1, 1.0)
        } else {
            1.0
        };
        if self.monospace_font.trim().is_empty() {
            self.monospace_font = Self::default().monospace_font;
        }
    }

    pub fn save(&self) -> Result<(), String> {
        let directory = directory();
        std::fs::create_dir_all(&directory).map_err(|error| error.to_string())?;
        let contents = toml::to_string_pretty(self).map_err(|error| error.to_string())?;
        let temporary = directory.join(format!("neoviolet_gui.{}.tmp", std::process::id()));
        std::fs::write(&temporary, contents).map_err(|error| error.to_string())?;
        std::fs::rename(temporary, directory.join("neoviolet_gui.toml"))
            .map_err(|error| error.to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn settings_keep_unknown_sections_and_clamp_invalid_dimensions() {
        let mut config: Config = toml::from_str(
            "font_size = 1000\nwindow_width = 0\nopacity = nan\n[desktop_lyrics]\nenabled = false\n",
        ).unwrap();
        config.normalize();
        assert_eq!(config.font_size, 48);
        assert_eq!(config.window_width, 480);
        assert_eq!(config.opacity, 1.0);
        let saved = toml::to_string(&config).unwrap();
        assert!(saved.contains("[desktop_lyrics]"));
        assert!(saved.contains("enabled = false"));
    }

    #[test]
    fn lyric_preferences_normalize_invalid_values_and_preserve_unknown_keys() {
        let mut config: Config = toml::from_str(
            "[desktop_lyrics]\nfont_size = 999\nopacity = nan\nadditional_lines = false\nnext_line = true\nlegacy_value = 'keep'\n[desktop_lyrics.position]\nx = inf\ny = 0.0\n",
        ).unwrap();
        config.normalize();
        assert_eq!(config.desktop_lyrics.font_size, 48);
        assert_eq!(config.desktop_lyrics.opacity, 1.0);
        assert!(config.desktop_lyrics.position.is_none());
        let saved = toml::to_string(&config).unwrap();
        let restored: Config = toml::from_str(&saved).unwrap();
        assert!(!restored.desktop_lyrics.additional_lines);
        assert!(restored.desktop_lyrics.next_line);
        assert_eq!(
            restored.desktop_lyrics.extra["legacy_value"].as_str(),
            Some("keep")
        );
    }
}
