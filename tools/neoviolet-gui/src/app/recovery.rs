//! Temporary player recovery. Only Go may persist the selected program path.
use super::{App, Dialog, Message};
use crate::ipc;
use iced::{Task, window};
use std::path::PathBuf;

#[derive(Default)]
pub(super) struct Recovery {
    pub(super) picker: bool,
    pub(super) generation: u64,
    pub(super) path: Option<String>,
    pub(super) original_path: Option<String>,
    pub(super) attempted: bool,
}

fn configured(path: Option<&str>) -> Option<&str> {
    path.filter(|path| !path.is_empty())
}

impl App {
    pub(super) fn choose_player(&mut self) -> Task<Message> {
        if self.running
            || self.closing
            || !self.basic_actions_available()
            || !matches!(self.dialog, Some(Dialog::Error(_)))
        {
            return Task::none();
        }
        self.recovery.picker = true;
        self.recovery.generation += 1;
        let generation = self.recovery.generation;
        self.dialog_navigation.reset();
        self.reveal_main().chain(
            window::run(self.main_window, |parent| {
                // Construct on the window thread; rfd resolves its AppKit parent here.
                super::startup::file_dialog(&parent, "player_binary")
                    .map(|dialog| dialog.pick_file())
            })
            .then(move |dialog| {
                Task::perform(
                    async move {
                        match dialog {
                            Ok(dialog) => Ok(dialog.await.map(|file| file.path().to_path_buf())),
                            Err(error) => Err(error),
                        }
                    },
                    move |result| Message::PlayerChosen { generation, result },
                )
            }),
        )
    }

    pub(super) fn player_chosen(
        &mut self,
        generation: u64,
        result: Result<Option<PathBuf>, String>,
    ) -> Task<Message> {
        if !self.recovery.picker
            || generation != self.recovery.generation
            || self.closing
            || self.running
        {
            return Task::none();
        }
        self.recovery.picker = false;
        match result {
            Ok(Some(path)) => {
                let Some(path) = path.to_str().filter(|_| path.is_absolute()) else {
                    self.dialog = Some(Dialog::Error(
                        "Choose an absolute player path with valid UTF-8 characters.".into(),
                    ));
                    return self.reveal_main();
                };
                self.recovery.original_path = self.config.neoviolet_path.clone();
                self.recovery.path = Some(path.into());
                self.recovery.attempted = false;
                self.dialog = None;
                self.start()
            }
            Ok(None) => self.reveal_main(),
            Err(error) => {
                self.dialog = Some(Dialog::Error(format!(
                    "Could not choose the player: {error}"
                )));
                self.reveal_main()
            }
        }
    }

    pub(super) fn reconcile_recovery_path(&mut self) {
        if self.recovery.path.is_some()
            && (configured(self.config.neoviolet_path.as_deref())
                == configured(self.recovery.path.as_deref())
                || configured(self.config.neoviolet_path.as_deref())
                    != configured(self.recovery.original_path.as_deref()))
        {
            self.recovery.path = None;
            self.recovery.original_path = None;
        }
    }

    pub(super) fn save_recovery_path(&mut self) -> Task<Message> {
        if self.recovery.attempted
            || !self.actions_available()
            || !self.ipc_connected
            || self.dialog.is_some()
            || (self.handshake.supports("settings_preferences") && self.preferences_revision == 0)
        {
            return Task::none();
        }
        let Some(path) = self.recovery.path.clone() else {
            return Task::none();
        };
        self.recovery.attempted = true;
        if self.handshake.supports("player_recovery") {
            return self.send_preference("gui.neoviolet_path", path.into());
        }
        self.notice=Some("Player started temporarily. Update the player and GUI together to save its path automatically.".into());
        if self.handshake.supports("settings_preferences")
            && self.handshake.supports("settings_navigation")
        {
            return self.send_all(vec![ipc::Request::OpenSettingsSection("gui")]);
        }
        Task::none()
    }
}
