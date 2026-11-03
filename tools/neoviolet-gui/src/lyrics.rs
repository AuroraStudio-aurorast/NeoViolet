//! Lyric display data derived from the MIT TUI's IPC contract.
use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

#[cfg(test)]
#[path = "lyrics_tests.rs"]
mod tests;

#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
#[serde(default)]
pub struct Frame {
    #[serde(deserialize_with = "null_lines")]
    pub lines: Vec<Line>,
    pub elapsed: f64,
    pub title: String,
    pub artist: String,
}

fn null_lines<'de, D: serde::Deserializer<'de>>(deserializer: D) -> Result<Vec<Line>, D::Error> {
    Ok(Option::<Vec<Line>>::deserialize(deserializer)?.unwrap_or_default())
}

#[derive(Debug, Clone, Default, PartialEq, Deserialize)]
#[serde(default)]
pub struct Line {
    pub time: f64,
    pub end: f64,
    pub text: String,
    pub parts: Vec<String>,
    pub words: Vec<Word>,
    pub prefix: String,
    pub agent: String,
    pub agent_name: String,
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct Word {
    pub time: f64,
    pub text: String,
}

impl Frame {
    pub fn active_lines(&self) -> Vec<&Line> {
        let elapsed = self.elapsed.max(0.0);
        let mut active: Vec<_> = self
            .lines
            .iter()
            .filter(|line| {
                if !line.valid_interval() || line.time > elapsed {
                    return false;
                }
                let end = if line.end > 0.0 {
                    line.end
                } else {
                    self.lines
                        .iter()
                        .filter(|next| next.valid_interval() && next.time > line.time)
                        .map(|next| next.time)
                        .min_by(f64::total_cmp)
                        .unwrap_or(f64::INFINITY)
                };
                elapsed < end
            })
            .collect();
        active.sort_by(|a, b| a.time.total_cmp(&b.time));
        active
    }

    pub fn next_line(&self) -> Option<&Line> {
        self.lines
            .iter()
            .filter(|line| line.valid_interval() && line.time > self.elapsed.max(0.0))
            .min_by(|a, b| a.time.total_cmp(&b.time))
    }
}

impl Line {
    fn valid_interval(&self) -> bool {
        self.time.is_finite()
            && self.time >= 0.0
            && self.end.is_finite()
            && (self.end == 0.0 || self.end > self.time)
    }
    pub fn first_text(&self) -> &str {
        self.parts
            .first()
            .map_or(self.text.as_str(), String::as_str)
    }

    pub fn additional_lines(&self) -> &[String] {
        self.parts.get(1..).unwrap_or_default()
    }

    pub fn fragments(&self, elapsed: f64) -> Vec<(&str, bool)> {
        let valid = !self.words.is_empty()
            && self
                .words
                .iter()
                .all(|word| word.time.is_finite() && word.time >= 0.0)
            && self
                .words
                .windows(2)
                .all(|pair| pair[0].time <= pair[1].time)
            && self
                .words
                .iter()
                .map(|word| word.text.as_str())
                .collect::<String>()
                == self.first_text();
        if valid {
            self.words
                .iter()
                .map(|word| (word.text.as_str(), word.time <= elapsed))
                .collect()
        } else {
            vec![(self.first_text(), true)]
        }
    }
}

#[derive(Debug, Clone, Copy, Default, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Highlight {
    #[default]
    Violet,
    White,
}

#[derive(Debug, Clone, Copy, Deserialize, Serialize)]
pub struct Position {
    pub x: f32,
    pub y: f32,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(default)]
pub struct Preferences {
    pub enabled: bool,
    pub font_size: u32,
    pub opacity: f32,
    pub highlight: Highlight,
    pub additional_lines: bool,
    pub next_line: bool,
    pub locked: bool,
    pub position: Option<Position>,
    #[serde(flatten)]
    pub extra: BTreeMap<String, toml::Value>,
}

impl Default for Preferences {
    fn default() -> Self {
        Self {
            enabled: true,
            font_size: 32,
            opacity: 1.0,
            highlight: Highlight::Violet,
            additional_lines: true,
            next_line: false,
            locked: false,
            position: None,
            extra: BTreeMap::new(),
        }
    }
}

impl Preferences {
    pub fn normalize(&mut self) {
        self.font_size = self.font_size.clamp(22, 48);
        self.opacity = if self.opacity.is_finite() {
            self.opacity.clamp(0.4, 1.0)
        } else {
            1.0
        };
        self.position = self
            .position
            .filter(|point| point.x.is_finite() && point.y.is_finite());
    }
}
