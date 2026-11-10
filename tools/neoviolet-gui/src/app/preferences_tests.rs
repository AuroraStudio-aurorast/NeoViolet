use super::*;
use crate::actions::Action;

fn ready() -> App {
    let mut app = app();
    app.dialog = None;
    app.pending.clear();
    app.running = true;
    app.ipc_connected = true;
    app.handshake.capabilities = vec![
        "startup".into(),
        "settings_preferences".into(),
        "settings_navigation".into(),
    ];
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    app
}

#[test]
fn saved_preferences_refresh_without_restarting_player_or_overwriting_geometry() {
    let mut app = ready();
    app.config.window_width = 729;
    app.config.desktop_lyrics.position = Some(crate::lyrics::Position { x: 299.0, y: 710.0 });
    let session = app.session;
    let preferences=serde_json::from_value(serde_json::json!({"gui":{"font_size":20,"opacity":0.8},"desktop_lyrics":{"enabled":false,"font_size":28}})).unwrap();
    let _ = app.apply_preferences(2, preferences);
    assert_eq!(app.config.font_size, 20);
    assert_eq!(app.config.window_width, 729);
    assert_eq!(app.config.desktop_lyrics.font_size, 28);
    assert_eq!(app.config.desktop_lyrics.position.unwrap().x, 299.0);
    assert_eq!(app.session, session);
    assert!(app.running);
    let _ = app.apply_preferences(1, Default::default());
    assert_eq!(app.config.font_size, 20, "stale preferences applied");
}

#[test]
fn startup_waits_for_go_preferences_and_never_pushes_bootstrap_preferences_back() {
    let mut app = ready();
    app.config.desktop_lyrics.enabled = false;
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.restore_ready();
    assert!(!app.lyrics_restored);
    let _ = app.apply_preferences(1, Default::default());
    assert!(app.config.desktop_lyrics.enabled);
    assert!(app.lyrics_restored);
    assert!(
        receiver.try_recv().is_err(),
        "GUI overwrote the Go preference during startup"
    );
}

#[test]
fn gui_shortcuts_send_preference_intents_without_mutating_live_config() {
    let mut app = ready();
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let old = app.config.font_size;
    let _ = app.action(Action::ZoomIn);
    assert_eq!(app.config.font_size, old);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::SetPreference {
            key: "gui.font_size",
            ..
        }
    ));
    let _ = app.action(Action::LyricsSettings);
    assert!(app.lyrics.settings_window.is_none());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::OpenSettingsSection("desktop_lyrics")
    ));
    let enabled = app.config.desktop_lyrics.enabled;
    let _ = app.action(Action::DesktopLyrics);
    assert_eq!(app.config.desktop_lyrics.enabled, enabled);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::SetPreference {
            key: "desktop_lyrics.enabled",
            ..
        }
    ));
}

#[test]
fn repeated_zoom_coalesces_intent_and_waits_for_save_receipt() {
    let mut app = ready();
    app.handshake.capabilities.push("results".into());
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.action(Action::ZoomIn);
    assert!(
        receiver.try_recv().is_err(),
        "concurrent writes must be coalesced"
    );
    let preferences = serde_json::from_value(
        serde_json::json!({"gui":{"font_size":16},"desktop_lyrics":{"enabled":false}}),
    )
    .unwrap();
    let _ = app.apply_preferences(1, preferences);
    let _ = app.command_result(first.id, "succeeded".into(), String::new());
    let next = receiver.try_recv().unwrap();
    assert!(
        matches!(next.request,ipc::Request::SetPreference{key:"gui.font_size",value} if value==20)
    );
    assert!(receiver.try_recv().is_err());
}

#[test]
fn failed_preference_save_clears_queued_intents() {
    let mut app = ready();
    app.handshake.capabilities.push("results".into());
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.command_result(first.id, "failed".into(), "disk unavailable".into());
    assert!(receiver.try_recv().is_err());
    assert_eq!(app.config.font_size, 14);
    let _ = app.action(Action::ZoomIn);
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::SetPreference{value,..} if value==16)
    );
}

#[test]
fn preference_refresh_during_picker_applies_overlay_when_picker_finishes() {
    let mut app = ready();
    app.handshake.capabilities.push("file_picker".into());
    let _ = app.lyrics.show(&app.config.desktop_lyrics);
    assert!(app.lyrics.window.is_some());
    app.lyrics_restored = true;
    let _ = app.setup_picker("audio".into(), "pick".into());
    assert!(app.picker.is_some());
    let prefs =
        serde_json::from_value(serde_json::json!({"desktop_lyrics":{"enabled":false}})).unwrap();
    let _ = app.apply_preferences(1, prefs);
    let _ = app.setup_picked(app.session, app.connection_epoch, "pick".into(), Ok(None));
    assert!(
        app.lyrics.window.is_none(),
        "saved hidden state was lost during native picker"
    );
}

#[test]
fn quit_waits_for_the_latest_coalesced_preference() {
    let mut app = ready();
    app.handshake
        .capabilities
        .extend(["results".into(), "request_quit".into()]);
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.action(Action::Quit);
    assert!(receiver.try_recv().is_err());
    let _ = app.command_result(first.id, "succeeded".into(), String::new());
    let last = receiver.try_recv().unwrap();
    assert!(matches!(last.request,ipc::Request::SetPreference{value,..} if value==18));
    let _ = app.command_result(last.id, "succeeded".into(), String::new());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::CheckQuit
    ));
}

#[test]
fn disconnect_discards_pending_preference_intents() {
    let mut app = ready();
    app.handshake.capabilities.push("results".into());
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.ipc_event(app.session, ipc::Event::Disconnected("lost".into()));
    let _ = app.command_result(first.id, "succeeded".into(), String::new());
    assert!(app.intended_preference("gui.font_size").is_none());
    assert!(receiver.try_recv().is_err());
}

#[test]
fn queued_zoom_survives_a_native_picker_until_input_returns() {
    let mut app = ready();
    app.handshake
        .capabilities
        .extend(["results".into(), "file_picker".into()]);
    let (sender, mut receiver) = tokio::sync::mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.setup_picker("audio".into(), "pick".into());
    let _ = app.command_result(first.id, "succeeded".into(), String::new());
    assert!(receiver.try_recv().is_err());
    assert!(app.intended_preference("gui.font_size").is_some());
    let _ = app.setup_picked(app.session, app.connection_epoch, "pick".into(), Ok(None));
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::FilePicked { .. }
    ));
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::SetPreference{value,..} if value==18)
    );
}
