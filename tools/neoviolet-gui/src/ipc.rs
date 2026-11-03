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
    Quit,
    DesktopLyrics(bool),
    PlayPause,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum Incoming {
    Hello,
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

#[derive(Debug, Clone)]
pub enum Event {
    Connected(mpsc::Sender<Request>),
    Received(Incoming),
    Disconnected(String),
}

pub fn subscription(session: u64, pid: u32) -> Subscription<Event> {
    Subscription::run_with((session, pid), |&(_, pid)| {
        stream::channel(16, async move |mut output| {
            let (sender, mut requests) = mpsc::channel(32);
            let path = std::env::temp_dir().join(format!("neoviolet-ipc-{pid}"));
            let connection = discover(&path).await;
            let peer = match connection {
                Ok(peer) => peer,
                Err(error) => {
                    let _ = output.send(Event::Disconnected(error)).await;
                    return;
                }
            };
            if output.send(Event::Connected(sender)).await.is_err() {
                return;
            }
            let (reader, mut writer) = tokio::io::split(peer);
            let mut reader = BufReader::new(reader);
            let result: Result<(), String> = {
                let receive = async {
                    loop {
                        match read_frame(&mut reader).await? {
                            Some(message) => {
                                if output.send(Event::Received(message)).await.is_err() {
                                    return Ok(());
                                }
                            }
                            None => return Err("Player IPC connection closed".into()),
                        }
                    }
                };
                let transmit = async {
                    while let Some(request) = requests.recv().await {
                        write_request(&mut writer, &request).await?;
                    }
                    Ok(())
                };
                tokio::select! {
                    result = receive => result,
                    result = transmit => result,
                }
            };
            if let Err(error) = result {
                let _ = output.send(Event::Disconnected(error)).await;
            }
        })
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

async fn discover(path: &Path) -> Result<BufReader<TcpStream>, String> {
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

async fn authenticate(mut stream: TcpStream, token: &str) -> Result<BufReader<TcpStream>, String> {
    stream
        .write_all(format!("{token}\n").as_bytes())
        .await
        .map_err(|error| error.to_string())?;
    let mut peer = BufReader::new(stream);
    match read_frame(&mut peer).await? {
        Some(Incoming::Hello) => Ok(peer),
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
) -> Result<(), String> {
    let value = match request {
        Request::Open(path) => serde_json::json!({"type": "open", "path": path}),
        Request::Quit => serde_json::json!({"type": "quit"}),
        Request::DesktopLyrics(enable) => {
            serde_json::json!({"type": "desktop_lyrics", "enable": enable})
        }
        Request::PlayPause => serde_json::json!({"type": "play_pause"}),
    };
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
