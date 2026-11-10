//! Go-owned preferences are applied without restarting the PTY or playback.
use super::{App, Message};
use crate::{config, ipc, lyrics_window};
use iced::Task;
use std::collections::BTreeMap;

#[derive(Default)]
pub(super) struct Writes {
    pending: BTreeMap<&'static str, serde_json::Value>,
    pub(super) active: Option<(String, &'static str, serde_json::Value)>,
    pub(super) quit: bool,
}

impl App {
    pub(super) fn apply_preferences(
        &mut self,
        revision: u64,
        preferences: config::Preferences,
    ) -> Task<Message> {
        if !self.ipc_connected
            || !self.handshake.supports("settings_preferences")
            || revision <= self.preferences_revision
            || self.closing
        {
            return Task::none();
        }
        self.preferences_revision = revision;
        let old = self.config.desktop_lyrics.clone();
        let mut config = preferences.gui;
        config.desktop_lyrics = preferences.desktop_lyrics;
        config.preserve_window_state(&self.config);
        config.normalize();
        let font_changed = self.config.font_size != config.font_size
            || self.config.monospace_font != config.monospace_font;
        if self.config.monospace_font != config.monospace_font {
            self.family = Box::leak(config.monospace_font.clone().into_boxed_str());
        }
        let chrome_changed = self.config.system_decorations != config.system_decorations;
        self.config = config;
        let active_decorations = self.titlebar.system_decorations;
        self.titlebar.configure(&self.config);
        self.titlebar.system_decorations = active_decorations;
        if chrome_changed {
            self.notice = Some("System chrome will apply when you reopen the GUI.".into());
        }
        let font = self.font();
        if let Some(terminal) = &mut self.terminal {
            if font_changed {
                terminal.handle(iced_term::Command::ChangeFont(font));
            }
            terminal.set_opacity(self.config.opacity);
            terminal.set_scroll_zoom(self.config.zoom_via_scroll);
        }
        if !self.actions_available() {
            self.lyrics_restored = false;
            return Task::none();
        }
        if !self.lyrics_restored {
            return self.restore_ready();
        }
        let task = if old.enabled != self.config.desktop_lyrics.enabled {
            self.lyrics
                .update(
                    lyrics_window::Event::Visible(self.config.desktop_lyrics.enabled),
                    &mut self.config.desktop_lyrics,
                )
                .0
        } else if old.locked != self.config.desktop_lyrics.locked {
            self.lyrics
                .update(
                    lyrics_window::Event::Locked(self.config.desktop_lyrics.locked),
                    &mut self.config.desktop_lyrics,
                )
                .0
        } else {
            Task::none()
        };
        let overlay = Task::batch([
            task,
            self.lyrics.fit(&self.config.desktop_lyrics),
            self.lyrics.refresh_mask(),
        ])
        .map(Message::Lyrics);
        self.track_preferences_task(overlay)
    }

    pub(super) fn send_preference(
        &mut self,
        key: &'static str,
        value: serde_json::Value,
    ) -> Task<Message> {
        if !self.actions_available() || self.dialog.is_some() || !self.ipc_connected {
            return Task::none();
        }
        if !self.handshake.supports("results") {
            return self.send_all(vec![ipc::Request::SetPreference { key, value }]);
        }
        self.preference_writes.pending.insert(key, value);
        self.flush_preference()
    }

    pub(super) fn intended_preference(&self, key: &str) -> Option<&serde_json::Value> {
        self.preference_writes.pending.get(key).or_else(|| {
            self.preference_writes
                .active
                .as_ref()
                .filter(|(_, active, _)| *active == key)
                .map(|(_, _, value)| value)
        })
    }

    pub(super) fn has_pending_preferences(&self) -> bool {
        self.preference_writes.active.is_some() || !self.preference_writes.pending.is_empty()
    }

    // A cancelled native dialog may stay physically open while Quit drains
    // previously accepted user intents. It still owns ordinary input.
    pub(super) fn finishing_preferences(&self) -> bool {
        self.preference_writes.quit
            && !self.closing
            && self.ipc_connected
            && self.startup == super::startup::Stage::Phase(ipc::StartupPhase::Ready)
            && self
                .picker
                .as_ref()
                .is_some_and(|picker| picker.cancelled())
    }

    pub(super) fn flush_preference(&mut self) -> Task<Message> {
        if (!self.actions_available() && !self.finishing_preferences())
            || !self.ipc_connected
            || self.connection.is_none()
        {
            return Task::none();
        }
        if self.preference_writes.active.is_some() {
            return Task::none();
        }
        let Some((key, value)) = self.preference_writes.pending.pop_first() else {
            if std::mem::take(&mut self.preference_writes.quit) {
                return self.action(crate::actions::Action::Quit);
            }
            return Task::none();
        };
        let previous_sequence = self.request_sequence;
        let task = self.send_all(vec![ipc::Request::SetPreference {
            key,
            value: value.clone(),
        }]);
        let id = format!(
            "{}:{}:{}",
            self.session, self.connection_epoch, self.request_sequence
        );
        if self.request_sequence > previous_sequence && self.requests.contains_key(&id) {
            self.preference_writes.active = Some((id, key, value));
        } else {
            self.preference_writes = Default::default();
        }
        task
    }

    pub(super) fn preference_result(&mut self, id: &str, status: &str) -> Task<Message> {
        if self
            .preference_writes
            .active
            .as_ref()
            .is_none_or(|(active, _, _)| active != id)
        {
            return Task::none();
        }
        match status {
            "succeeded" => {
                self.preference_writes.active = None;
                self.flush_preference()
            }
            "failed" | "cancelled" => {
                let quit = self.preference_writes.quit;
                self.preference_writes = Default::default();
                if quit {
                    self.action(crate::actions::Action::Quit)
                } else {
                    Task::none()
                }
            }
            _ => Task::none(),
        }
    }

    pub(super) fn cancel_preference(&mut self, id: &str) {
        if self
            .preference_writes
            .active
            .as_ref()
            .is_some_and(|(active, _, _)| active == id)
        {
            self.preference_writes = Default::default();
        }
    }

    pub(super) fn open_lyrics_settings(&mut self) -> Task<Message> {
        if self.dialog.is_some() {
            return Task::none();
        }
        if !self.handshake.supports("settings_navigation") {
            self.notice=Some("Settings requires a player with settings navigation support. Update the player and GUI together.".into());
            return self
                .reveal_main()
                .chain(Task::done(Message::RestoreTerminalFocus));
        }
        let request = if self.handshake.supports("settings_preferences") {
            ipc::Request::OpenSettingsSection("desktop_lyrics")
        } else {
            ipc::Request::OpenSettings
        };
        Task::batch([
            self.send_all(vec![request]),
            self.reveal_main()
                .chain(Task::done(Message::RestoreTerminalFocus)),
        ])
    }
}
