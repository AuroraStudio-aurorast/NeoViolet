//! Desktop lyrics overlay — transparent, borderless, always-on-top window
//! showing the currently-playing lyric line (centre-aligned).
//!
//! Marquee threshold is calculated dynamically from the window width and font
//! size so it adapts when the window is resized.

use gpui::*;
use std::sync::{Arc, Mutex};

use crate::ipc::{IpcClient, LyricLineData, WordData, split_line};
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
        | '\u{2F800}'..='\u{2FA1F}' => 2,
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
    ipc_client: Arc<IpcClient>,
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
    /// Cached window bounds for position save on app quit (on_release).
    last_bounds: Option<Bounds<Pixels>>,
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
        let (
            lyrics_state,
            enabled_flag,
            font_family,
            font_size,
            opacity,
            show_song_info,
            text_color,
            highlight_color,
        ) = cfg;
        let ipc_client = cx.global::<AppState>().ipc.clone();

        // Save position on app quit (entity released = window destroyed).
        cx.on_release(|this, app| {
            if let Some(bounds) = this.last_bounds {
                let x = f32::from(bounds.origin.x) as i32;
                let y = f32::from(bounds.origin.y) as i32;
                Self::persist_lyrics_position(x, y, app);
            }
        })
        .detach();
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
                            Self::save_window_position(window, _app);
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
            lyrics_state,
            enabled_flag,
            ipc_client,
            focus_handle: cx.focus_handle(),
            font_family,
            font_size,
            opacity,
            show_song_info,
            text_color,
            highlight_color,
            scroll_offset: 0.0,
            last_active_text: String::new(),
            last_line_count: 0,
            last_bounds: None,
        }
    }

    /// Persist window position to the TOML config file AND update the
    /// in-memory AppState so the next window open uses the saved position.
    fn save_window_position(window: &mut Window, cx: &mut App) {
        let bounds = window.bounds();
        let x = f32::from(bounds.origin.x) as i32;
        let y = f32::from(bounds.origin.y) as i32;
        Self::persist_lyrics_position(x, y, cx);
    }

    /// Persist lyrics window position (x, y) to config and TOML on disk.
    fn persist_lyrics_position(x: i32, y: i32, cx: &mut App) {
        let state = cx.global_mut::<AppState>();
        state.config.desktop_lyrics.position_x = Some(x);
        state.config.desktop_lyrics.position_y = Some(y);

        let path = crate::config::config_dir_path().join("neoviolet_gui.toml");
        let Ok(content) = std::fs::read_to_string(&path) else {
            return;
        };
        let Ok(mut cfg) = toml::from_str::<crate::config::GuiConfig>(&content) else {
            return;
        };
        cfg.desktop_lyrics.position_x = Some(x);
        cfg.desktop_lyrics.position_y = Some(y);
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

        // Cache bounds for position save on app quit.
        self.last_bounds = Some(window.bounds());

        let (title, artist, elapsed, lines) = {
            let g = self.lyrics_state.lock().unwrap();
            (
                g.title.clone(),
                g.artist.clone(),
                g.elapsed,
                g.lines.clone(),
            )
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
                        &song_info,
                        font.clone(),
                        (size * 0.65).max(12.0),
                        text_c,
                        shadow_c,
                        FontWeight::NORMAL,
                        0.7,
                    ))
                    .into_any_element(),
            );
        }

        if !active_lines.is_empty() {
            let mut parts: Vec<AnyElement> = Vec::new();
            for (idx, item) in active_lines.iter().enumerate() {
                let primary = idx == 0;
                let color = if primary { highlight_c } else { text_c };
                let sz = if primary {
                    size
                } else {
                    (size * 0.72).max(13.0)
                };
                let w = if primary {
                    FontWeight::BOLD
                } else {
                    FontWeight::NORMAL
                };
                let o: f32 = if primary { 1.0 } else { 0.7 };

                let display = if item.label.is_empty() {
                    item.text.clone()
                } else {
                    format!("{}{}", item.label, item.text)
                };

                // The label is sung from the start, so the split counts it: the
                // karaoke boundary must fall after it, never inside it.
                let split = match sung_prefix(item, elapsed) {
                    Some(n) => item.label.chars().count() + n,
                    None => display.chars().count(),
                };
                let pieces = marquee_pieces(&display, split, self.scroll_offset, max_chars);

                if pieces.len() == 1 {
                    let (is_sung, text) = &pieces[0];
                    let piece_c = if *is_sung { color } else { text_c };
                    parts.push(
                        shadow(text, font.clone(), sz, piece_c, shadow_c, w, o).into_any_element(),
                    );
                } else {
                    // gpui colours a text run as a whole, so each piece is its own
                    // element in one row: the boundary lands mid-line.
                    parts.push(
                        div()
                            .flex()
                            .flex_row()
                            .children(pieces.iter().map(|(is_sung, text)| {
                                let piece_c = if *is_sung { color } else { text_c };
                                shadow(text, font.clone(), sz, piece_c, shadow_c, w, o)
                            }))
                            .into_any_element(),
                    );
                }
            }
            children.push(centered().children(parts).into_any_element());
        } else {
            children.push(
                div()
                    .flex()
                    .flex_row()
                    .justify_center()
                    .child(shadow(
                        "\u{2014}",
                        font.clone(),
                        size * 0.7,
                        text_c,
                        shadow_c,
                        FontWeight::NORMAL,
                        0.35,
                    ))
                    .into_any_element(),
            );
        }

        div()
            .size_full()
            .opacity(self.opacity)
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .px(px(H_PADDING))
            .track_focus(&self.focus_handle)
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, window, cx| {
                if event.keystroke.key.eq_ignore_ascii_case("space") {
                    log::debug!("[desktop-lyrics] space pressed → play/pause");
                    let _ = this.ipc_client.send(&crate::ipc::IpcMessage::play_pause());
                    window.prevent_default();
                    cx.stop_propagation();
                }
            }))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(|_this, _ev: &MouseDownEvent, window, _cx| window.start_window_move()),
            )
            .on_mouse_down(
                MouseButton::Right,
                cx.listener(|this, _ev: &MouseDownEvent, _window, cx| {
                    log::info!("[desktop-lyrics] right-click: closing");
                    *this.enabled_flag.lock().unwrap() = false;
                    let ipc = cx.global::<AppState>().ipc.clone();
                    let _ = ipc.send(&crate::ipc::IpcMessage::enable_desktop_lyrics(false));
                    cx.notify();
                }),
            )
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
    div()
        .relative()
        .child(
            div()
                .absolute()
                .left(px(SHADOW_OFFSET))
                .top(px(SHADOW_OFFSET))
                .text_size(px(size_px))
                .font_family(font.clone())
                .font_weight(weight)
                .text_color(shadow_c)
                .overflow_x_hidden()
                .whitespace_nowrap()
                .child(t.clone()),
        )
        .child(
            div()
                .text_size(px(size_px))
                .font_family(font)
                .font_weight(weight)
                .text_color(color)
                .opacity(opacity)
                .overflow_x_hidden()
                .whitespace_nowrap()
                .child(t),
        )
}

