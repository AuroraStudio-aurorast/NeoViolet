//! Go owns startup readiness; the GUI enables actions and hosts native file pickers.
use super::{App, Message};
use crate::{ipc, lyrics_window};
use iced::window::raw_window_handle::{HasDisplayHandle, HasWindowHandle};
use iced::{Task, window};
use std::future::Future;
use std::path::PathBuf;
use std::pin::Pin;

type PickerFuture = Pin<Box<dyn Future<Output = Option<rfd::FileHandle>> + Send>>;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) enum Stage {
    #[default]
    AwaitingPhase,
    Legacy,
    Phase(ipc::StartupPhase),
}

#[derive(Debug, Clone)]
pub(super) struct Picker {
    session: u64,
    epoch: u64,
    request_id: String,
    purpose: String,
    cancelled: bool,
    stage: Stage,
    waiting_reply: bool,
    acknowledged: bool,
}

impl Picker {
    pub(super) fn waiting_reply(&self, id: &str) -> bool {
        self.waiting_reply && self.request_id == id
    }
    pub(super) fn reply_pending(&self) -> bool {
        self.waiting_reply
    }
    pub(super) fn cancelled(&self) -> bool {
        self.cancelled
    }
    pub(super) fn allowed_in(&self, stage: Stage) -> bool {
        self.stage == stage && picker_allowed(&self.purpose, stage)
    }
}

fn picker_allowed(purpose: &str, stage: Stage) -> bool {
    matches!(
        (purpose, stage),
        ("soundfont", Stage::Phase(ipc::StartupPhase::Wizard))
            | (
                "audio" | "soundfont" | "library_directory" | "player_binary",
                Stage::Phase(ipc::StartupPhase::Ready)
            )
    )
}

fn file_dialog<W: HasWindowHandle + HasDisplayHandle>(
    parent: &W,
    purpose: &str,
) -> Result<rfd::AsyncFileDialog, String> {
    if purpose == "soundfont" {
        return soundfont_dialog(parent);
    }
    parent.window_handle().map_err(|error| error.to_string())?;
    parent.display_handle().map_err(|error| error.to_string())?;
    let title = match purpose {
        "library_directory" => "Choose Music Folder",
        "player_binary" => "Choose NeoViolet Player",
        _ => "Open Audio File",
    };
    let dialog = rfd::AsyncFileDialog::new()
        .set_title(title)
        .set_parent(parent);
    #[cfg(target_os = "windows")]
    let dialog = if purpose == "player_binary" {
        dialog.add_filter("Windows executable", &["exe"])
    } else {
        dialog
    };
    Ok(dialog)
}

pub(super) fn soundfont_dialog<W: HasWindowHandle + HasDisplayHandle>(
    parent: &W,
) -> Result<rfd::AsyncFileDialog, String> {
    parent.window_handle().map_err(|error| error.to_string())?;
    parent.display_handle().map_err(|error| error.to_string())?;
    Ok(rfd::AsyncFileDialog::new()
        .set_title("Choose SoundFont")
        .add_filter("SoundFont", &["sf2"])
        .set_parent(parent))
}

impl App {
    pub(crate) fn basic_actions_available(&self) -> bool {
        !self.closing && self.picker.is_none()
    }

    pub(crate) fn actions_available(&self) -> bool {
        !self.closing
            && self.picker.is_none()
            && match self.startup {
                Stage::Legacy => true,
                Stage::Phase(ipc::StartupPhase::Ready) => self.ipc_connected,
                _ => false,
            }
    }

    pub(super) fn startup_phase(&mut self, phase: ipc::StartupPhase) -> Task<Message> {
        if !self.ipc_connected || !self.handshake.supports("startup") {
            return Task::none();
        }
        self.startup = Stage::Phase(phase);
        if let Some(picker) = &mut self.picker {
            picker.cancelled |= !picker.allowed_in(Stage::Phase(phase));
        }
        if phase == ipc::StartupPhase::Ready {
            return self.restore_ready();
        }
        self.pending.clear();
        self.suspend_lyrics()
    }

