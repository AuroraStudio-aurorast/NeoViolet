//! PTY terminal events and authenticated IPC session updates.
use super::{App, Dialog, Message};
use crate::{ipc, lyrics_window};
use iced::Task;
use iced_term::{BackendCommand, actions};

impl App {
    pub(super) fn terminal_event(&mut self, event: iced_term::Event) -> Task<Message> {
        match event {
            iced_term::Event::BackendCall(id, _) if id != self.session => {
                return Task::none();
            }
            iced_term::Event::BackendCall(_, command) => {
                if let BackendCommand::ProcessAlacrittyEvent(
                    iced_term::AlacrittyEvent::ChildExit(code),
                ) = &command
                {
                    self.exit_code = Some(*code);
                }
                if self.playback.is_none()
                    && matches!(
                        &command,
                        BackendCommand::ProcessAlacrittyEvent(
                            iced_term::AlacrittyEvent::ResetTitle
                        )
                    )
                {
                    self.window_title = "NeoViolet".into();
                }
                if let Some(terminal) = &mut self.terminal {
                    match terminal.handle(iced_term::Command::ProxyToBackend(command)) {
                        actions::Action::Shutdown => {
                            self.running = false;
                            self.connection = None;
                            self.ipc_connected = false;
                            self.lyrics.clear();
                            self.lyrics.playback = None;
                            self.playback = None;
                            self.requests.clear();
                            let lyrics = self.suspend_lyrics();
                            if self.closing {
                                return lyrics.chain(self.finish_quit());
                            }
                            self.dialog = Some(Dialog::Error(match self.exit_code {
                                Some(0) => {
                                    "The player exited. You can restart it or close this window."
                                        .into()
                                }
                                Some(code) => format!(
                                    "The player exited with status {code}. Check the launch arguments and player settings."
                                ),
                                None => {
                                    "The player stopped. You can restart it or close this window."
                                        .into()
                                }
                            }));
                            return Task::batch([self.reveal_main(), lyrics]);
                        }
                        actions::Action::ChangeTitle(title) if self.playback.is_none() => {
                            self.window_title = if title.trim().is_empty() {
                                "NeoViolet".into()
                            } else {
                                title
                            };
                        }
                        _ => {}
                    }
                }
            }
        }
        Task::none()
    }

    pub(super) fn ipc_event(&mut self, session: u64, event: ipc::Event) -> Task<Message> {
        if session != self.session {
            return Task::none();
        }
        match event {
            ipc::Event::Connected(sender) => {
                self.connection_epoch += 1;
                self.connection_error = None;
                self.playback = None;
                self.lyrics.playback = (self.handshake.protocol == 0).then_some(false);
                self.connection = Some(sender);
                self.ipc_connected = true;
                eprintln!(
                    "NeoViolet GUI: IPC authenticated (player PID {})",
                    self.pid.unwrap_or_default()
                );
                self.startup = if self.handshake.supports("startup") {
                    super::startup::Stage::AwaitingPhase
                } else {
                    super::startup::Stage::Legacy
                };
                self.lyrics_restored = false;
                if self.actions_available() {
                    return self.restore_ready();
                }
                self.pending.clear();
                return self.suspend_lyrics();
            }
            ipc::Event::Handshake(handshake) => {
                self.version = if handshake.version.trim().is_empty() {
                    "unavailable".into()
                } else {
                    format!("NeoViolet version {}", handshake.version)
                };
                self.startup = if handshake.supports("startup") {
                    super::startup::Stage::AwaitingPhase
                } else {
                    super::startup::Stage::Legacy
                };
                self.handshake = handshake;
            }
            ipc::Event::Received(ipc::Incoming::Phase { phase }) => {
                return self.startup_phase(phase);
            }
            ipc::Event::Received(ipc::Incoming::PickFile {
                purpose,
                request_id,
            }) => return self.setup_picker(purpose, request_id),
            ipc::Event::Received(ipc::Incoming::State { state }) => {
                return self.apply_snapshot(state);
            }
            ipc::Event::Received(ipc::Incoming::Result {
                request_id,
                status,
                error,
            }) => return self.command_result(request_id, status, error),
            ipc::Event::Received(ipc::Incoming::Lyrics(frame)) => {
                if self.actions_available() && self.config.desktop_lyrics.enabled {
                    let changed = self.lyrics.frame.display_lines() != frame.display_lines()
                        || self.lyrics.frame.next_line() != frame.next_line()
                        || self.lyrics.frame.title != frame.title
                        || self.lyrics.frame.artist != frame.artist;
                    self.lyrics.frame = frame;
                    return Task::batch([
                        self.lyrics.fit(&self.config.desktop_lyrics),
                        if changed {
                            self.lyrics.refresh_mask()
                        } else {
                            Task::none()
                        },
                    ])
                    .map(Message::Lyrics);
                }
            }
            ipc::Event::Received(ipc::Incoming::DesktopLyrics { enable }) => {
                if !self.actions_available() {
                    return Task::none();
                }
                let (task, _) = self.lyrics.update(
                    lyrics_window::Event::Visible(enable),
                    &mut self.config.desktop_lyrics,
                );
                return task.map(Message::Lyrics);
            }
            ipc::Event::Received(ipc::Incoming::Quit { dialog }) => {
                if self.closing {
                    return Task::none();
                }
                if dialog {
                    self.dialog = Some(Dialog::Quit);
                    return self.reveal_main();
                } else if self.handshake.supports("request_quit") {
                    return self.update(Message::ConfirmQuit);
                } else {
                    self.closing = true;
                    return self.finish_quit();
                }
            }
            ipc::Event::Disconnected(error) => {
                self.connection = None;
                self.ipc_connected = false;
                if self.handshake.supports("startup") {
                    self.startup = super::startup::Stage::AwaitingPhase;
                }
                self.playback = None;
                self.lyrics.playback = None;
                self.window_title = "NeoViolet".into();
                self.connection_error = Some(error.clone());
                if !self.requests.is_empty() {
                    self.notice = Some("The connection was interrupted before some actions could be confirmed. Check playback before trying again.".into());
                }
                self.requests.clear();
                self.seek_preview = None;
                self.volume_preview = None;
                self.lyrics.clear();
                if !self.closing && self.running {
                    eprintln!("NeoViolet GUI: {error}");
                }
                return self.suspend_lyrics();
            }
            ipc::Event::Received(_) => {}
        }
        Task::none()
    }
}
