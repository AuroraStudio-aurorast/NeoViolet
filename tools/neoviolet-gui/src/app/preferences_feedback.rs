//! A saved revision is acknowledged after its overlay task cascade completes.
use super::{App, Message};
use crate::ipc;
use iced::Task;

impl App {
    // Keep work from older snapshots in the same connection until it drains:
    // e.g. a late Opened event must still finish creating the current overlay.
    pub(super) fn track_preferences_task(&mut self, task: Task<Message>) -> Task<Message> {
        if !self.handshake.supports("preferences_feedback") {
            return task;
        }
        self.preferences_tasks += 1;
        let session = self.session;
        let epoch = self.connection_epoch;
        task.map(move |message| match message {
            Message::Lyrics(event) => Message::PreferencesLyrics {
                session,
                epoch,
                event,
            },
            other => other,
        })
        .chain(Task::done(Message::PreferencesWorkDone { session, epoch }))
    }

    pub(super) fn acknowledge_preferences(&mut self) -> Task<Message> {
        if self.preferences_tasks > 0
            || !self.actions_available()
            || !self.ipc_connected
            || !self.handshake.supports("preferences_feedback")
            || self.preferences_revision <= self.preferences_acknowledged
        {
            return Task::none();
        }
        let mut restart_required = Vec::new();
        if !cfg!(target_os = "macos")
            && self.config.system_decorations != self.titlebar.system_decorations
        {
            restart_required.push("gui.system_decorations");
        }
        let current = self
            .running_player_path
            .as_deref()
            .filter(|path| !path.is_empty());
        let desired = self
            .config
            .neoviolet_path
            .as_deref()
            .filter(|path| !path.is_empty());
        if current != desired {
            restart_required.push("gui.neoviolet_path");
        }
        self.preferences_acknowledged = self.preferences_revision;
        let status = if restart_required.is_empty() {
            "applied"
        } else {
            "restart_required"
        };
        self.send_all(vec![ipc::Request::PreferencesApplied {
            revision: self.preferences_revision,
            status,
            restart_required,
            error: None,
        }])
    }
}
