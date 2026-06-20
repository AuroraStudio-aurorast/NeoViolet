//! Bidirectional IPC client for communicating with the NeoViolet TUI process
//! via TCP localhost. The TUI listens on 127.0.0.1:0 (random port) and writes
//! the assigned address to a temp file; the GUI reads this file and connects.

use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::sync::{Arc, Mutex};
use std::time::Duration;

/// Wraps a TcpStream connection to the TUI's IPC endpoint.
/// Cloneable — the inner stream is behind Arc<Mutex<…>>.
#[derive(Clone)]
pub struct IpcClient {
    stream: Arc<Mutex<Option<TcpStream>>>,
}

impl IpcClient {
    /// Create a new unconnected client.
    pub fn new() -> Self {
        Self {
            stream: Arc::new(Mutex::new(None)),
        }
    }

    /// Connect to the TUI's IPC endpoint. Reads the port file written by
    /// the TUI, then connects via TCP with a 5-second retry window.
    pub fn connect(&self, pid: u32) -> Result<(), String> {
        let port_path = port_file_path(pid);
        let deadline = std::time::Instant::now() + Duration::from_secs(5);

        let addr = loop {
            match std::fs::read_to_string(&port_path) {
                Ok(s) => {
                    let addr = s.trim().to_string();
                    if !addr.is_empty() {
                        break addr;
                    }
                }
                Err(_) => {
                    if std::time::Instant::now() > deadline {
                        return Err(format!("IPC port file not found after 5 s: {}", port_path));
                    }
                    std::thread::sleep(Duration::from_millis(100));
                }
            }
        };

        loop {
            match TcpStream::connect(&addr) {
                Ok(stream) => {
                    stream
                        .set_read_timeout(Some(Duration::from_millis(100)))
                        .ok();
                    log::info!("[ipc] connected to {}", addr);
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

    /// Send an "open <path>" message to the TUI.
    pub fn send_open(&self, path: &str) -> Result<(), String> {
        let mut guard = self.stream.lock().unwrap();
        let Some(ref mut stream) = *guard else {
            return Err("IPC not connected".into());
        };
        let msg = format!("open {}\n", path);
        stream
            .write_all(msg.as_bytes())
            .map_err(|e| format!("IPC write error: {}", e))?;
        log::info!("[ipc] sent open: {}", path);
        Ok(())
    }

    /// Read a line from the TUI (for future bidirectional use).
    #[allow(dead_code)]
    pub fn read_line(&self) -> Result<Option<String>, String> {
        let mut guard = self.stream.lock().unwrap();
        let Some(ref mut stream) = *guard else {
            return Err("IPC not connected".into());
        };
        let mut reader = BufReader::new(stream.try_clone().map_err(|e| format!("{}", e))?);
        let mut line = String::new();
        match reader.read_line(&mut line) {
            Ok(0) => Ok(None),
            Ok(_) => Ok(Some(line.trim_end().to_string())),
            Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => Ok(None),
            Err(e) => Err(format!("IPC read error: {}", e)),
        }
    }

    /// Start a background reader thread that pushes incoming lines from
    /// the TUI into the given queue. The stream is cloned so the main
    /// thread can continue sending concurrently.
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
