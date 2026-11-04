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
                if matches!(
                    &command,
                    BackendCommand::ProcessAlacrittyEvent(iced_term::AlacrittyEvent::ResetTitle)
                ) {
                    self.window_title = "NeoViolet".into();
                }
                if let Some(terminal) = &mut self.terminal {
                    match terminal.handle(iced_term::Command::ProxyToBackend(command)) {
                        actions::Action::Shutdown => {
                            self.running = false;
                            self.connection = None;
                            self.ipc_connected = false;
                            self.lyrics.clear();
                            if self.closing {
                                return self.finish_quit();
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
                            return Task::batch([
                                self.reveal_main(),
                                self.lyrics.refresh_mask().map(Message::Lyrics),
                            ]);
                        }
                        actions::Action::ChangeTitle(title) => {
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
                self.connection = Some(sender);
                self.ipc_connected = true;
                eprintln!(
                    "NeoViolet GUI: IPC authenticated (player PID {})",
                    self.pid.unwrap_or_default()
                );
                let mut requests = vec![ipc::Request::DesktopLyrics(
                    self.config.desktop_lyrics.enabled,
                )];
                if self.dialog.is_none() {
                    requests.extend(self.pending.drain(..).map(ipc::Request::Open));
                }
                return self.send_all(requests);
            }
            ipc::Event::Received(ipc::Incoming::Lyrics(frame)) => {
                if self.config.desktop_lyrics.enabled {
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
                let (task, _) = self.lyrics.update(
                    lyrics_window::Event::Visible(enable),
                    &mut self.config.desktop_lyrics,
                );
                return task.map(Message::Lyrics);
            }
            ipc::Event::Received(ipc::Incoming::Quit { dialog }) => {
                if dialog {
                    self.dialog = Some(Dialog::Quit);
                    return self.reveal_main();
                } else {
                    self.closing = true;
                    return self.finish_quit();
                }
            }
            ipc::Event::Disconnected(error) => {
                self.connection = None;
                self.ipc_connected = false;
                self.lyrics.clear();
                if !self.closing && self.running {
                    eprintln!("NeoViolet GUI: {error}");
                }
                return self.lyrics.refresh_mask().map(Message::Lyrics);
            }
            ipc::Event::Received(_) => {}
        }
        Task::none()
    }
}
