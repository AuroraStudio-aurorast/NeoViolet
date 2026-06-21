//! Desktop lyrics overlay — transparent, borderless, always-on-top window
//! showing the currently-playing lyric line (centre-aligned).
//!
//! Marquee threshold is calculated dynamically from the window width and font
//! size so it adapts when the window is resized.

use gpui::*;
use std::sync::{Arc, Mutex};

use crate::ipc::LyricLineData;
use crate::state::{AppState, LyricsState};
use crate::util::hex_to_hsla;

// ── Constants ──

const MARQUEE_SPEED: f32 = 10.0;
const MARQUEE_MARGIN: usize = 6;
const H_PADDING: f32 = 24.0;
const SHADOW_OFFSET: f32 = 1.5;
/// Avg char-width / font-size ratio for width estimation.
const CHAR_WIDTH_RATIO: f32 = 0.55;

// ── CJK width ──

fn char_width(c: char) -> usize {
    match c {
        '\u{1100}'..='\u{11FF}'
        | '\u{2E80}'..='\u{2EFF}'
        | '\u{3000}'..='\u{303F}'
        | '\u{3040}'..='\u{309F}'
        | '\u{30A0}'..='\u{30FF}'
        | '\u{3100}'..='\u{312F}'
        | '\u{3130}'..='\u{318F}'
        | '\u{31C0}'..='\u{31EF}'
        | '\u{31F0}'..='\u{31FF}'
        | '\u{3200}'..='\u{32FF}'
        | '\u{3300}'..='\u{33FF}'
        | '\u{3400}'..='\u{4DBF}'
        | '\u{4E00}'..='\u{9FFF}'
        | '\u{A000}'..='\u{A4CF}'
        | '\u{AC00}'..='\u{D7AF}'
        | '\u{F900}'..='\u{FAFF}'
        | '\u{FE10}'..='\u{FE1F}'
        | '\u{FE30}'..='\u{FE4F}'
        | '\u{FF01}'..='\u{FF60}'
        | '\u{FFE0}'..='\u{FFE6}'
        | '\u{1F200}'..='\u{1F2FF}'
        | '\u{1F600}'..='\u{1F64F}'
        | '\u{1F680}'..='\u{1F6FF}'
        | '\u{1F900}'..='\u{1F9FF}'
        | '\u{20000}'..='\u{2A6DF}'
        | '\u{2A700}'..='\u{2B73F}'
        | '\u{2B740}'..='\u{2B81F}'
        | '\u{2F800}'..='\u{2FA1F}'
        => 2,
        _ => 1,
    }
}

fn display_width(text: &str) -> usize {
    text.chars().map(char_width).sum()
}

// ── Entity ──

pub struct DesktopLyricsView {
    lyrics_state: Arc<Mutex<LyricsState>>,
    enabled_flag: Arc<Mutex<bool>>,
    focus_handle: FocusHandle,
    font_family: SharedString,
    font_size: u32,
    opacity: f32,
    show_song_info: bool,
    text_color: Hsla,
    highlight_color: Hsla,
    scroll_offset: f32,
    last_active_text: String,
    last_line_count: usize,
}

