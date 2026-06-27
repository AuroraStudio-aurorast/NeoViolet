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

use serde::{Deserialize, Serialize};
use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::sync::{Arc, Mutex};
use std::time::Duration;

/// A single lyric line received from the TUI via IPC.
#[derive(Serialize, Deserialize, Debug, Clone)]
pub struct LyricLineData {
    pub time: f64,
    /// End time in seconds; 0.0 = unbounded (legacy LRC/QRC/YRC/ESLRC).
    #[serde(default)]
    pub end: f64,
    pub text: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent_name: Option<String>,
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
            match std::fs::read_to_string(&port_path) {
                Ok(s) => {
                    let mut lines = s.lines();
                    let addr = lines.next().unwrap_or("").trim().to_string();
                    let token = lines.next().unwrap_or("").trim().to_string();
                    if !addr.is_empty() && !token.is_empty() {
                        break (addr, token);
                    }
                }
                Err(_) => {}
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
                    Err(e) if e.kind() == std::io::ErrorKind::WouldBlock
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