    pub(super) fn restore_ready(&mut self) -> Task<Message> {
        let preferences = self.flush_preference();
        if !self.actions_available()
            || self.lyrics_restored
            || (self.handshake.supports("settings_preferences") && self.preferences_revision == 0)
        {
            return preferences;
        }
        self.lyrics_restored = true;
        let (visibility, _) = self.lyrics.update(
            lyrics_window::Event::Visible(self.config.desktop_lyrics.enabled),
            &mut self.config.desktop_lyrics,
        );
        let lyrics = Task::batch([visibility, self.lyrics.fit(&self.config.desktop_lyrics)])
            .map(Message::Lyrics);
        let mut requests = if self.handshake.supports("settings_preferences") {
            Vec::new()
        } else {
            vec![ipc::Request::DesktopLyrics(
                self.config.desktop_lyrics.enabled,
            )]
        };
        if self.dialog.is_none() {
            requests.extend(self.pending.drain(..).map(ipc::Request::Open));
        }
        Task::batch([lyrics, self.send_all(requests), preferences])
    }

    pub(super) fn suspend_lyrics(&mut self) -> Task<Message> {
        self.lyrics_restored = false;
        let mut preferences = self.config.desktop_lyrics.clone();
        let (lyrics, _) = self
            .lyrics
            .update(lyrics_window::Event::Visible(false), &mut preferences);
        let (settings, _) = self
            .lyrics
            .update(lyrics_window::Event::CloseSettings, &mut preferences);
        Task::batch([lyrics, settings]).map(Message::Lyrics)
    }

    pub(super) fn setup_picker(&mut self, purpose: String, request_id: String) -> Task<Message> {
        if self.closing
            || !self.ipc_connected
            || !self.handshake.supports("file_picker")
            || !picker_allowed(&purpose, self.startup)
            || (purpose != "audio"
                && self.startup == Stage::Phase(ipc::StartupPhase::Ready)
                && !self.handshake.supports("settings_file_picker"))
            || request_id.is_empty()
        {
            return Task::none();
        }
        if let Some(active) = &self.picker {
            if active.session == self.session
                && active.epoch == self.connection_epoch
                && active.request_id == request_id
                && active.purpose == purpose
            {
                return Task::none();
            }
            return self.send_all(vec![ipc::Request::FilePicked {
                purpose,
                request_id,
                status: "failed".into(),
                path: None,
                error: Some("A file chooser is still open. Close it before trying again.".into()),
            }]);
        }
        if self.dialog.is_some() || self.titlebar.visible() {
            return self.send_all(vec![ipc::Request::FilePicked {
                purpose,
                request_id,
                status: "failed".into(),
                path: None,
                error: Some("Finish the current dialog before choosing a file.".into()),
            }]);
        }
        let picker = Picker {
            session: self.session,
            epoch: self.connection_epoch,
            request_id,
            purpose: purpose.clone(),
            cancelled: false,
            stage: self.startup,
            waiting_reply: false,
            acknowledged: false,
        };
        self.picker = Some(picker.clone());
        self.reveal_main().chain(
            window::run(self.main_window, move |parent| {
                // Construct the native operation on the window thread: rfd
                // resolves its AppKit parent before returning the future.
                file_dialog(&parent, &purpose).map(|dialog| -> PickerFuture {
                    if purpose == "library_directory" {
                        Box::pin(dialog.pick_folder())
                    } else {
                        Box::pin(dialog.pick_file())
                    }
                })
            })
            .then(move |dialog| {
                let picker = picker.clone();
                Task::perform(
                    async move {
                        match dialog {
                            Ok(dialog) => Ok(dialog.await.map(|file| file.path().to_path_buf())),
                            Err(error) => Err(error),
                        }
                    },
                    move |result| Message::SetupPicked {
                        session: picker.session,
                        epoch: picker.epoch,
                        request_id: picker.request_id.clone(),
                        result,
                    },
                )
            }),
        )
    }

    pub(super) fn cancel_native_picker(&mut self) -> Task<Message> {
        let Some(picker) = &mut self.picker else {
            return Task::none();
        };
        if picker.cancelled
            || picker.session != self.session
            || picker.epoch != self.connection_epoch
        {
            return Task::none();
        }
        picker.cancelled = true;
        let reply = ipc::Request::FilePicked {
            purpose: picker.purpose.clone(),
            request_id: picker.request_id.clone(),
            status: "cancelled".into(),
            path: None,
            error: None,
        };
        self.send_all(vec![reply])
    }