impl DesktopLyricsView {
    pub fn new(cx: &mut Context<Self>) -> Self {
        let cfg = {
            let state = cx.global::<AppState>();
            let c = state.config.desktop_lyrics.clone();
            (
                state.lyrics_state.clone(),
                state.desktop_lyrics_enabled.clone(),
                SharedString::from(c.font_family),
                c.font_size,
                c.opacity,
                c.show_song_info,
                hex_to_hsla(&c.text_color),
                hex_to_hsla(&c.highlight_color),
            )
        };
        let (lyrics_state, enabled_flag, font_family, font_size, opacity, show_song_info, text_color, highlight_color) = cfg;

        // Timer: close-check + marquee (~30 Hz).
        let poll_state = lyrics_state.clone();
        let poll_enabled = enabled_flag.clone();
        let close_handle = cx.global::<AppState>().lyrics_window_handle.clone();

        cx.spawn(async move |this, cx| {
            let tick = std::time::Duration::from_millis(33);
            loop {
                cx.background_executor().timer(tick).await;

                if poll_enabled.lock().map(|g| !*g).unwrap_or(false) {
                    log::info!("[desktop-lyrics] closing");
                    if let Ok(mut guard) = close_handle.lock()
                        && let Some(handle) = guard.take()
                    {
                        let _ = cx.update_window(handle, |_view, window, _app| {
                            Self::save_window_position(window);
                            window.remove_window();
                        });
                    }
                    return;
                }

                let _ = this.update(cx, |this, cx| {
                    let (elapsed, lines) = {
                        let guard = poll_state.lock().unwrap();
                        (guard.elapsed, guard.lines.clone())
                    };

                    // Lyric-source change (e.g. :lrc switch, new song).
                    let source_changed = lines.len() != this.last_line_count;
                    if source_changed {
                        this.last_line_count = lines.len();
                        this.scroll_offset = 0.0;
                        this.last_active_text.clear();
                    }

                    // Content change within same source.
                    let active = active_text(&lines, elapsed);
                    if active != this.last_active_text {
                        this.scroll_offset = 0.0;
                        this.last_active_text = active;
                    }

                    let notify = !this.last_active_text.is_empty() || source_changed;
                    if !this.last_active_text.is_empty() {
                        this.scroll_offset += MARQUEE_SPEED * tick.as_secs_f32();
                    }
                    if notify {
                        cx.notify();
                    }
                });
            }
        })
        .detach();

        Self {
            lyrics_state, enabled_flag,
            focus_handle: cx.focus_handle(),
            font_family, font_size, opacity, show_song_info,
            text_color, highlight_color,
            scroll_offset: 0.0,
            last_active_text: String::new(),
            last_line_count: 0,
        }
    }

    fn save_window_position(window: &mut Window) {
        let bounds = window.bounds();
        let path = crate::config::config_dir_path().join("neoviolet_gui.toml");
        let Ok(content) = std::fs::read_to_string(&path) else { return };
        let Ok(mut cfg) = toml::from_str::<crate::config::GuiConfig>(&content) else { return };
        cfg.desktop_lyrics.position_x = Some(f32::from(bounds.origin.x) as i32);
        cfg.desktop_lyrics.position_y = Some(f32::from(bounds.origin.y) as i32);
        if let Ok(out) = toml::to_string_pretty(&cfg) {
            let _ = std::fs::write(&path, out);
        }
    }

    /// How many ASCII-char units fit in the window.
    fn visible_chars(&self, window: &Window) -> usize {
        let w = f32::from(window.bounds().size.width);
        let avail = (w - H_PADDING * 2.0).max(100.0);
        (avail / (self.font_size as f32 * CHAR_WIDTH_RATIO)) as usize
    }
}

// ── Render ──

impl Render for DesktopLyricsView {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !*self.enabled_flag.lock().unwrap() {
            return div().into_any_element();
        }

        let (title, artist, elapsed, lines) = {
            let g = self.lyrics_state.lock().unwrap();
            (g.title.clone(), g.artist.clone(), g.elapsed, g.lines.clone())
        };

        let font = self.font_family.clone();
        let text_c = self.text_color;
        let highlight_c = self.highlight_color;
        let size = self.font_size as f32;
        let shadow_c = hsla(0.0, 0.0, 0.0, 0.55);
        let max_chars = self.visible_chars(window);

        // Song info line.
        let song_info = if self.show_song_info && !title.is_empty() {
            let artist_part = if artist.is_empty() || artist == "Unknown Artist" {
                String::new()
            } else {
                format!("  \u{2014}  {}", artist)
            };
            format!("\u{266A}  {}{}", title, artist_part)
        } else {
            String::new()
        };

        let active_lines = find_active_lines(&lines, elapsed);
        let mut children: Vec<AnyElement> = Vec::new();

        if !song_info.is_empty() {
            children.push(
                centered()
                    .child(shadow(
                        &song_info, font.clone(), (size * 0.65).max(12.0),
                        text_c, shadow_c, FontWeight::NORMAL, 0.7,
                    ))
                    .into_any_element(),
            );
        }

        if !active_lines.is_empty() {
            let mut parts: Vec<AnyElement> = Vec::new();
            for (idx, text) in active_lines.iter().enumerate() {
                let primary = idx == 0;
                let color = if primary { highlight_c } else { text_c };
                let sz = if primary { size } else { (size * 0.72).max(13.0) };
                let w = if primary { FontWeight::BOLD } else { FontWeight::NORMAL };
                let o: f32 = if primary { 1.0 } else { 0.7 };

                let display = if primary && display_width(text) > max_chars {
                    marquee_text(text, self.scroll_offset, max_chars)
                } else {
                    text.clone()
                };

                parts.push(
                    shadow(&display, font.clone(), sz, color, shadow_c, w, o)
                        .into_any_element(),
                );
            }
            children.push(centered().children(parts).into_any_element());
        } else {
            children.push(
                div().flex().flex_row().justify_center()
                    .child(shadow(
                        "\u{2014}", font.clone(), size * 0.7,
                        text_c, shadow_c, FontWeight::NORMAL, 0.35,
                    ))
                    .into_any_element(),
            );
        }

