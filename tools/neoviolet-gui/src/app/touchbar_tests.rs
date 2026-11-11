use super::integration_tests::connected;
use super::*;
use crate::touchbar::{Command, Event};

fn playing() -> (App, mpsc::Receiver<ipc::Command>) {
    let (mut app, receiver) = connected();
    app.config.desktop_lyrics.enabled = false;
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        track_id: 3,
        has_track: true,
        playing: true,
        position: 10.0,
        duration: 60.0,
        volume: 0.6,
        ..Default::default()
    });
    (app, receiver)
}

fn press(app: &mut App, command: Command) {
    let event = Event {
        context: app.touchbar_state().context,
        command,
    };
    let _ = app.update(Message::Desktop(desktop::Event::TouchBar(event)));
}

#[test]
fn touchbar_pause_waits_for_the_go_snapshot() {
    let (mut app, mut receiver) = playing();
    press(&mut app, Command::PlayPause);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Pause
    ));
    assert!(app.touchbar_state().playing);
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 2,
        track_id: 3,
        has_track: true,
        playing: false,
        volume: 0.6,
        ..Default::default()
    });
    assert!(!app.touchbar_state().playing);
    press(&mut app, Command::PlayPause);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Play
    ));
}

#[test]
fn touchbar_seek_clamps_to_the_track_and_never_changes_the_snapshot() {
    let (mut app, mut receiver) = playing();
    for (offset, expected) in [(-15.0, 0.0), (15.0, 25.0), (100.0, 60.0)] {
        press(&mut app, Command::SeekBy(offset));
        assert!(
            matches!(receiver.try_recv().unwrap().request, ipc::Request::Seek(value) if value == expected)
        );
    }
    for offset in [f64::NAN, f64::INFINITY, f64::NEG_INFINITY] {
        press(&mut app, Command::SeekBy(offset));
    }
    assert!(receiver.try_recv().is_err());
    assert_eq!(app.playback.as_ref().unwrap().position, 10.0);
    app.playback.as_mut().unwrap().duration = 0.0;
    press(&mut app, Command::SeekBy(15.0));
    assert!(!app.touchbar_state().can_seek);
    assert!(receiver.try_recv().is_err());
}

#[test]
fn touchbar_rechecks_readiness_loading_modals_and_capabilities() {
    for blocked in 0..7 {
        let (mut app, mut receiver) = playing();
        let context = app.touchbar_state().context;
        match blocked {
            0 => app.dialog = Some(Dialog::Quit),
            1 => app.ipc_connected = false,
            2 => app.closing = true,
            3 => app.playback.as_mut().unwrap().loading = true,
            4 => app.handshake.capabilities.retain(|cap| cap != "playback"),
            5 => app.startup = super::super::startup::Stage::AwaitingPhase,
            _ => {
                app.titlebar.update(titlebar::Message::ToggleMenu);
            }
        }
        for command in [Command::PlayPause, Command::SeekBy(15.0)] {
            let _ = app.update(Message::Desktop(desktop::Event::TouchBar(Event {
                context,
                command,
            })));
        }
        assert!(receiver.try_recv().is_err(), "blocked state {blocked}");
        assert!(!app.touchbar_state().can_play);
    }
}

#[test]
fn touchbar_rejects_events_from_previous_windows_sessions_connections_and_tracks() {
    for stale in 0..4 {
        let (mut app, mut receiver) = playing();
        let mut context = app.touchbar_state().context;
        match stale {
            0 => context.session -= 1,
            1 => context.epoch -= 1,
            2 => context.track_id = Some(2),
            _ => context.window = window::Id::unique(),
        }
        for command in [
            Command::Open,
            Command::PlayPause,
            Command::SeekBy(15.0),
            Command::Lyrics,
        ] {
            let _ = app.update(Message::Desktop(desktop::Event::TouchBar(Event {
                context,
                command,
            })));
        }
        assert!(receiver.try_recv().is_err());
        assert!(!app.config.desktop_lyrics.enabled);
    }
}

#[test]
fn touchbar_lyrics_reuses_go_preferences() {
    let (mut app, mut receiver) = playing();
    app.handshake
        .capabilities
        .push("settings_preferences".into());
    press(&mut app, Command::Lyrics);
    assert!(matches!(receiver.try_recv().unwrap().request,
        ipc::Request::SetPreference { key: "desktop_lyrics.enabled", value } if value == true));
    assert!(!app.config.desktop_lyrics.enabled);
}
