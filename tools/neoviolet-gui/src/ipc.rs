//! Bidirectional IPC client for communicating with the NeoViolet TUI process
//! via TCP localhost with shared-secret authentication and JSON messages.
//!
//! The TUI writes "<addr>\n<token>" to a temp file. The GUI reads both,
//! connects to the address, and sends the token as its first line.
//! All subsequent messages are newline-delimited JSON objects.
//!
//! GUI → TUI:  {"type":"open","path":"..."}
//!             {"type":"desktop_lyrics","enable":true|false}
//!             {"type":"play_pause"}
//! TUI → GUI:  {"type":"quit","dialog":true|false}
//!             {"type":"lyrics","lines":[...],"elapsed":12.3,"title":"...","artist":"..."}
//!
//! A line in the lyrics payload renders as its `prefix` followed by its display
//! sub-lines (`parts` when present, otherwise `text`). `words` holds the timings
//! of the first sub-line so the overlay can highlight it as it is sung, and is
//! absent for a format without word timings.

use serde::{Deserialize, Serialize};
use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::sync::{Arc, Mutex};
use std::time::Duration;

/// One timed fragment of a line's first display sub-line. The fragments
/// concatenate back to that sub-line exactly, so a karaoke renderer can split it
/// at any elapsed value without losing or reordering characters: a fragment is
/// sung once its `time` is not after `elapsed`.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct WordData {
    pub time: f64,
    pub text: String,
}

/// A single lyric line received from the TUI via IPC.
///
/// A line renders as `prefix` followed by its display sub-lines: `parts` when it
/// has any, otherwise `text` alone.
#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct LyricLineData {
    pub time: f64,
    /// End time in seconds; 0.0 = unbounded (legacy LRC/QRC/YRC/ESLRC).
    #[serde(default)]
    pub end: f64,
    /// The line's own text. The agent label is not part of it; the TUI sends that
    /// separately as `prefix`.
    pub text: String,
    /// Display sub-lines of an event that carries several lines of text (LRC
    /// merges same-timestamp entries, a SRT cue can have several lines). Empty
    /// for a plain line and for payloads from a TUI older than this field.
    #[serde(default)]
    pub parts: Vec<String>,
    /// Word timings of the first display sub-line, the only sub-line they cover.
    /// Empty for a format without word timings and for payloads from a TUI older
    /// than this field, in which case the line highlights as a whole.
    #[serde(default)]
    pub words: Vec<WordData>,
    /// Agent label to draw ahead of the first display sub-line, already resolved
    /// by the TUI for the show-it-where-the-singer-changes rule. None means no
    /// label is due on this line.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub prefix: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent_name: Option<String>,
}

/// Sub-lines of a lyric line, in display order. The parser now marks them
/// explicitly, so prefer `parts` and only fall back to splitting the merged
/// " | " text: that keeps this GUI working against an older TUI binary.
pub fn split_line(line: &LyricLineData) -> Vec<String> {
    if !line.parts.is_empty() {
        return line.parts.clone();
    }
    if line.text.contains(" | ") {
        return line.text.split(" | ").map(str::to_string).collect();
    }
    vec![line.text.clone()]
}

/// JSON message exchanged between GUI and TUI.
#[derive(Serialize, Deserialize, Debug, Clone, Default)]
pub struct IpcMessage {
    #[serde(rename = "type")]
    pub msg_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dialog: Option<bool>,

    // desktop_lyrics: enable/disable streaming
    #[serde(skip_serializing_if = "Option::is_none")]
    pub enable: Option<bool>,

    // lyrics: streaming payload from TUI to GUI
    #[serde(skip_serializing_if = "Option::is_none")]
    pub lines: Option<Vec<LyricLineData>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub elapsed: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub artist: Option<String>,
}

impl IpcMessage {
    pub fn open(path: &str) -> Self {
        Self {
            msg_type: "open".into(),
            path: Some(path.to_string()),
            ..Default::default()
        }
    }

    /// Send a desktop_lyrics enable/disable command to the TUI.
    pub fn enable_desktop_lyrics(enabled: bool) -> Self {
        Self {
            msg_type: "desktop_lyrics".into(),
            enable: Some(enabled),
            ..Default::default()
        }
    }

