use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GuiConfig {
    pub monospace_font: String,
    pub font_size: u32,
    pub window_width: u32,
    pub window_height: u32,
    pub opacity: f32,
    pub neoviolet_path: Option<String>,
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
        }
    }
}

pub fn load_or_create() -> GuiConfig {
    let config_dir = config_dir_path();
    let config_path = config_dir.join("neoviolet_gui.toml");

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
