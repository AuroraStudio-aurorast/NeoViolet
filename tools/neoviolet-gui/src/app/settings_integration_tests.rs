use super::*;
use crate::actions::Action;

#[test]
fn lyrics_settings_always_navigate_to_tui_when_navigation_is_supported() {
    let mut app = app();
    app.dialog = None;
    app.ipc_connected = true;
    app.pending.clear();
    app.handshake.capabilities = vec!["settings_navigation".into()];
    let (sender, mut receiver) = mpsc::channel(8);
    app.connection = Some(sender);
    let before = app.config.desktop_lyrics.font_size;
    let _ = app.action(Action::LyricsSettings);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::OpenSettings
    ));
    assert_eq!(app.config.desktop_lyrics.font_size, before);
}

#[test]
fn unsupported_lyrics_settings_explain_update_without_opening_local_editor() {
    let mut app = app();
    app.dialog = None;
    app.ipc_connected = true;
    let _ = app.action(Action::LyricsSettings);
    assert!(
        app.notice
            .as_ref()
            .is_some_and(|notice| notice.contains("Settings"))
    );
}

#[test]
fn restarting_player_preserves_live_window_state_and_reloads_preferences() {
    let mut app = app();
    app.dialog = None;
    app.config.window_width = 729;
    app.config.window_height = 556;
    app.config.desktop_lyrics.position = Some(crate::lyrics::Position { x: 299.0, y: 710.0 });
    let mut reload = config::Config {
        font_size: 20,
        neoviolet_path: Some("/no/such/neoviolet-test-player".into()),
        ..Default::default()
    };
    reload.desktop_lyrics.font_size = 28;
    let _ = app.restart_with_config(reload, None);
    assert_eq!(
        (app.config.window_width, app.config.window_height),
        (729, 556)
    );
    assert_eq!(app.config.desktop_lyrics.position.unwrap().x, 299.0);
    assert_eq!(app.config.font_size, 20);
    assert_eq!(app.config.desktop_lyrics.font_size, 28);
}

fn feedback_app() -> (App, mpsc::Receiver<ipc::Command>) {
    let mut app = app();
    app.dialog = None;
    app.ipc_connected = true;
    app.pending.clear();
    app.config.desktop_lyrics.enabled = false;
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    app.handshake.capabilities = vec!["settings_preferences".into(), "preferences_feedback".into()];
    let (sender, receiver) = mpsc::channel(16);
    app.connection = Some(sender);
    (app, receiver)
}

fn quiet_preferences() -> config::Preferences {
    let mut preferences = config::Preferences::default();
    preferences.desktop_lyrics.enabled = false;
    preferences
}

// Drive actual completion messages from the task graph. These tests keep the
// overlay disabled, so no native window or screenshot is fabricated.
async fn complete_feedback(app: &mut App, task: Task<Message>) {
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
    let mut tasks = VecDeque::from([task]);
    let mut count = 0;
    while let Some(task) = tasks.pop_front() {
        count += 1;
        assert!(count < 64, "refresh graph did not settle");
        if let Some(mut stream) = iced_runtime::task::into_stream(task) {
            while let Some(action) = stream.next().await {
                match action {
                    iced_runtime::Action::Output(message) => tasks.push_back(app.update(message)),
                    iced_runtime::Action::Window(window::Action::Run(_, callback)) => {
                        callback(&NoNativeWindow);
                    }
                    iced_runtime::Action::Window(window::Action::GainFocus(_))
                    | iced_runtime::Action::Widget(_) => {}
                    _ => panic!("unexpected native action in disabled-overlay refresh"),
                }
            }
        }
    }
}

#[tokio::test]
async fn applied_preferences_report_revision_after_work_without_a_receipt_loop() {
    let (mut app, mut receiver) = feedback_app();
    let task = app.apply_preferences(3, quiet_preferences());
    assert!(
        receiver.try_recv().is_err(),
        "feedback sent before refresh ran"
    );
    complete_feedback(&mut app, task).await;
    let request = receiver.try_recv().unwrap();
    assert!(matches!(
        request.request,
        ipc::Request::PreferencesApplied { revision: 3, .. }
    ));
    assert!(
        request.id.is_empty(),
        "feedback must not create another receipt timer"
    );
    let task = app.apply_preferences(2, quiet_preferences());
    complete_feedback(&mut app, task).await;
    assert!(receiver.try_recv().is_err());
}

#[tokio::test]
async fn player_path_feedback_remains_deferred_until_that_program_is_started() {
    let (mut app, mut receiver) = feedback_app();
    let preferences=serde_json::from_value(serde_json::json!({"gui":{"neoviolet_path":"/tmp/new-player"},"desktop_lyrics":{"enabled":false}})).unwrap();
    let task = app.apply_preferences(1, preferences);
    complete_feedback(&mut app, task).await;
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::PreferencesApplied{status:"restart_required",restart_required,..} if restart_required==vec!["gui.neoviolet_path"])
    );
}

#[test]
fn local_position_reset_preserves_all_go_owned_preferences() {
    let (mut app, mut receiver) = feedback_app();
    app.config.desktop_lyrics.position = Some(crate::lyrics::Position { x: 299.0, y: 710.0 });
    let before = serde_json::to_value(&app.config.desktop_lyrics).unwrap();
    let _ = app.action(Action::ResetLyricsPosition);
    assert!(app.config.desktop_lyrics.position.is_none());
    assert_eq!(
        serde_json::to_value(&app.config.desktop_lyrics).unwrap(),
        before
    );
    assert!(receiver.try_recv().is_err());
}