// ── Marquee ──

/// The marquee window as (sung, text) pieces in drawing order: `sung_chars` counts
/// the leading characters of `text` that are already sung, so the boundary travels
/// with the text instead of sitting at a fixed screen position.
///
/// The window cycles, so a wrapped one can carry the sung head around to its tail
/// again and hold an unsung run on either side of it: the pieces are returned in
/// drawing order rather than as a single split. A line that fits never scrolls and
/// yields at most two.
fn marquee_pieces(
    text: &str,
    sung_chars: usize,
    scroll: f32,
    max_chars: usize,
) -> Vec<(bool, String)> {
    let chars: Vec<char> = text.chars().collect();
    let sung = sung_chars.min(chars.len());
    let mut pieces: Vec<(bool, String)> = Vec::new();
    if display_width(text) <= max_chars {
        let split = chars.iter().take(sung).map(|c| c.len_utf8()).sum();
        if split > 0 {
            pieces.push((true, text[..split].to_string()));
        }
        if split < text.len() {
            pieces.push((false, text[split..].to_string()));
        }
        return pieces;
    }
    let cycle_len = chars.len() + MARQUEE_MARGIN;
    let offset = (scroll as usize) % cycle_len;
    let mut w = 0usize;
    for i in 0..cycle_len {
        let idx = (offset + i) % cycle_len;
        // Past the end of the text the window is padding, and only a real
        // character can end it: that keeps the window's shape as it always was.
        // Padding counts as sung once the whole line has been sung, so a finished
        // line does not flicker at its tail.
        let (c, is_sung) = if idx < chars.len() {
            let c = chars[idx];
            if w + char_width(c) > max_chars + MARQUEE_MARGIN {
                break;
            }
            (c, idx < sung)
        } else {
            (' ', sung >= chars.len())
        };
        match pieces.last_mut() {
            Some((last_sung, last)) if *last_sung == is_sung => last.push(c),
            _ => pieces.push((is_sung, c.to_string())),
        }
        w += char_width(c);
    }
    pieces
}

