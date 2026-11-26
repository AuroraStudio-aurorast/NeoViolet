//! Validate native commands against the current session; Go confirms all changes.
use super::{App, Message};
use crate::actions::Action;
use crate::touchbar::{Command, Context, Event, State as NativeState};
use crate::{desktop, ipc};
use iced::Task;

#[derive(Default)]
pub(super) struct State {
    pub(super) native: Option<NativeState>,
}

impl App {
    pub(super) fn touchbar_state(&self) -> NativeState {
        let available = self.running
            && self.ipc_connected
            && self.actions_available()
            && self.dialog.is_none()
            && !self.titlebar.visible();
        let snapshot = self.playback.as_ref();
        let can_play = available
            && self.handshake.supports("track_playback")
            && snapshot.is_some_and(|state| state.has_track && !state.loading);
        NativeState {
            context: Context {
                window: self.main_window,
                session: self.session,
                epoch: self.connection_epoch,
                track_id: snapshot.map(|state| state.track_id),
            },
            available,
            can_play,
            can_seek: can_play && snapshot.is_some_and(|state| state.duration > 0.0),
            playing: snapshot.is_some_and(|state| state.playing),
            lyrics: self.config.desktop_lyrics.enabled,
        }
    }

    pub(super) fn touchbar_event(&mut self, event: Event) -> Task<Message> {
        let state = self.touchbar_state();
        if event.context != state.context || !state.allows(event.command) {
            return Task::none();
        }
        let request = match event.command {
            Command::Open => return self.action(Action::Open),
            Command::Lyrics => return self.action(Action::DesktopLyrics),
            Command::PlayPause => {
                ipc::Request::PlayPauseTrack(state.context.track_id.expect("validated track"))
            }
            Command::SeekBy(delta) => ipc::Request::SeekRelative {
                track_id: state.context.track_id.expect("validated track"),
                delta,
            },
        };
        self.send_all(vec![request])
    }

    pub(super) fn sync_touchbar(&mut self) -> Task<Message> {
        let state = self.touchbar_state();
        if self.touchbar.native.replace(state) != Some(state) && self.desktop.is_some() {
            desktop::sync_touchbar(self.main_window, state).map(|_| Message::Ready)
        } else {
            Task::none()
        }
    }
}
