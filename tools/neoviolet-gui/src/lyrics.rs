//! Lyric display data derived from the MIT TUI's IPC contract.
use serde::{Deserialize, Serialize};
use unicode_segmentation::UnicodeSegmentation;

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
    #[serde(default)]
    pub end: f64,
    pub text: String,
}

impl Frame {
    /// Bridge only short, bounded cue gaps without caching stale track data or
    /// changing the actual timing used by word highlighting and next-line lookup.
    pub fn display_lines(&self) -> Vec<&Line> {
        let active = self.active_lines();
        if !active.is_empty() {
            return active;
        }
        let Some(next) = self.next_line() else {
            return active;
        };
        let Some(end) = self
            .lines
            .iter()
            .filter(|line| line.valid_interval() && line.end > 0.0 && line.end <= self.elapsed)
            .map(|line| line.end)
            .max_by(f64::total_cmp)
        else {
            return active;
        };
        if next.time - end > 0.35 + 1e-9 {
            return active;
        }
        let mut previous: Vec<_> = self
            .lines
            .iter()
            .filter(|line| line.valid_interval() && line.end == end)
            .collect();
        previous.sort_by(|a, b| a.time.total_cmp(&b.time));
        previous
    }

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

    pub fn fragments(&self, elapsed: f64, width: impl Fn(&str) -> f32) -> Vec<(&str, f32)> {
        let text = self.first_text();
        let graphemes: Vec<_> = text.grapheme_indices(true).collect();
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
                == text;
        let mut boundary = 0;
        let aligned = self.words.iter().all(|word| {
            let start = boundary;
            boundary += word.text.len();
            start == text.len() || graphemes.iter().any(|(offset, _)| *offset == start)
        });
        if !valid || !aligned {
            return vec![(text, 1.0)];
        }
        // Keep the same grapheme boundaries at every elapsed value. Only color changes.
        let mut fragments: Vec<_> = graphemes.iter().map(|(_, text)| (*text, 0.0)).collect();
        let elapsed = if elapsed.is_finite() { elapsed } else { 0.0 };
        let current = self.words.iter().rposition(|word| word.time <= elapsed);
        let mut byte = 0;
        for (index, word) in self.words.iter().enumerate() {
            let end_byte = byte + word.text.len();
            let start = graphemes.partition_point(|(offset, _)| *offset < byte);
            let end = graphemes.partition_point(|(offset, _)| *offset < end_byte);
            byte = end_byte;
            let progress = if current.is_some_and(|current| index < current) {
                1.0
            } else if current != Some(index) {
                0.0
            } else {
                let end_time = if word.end.is_finite() && word.end > word.time {
                    word.end
                } else if let Some(next) = self.words.get(index + 1) {
                    next.time
                } else if self.end > word.time && self.end.is_finite() {
                    self.end
                } else {
                    word.time
                };
                if end_time > word.time {
                    ((elapsed - word.time) / (end_time - word.time)).clamp(0.0, 1.0) as f32
                } else {
                    1.0
                }
            };
            let widths: Vec<_> = fragments[start..end]
                .iter()
                .map(|(text, _)| {
                    let measured = width(text);
                    if measured.is_finite() && measured > 0.0 {
                        measured
                    } else {
                        1.0
                    }
                })
                .collect();
            let mut front = progress * widths.iter().sum::<f32>();
            for ((_, intensity), width) in fragments[start..end].iter_mut().zip(widths) {
                *intensity = (front / width).clamp(0.0, 1.0);
                front -= width;
            }
        }
        fragments
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
    #[serde(skip)]
    pub position: Option<Position>,
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
