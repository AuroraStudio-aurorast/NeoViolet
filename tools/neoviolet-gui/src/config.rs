use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(default)]
pub struct Config {
    pub monospace_font: String,
    pub font_size: u32,
    #[serde(skip)]
    pub window_width: u32,
    #[serde(skip)]
    pub window_height: u32,
    pub opacity: f32,
    pub neoviolet_path: Option<String>,
    pub zoom_via_scroll: bool,
    pub menu_bar: bool,
    pub system_decorations: bool,
    pub minimize_button: bool,
    #[serde(skip)]
    pub desktop_lyrics: crate::lyrics::Preferences,
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
            menu_bar: false,
            system_decorations: false,
            minimize_button: true,
            desktop_lyrics: crate::lyrics::Preferences::default(),
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
        Self::load_from(&directory())
    }

    fn load_from(root: &Path) -> (Self, Option<String>) {
        let (preferences, mut warning) = read_json::<Preferences>(&root.join("config.json"));
        let mut config = preferences.gui;
        config.desktop_lyrics = preferences.desktop_lyrics;
        let (state, state_warning) = read_json::<LocalState>(&root.join("gui-state.json"));
        warning = warning.or(state_warning);
        config.window_width = state.main_window.width;
        config.window_height = state.main_window.height;
        config.desktop_lyrics.position = state.desktop_lyrics.position;
        config.normalize();
        (config, warning)
    }

    pub(crate) fn normalize(&mut self) {
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
        self.save_to(&directory())
    }

    fn save_to(&self, root: &Path) -> Result<(), String> {
        std::fs::create_dir_all(root).map_err(|error| error.to_string())?;
        let state = LocalState {
            main_window: MainWindow {
                width: self.window_width,
                height: self.window_height,
            },
            desktop_lyrics: LyricsState {
                position: self.desktop_lyrics.position,
            },
        };
        let contents = serde_json::to_vec_pretty(&state).map_err(|error| error.to_string())?;
        let temporary = root.join(format!(".gui-state.{}.tmp", std::process::id()));
        let result = (|| {
            use std::io::Write;
            let mut file = std::fs::File::create(&temporary).map_err(|error| error.to_string())?;
            file.write_all(&contents)
                .map_err(|error| error.to_string())?;
            file.sync_all().map_err(|error| error.to_string())?;
            drop(file);
            std::fs::rename(&temporary, root.join("gui-state.json"))
                .map_err(|error| error.to_string())
        })();
        if result.is_err() {
            let _ = std::fs::remove_file(temporary);
        }
        result
    }
}

// This is the Go-owned config projection, also used by the IPC snapshot.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct Preferences {
    pub gui: Config,
    pub desktop_lyrics: crate::lyrics::Preferences,
}

#[derive(Default, Deserialize, Serialize)]
#[serde(default)]
struct LocalState {
    main_window: MainWindow,
    desktop_lyrics: LyricsState,
}
#[derive(Deserialize, Serialize)]
#[serde(default)]
struct MainWindow {
    width: u32,
    height: u32,
}
impl Default for MainWindow {
    fn default() -> Self {
        Self {
            width: 1000,
            height: 700,
        }
    }
}
#[derive(Default, Deserialize, Serialize)]
#[serde(default)]
struct LyricsState {
    position: Option<crate::lyrics::Position>,
}

fn read_json<T: serde::de::DeserializeOwned + Default>(path: &Path) -> (T, Option<String>) {
    match std::fs::read(path) {
        Ok(data) => match serde_json::from_slice(&data) {
            Ok(value) => (value, None),
            Err(error) => (
                T::default(),
                Some(format!("Could not read {}: {error}", path.display())),
            ),
        },
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => (T::default(), None),
        Err(error) => (
            T::default(),
            Some(format!("Could not open {}: {error}", path.display())),
        ),
    }
}

#[cfg(test)]
#[path = "config_tests.rs"]
mod tests;