    /// Send a play/pause toggle command to the TUI.
    pub fn play_pause() -> Self {
        Self {
            msg_type: "play_pause".into(),
            ..Default::default()
        }
    }
}

/// Wraps an authenticated TcpStream connection.
#[derive(Clone)]
pub struct IpcClient {
    stream: Arc<Mutex<Option<TcpStream>>>,
}

impl IpcClient {
    pub fn new() -> Self {
        Self {
            stream: Arc::new(Mutex::new(None)),
        }
    }

    /// Connect to the TUI's IPC endpoint with token authentication.
    pub fn connect(&self, pid: u32) -> Result<(), String> {
        let port_path = port_file_path(pid);
        let deadline = std::time::Instant::now() + Duration::from_secs(5);

        let (addr, token) = loop {
            if let Ok(s) = std::fs::read_to_string(&port_path) {
                let mut lines = s.lines();
                let addr = lines.next().unwrap_or("").trim().to_string();
                let token = lines.next().unwrap_or("").trim().to_string();
                if !addr.is_empty() && !token.is_empty() {
                    break (addr, token);
                }
            }
            if std::time::Instant::now() > deadline {
                return Err(format!("IPC port file not ready after 5 s: {}", port_path));
            }
            std::thread::sleep(Duration::from_millis(100));
        };

        loop {
            match TcpStream::connect(&addr) {
                Ok(mut stream) => {
                    if let Err(e) = stream.write_all(format!("{}\n", token).as_bytes()) {
                        return Err(format!("IPC auth write error: {}", e));
                    }
                    stream
                        .set_read_timeout(Some(Duration::from_millis(100)))
                        .ok();
                    log::info!("[ipc] connected and authenticated to {}", addr);
                    *self.stream.lock().unwrap() = Some(stream);
                    return Ok(());
                }
                Err(e) => {
                    if std::time::Instant::now() > deadline {
                        return Err(format!("IPC connect to {} timeout: {}", addr, e));
                    }
                    std::thread::sleep(Duration::from_millis(200));
                }
            }
        }
    }

    /// Send a JSON message to the TUI.
    pub fn send(&self, msg: &IpcMessage) -> Result<(), String> {
        let mut guard = self.stream.lock().unwrap();
        let Some(ref mut stream) = *guard else {
            return Err("IPC not connected".into());
        };
        let json = serde_json::to_string(msg).map_err(|e| format!("IPC serialize: {}", e))?;
        stream
            .write_all(format!("{}\n", json).as_bytes())
            .map_err(|e| format!("IPC write error: {}", e))?;
        Ok(())
    }

    /// Convenience: send an "open" message for the given file path.
    pub fn send_open(&self, path: &str) -> Result<(), String> {
        self.send(&IpcMessage::open(path))
    }

    /// Start a background reader thread that pushes incoming JSON lines
    /// (as raw strings) into the given queue for the render loop to parse.
    pub fn start_reader(&self, queue: Arc<Mutex<Vec<String>>>) {
        let stream_opt = {
            let guard = self.stream.lock().unwrap();
            guard.as_ref().and_then(|s| s.try_clone().ok())
        };
        let Some(reader_stream) = stream_opt else {
            log::warn!("[ipc] start_reader: not connected");
            return;
        };
        reader_stream
            .set_read_timeout(Some(std::time::Duration::from_millis(500)))
            .ok();

        std::thread::spawn(move || {
            let mut buf = BufReader::new(reader_stream);
            loop {
                let mut line = String::new();
                match buf.read_line(&mut line) {
                    Ok(0) => {
                        log::info!("[ipc] reader: connection closed");
                        break;
                    }
                    Ok(_) => {
                        let msg = line.trim_end().to_string();
                        if !msg.is_empty() {
                            log::debug!("[ipc] received: {}", msg);
                            if let Ok(mut guard) = queue.lock() {
                                guard.push(msg);
                            }
                        }
                    }
                    Err(e)
                        if e.kind() == std::io::ErrorKind::WouldBlock
                            || e.kind() == std::io::ErrorKind::TimedOut => {}
                    Err(e) => {
                        log::warn!("[ipc] reader error: {}", e);
                        break;
                    }
                }
            }
        });
    }
}