        div()
            .size_full()
            .opacity(self.opacity)
            .flex().flex_col().items_center().justify_center()
            .px(px(H_PADDING))
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(
                |_this, _ev: &MouseDownEvent, window, _cx| window.start_window_move(),
            ))
            .on_mouse_down(MouseButton::Right, cx.listener(
                |this, _ev: &MouseDownEvent, _window, cx| {
                    log::info!("[desktop-lyrics] right-click: closing");
                    *this.enabled_flag.lock().unwrap() = false;
                    let ipc = cx.global::<AppState>().ipc.clone();
                    let _ = ipc.send(&crate::ipc::IpcMessage::enable_desktop_lyrics(false));
                    cx.notify();
                },
            ))
            .children(children)
            .into_any_element()
    }
}

impl Focusable for DesktopLyricsView {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

// ── Widget helpers ──

fn centered() -> Div {
    div().flex().flex_col().items_center()
}

fn shadow(
    text: &str,
    font: SharedString,
    size_px: f32,
    color: Hsla,
    shadow_c: Hsla,
    weight: FontWeight,
    opacity: f32,
) -> impl IntoElement {
    let t = text.to_string();
    div().relative()
        .child(
            div().absolute()
                .left(px(SHADOW_OFFSET)).top(px(SHADOW_OFFSET))
                .text_size(px(size_px)).font_family(font.clone())
                .font_weight(weight).text_color(shadow_c)
                .overflow_x_hidden().whitespace_nowrap()
                .child(t.clone()),
        )
        .child(
            div()
                .text_size(px(size_px)).font_family(font)
                .font_weight(weight).text_color(color)
                .opacity(opacity)
                .overflow_x_hidden().whitespace_nowrap()
                .child(t),
        )
}

// ── Marquee ──

fn marquee_text(text: &str, scroll: f32, max_chars: usize) -> String {
    let chars: Vec<char> = text.chars().collect();
    if display_width(text) <= max_chars {
        return text.to_string();
    }
    let cycle_len = chars.len() + MARQUEE_MARGIN;
    let offset = (scroll as usize) % cycle_len;
    let mut result = String::with_capacity(max_chars + MARQUEE_MARGIN);
    let mut w = 0usize;
    for i in 0..cycle_len {
        let idx = (offset + i) % cycle_len;
        if idx < chars.len() {
            let c = chars[idx];
            let cw = char_width(c);
            if w + cw > max_chars + MARQUEE_MARGIN {
                break;
            }
            result.push(c);
            w += cw;
        } else {
            result.push(' ');
            w += 1;
        }
    }
    result
}

// ── Active-line detection ──

fn find_active_lines(lines: &[LyricLineData], elapsed: f64) -> Vec<String> {
    if lines.is_empty() {
        return vec![];
    }
    let elapsed_ms = (elapsed * 1000.0) as u64;
    let any_bounded = lines.iter().any(|l| l.end > 0.0);

    let raw: Vec<String> = if any_bounded {
        lines.iter()
            .filter(|l| {
                l.end > 0.0
                    && (l.time * 1000.0) as u64 <= elapsed_ms
                    && elapsed_ms < (l.end * 1000.0) as u64
            })
            .map(|l| l.text.clone())
            .collect()
    } else {
        lines.iter()
            .rfind(|l| (l.time * 1000.0) as u64 <= elapsed_ms)
            .map(|l| l.text.clone())
            .into_iter()
            .collect()
    };

    // Split LRC-merged " | " lines so each language is its own sub-line.
    let mut out = Vec::new();
    for text in raw {
        if text.contains(" | ") {
            for part in text.split(" | ") {
                out.push(part.to_string());
            }
        } else {
            out.push(text);
        }
    }
    out
}

fn active_text(lines: &[LyricLineData], elapsed: f64) -> String {
    find_active_lines(lines, elapsed).join("\u{1F}")
}