// ── Active-line detection ──

/// Mirrors Go's `lyrics.Data.ActiveLines`, which the TUI uses, so the panel and
/// the desktop lyrics cannot disagree about which lines are active.
///
/// A line is bounded when `end > time`: it is active for `time <= t < end`. A
/// line is unbounded when `end == 0` or `end <= time`: it is active from `time`
/// until the `time` of the next line with a greater `time` (the last line never
/// expires). A zero-length or reversed interval is treated as unbounded because
/// its `time <= t < end` window is empty — the line would be unreachable — and
/// because the `end > 0` ⇒ `end > time` invariant keeps producers from emitting
/// that shape in the first place (Go's `ttmlLine` normalises it).
///
/// This is evaluated per line. Treating "the file contains at least one bounded
/// line" as a global switch — the previous shape — made every unbounded line
/// unreachable in a mixed file, which is exactly what a SMI file looks like.
///
/// The bound is "the next line with a greater `time`" rather than "the next
/// line", so lines sharing a `time` (a SMI SYNC with several `<P Class=...>`
/// children) must not cut each other off.
fn find_active_lines<'a>(lines: &'a [LyricLineData], elapsed: f64) -> Vec<ActiveLine<'a>> {
    if lines.is_empty() {
        return vec![];
    }
    let elapsed_ms = (elapsed * 1000.0) as u64;
    let ms = |secs: f64| (secs * 1000.0) as u64;

    let mut active: Vec<&LyricLineData> = Vec::new();
    for (i, l) in lines.iter().enumerate() {
        if l.end > l.time {
            if ms(l.time) <= elapsed_ms && elapsed_ms < ms(l.end) {
                active.push(l);
            }
            continue;
        }
        if ms(l.time) > elapsed_ms {
            continue;
        }
        // The first later line with a greater time closes this one's window;
        // "greater" and not "next" so that same-time siblings survive.
        if let Some(next) = lines[i + 1..].iter().find(|n| ms(n.time) > ms(l.time))
            && elapsed_ms >= ms(next.time)
        {
            continue;
        }
        active.push(l);
    }

    // split_line expands the parser's display parts and falls back to the
    // legacy " | " split for an older TUI, so a merged line is always one
    // sub-line per language.
    //
    // The label belongs to the line and the word timings cover its first sub-line
    // alone, so only that sub-line carries either one.
    let mut out = Vec::new();
    for line in active {
        let label = line.prefix.as_deref().unwrap_or("");
        for (i, text) in split_line(line).into_iter().enumerate() {
            let first = i == 0;
            out.push(ActiveLine {
                text,
                label: if first { label } else { "" },
                words: if first { line.words.as_slice() } else { &[] },
            });
        }
    }
    out
}

/// One sub-line the overlay draws, with what covers it: the agent label belongs to
/// the line, and the word timings cover its first sub-line alone. The rest get an
/// empty slice and highlight as a whole, exactly as the TUI panel karaokes only
/// the first part of a merged line.
struct ActiveLine<'a> {
    text: String,
    label: &'a str,
    words: &'a [WordData],
}

/// Characters of a sub-line that are already sung at `elapsed`, or None when the
/// word timings do not reproduce the text exactly.
///
/// None makes the caller fall back to a whole-line highlight instead of dropping
/// or reordering characters: a set of fragments that does not concatenate back to
/// the text, or that marks a sung fragment after an unsung one, has no split that
/// is a prefix of the text. This is the guard the TUI panel applies too.
fn sung_prefix(line: &ActiveLine<'_>, elapsed: f64) -> Option<usize> {
    if line.words.is_empty() {
        return None;
    }
    let elapsed_ms = (elapsed * 1000.0) as u64;
    let mut rebuilt = String::with_capacity(line.text.len());
    let mut sung_chars = 0usize;
    let mut unsung_seen = false;
    for word in line.words {
        if (word.time * 1000.0) as u64 <= elapsed_ms {
            if unsung_seen {
                return None;
            }
            sung_chars += word.text.chars().count();
        } else {
            unsung_seen = true;
        }
        rebuilt.push_str(&word.text);
    }
    if rebuilt != line.text {
        return None;
    }
    Some(sung_chars)
}

