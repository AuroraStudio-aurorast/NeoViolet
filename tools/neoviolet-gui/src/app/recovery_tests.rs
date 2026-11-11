use super::*;

fn stopped() -> App {
    let mut app = app();
    app.running = false;
    app.dialog = Some(Dialog::Error("old failure".into()));
    app.pending.clear();
    app.config.neoviolet_path = Some("/old/missing/player".into());
    app
}

#[test]
fn recovery_picker_is_exclusive_and_cancel_keeps_config_and_failure() {
    let mut app = stopped();
    let _ = app.choose_player();
    assert!(app.recovery.picker);
    assert!(!app.basic_actions_available());
    let generation = app.recovery.generation;
    let _ = app.choose_player();
    assert_eq!(app.recovery.generation, generation);
    let _ = app.player_chosen(generation - 1, Ok(None));
    assert!(app.recovery.picker);
    let _ = app.player_chosen(generation, Ok(None));
    assert!(!app.recovery.picker);
    assert_eq!(
        app.config.neoviolet_path.as_deref(),
        Some("/old/missing/player")
    );
    assert!(matches!(app.dialog,Some(Dialog::Error(ref text)) if text=="old failure"));
    assert!(app.recovery.path.is_none());
}

#[test]
fn selected_player_is_temporary_until_go_accepts_it() {
    let mut app = stopped();
    let _ = app.choose_player();
    let generation = app.recovery.generation;
    let root = tempfile::tempdir().unwrap();
    let selected = root.path().join("missing-player.exe");
    let _ = app.player_chosen(generation, Ok(Some(selected.clone())));
    assert_eq!(
        app.config.neoviolet_path.as_deref(),
        Some("/old/missing/player")
    );
    assert_eq!(app.running_player_path.as_deref(), selected.to_str());
    assert!(
        app.dialog.is_some(),
        "failed selected program must remain recoverable"
    );
}

#[test]
fn ready_recovery_submits_only_one_go_owned_path_change() {
    let mut app = stopped();
    app.running = true;
    app.dialog = None;
    app.ipc_connected = true;
    app.preferences_revision = 1;
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    app.handshake.capabilities = vec![
        "player_recovery".into(),
        "settings_preferences".into(),
        "results".into(),
    ];
    app.recovery.path = Some("/new/player".into());
    app.recovery.original_path = app.config.neoviolet_path.clone();
    let (sender, mut receiver) = mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.save_recovery_path();
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::SetPreference{key:"gui.neoviolet_path",value} if value=="/new/player")
    );
    let _ = app.save_recovery_path();
    assert!(receiver.try_recv().is_err());
    assert_eq!(
        app.config.neoviolet_path.as_deref(),
        Some("/old/missing/player")
    );
}

#[test]
fn authoritative_path_change_supersedes_temporary_override() {
    for desired in ["/new/player", "/another/user-selected/player"] {
        let mut app = stopped();
        app.recovery.path = Some("/new/player".into());
        app.recovery.original_path = app.config.neoviolet_path.clone();
        app.config.neoviolet_path = Some(desired.into());
        app.reconcile_recovery_path();
        assert!(app.recovery.path.is_none());
    }
}

#[test]
fn closed_recovery_rejects_late_selection_and_does_not_restart() {
    let mut app = stopped();
    let _ = app.choose_player();
    let generation = app.recovery.generation;
    let _ = app.update(Message::ConfirmQuit);
    let _ = app.player_chosen(generation, Ok(Some("/tmp/ignored-player".into())));
    assert!(!app.running);
    assert!(app.recovery.path.is_none());
}

#[test]
fn old_player_does_not_receive_an_unsupported_path_write() {
    let mut app = stopped();
    app.running = true;
    app.dialog = None;
    app.ipc_connected = true;
    app.startup = super::super::startup::Stage::Legacy;
    app.recovery.path = Some("/new/player".into());
    app.recovery.original_path = app.config.neoviolet_path.clone();
    let (sender, mut receiver) = mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.save_recovery_path();
    assert!(receiver.try_recv().is_err());
    assert!(
        app.notice
            .as_ref()
            .is_some_and(|text| text.contains("temporarily"))
    );
    assert_eq!(
        app.config.neoviolet_path.as_deref(),
        Some("/old/missing/player")
    );
}