#[tokio::test]
async fn deferred_program_change_is_reported_again_after_other_preferences_change() {
    let (mut app, mut receiver) = feedback_app();
    for (revision, size) in [(1, 14), (2, 20)] {
        let prefs=serde_json::from_value(serde_json::json!({"gui":{"neoviolet_path":"/tmp/new-player","font_size":size},"desktop_lyrics":{"enabled":false}})).unwrap();
        let task = app.apply_preferences(revision, prefs);
        complete_feedback(&mut app, task).await;
        assert!(
            matches!(receiver.try_recv().unwrap().request,ipc::Request::PreferencesApplied{status:"restart_required",restart_required,..} if restart_required.contains(&"gui.neoviolet_path"))
        );
    }
}

#[tokio::test]
async fn picker_defers_gui_application_feedback_until_refresh_after_return() {
    let (mut app, mut receiver) = feedback_app();
    app.handshake
        .capabilities
        .extend(["file_picker".into(), "settings_file_picker".into()]);
    let _ = app.setup_picker("audio".into(), "p".into());
    let task = app.apply_preferences(1, quiet_preferences());
    complete_feedback(&mut app, task).await;
    assert!(receiver.try_recv().is_err());
    let _ = app.setup_picked(app.session, app.connection_epoch, "p".into(), Ok(None));
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::FilePicked { .. }
    ));
    let task = app.finish_picker_reply("p", "cancelled", "").unwrap();
    assert!(receiver.try_recv().is_err(), "refresh has not run yet");
    complete_feedback(&mut app, task).await;
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::PreferencesApplied { revision: 1, .. }
    ));
}

#[tokio::test]
async fn overlapping_preference_work_acknowledges_only_latest_revision() {
    let (mut app, mut receiver) = feedback_app();
    let first = app.apply_preferences(1, quiet_preferences());
    let second = app.apply_preferences(2, quiet_preferences());
    complete_feedback(&mut app, first).await;
    assert!(
        receiver.try_recv().is_err(),
        "second refresh is still pending"
    );
    complete_feedback(&mut app, second).await;
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::PreferencesApplied { revision: 2, .. }
    ));
    assert!(receiver.try_recv().is_err());
}

#[test]
fn opening_overlay_keeps_feedback_pending_through_spawned_lifecycle_work() {
    let (mut app, mut receiver) = feedback_app();
    let _root = app.apply_preferences(1, Default::default());
    let id = app.lyrics.window.unwrap();
    let _child = app.update(Message::PreferencesLyrics {
        session: app.session,
        epoch: app.connection_epoch,
        event: lyrics_window::Event::Opened(id),
    });
    let _ = app.update(Message::PreferencesWorkDone {
        session: app.session,
        epoch: app.connection_epoch,
    });
    assert!(
        receiver.try_recv().is_err(),
        "native size/mask/toolbar tasks are still pending"
    );
}

#[tokio::test]
async fn lyric_toolbar_completion_does_not_focus_main_while_native_picker_owns_input() {
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
    let (mut app, _) = feedback_app();
    app.handshake
        .capabilities
        .extend(["file_picker".into(), "settings_file_picker".into()]);
    app.lyrics.window = Some(window::Id::unique());
    let toolbar = window::Id::unique();
    app.lyrics.toolbar_window = Some(toolbar);
    let _ = app.setup_picker("audio".into(), "audio".into());
    let task = app.lyrics_event(lyrics_window::Event::Opened(toolbar));
    let mut stream = iced_runtime::task::into_stream(task).unwrap();
    while let Some(action) = stream.next().await {
        match action {
            iced_runtime::Action::Window(window::Action::Run(_, callback)) => {
                callback(&NoNativeWindow);
            }
            iced_runtime::Action::Window(window::Action::GainFocus(id))
                if id == app.main_window =>
            {
                panic!("overlay stole native picker focus")
            }
            _ => {}
        }
    }
}

#[cfg(target_os = "macos")]
#[tokio::test]
async fn opening_lyrics_does_not_request_key_window_changes() {
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
    for toolbar_open in [false, true] {
        let (mut app, _) = feedback_app();
        let root = window::Id::unique();
        let toolbar = window::Id::unique();
        app.lyrics.window = Some(root);
        app.lyrics.toolbar_window = toolbar_open.then_some(toolbar);
        let task = app.lyrics_event(lyrics_window::Event::Opened(if toolbar_open {
            toolbar
        } else {
            root
        }));
        let mut native_operations = 0;
        if let Some(mut stream) = iced_runtime::task::into_stream(task) {
            while let Some(action) = stream.next().await {
                match action {
                    iced_runtime::Action::Window(window::Action::GainFocus(_)) => {
                        panic!("opening lyrics must preserve the current key window")
                    }
                    iced_runtime::Action::Window(window::Action::Run(_, callback)) => {
                        native_operations += 1;
                        callback(&NoNativeWindow);
                    }
                    iced_runtime::Action::Window(window::Action::GetPosition(_, sender)) => {
                        let _ = sender.send(Some(iced::Point::ORIGIN));
                    }
                    iced_runtime::Action::Window(window::Action::GetSize(_, sender)) => {
                        let _ = sender.send(iced::Size::new(300.0, 50.0));
                    }
                    _ => {}
                }
            }
        }
        assert!(
            native_operations >= 2,
            "presentation must run in addition to passthrough, even without desktop menus"
        );
    }
}