fn active_text(lines: &[LyricLineData], elapsed: f64) -> String {
    find_active_lines(lines, elapsed)
        .iter()
        .map(|line| format!("{}{}", line.label, line.text))
        .collect::<Vec<_>>()
        .join("\u{1F}")
}

#[cfg(test)]
mod tests {
    use super::{
        ActiveLine, MARQUEE_MARGIN, char_width, display_width, find_active_lines, marquee_pieces,
        sung_prefix,
    };
    use crate::ipc::{LyricLineData, WordData};

    #[test]
    fn char_width_ascii_and_cjk() {
        assert_eq!(char_width('a'), 1);
        assert_eq!(char_width('中'), 2);
    }

    /// A plain line with no sub-parts, no label and no timings, so the tests read
    /// as data.
    fn line(time: f64, end: f64, text: &str) -> LyricLineData {
        LyricLineData {
            time,
            end,
            text: text.into(),
            parts: Vec::new(),
            words: Vec::new(),
            prefix: None,
            agent: None,
            agent_name: None,
        }
    }

    fn word(time: f64, text: &str) -> WordData {
        WordData {
            time,
            text: text.into(),
        }
    }

    /// The sub-line texts the overlay would draw, in order.
    fn texts(active: &[ActiveLine<'_>]) -> Vec<String> {
        active.iter().map(|line| line.text.clone()).collect()
    }

    /// The window the overlay would draw: the pieces in order, ignoring the
    /// karaoke split.
    fn marquee_window(text: &str, scroll: f32, max_chars: usize) -> String {
        pieces_text(&marquee_pieces(
            text,
            text.chars().count(),
            scroll,
            max_chars,
        ))
    }

    fn pieces_text(pieces: &[(bool, String)]) -> String {
        pieces.iter().map(|(_, text)| text.as_str()).collect()
    }

    #[test]
    fn display_width_counts_wide_chars() {
        assert_eq!(display_width("abc"), 3);
        assert_eq!(display_width("中文"), 4);
    }

    #[test]
    fn marquee_scroll_wraps() {
        // "hello" is wider than max_chars=3, so it is padded with the margin.
        let text = marquee_window("hello", 0.0, 3);
        assert_eq!(text, format!("hello{}", " ".repeat(MARQUEE_MARGIN)));
        // Scrolling by 1 shifts the window one character to the right.
        let next = marquee_window("hello", 1.0, 3);
        assert_eq!(next, format!("ello{}", " ".repeat(MARQUEE_MARGIN)));
        assert_ne!(text, next);
    }

    /// A line that fits never scrolls, so the window is at most two pieces and the
    /// pieces say which is which.
    #[test]
    fn marquee_pieces_splits_a_fitting_line() {
        assert_eq!(
            marquee_pieces("Stop and stare", 5, 0.0, 40),
            vec![
                (true, "Stop ".to_string()),
                (false, "and stare".to_string())
            ]
        );
        // Nothing sung yet stays one unsung piece, so the line draws in the base
        // colour rather than the highlight colour.
        assert_eq!(
            marquee_pieces("Stop and stare", 0, 0.0, 40),
            vec![(false, "Stop and stare".to_string())]
        );
        // Everything sung collapses to a single sung piece.
        assert_eq!(
            marquee_pieces("Stop and stare", 14, 0.0, 40),
            vec![(true, "Stop and stare".to_string())]
        );
    }

    /// The boundary has to travel with the text instead of sitting at a fixed
    /// screen position, or a scrolling line would highlight the wrong words.
    #[test]
    fn marquee_pieces_keeps_the_boundary_with_the_text() {
        let width = 3;
        let window = |scroll: f32| marquee_window("abcdef", scroll, width);
        let pieces = |scroll: f32| marquee_pieces("abcdef", 3, scroll, width);

        // Scroll 0 opens on the sung head.
        let at0 = pieces(0.0);
        assert_eq!(at0[0], (true, "abc".to_string()));
        assert_eq!(pieces_text(&at0), window(0.0));

        // Scroll 2 opens on 'c', so only 'c' is still sung.
        let at2 = pieces(2.0);
        assert_eq!(at2[0], (true, "c".to_string()));
        assert_eq!(pieces_text(&at2), window(2.0));

        // Scrolled further the window wraps, so the sung head comes around at the
        // tail and the unsung run sits on both sides of it rather than trailing:
        // one split would draw the halves in the wrong order.
        let at4 = pieces(4.0);
        assert_eq!(at4[0], (false, "ef      ".to_string()));
        assert_eq!(at4.last().unwrap(), &(true, "a".to_string()));
        assert_eq!(pieces_text(&at4), window(4.0));
    }

    /// The overlay draws the sung half in the highlight colour and the rest in the
    /// base colour, so the split has to be a prefix of the drawn text and advance
    /// with elapsed.
    #[test]
    fn sung_prefix_advances_with_elapsed() {
        let mut lyric = line(5.0, 6.5, "Stop and stare");
        lyric.words = vec![word(5.0, "Stop "), word(5.5, "and "), word(6.0, "stare")];
        let lines = vec![lyric];

        let at = |elapsed: f64| {
            let active = find_active_lines(&lines, elapsed);
            let split = sung_prefix(&active[0], elapsed).unwrap_or(active[0].text.chars().count());
            marquee_pieces(&active[0].text, split, 0.0, 40)
                .iter()
                .map(|(is_sung, text)| match is_sung {
                    true => format!("[{}]", text),
                    false => text.clone(),
                })
                .collect::<Vec<_>>()
                .join("")
        };

        assert_eq!(at(5.0), "[Stop ]and stare");
        assert_eq!(at(5.4), "[Stop ]and stare");
        assert_eq!(at(5.5), "[Stop and ]stare");
        assert_eq!(at(6.0), "[Stop and stare]");
    }

    /// Timings that do not reproduce the text - a translation suffix, a stray
    /// fragment, a timeline that runs backwards - have no split that is a prefix
    /// of the text, so the caller falls back to a whole-line highlight instead of
    /// dropping or reordering characters.
    #[test]
    fn sung_prefix_falls_back_when_timings_do_not_tile() {
        let mut partial = line(0.0, 0.0, "hello world");
        partial.words = vec![word(0.0, "hello")];
        let partial_lines = vec![partial];
        assert_eq!(
            sung_prefix(&find_active_lines(&partial_lines, 0.0)[0], 0.0),
            None
        );

        let mut backwards = line(0.0, 0.0, "ab");
        // 'a' at 1s is unsung at 0s, so 'b' at 0s would be a sung fragment after
        // an unsung one: the sung part would no longer be a prefix.
        backwards.words = vec![word(1.0, "a"), word(0.0, "b")];
        let backwards_lines = vec![backwards];
        assert_eq!(
            sung_prefix(&find_active_lines(&backwards_lines, 0.0)[0], 0.0),
            None
        );

        // And a line without timings is never split.
        let plain_lines = vec![line(0.0, 0.0, "hello")];
        assert_eq!(
            sung_prefix(&find_active_lines(&plain_lines, 0.0)[0], 0.0),
            None
        );
    }

    #[test]
    fn find_active_lines_picks_elapsed() {
        let lines = vec![
            line(0.0, 0.0, "one"),
            line(5.0, 0.0, "two"),
            line(10.0, 0.0, "three"),
        ];
        let active = texts(&find_active_lines(&lines, 6.0));
        assert!(active.iter().any(|t| t.contains("two")));
        assert!(!active.iter().any(|t| t.contains("three")));
    }

    #[test]
    fn find_active_lines_uses_parts() {
        let mut merged = line(5.0, 0.0, "hello | 你好");
        merged.parts = vec!["hello".to_string(), "你好".to_string()];
        let lines = vec![merged];
        assert_eq!(
            texts(&find_active_lines(&lines, 6.0)),
            vec!["hello", "你好"]
        );
    }

    /// The label belongs to the line and the timings cover its first sub-line, so
    /// a translation sub-line repeats neither of them.
    #[test]
    fn find_active_lines_carries_label_and_words_on_the_first_sub_line_only() {
        let mut merged = line(5.0, 0.0, "hello | 你好");
        merged.parts = vec!["hello".to_string(), "你好".to_string()];
        merged.prefix = Some("Taylor Swift: ".to_string());
        merged.words = vec![word(5.0, "hel"), word(6.0, "lo")];
        let lines = vec![merged];

        let active = find_active_lines(&lines, 5.5);
        assert_eq!(texts(&active), vec!["hello", "你好"]);
        assert_eq!(active[0].label, "Taylor Swift: ");
        assert_eq!(active[0].words.len(), 2);
        assert_eq!(active[1].label, "");
        assert!(active[1].words.is_empty());
    }

    #[test]
    fn find_active_lines_smi_shape_reaches_the_unbounded_last_line() {
        // 3 行有界 + 末行 end == 0（SMI 的形状）。旧的 any_bounded
        // 全局开关让末行永远进不了候选，桌面歌词因此空白。
        let mut lines: Vec<LyricLineData> = (0..3)
            .map(|i| line(i as f64, i as f64 + 1.0, &format!("bounded{i}")))
            .collect();
        lines.push(line(3.0, 0.0, "last"));

        let at_one = texts(&find_active_lines(&lines, 0.5));
        assert_eq!(at_one, vec!["bounded0"]);

        let at_last = texts(&find_active_lines(&lines, 3.0));
        assert_eq!(at_last, vec!["last"]);

        // 末行之后也不过期。
        assert_eq!(texts(&find_active_lines(&lines, 60.0)), vec!["last"]);
    }

    #[test]
    fn find_active_lines_same_time_siblings_both_return() {
        // 同一 time 的无界兄弟（SMI 一个 SYNC 下的双语 <P>）必须同时 active，
        // 因此无界行的右边界取“下一个 time **更大**的行”，而不是“下一行”。
        let lines = vec![
            line(1.0, 0.0, "v1"),
            line(1.0, 0.0, "v2"),
            line(3.0, 0.0, "next"),
        ];
        assert_eq!(texts(&find_active_lines(&lines, 1.0)), vec!["v1", "v2"]);
        assert_eq!(texts(&find_active_lines(&lines, 2.0)), vec!["v1", "v2"]);
        assert_eq!(texts(&find_active_lines(&lines, 3.0)), vec!["next"]);
    }

    #[test]
    fn find_active_lines_bounded_siblings_also_both_return() {
        // 有界路径下的同 time 不互相截断（回归钉，旧实现即绿）。
        let lines = vec![line(1.0, 2.0, "b1"), line(1.0, 2.0, "b2")];
        assert_eq!(texts(&find_active_lines(&lines, 1.0)), vec!["b1", "b2"]);
        assert_eq!(texts(&find_active_lines(&lines, 1.5)), vec!["b1", "b2"]);
        assert_eq!(texts(&find_active_lines(&lines, 2.0)), Vec::<String>::new());
    }

    #[test]
    fn find_active_lines_all_unbounded_keeps_a_single_line() {
        // 无回归：全 end == 0 的文件（今天的 LRC 形状）行为不变。
        let lines = vec![
            line(0.0, 0.0, "one"),
            line(5.0, 0.0, "two"),
            line(10.0, 0.0, "three"),
        ];
        assert_eq!(texts(&find_active_lines(&lines, 6.0)), vec!["two"]);
        assert_eq!(texts(&find_active_lines(&lines, 0.0)), vec!["one"]);
    }

    #[test]
    fn find_active_lines_bounded_gap_stays_empty() {
        // 无回归：全有界文件的句间留白仍然是留白。
        let lines = vec![line(0.0, 1.0, "first"), line(3.0, 4.0, "second")];
        assert_eq!(texts(&find_active_lines(&lines, 2.0)), Vec::<String>::new());
        assert_eq!(texts(&find_active_lines(&lines, 3.5)), vec!["second"]);
    }

    /// Mirrors Go's `TestActiveLines_ZeroLengthLineIsReachable`: a line whose
    /// `end == time` carries no usable duration. Under the old "bounded when
    /// `end > 0`" rule its window was empty, so it was unreachable forever (the
    /// corpus hit: an AMLL TTML `<p>` with `begin == end`). `end <= time` is now
    /// unbounded: reachable from `time` until the next greater `time`.
    #[test]
    fn find_active_lines_zero_length_line_is_reachable() {
        let with_next = vec![line(10.0, 10.0, "啊"), line(20.0, 0.0, "next")];
        assert_eq!(texts(&find_active_lines(&with_next, 10.0)), vec!["啊"]);
        assert_eq!(texts(&find_active_lines(&with_next, 10.5)), vec!["啊"]);
        assert_eq!(texts(&find_active_lines(&with_next, 20.0)), vec!["next"]);

        // 末行零长线不过期（无界语义），且它之前不可达。
        let lone = vec![line(10.0, 10.0, "啊")];
        assert_eq!(
            texts(&find_active_lines(&lone, 9.999)),
            Vec::<String>::new()
        );
        assert_eq!(texts(&find_active_lines(&lone, 10.0)), vec!["啊"]);
        assert_eq!(texts(&find_active_lines(&lone, 60.0)), vec!["啊"]);
    }
}
