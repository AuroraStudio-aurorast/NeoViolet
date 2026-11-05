use std::net::SocketAddr;
use std::path::{Path, PathBuf};
use std::time::Duration;

use iced::futures::SinkExt;
use iced::{Subscription, stream};
use serde::Deserialize;
use tokio::io::{
    AsyncBufRead, AsyncBufReadExt, AsyncReadExt, AsyncWrite, AsyncWriteExt, BufReader,
};
use tokio::net::TcpStream;
use tokio::sync::mpsc;

const MAX_FRAME: usize = 10 * 1024 * 1024;

#[derive(Debug, Clone)]
pub enum Request {
    Open(PathBuf),
    FilePicked {
        purpose: String,
        request_id: String,
        status: String,
        path: Option<PathBuf>,
        error: Option<String>,
    },
    Quit,
    CheckQuit,
    DesktopLyrics(bool),
    PlayPause,
    Play,
    Pause,
    Seek(f64),
    SetVolume(f64),
    GetState,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum Incoming {
    Hello(Handshake),
    Phase {
        phase: StartupPhase,
    },
    PickFile {
        purpose: String,
        request_id: String,
    },
    State {
        state: PlaybackState,
    },
    Result {
        request_id: String,
        status: String,
        #[serde(default)]
        error: String,
    },
    Lyrics(crate::lyrics::Frame),
    DesktopLyrics {
        enable: bool,
    },
    Quit {
        dialog: bool,
    },
    #[serde(other)]
    Other,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum StartupPhase {
    Starting,
    Wizard,
    InitialLoading,
    Ready,
    #[serde(other)]
    Unknown,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct Handshake {
    pub protocol: u32,
    pub version: String,
    pub libopenmpt_version: String,
    pub capabilities: Vec<String>,
}

impl Handshake {
    pub fn supports(&self, capability: &str) -> bool {
        self.capabilities.iter().any(|value| value == capability)
    }
}

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct PlaybackState {
    pub revision: u64,
    pub track_id: u64,
    pub path: String,
    pub title: String,
    pub artist: String,
    pub album: String,
    pub has_track: bool,
    pub playing: bool,
    pub loading: bool,
    pub desktop_lyrics: bool,
    pub duration: f64,
    pub position: f64,
    pub volume: f64,
    pub config_dir: PathBuf,
}

#[derive(Debug, Clone)]
pub struct Command {
    pub request: Request,
    pub id: String,
}

#[derive(Debug, Clone)]
pub enum Event {
    Connected(mpsc::Sender<Command>),
    Handshake(Handshake),
    Received(Incoming),
    Disconnected(String),
}

pub fn subscription(session: u64, pid: u32) -> Subscription<Event> {
    Subscription::run_with((session, pid), |&(_, pid)| {
        events(std::env::temp_dir().join(format!("neoviolet-ipc-{pid}")))
    })
}

fn events(path: PathBuf) -> impl iced::futures::Stream<Item = Event> {
    stream::channel(16, async move |mut output| {
        loop {
            let (peer, handshake) = match discover(&path).await {
                Ok(peer) => peer,
                Err(error) => {
                    let incompatible = error.starts_with("Incompatible");
                    if output.send(Event::Disconnected(error)).await.is_err() || incompatible {
                        return;
                    }
                    tokio::time::sleep(Duration::from_millis(500)).await;
                    continue;
                }
            };
            let (sender, mut requests) = mpsc::channel::<Command>(32);
            if output.send(Event::Handshake(handshake)).await.is_err()
                || output.send(Event::Connected(sender)).await.is_err()
            {
                return;
            }
            let (reader, mut writer) = tokio::io::split(peer);
            let mut reader = BufReader::new(reader);
            let receive = async {
                loop {
                    match read_frame(&mut reader).await? {
                        Some(message) => {
                            if output.send(Event::Received(message)).await.is_err() {
                                return Ok(());
                            }
                        }
                        None => return Err("Player connection interrupted. Reconnecting…".into()),
                    }
                }
            };
            let transmit = async {
                while let Some(command) = requests.recv().await {
                    tokio::time::timeout(
                        Duration::from_secs(2),
                        write_request(&mut writer, &command.request, &command.id),
                    )
                    .await
                    .map_err(|_| "Player command timed out".to_string())??;
                }
                Ok(())
            };
            let result: Result<(), String> = tokio::select! {
                result = receive => result,
                result = transmit => result,
            };
            if let Err(error) = result {
                if output.send(Event::Disconnected(error)).await.is_err() {
                    return;
                }
            } else {
                return;
            }
            tokio::time::sleep(Duration::from_millis(250)).await;
        }
    })
}

fn parse_endpoint(contents: &str) -> Result<(SocketAddr, &str), String> {
    let mut lines = contents.lines();
    let address: SocketAddr = lines
        .next()
        .ok_or("Missing IPC address")?
        .parse()
        .map_err(|_| "Invalid IPC address")?;
    if !address.ip().is_loopback() || address.port() == 0 {
        return Err("IPC must use a loopback address".into());
    }
    let token = lines.next().ok_or("Missing IPC token")?;
    if token.len() != 64 || !token.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("Invalid IPC token".into());
    }
    Ok((address, token))
}

async fn discover(path: &Path) -> Result<(BufReader<TcpStream>, Handshake), String> {
    loop {
        if let Ok(contents) = tokio::fs::read_to_string(path).await
            && let Ok((address, token)) = parse_endpoint(&contents)
            && let Ok(stream) = TcpStream::connect(address).await
        {
            return tokio::time::timeout(Duration::from_secs(3), authenticate(stream, token))
                .await
                .map_err(|_| "IPC authentication timed out".to_string())?;
        }
        tokio::time::sleep(Duration::from_millis(50)).await;
    }
}

async fn authenticate(
    mut stream: TcpStream,
    token: &str,
) -> Result<(BufReader<TcpStream>, Handshake), String> {
    stream
        .write_all(format!("{token}\n").as_bytes())
        .await
        .map_err(|error| error.to_string())?;
    let mut peer = BufReader::new(stream);
    match read_frame(&mut peer).await? {
        Some(Incoming::Hello(hello)) if hello.protocol <= 1 => {
            if hello.supports("startup") {
                peer.get_mut().write_all(
                    b"{\"type\":\"client_hello\",\"client\":\"neoviolet-gui\",\"protocol\":1,\"capabilities\":[\"file_picker\",\"audio_file_picker\"]}\n"
                ).await.map_err(|error| error.to_string())?;
            }
            Ok((peer, hello))
        }
        Some(Incoming::Hello(hello)) => Err(format!(
            "Incompatible player protocol {}. Update the player and GUI together.",
            hello.protocol
        )),
        _ => Err("Player did not acknowledge IPC authentication".into()),
    }
}

async fn read_frame<R: AsyncBufRead + Unpin>(reader: &mut R) -> Result<Option<Incoming>, String> {
    let mut data = Vec::new();
    let size = reader
        .take((MAX_FRAME + 1) as u64)
        .read_until(b'\n', &mut data)
        .await
        .map_err(|error| error.to_string())?;
    if size == 0 {
        return Ok(None);
    }
    if size > MAX_FRAME || data.last() != Some(&b'\n') {
        return Err("Incomplete or oversized IPC frame".into());
    }
    serde_json::from_slice(&data)
        .map(Some)
        .map_err(|error| format!("Invalid IPC message: {error}"))
}

async fn write_request<W: AsyncWrite + Unpin>(
    stream: &mut W,
    request: &Request,
    id: &str,
) -> Result<(), String> {
    let mut value = match request {
        Request::FilePicked {
            purpose,
            request_id,
            status,
            path,
            error,
        } => {
            let mut value = serde_json::json!({"type":"file_picked", "purpose":purpose, "request_id":request_id, "status":status});
            if let Some(path) = path {
                value["path"] = serde_json::json!(path);
            }
            if let Some(error) = error {
                value["error"] = error.clone().into();
            }
            value
        }
        Request::Open(path) => serde_json::json!({"type": "open", "path": path}),
        Request::Quit => serde_json::json!({"type": "quit"}),
        Request::CheckQuit => serde_json::json!({"type":"request_quit"}),
        Request::DesktopLyrics(enable) => {
            serde_json::json!({"type": "desktop_lyrics", "enable": enable})
        }
        Request::PlayPause => serde_json::json!({"type": "play_pause"}),
        Request::Play => serde_json::json!({"type": "play"}),
        Request::Pause => serde_json::json!({"type": "pause"}),
        Request::Seek(position) => serde_json::json!({"type": "seek", "position":position}),
        Request::SetVolume(volume) => serde_json::json!({"type": "set_volume", "volume":volume}),
        Request::GetState => serde_json::json!({"type": "get_state"}),
    };
    if !id.is_empty() {
        value["request_id"] = id.into();
    }
    let mut data = serde_json::to_vec(&value).map_err(|error| error.to_string())?;
    data.push(b'\n');
    stream
        .write_all(&data)
        .await
        .map_err(|error| error.to_string())
}

#[cfg(test)]
#[path = "ipc_tests.rs"]
mod tests;

#[cfg(test)]
#[path = "ipc_startup_tests.rs"]
mod startup_tests;
