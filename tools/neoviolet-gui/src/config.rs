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
            monospace_font: "Menlo".into(),
            font_size: 14,
            window_width: 800,
            window_height: 600,
            opacity: 1.0,
            neoviolet_path: None,
        }
    }
}

pub fn load_or_create() -> GuiConfig {
    let config_dir = dirs::config_dir()
        .unwrap_or_else(|| {
            let home = dirs::home_dir().unwrap_or_default();
            home.join(".config")
        })
        .join("neoviolet");

    let config_path = config_dir.join("neoviolet_gui.toml");

    if let Ok(content) = std::fs::read_to_string(&config_path)
        && let Ok(cfg) = toml::from_str::<GuiConfig>(&content)
    {
        return cfg;
    }

    // Create defaults
    let cfg = GuiConfig::default();
    if let Ok(toml_str) = toml::to_string_pretty(&cfg) {
        let _ = std::fs::create_dir_all(&config_dir);
        let _ = std::fs::write(&config_path, toml_str);
    }
    cfg
}
