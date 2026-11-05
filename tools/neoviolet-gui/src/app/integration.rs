//! Command receipts and display-only playback snapshots for one child session.
use super::{App, Message};
use crate::ipc;
use iced::Task;

impl App {
    pub(crate) fn controls_available(&self) -> bool {
        self.actions_available()
            && self.ipc_connected
            && !self.closing
            && self.handshake.supports("playback")
            && self.dialog.is_none()
            && !self.titlebar.visible()
            && self.playback.as_ref().is_some_and(|state| state.has_track)
    }
    pub fn handshake_supports_state(&self) -> bool {
        self.handshake.supports("state")
    }
    pub(super) fn send_all(&mut self, requests: Vec<ipc::Request>) -> Task<Message> {
        let Some(sender) = self.connection.clone() else {
            return Task::none();
        };
        let closing = self.closing;
        let available = self.actions_available();
        let requests = requests.into_iter().filter(|request| {
            (!closing || matches!(request, ipc::Request::Quit))
                && (available
                    || matches!(
                        request,
                        ipc::Request::Quit
                            | ipc::Request::CheckQuit
                            | ipc::Request::GetState
                            | ipc::Request::FilePicked { .. }
                    ))
        });
        let mut timers = Vec::new();
        let mut requests = requests;
        while let Some(request) = requests.next() {
            if matches!(
                request,
                ipc::Request::CheckQuit | ipc::Request::Quit | ipc::Request::FilePicked { .. }
            ) {
                let command = ipc::Command {
                    request,
                    id: String::new(),
                };
                match sender.try_send(command) {
                    Ok(()) => {}
                    Err(tokio::sync::mpsc::error::TrySendError::Full(command)) => {
                        let sender = sender.clone();
                        let session = self.session;
                        let epoch = self.connection_epoch;
                        timers.push(Task::perform(
                            async move {
                                tokio::time::timeout(
                                    std::time::Duration::from_secs(2),
                                    sender.send(command),
                                )
                                .await
                                .map_err(|_| "The player is busy. Try closing again.".to_string())?
                                .map_err(|_| {
                                    "The player connection closed. Try closing again.".to_string()
                                })
                            },
                            move |result| Message::LifecycleSent {
                                session,
                                epoch,
                                result,
                            },
                        ));
                    }
                    Err(_) => {
                        self.notice =
                            Some("The player connection closed. Try closing again.".into());
                    }
                }
                continue;
            }
            if self.requests.len() >= 64 {
                self.notice = Some("Too many player actions are pending. File opens will resume when playback responds.".into());
                self.retain_opens(std::iter::once(request).chain(requests));
                break;
            }
            let id = if self.handshake.supports("results") {
                self.request_sequence += 1;
                format!(
                    "{}:{}:{}",
                    self.session, self.connection_epoch, self.request_sequence
                )
            } else {
                String::new()
            };
            if let Err(error) = sender.try_send(ipc::Command {
                request,
                id: id.clone(),
            }) {
                self.retain_opens(std::iter::once(error.into_inner().request).chain(requests));
                self.notice=Some("The player is busy. Pending file opens will resume when the connection is ready.".into());
                break;
            }
            if !id.is_empty() {
                self.requests.insert(id.clone(), false);
                timers.push(self.request_timeout(id, false));
            }
        }
        if !self.pending.is_empty() {
            let session = self.session;
            let epoch = self.connection_epoch;
            timers.push(Task::perform(
                async {
                    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                },
                move |_| Message::FlushPending { session, epoch },
            ));
        }
        Task::batch(timers)
    }

    fn retain_opens(&mut self, requests: impl Iterator<Item = ipc::Request>) {
        self.pending
            .extend(requests.filter_map(|request| match request {
                ipc::Request::Open(path) => Some(path),
                _ => None,
            }));
    }

    fn request_timeout(&self, id: String, accepted: bool) -> Task<Message> {
        let session = self.session;
        let epoch = self.connection_epoch;
        Task::perform(
            async move {
                tokio::time::sleep(std::time::Duration::from_secs(if accepted {
                    120
                } else {
                    15
                }))
                .await;
                id
            },
            move |id| Message::RequestTimeout {
                session,
                epoch,
                id,
                accepted,
            },
        )
    }

    pub(super) fn command_result(
        &mut self,
        id: String,
        status: String,
        error: String,
    ) -> Task<Message> {
        if !self.requests.contains_key(&id) {
            return Task::none();
        }
        match status.as_str() {
            "accepted" => {
                self.requests.insert(id.clone(), true);
                return self.request_timeout(id, true);
            }
            "succeeded" | "cancelled" => {
                self.requests.remove(&id);
            }
            "failed" => {
                self.requests.remove(&id);
                self.notice = Some(if error.is_empty() {
                    "The player could not complete the action.".into()
                } else {
                    error
                });
            }
            _ => {}
        }
        Task::none()
    }

    pub(super) fn apply_snapshot(&mut self, state: ipc::PlaybackState) -> Task<Message> {
        if !self.ipc_connected
            || self
                .playback
                .as_ref()
                .is_some_and(|old| state.revision <= old.revision || state.track_id < old.track_id)
        {
            return Task::none();
        }
        if !state.volume.is_finite()
            || !(0.0..=1.0).contains(&state.volume)
            || !state.position.is_finite()
            || state.position < 0.0
            || !state.duration.is_finite()
            || state.duration < 0.0
        {
            return Task::none();
        }
        if self
            .playback
            .as_ref()
            .is_some_and(|old| old.track_id != state.track_id)
        {
            self.seek_preview = None;
        }
        self.window_title = if state.has_track && !state.title.is_empty() {
            state.title.clone()
        } else {
            "NeoViolet".into()
        };
        self.lyrics.playback = state.has_track.then_some(state.playing);
        // The desktop window is GUI-owned; its explicit preference wins at each
        // connection. Runtime changes in the TUI arrive through desktop_lyrics.
        self.playback = Some(state);
        Task::none()
    }

    pub(super) fn toggle_playback(&mut self) -> Task<Message> {
        if !self.actions_available()
            || !self.ipc_connected
            || self.closing
            || self.dialog.is_some()
            || self.titlebar.visible()
            || (self.handshake.protocol > 0 && !self.handshake.supports("playback"))
        {
            return Task::none();
        }
        let request = match self.playback.as_ref() {
            Some(state) if state.has_track => {
                if state.playing {
                    ipc::Request::Pause
                } else {
                    ipc::Request::Play
                }
            }
            None if self.handshake.protocol == 0 => ipc::Request::PlayPause,
            _ => return Task::none(),
        };
        self.send_all(vec![request])
    }
}
