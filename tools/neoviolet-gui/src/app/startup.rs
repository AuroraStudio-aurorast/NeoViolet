//! Go owns startup readiness; the GUI enables actions and hosts native file pickers.
use super::{App, Message};
use crate::{ipc, lyrics_window};
use iced::window::raw_window_handle::{HasDisplayHandle, HasWindowHandle};
use iced::{Task, window};
use std::path::PathBuf;

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
}

impl Picker {
    pub(super) fn allowed_in(&self, stage: Stage) -> bool {
        matches!(
            (self.purpose.as_str(), stage),
            ("soundfont", Stage::Phase(ipc::StartupPhase::Wizard))
                | ("audio", Stage::Phase(ipc::StartupPhase::Ready))
        )
    }
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
    Ok(rfd::AsyncFileDialog::new()
        .set_title("Open Audio File")
        .set_parent(parent))
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
            || !matches!(
                (purpose.as_str(), self.startup),
                ("soundfont", Stage::Phase(ipc::StartupPhase::Wizard))
                    | ("audio", Stage::Phase(ipc::StartupPhase::Ready))
            )
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
        let picker = Picker {
            session: self.session,
            epoch: self.connection_epoch,
            request_id,
            purpose: purpose.clone(),
            cancelled: false,
        };
        self.picker = Some(picker.clone());
        self.reveal_main().chain(
            window::run(self.main_window, move |parent| {
                file_dialog(&parent, &purpose).map(|dialog| dialog.pick_file())
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
        if picker.session != session || picker.epoch != epoch || picker.request_id != request_id {
            return Task::none();
        }
        let cancelled = picker.cancelled || !picker.allowed_in(self.startup);
        let purpose = picker.purpose.clone();
        self.picker = None;
        if cancelled
            || session != self.session
            || epoch != self.connection_epoch
            || !self.ipc_connected
            || self.closing
        {
            return self.restore_ready();
        }
        let (status, path, error) = match result {
            Ok(Some(path)) => ("selected", Some(path), None),
            Ok(None) => ("cancelled", None, None),
            Err(error) => ("failed", None, Some(error)),
        };
        Task::batch([
            self.send_all(vec![ipc::Request::FilePicked {
                purpose,
                request_id,
                status: status.into(),
                path,
                error,
            }]),
            self.restore_ready(),
            window::gain_focus(self.main_window),
            self.focus(),
            crate::desktop::align_main_controls(self.main_window).map(|_| Message::Ready),
        ])
    }
}
