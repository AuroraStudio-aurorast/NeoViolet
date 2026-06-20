use std::{
    io::{Read, Write},
    sync::{
        mpsc::{self, Sender},
        Arc, Mutex,
    },
    thread,
};

use anyhow::{Context, Result};
use portable_pty::{CommandBuilder, PtySize, native_pty_system};

use crate::platform;
use crate::terminal::{BackendCommand, BackendEvent};

pub fn spawn_neoviolet_terminal(
    tab_id: String,
    cols: u16,
    rows: u16,
    events: Sender<BackendEvent>,
    launch_args: &[String],
    child_pid: Arc<Mutex<Option<u32>>>,
) -> Result<Sender<BackendCommand>> {
    let pty_system = native_pty_system();
    let pair = pty_system
        .openpty(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })
        .context("open PTY")?;

    let bin = platform::find_neoviolet_binary(None);

    let mut cmd = CommandBuilder::new(&bin);

    // Always pass --xdg-config to the PTY process, deduplicating if the
    // user already supplied it on the command line.
    cmd.arg("--xdg-config");
    for arg in launch_args {
        if arg == "--xdg-config" {
            continue;
        }
        cmd.arg(arg);
    }
    cmd.env(
        "TERM",
        std::env::var("TERM").unwrap_or_else(|_| "xterm-256color".into()),
    );
    cmd.env(
        "COLORTERM",
        std::env::var("COLORTERM").unwrap_or_else(|_| "truecolor".into()),
    );
    cmd.env("TERM_PROGRAM", "neoviolet-gui");
    if let Ok(path) = std::env::var("PATH") {
        cmd.env("PATH", path);
    }
    if let Ok(lang) = std::env::var("LANG") {
        cmd.env("LANG", lang);
    } else {
        cmd.env("LANG", "en_US.UTF-8");
    }
    if let Ok(home) = std::env::var("HOME") {
        cmd.env("HOME", home);
    }
    let mut child = pair.slave.spawn_command(cmd).context("spawn neoviolet")?;
    // Store child PID so GUI can write IPC control file
    *child_pid.lock().unwrap() = child.process_id();
    drop(pair.slave);

    let master = pair.master;
    let mut reader = master.try_clone_reader().context("clone PTY reader")?;
    let mut writer = master.take_writer().context("take PTY writer")?;
    let (cmd_tx, cmd_rx) = mpsc::channel::<BackendCommand>();

    let read_tab = tab_id.clone();
    let read_events = events.clone();
    thread::spawn(move || {
        let mut buf = [0u8; 8192];
        loop {
            match reader.read(&mut buf) {
                Ok(0) => break,
                Ok(n) => {
                    let _ = read_events.send(BackendEvent::Output {
                        tab_id: read_tab.clone(),
                        bytes: buf[..n].to_vec(),
                    });
                }
                Err(err) => {
                    let _ = read_events.send(BackendEvent::Closed {
                        tab_id: read_tab.clone(),
                        reason: format!("read error: {err}"),
                    });
                    return;
                }
            }
        }
        let _ = read_events.send(BackendEvent::Closed {
            tab_id: read_tab,
            reason: "neoviolet closed".into(),
        });
    });

    let write_tab = tab_id.clone();
    let write_events = events.clone();
    thread::spawn(move || {
        loop {
            match cmd_rx.recv_timeout(std::time::Duration::from_millis(100)) {
                Ok(command) => match command {
                    BackendCommand::Input(bytes) => {
                        if let Err(err) = writer.write_all(&bytes) {
                            let _ = write_events.send(BackendEvent::Closed {
                                tab_id: write_tab.clone(),
                                reason: format!("write error: {err}"),
                            });
                            break;
                        }
                        let _ = writer.flush();
                    }
                    BackendCommand::Resize { cols, rows } => {
                        let _ = master.resize(PtySize {
                            rows,
                            cols,
                            pixel_width: 0,
                            pixel_height: 0,
                        });
                    }
                    BackendCommand::Close => break,
                },
                Err(mpsc::RecvTimeoutError::Timeout) => {
                    if let Ok(Some(status)) = child.try_wait() {
                        let _ = write_events.send(BackendEvent::Closed {
                            tab_id: write_tab,
                            reason: format!("neoviolet exited: {status}"),
                        });
                        return;
                    }
                }
                Err(mpsc::RecvTimeoutError::Disconnected) => break,
            }
        }
        let _ = child.kill();
    });

    let _ = events.send(BackendEvent::Status {
        tab_id,
        text: "neoviolet ready".into(),
    });

    Ok(cmd_tx)
}