    pub(super) fn setup_picked(
        &mut self,
        session: u64,
        epoch: u64,
        request_id: String,
        result: Result<Option<PathBuf>, String>,
    ) -> Task<Message> {
        let Some(picker) = &self.picker else {
            return Task::none();
        };
        if picker.waiting_reply
            || picker.session != session
            || picker.epoch != epoch
            || picker.request_id != request_id
        {
            return Task::none();
        }
        let cancelled = picker.cancelled || !picker.allowed_in(self.startup);
        let purpose = picker.purpose.clone();
        if cancelled
            || session != self.session
            || epoch != self.connection_epoch
            || !self.ipc_connected
            || self.closing
        {
            let wait_for_cancel = picker.cancelled
                && !picker.acknowledged
                && picker.allowed_in(self.startup)
                && self.handshake.supports("settings_file_picker")
                && self.startup == Stage::Phase(ipc::StartupPhase::Ready)
                && session == self.session
                && epoch == self.connection_epoch
                && self.ipc_connected
                && !self.closing;
            if wait_for_cancel {
                if let Some(picker) = &mut self.picker {
                    picker.waiting_reply = true;
                }
                return self.picker_deadline(session, epoch, request_id);
            }
            self.picker = None;
            return self.restore_picker_input();
        }
        let (status, path, error) = match result {
            Ok(Some(path)) => ("selected", Some(path), None),
            Ok(None) => ("cancelled", None, None),
            Err(error) => ("failed", None, Some(error)),
        };
        let request = ipc::Request::FilePicked {
            purpose,
            request_id: request_id.clone(),
            status: status.into(),
            path,
            error,
        };
        if self.handshake.supports("settings_file_picker")
            && self.startup == Stage::Phase(ipc::StartupPhase::Ready)
        {
            if let Some(picker) = &mut self.picker {
                picker.waiting_reply = true;
            }
            return Task::batch([
                self.send_all(vec![request]),
                self.picker_deadline(session, epoch, request_id),
            ]);
        }
        self.picker = None;
        Task::batch([self.send_all(vec![request]), self.restore_picker_input()])
    }

    pub(super) fn finish_picker_reply(
        &mut self,
        id: &str,
        status: &str,
        error: &str,
    ) -> Option<Task<Message>> {
        let picker = self.picker.as_mut()?;
        if picker.cancelled
            && !picker.waiting_reply
            && picker.request_id == id
            && picker.session == self.session
            && picker.epoch == self.connection_epoch
            && matches!(status, "succeeded" | "failed" | "cancelled")
        {
            picker.acknowledged = true;
            return Some(Task::none());
        }
        if !picker.waiting_reply(id)
            || picker.session != self.session
            || picker.epoch != self.connection_epoch
            || !matches!(status, "succeeded" | "failed" | "cancelled")
        {
            return None;
        }
        self.picker = None;
        if status == "failed" && !error.is_empty() {
            self.notice = Some(error.into());
        }
        Some(self.restore_picker_input())
    }

    pub(super) fn picker_reply_timeout(&mut self) -> Task<Message> {
        self.notice = Some("The player did not confirm the file selection. Cancelling…".into());
        self.cancel_native_picker()
    }

    fn picker_deadline(&self, session: u64, epoch: u64, request_id: String) -> Task<Message> {
        Task::perform(
            async move {
                tokio::time::sleep(std::time::Duration::from_secs(15)).await;
                request_id
            },
            move |id| Message::RequestTimeout {
                session,
                epoch,
                id,
                accepted: false,
            },
        )
    }

    fn restore_picker_input(&mut self) -> Task<Message> {
        if self.closing || !self.ipc_connected {
            return Task::none();
        }
        Task::batch([
            self.restore_ready(),
            window::gain_focus(self.main_window),
            self.focus(),
            crate::desktop::align_main_controls(self.main_window).map(|_| Message::Ready),
        ])
    }
}