fn port_file_path(pid: u32) -> String {
    let dir = std::env::temp_dir();
    let path = dir.join(format!("neoviolet-ipc-{}", pid));
    path.to_string_lossy().to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The wire shape the TUI emits: the label travels in `prefix` and the line
    /// text carries none, while `words` tiles the text exactly. A renamed field
    /// would fall back to the serde default and silently drop the karaoke data, so
    /// this is pinned against a payload captured from the TUI.
    #[test]
    fn parses_a_tui_lyrics_payload() {
        let raw = r#"{"time":39.345,"end":43.071,
            "text":"I could never find the right way to tell you",
            "words":[{"time":39.345,"text":"I"},{"time":39.548,"text":" "},
                     {"time":39.548,"text":"could"}],
            "prefix":"Taylor Swift: ","agent":"v1","agent_name":"Taylor Swift"}"#;
        let line: LyricLineData = serde_json::from_str(raw).expect("parse line");
        assert_eq!(line.text, "I could never find the right way to tell you");
        assert_eq!(line.prefix.as_deref(), Some("Taylor Swift: "));
        assert_eq!(line.agent.as_deref(), Some("v1"));
        assert_eq!(line.words.len(), 3);
        assert_eq!(line.words[1].time, 39.548);
        assert_eq!(line.words[1].text, " ");
        // The fragments reconstruct the text, which is what lets the overlay cut
        // it into a sung prefix and the rest.
        let rebuilt: String = line.words.iter().map(|w| w.text.as_str()).collect();
        assert_eq!(rebuilt, "I could");
    }

    /// A payload from a TUI older than the karaoke fields still parses and simply
    /// carries no word timings and no label.
    #[test]
    fn parses_a_payload_from_an_older_tui() {
        let raw = r#"{"time":1.0,"end":2.0,"text":"hello","agent":"v1"}"#;
        let line: LyricLineData = serde_json::from_str(raw).expect("parse line");
        assert!(line.words.is_empty());
        assert!(line.prefix.is_none());
        assert_eq!(line.text, "hello");
    }

    #[test]
    fn message_serde_roundtrip() {
        let m = IpcMessage::open("/tmp/a.flac");
        let json = serde_json::to_string(&m).expect("serialize");
        let back: IpcMessage = serde_json::from_str(&json).expect("deserialize");
        // IpcMessage does not derive PartialEq, so compare fields individually.
        assert_eq!(back.msg_type, m.msg_type);
        assert_eq!(back.path, m.path);
        assert_eq!(back.dialog, m.dialog);
        assert_eq!(back.enable, m.enable);
        assert_eq!(back.elapsed, m.elapsed);
        assert_eq!(back.title, m.title);
        assert_eq!(back.artist, m.artist);
        // LyricLineData has no PartialEq either; open() leaves lines unset,
        // so both sides must be None here.
        assert!(m.lines.is_none());
        assert!(back.lines.is_none());
    }

    #[test]
    fn constructors_set_type_field() {
        assert_eq!(IpcMessage::play_pause().msg_type, "play_pause");
        assert_eq!(IpcMessage::open("x").path.as_deref(), Some("x"));
    }

    #[test]
    fn port_file_path_is_absolute() {
        let p = port_file_path(12345);
        assert!(p.contains("12345"), "port path should embed pid: {p}");
    }

    #[test]
    fn split_line_prefers_parts_and_falls_back() {
        let mut line = LyricLineData {
            time: 0.0,
            end: 0.0,
            text: "hello | 你好".to_string(),
            parts: vec!["hello".to_string(), "你好".to_string()],
            words: Vec::new(),
            prefix: None,
            agent: None,
            agent_name: None,
        };
        assert_eq!(split_line(&line), vec!["hello", "你好"]);

        // An older TUI sends no parts: keep splitting the flat text.
        line.parts.clear();
        assert_eq!(split_line(&line), vec!["hello", "你好"]);

        // Nothing to split: the whole text is one sub-line.
        line.text = "hello".to_string();
        assert_eq!(split_line(&line), vec!["hello"]);
    }
}