#[test]
fn recovery_waits_for_the_authoritative_snapshot_and_preserves_launch_arguments() {
    let mut app = stopped();
    app.running = true;
    app.dialog = None;
    app.ipc_connected = true;
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    app.handshake.capabilities = vec!["player_recovery".into(), "settings_preferences".into()];
    app.recovery.path = Some("/new/player".into());
    app.recovery.original_path = app.config.neoviolet_path.clone();
    let (sender, mut receiver) = mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.save_recovery_path();
    assert!(receiver.try_recv().is_err());
    assert!(!app.recovery.attempted);
    let mut app = stopped();
    app.arguments = vec!["--xdg-config".into(), "/music/a song 雪.flac".into()];
    let _ = app.choose_player();
    let generation = app.recovery.generation;
    let root = tempfile::tempdir().unwrap();
    let _ = app.player_chosen(generation, Ok(Some(root.path().join("missing-player.exe"))));
    assert_eq!(app.arguments, vec!["--xdg-config", "/music/a song 雪.flac"]);
}

#[test]
fn restart_cannot_bypass_an_active_recovery_chooser() {
    let mut app = stopped();
    let _ = app.choose_player();
    let session = app.session;
    let _ = app.update(Message::Restart);
    assert!(app.recovery.picker);
    assert_eq!(app.session, session);
}

#[test]
fn matching_already_saved_path_drops_the_temporary_override() {
    let mut app = stopped();
    app.recovery.path = app.config.neoviolet_path.clone();
    app.recovery.original_path = app.config.neoviolet_path.clone();
    app.reconcile_recovery_path();
    assert!(app.recovery.path.is_none());
}

#[tokio::test]
async fn missing_native_parent_returns_to_recoverable_error() {
    use iced::futures::StreamExt;
    use iced::window::raw_window_handle::{
        DisplayHandle, HandleError, HasDisplayHandle, HasWindowHandle, WindowHandle,
    };
    struct NoNativeWindow;
    impl HasWindowHandle for NoNativeWindow {
        fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
            Err(HandleError::Unavailable)
        }
    }
    impl HasDisplayHandle for NoNativeWindow {
        fn display_handle(&self) -> Result<DisplayHandle<'_>, HandleError> {
            Err(HandleError::Unavailable)
        }
    }
    let mut app = stopped();
    let task = app.choose_player();
    let mut stream = iced_runtime::task::into_stream(task).unwrap();
    let mut returned = false;
    while let Some(action) = stream.next().await {
        match action {
            iced_runtime::Action::Window(window::Action::Run(id, callback)) => {
                assert_eq!(id, app.main_window);
                callback(&NoNativeWindow);
            }
            iced_runtime::Action::Output(message @ Message::PlayerChosen { .. }) => {
                let _ = app.update(message);
                returned = true;
            }
            _ => {}
        }
    }
    assert!(returned && !app.recovery.picker && !app.running);
    assert!(
        matches!(app.dialog,Some(Dialog::Error(ref text)) if text.contains("Could not choose"))
    );
}

#[test]
fn recovery_deferred_by_about_is_saved_once_after_dismissal() {
    let mut app = stopped();
    app.running = true;
    app.dialog = Some(Dialog::About);
    app.ipc_connected = true;
    app.preferences_revision = 1;
    app.config.desktop_lyrics.enabled = false;
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    app.handshake.capabilities = vec![
        "player_recovery".into(),
        "settings_preferences".into(),
        "results".into(),
    ];
    app.recovery.path = Some("/new/player".into());
    app.recovery.original_path = app.config.neoviolet_path.clone();
    let (sender, mut receiver) = mpsc::channel(8);
    app.connection = Some(sender);
    let _ = app.restore_ready();
    assert!(receiver.try_recv().is_err());
    assert!(!app.recovery.attempted);
    let _ = app.update(Message::Dismiss);
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::SetPreference{key:"gui.neoviolet_path",value} if value=="/new/player")
    );
    let _ = app.update(Message::Dismiss);
    assert!(receiver.try_recv().is_err());
}
