use std::{
    io::{Read, Write},
    sync::{
        Arc, Mutex,
        mpsc::{self, Sender},
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
    monospace_font: &str,
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
    cmd.env("NEOVIOLET_FONT", monospace_font);
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

#[cfg(all(test, unix))]
mod tests {
    use super::*;
    use std::time::{Duration, Instant};

    /// Everything the GUI shows sits on top of this file's small portable-pty
    /// contract (openpty, spawn, drop the slave, take the reader/writer, resize,
    /// then try_wait), and nothing else in the suite touches it — so a dependency
    /// bump can change real behaviour while still compiling. That is precisely
    /// what 0.9 did (it reworked the slave/`tty_name` handling and swapped the nix
    /// version underneath), so this drives a real shell through the same sequence.
    ///
    /// Each step waits for the observable that proves the previous one landed
    /// instead of sleeping: the child blocks on a second `read` until after the
    /// resize, otherwise it could report the old size and flake.
    #[test]
    fn pty_contract_round_trips_input_resize_and_exit_status() {
        let pty_system = native_pty_system();
        let pair = pty_system
            .openpty(PtySize {
                rows: 24,
                cols: 80,
                pixel_width: 0,
                pixel_height: 0,
            })
            .expect("open PTY");

        let mut cmd = CommandBuilder::new("/bin/sh");
        cmd.arg("-c");
        // Echo what it read, wait for a second line so the resize below has
        // certainly happened, report the size it sees, then exit non-zero so the
        // status is checked rather than merely observed.
        cmd.arg("read line; echo got:$line; read again; stty size; exit 7");
        cmd.env("TERM", "xterm-256color");

        let mut child = pair.slave.spawn_command(cmd).expect("spawn /bin/sh");
        assert!(
            child.process_id().is_some(),
            "the child pid is what the GUI hands to the IPC handshake"
        );
        drop(pair.slave);

        let master = pair.master;
        let mut reader = master.try_clone_reader().expect("clone PTY reader");
        let mut writer = master.take_writer().expect("take PTY writer");

        // Drain the master on a thread, exactly as the backend does, so a
        // blocking read can never hang the test.
        let (tx, rx) = mpsc::channel::<Vec<u8>>();
        thread::spawn(move || {
            let mut buf = [0u8; 4096];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) => break,
                    Ok(n) => {
                        if tx.send(buf[..n].to_vec()).is_err() {
                            break;
                        }
                    }
                    // Some platforms report the end of the stream as an error
                    // once the child is gone; the backend treats it as the end too.
                    Err(_) => break,
                }
            }
        });

        let mut output = Vec::new();
        let wait_for = |needles: &[&str], output: &mut Vec<u8>| {
            let deadline = Instant::now() + Duration::from_secs(10);
            loop {
                let seen = String::from_utf8_lossy(output).to_string();
                if needles.iter().all(|n| seen.contains(n)) {
                    return seen;
                }
                assert!(
                    Instant::now() < deadline,
                    "timed out waiting for {needles:?}, saw {seen:?}"
                );
                match rx.recv_timeout(Duration::from_millis(200)) {
                    Ok(chunk) => output.extend_from_slice(&chunk),
                    Err(mpsc::RecvTimeoutError::Timeout) => {}
                    Err(mpsc::RecvTimeoutError::Disconnected) => {
                        panic!("PTY reader closed while waiting for {needles:?}")
                    }
                }
            }
        };

        writer.write_all(b"hello\n").expect("write input");
        writer.flush().expect("flush input");
        wait_for(&["got:hello"], &mut output);

        master
            .resize(PtySize {
                rows: 30,
                cols: 100,
                pixel_width: 0,
                pixel_height: 0,
            })
            .expect("resize PTY");

        writer.write_all(b"go\n").expect("write second input");
        writer.flush().expect("flush second input");
        wait_for(&["30 100"], &mut output);

        let mut status = None;
        let deadline = Instant::now() + Duration::from_secs(10);
        while Instant::now() < deadline && status.is_none() {
            status = child.try_wait().expect("try_wait");
            if status.is_none() {
                thread::sleep(Duration::from_millis(20));
            }
        }
        assert_eq!(
            status.expect("child never exited").exit_code(),
            7,
            "exit status did not survive the PTY"
        );
    }
}
