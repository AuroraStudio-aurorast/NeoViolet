use super::*;

fn starting() -> (App, mpsc::Receiver<ipc::Command>) {
    let mut app = app();
    app.dialog = None;
    app.pending.clear();
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Handshake(ipc::Handshake {
            protocol: 1,
            capabilities: vec!["startup".into(), "playback".into(), "state".into()],
            ..Default::default()
        }),
    ));
    let (sender, receiver) = mpsc::channel(32);
    let _ = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    (app, receiver)
}

#[test]
fn authenticated_startup_waits_for_ready_before_subscribing_or_restoring_lyrics() {
    let (mut app, mut receiver) = starting();
    app.config.desktop_lyrics.enabled = true;
    assert!(
        receiver.try_recv().is_err(),
        "authentication must not subscribe during setup"
    );
    assert!(app.lyrics.window.is_none());
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        has_track: true,
        ..Default::default()
    });
    assert!(
        !app.actions_available(),
        "a snapshot cannot bypass the startup phase"
    );
}

#[test]
fn disabled_player_actions_and_open_intents_have_no_effect_or_pending_files() {
    let (mut app, mut receiver) = starting();
    while receiver.try_recv().is_ok() {}
    for message in [
        Message::Desktop(desktop::Event::Files(vec!["/tmp/os.flac".into()])),
        Message::Native(
            app.main_window,
            iced::Event::Window(window::Event::FileDropped("/tmp/drop.flac".into())),
        ),
        Message::Picked(Some("/tmp/late-picker.flac".into())),
        Message::Desktop(desktop::Event::Action(Action::DesktopLyrics)),
        Message::Titlebar(titlebar::Message::Action(Action::LyricsSettings)),
    ] {
        let _ = app.update(message);
    }
    assert!(app.pending.is_empty());
    assert!(receiver.try_recv().is_err());
    assert!(app.dialog.is_none());
}

fn phase(app: &mut App, name: &str) {
    let incoming: ipc::Incoming =
        serde_json::from_value(serde_json::json!({"type":"phase", "phase":name})).unwrap();
    let _ = app.update(Message::Ipc(2, ipc::Event::Received(incoming)));
}

#[test]
fn ready_restores_saved_lyrics_once_and_runtime_loading_keeps_actions_available() {
    let (mut app, mut receiver) = starting();
    app.config.desktop_lyrics.enabled = true;
    for name in ["starting", "wizard", "initial_loading"] {
        phase(&mut app, name);
        assert!(!app.actions_available());
        assert!(app.lyrics.window.is_none());
        assert!(receiver.try_recv().is_err());
    }
    phase(&mut app, "ready");
    assert!(app.actions_available());
    let overlay = app
        .lyrics
        .window
        .expect("saved lyrics preference restored after ready");
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(true)
    ));
    phase(&mut app, "ready");
    assert_eq!(app.lyrics.window, Some(overlay));
    assert!(receiver.try_recv().is_err());
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        loading: true,
        ..Default::default()
    });
    assert!(app.actions_available());
    let _ = app.update(Message::Ipc(2, ipc::Event::Disconnected("test".into())));
    assert!(!app.actions_available());
    assert!(app.lyrics.window.is_none());
    assert!(app.config.desktop_lyrics.enabled);
    let (sender, mut reconnected) = mpsc::channel(32);
    let _ = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    assert!(!app.actions_available());
    assert!(reconnected.try_recv().is_err());
    phase(&mut app, "ready");
    assert!(app.lyrics.window.is_some());
    assert_ne!(app.lyrics.window, Some(overlay));
    assert!(matches!(
        reconnected.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(true)
    ));
}

#[test]
fn restart_closes_previous_lyrics_before_new_child_readiness() {
    let (mut app, _) = starting();
    app.config.desktop_lyrics.enabled = true;
    phase(&mut app, "ready");
    assert!(app.lyrics.window.is_some());
    app.config.neoviolet_path = Some("/definitely/missing/neoviolet".into());
    let _ = app.start();
    assert!(
        app.lyrics.window.is_none(),
        "a restart must not leave a saved overlay visible over setup"
    );
    assert!(app.config.desktop_lyrics.enabled);
    assert!(!app.actions_available());
}

#[test]
fn basic_window_mode_commands_keep_player_gate_closed() {
    let (mut app, _) = starting();
    let task = app.update(Message::SetMode(window::Mode::Fullscreen));
    assert!(iced_runtime::task::into_stream(task).is_some());
    assert!(!app.actions_available());
    let task = app.titlebar_event(titlebar::Message::WindowAction(Action::Open));
    assert!(
        iced_runtime::task::into_stream(task).is_none(),
        "only window-manager operations may bypass readiness"
    );
}

#[test]
fn wizard_picker_keeps_one_dialog_and_replies_to_only_current_session_epoch_and_request() {
    let (mut app, mut receiver) = starting();
    app.handshake.capabilities.push("file_picker".into());
    phase(&mut app, "wizard");
    let request = |id: &str| {
        ipc::Event::Received(ipc::Incoming::PickFile {
            purpose: "soundfont".into(),
            request_id: id.into(),
        })
    };
    let task = app.ipc_event(2, request("pick-1"));
    assert!(iced_runtime::task::into_stream(task).is_some());
    let _ = app.ipc_event(2, request("pick-1"));
    assert!(
        receiver.try_recv().is_err(),
        "duplicate active request still waits for its chooser"
    );
    let _ = app.ipc_event(2, request("pick-2"));
    assert!(
        matches!(receiver.try_recv().unwrap().request, ipc::Request::FilePicked { request_id, status, .. } if request_id == "pick-2" && status == "failed")
    );
    for (session, epoch, request_id) in [
        (1, app.connection_epoch, "pick-1"),
        (2, 0, "pick-1"),
        (2, app.connection_epoch, "other"),
    ] {
        let _ = app.update(Message::SetupPicked {
            session,
            epoch,
            request_id: request_id.into(),
            result: Ok(Some("/tmp/stale.sf2".into())),
        });
        assert!(receiver.try_recv().is_err());
        assert!(app.picker.is_some());
    }
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: app.connection_epoch,
        request_id: "pick-1".into(),
        result: Ok(None),
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request, ipc::Request::FilePicked { status, path: None, .. } if status == "cancelled")
    );
    assert!(app.picker.is_none());
    let _ = app.ipc_event(2, request("pick-3"));
    let old_epoch = app.connection_epoch;
    let _ = app.ipc_event(2, ipc::Event::Disconnected("test".into()));
    let (sender, mut next) = mpsc::channel(32);
    let _ = app.ipc_event(2, ipc::Event::Connected(sender));
    phase(&mut app, "wizard");
    let _ = app.ipc_event(2, request("pick-4"));
    assert!(
        app.picker.is_some(),
        "the old physical dialog remains active"
    );
    let reply = next
        .try_recv()
        .expect("a replacement connection must receive an automatic busy reply");
    assert!(
        matches!(reply.request, ipc::Request::FilePicked { request_id, status, .. } if request_id == "pick-4" && status == "failed")
    );
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: old_epoch,
        request_id: "pick-3".into(),
        result: Ok(Some("/tmp/old.sf2".into())),
    });
    assert!(next.try_recv().is_err());
    assert!(app.picker.is_none());
    let _ = app.ipc_event(2, request("pick-5"));
    phase(&mut app, "ready");
    assert!(next.try_recv().is_err());
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: app.connection_epoch,
        request_id: "pick-5".into(),
        result: Ok(Some("/tmp/late.sf2".into())),
    });
    assert!(matches!(
        next.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(_)
    ));
    assert!(next.try_recv().is_err());
}

#[tokio::test]
async fn lyrics_window_lifecycle_finishes_while_audio_picker_is_open() {
    use iced::futures::StreamExt;
    let (mut app, _) = starting();
    app.handshake.capabilities.push("file_picker".into());
    app.config.desktop_lyrics.enabled = true;
    phase(&mut app, "ready");
    let lyrics = app.lyrics.window.expect("ready restores the lyrics window");
    let _ = app.setup_picker("audio".into(), "audio-1".into());
    assert!(!app.actions_available());
    let blocked = app.lyrics_event(lyrics_window::Event::PlayPause);
    assert!(iced_runtime::task::into_stream(blocked).is_none());
    let task = app.lyrics_event(lyrics_window::Event::Opened(lyrics));
    if let Some(mut stream) = iced_runtime::task::into_stream(task) {
        let action = tokio::time::timeout(std::time::Duration::from_secs(1), stream.next())
            .await
            .expect("lyrics lifecycle task must produce an immediate action")
            .expect("lyrics lifecycle task unexpectedly ended");
        assert!(
            !matches!(
                action,
                iced_runtime::Action::Window(iced::window::Action::Close(id)) if id == lyrics
            ),
            "a valid lyrics Opened event must not be closed while the picker is active"
        );
    }
    assert_eq!(app.lyrics.window, Some(lyrics));
}

#[test]
fn window_manager_close_and_caption_controls_remain_available() {
    let (mut app, _) = starting();
    let task = app.update(Message::Native(
        app.main_window,
        iced::Event::Window(window::Event::CloseRequested),
    ));
    assert!(app.dialog.is_some());
    assert!(iced_runtime::task::into_stream(task).is_some());
    app.dialog = None;
    assert!(
        iced_runtime::task::into_stream(
            app.titlebar_event(titlebar::Message::WindowAction(Action::Minimize))
        )
        .is_some()
    );
    assert!(
        iced_runtime::task::into_stream(
            app.titlebar_event(titlebar::Message::Resize(window::Direction::SouthEast))
        )
        .is_some()
    );
}

#[test]
fn player_menu_actions_and_shortcuts_stay_blocked_during_setup() {
    let actions = [
        Action::Open,
        Action::DesktopLyrics,
        Action::LyricsSettings,
        Action::UnlockLyrics,
    ];
    for name in ["starting", "wizard", "initial_loading"] {
        let (mut app, mut receiver) = starting();
        phase(&mut app, name);
        for action in actions {
            assert!(
                iced_runtime::task::into_stream(app.action(action)).is_none(),
                "{name}: {action:?}"
            );
        }
        for key in ["o", "l"] {
            let modifiers = if key == "l" {
                Modifiers::COMMAND | Modifiers::SHIFT
            } else {
                Modifiers::COMMAND
            };
            let task = app.update(Message::Native(
                app.main_window,
                modal_key(Key::Character(key.into()), modifiers),
            ));
            assert!(
                iced_runtime::task::into_stream(task).is_none(),
                "shortcut {key} in {name}"
            );
        }
        assert!(app.dialog.is_none());
        assert!(app.pending.is_empty());
        assert!(receiver.try_recv().is_err());
    }
}

#[test]
fn soundfont_dialog_builder_has_fixed_filter_and_explicit_parent() {
    use iced::window::raw_window_handle::{
        DisplayHandle, HandleError, HasDisplayHandle, HasWindowHandle, RawDisplayHandle,
        RawWindowHandle, WindowHandle, XlibDisplayHandle, XlibWindowHandle,
    };
    struct Parent;
    impl HasWindowHandle for Parent {
        fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
            // SAFETY: The builder only stores this handle; this test never opens a dialog.
            Ok(unsafe {
                WindowHandle::borrow_raw(RawWindowHandle::Xlib(XlibWindowHandle::new(123)))
            })
        }
    }
    impl HasDisplayHandle for Parent {
        fn display_handle(&self) -> Result<DisplayHandle<'_>, HandleError> {
            // SAFETY: The builder only stores this handle; no display operation occurs.
            Ok(unsafe {
                DisplayHandle::borrow_raw(RawDisplayHandle::Xlib(XlibDisplayHandle::new(None, 0)))
            })
        }
    }
    let dialog = super::super::startup::soundfont_dialog(&Parent).unwrap();
    let settings = format!("{dialog:?}");
    assert!(settings.contains("extensions: [\"sf2\"]"), "{settings}");
    assert!(settings.contains("parent: Some(Xlib"), "{settings}");
    assert!(settings.contains("parent_display: Some(Xlib"), "{settings}");
    assert!(settings.contains("Choose SoundFont"), "{settings}");
}

#[test]
fn stale_phases_and_disabled_retry_tasks_cannot_enable_or_replay_files() {
    let (mut app, mut receiver) = starting();
    app.pending.push_back("/tmp/previous-open.flac".into());
    let task = app.update(Message::FlushPending {
        session: 2,
        epoch: app.connection_epoch,
    });
    assert!(iced_runtime::task::into_stream(task).is_none());
    assert!(receiver.try_recv().is_err());
    phase(&mut app, "wizard");
    assert!(app.pending.is_empty());
    let _ = app.update(Message::Ipc(
        1,
        ipc::Event::Received(ipc::Incoming::Phase {
            phase: ipc::StartupPhase::Ready,
        }),
    ));
    assert!(!app.actions_available());
    phase(&mut app, "future_phase");
    assert!(!app.actions_available());
    let _ = app.update(Message::Dismiss);
    assert!(receiver.try_recv().is_err());
    phase(&mut app, "ready");
    let _ = receiver.try_recv().unwrap();
    let _ = app.ipc_event(2, ipc::Event::Disconnected("test".into()));
    let _ = app.open("/tmp/disconnected.flac".into());
    phase(&mut app, "ready");
    assert!(!app.actions_available());
    assert!(app.pending.is_empty());
}

#[test]
fn legacy_players_ignore_startup_messages_and_keep_original_availability() {
    let mut app = app();
    app.dialog = None;
    app.pending.clear();
    let (sender, mut receiver) = mpsc::channel(32);
    let _ = app.ipc_event(2, ipc::Event::Connected(sender));
    assert!(app.actions_available());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(_)
    ));
    phase(&mut app, "wizard");
    assert!(app.actions_available());
    let _ = app.open("/tmp/legacy.flac".into());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Open(_)
    ));
}

#[tokio::test]
async fn setup_picker_uses_main_window_and_missing_parent_reports_failure() {
    use iced::futures::StreamExt;
    use iced::window::raw_window_handle::{
        DisplayHandle, HandleError, HasDisplayHandle, HasWindowHandle, WindowHandle,
    };
    struct MissingParent;
    impl HasWindowHandle for MissingParent {
        fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
            Err(HandleError::Unavailable)
        }
    }
    impl HasDisplayHandle for MissingParent {
        fn display_handle(&self) -> Result<DisplayHandle<'_>, HandleError> {
            Err(HandleError::Unavailable)
        }
    }
    let (mut app, mut receiver) = starting();
    app.handshake.capabilities.push("file_picker".into());
    phase(&mut app, "wizard");
    let task = app.setup_picker("soundfont".into(), "choose-bank".into());
    let mut stream = iced_runtime::task::into_stream(task).unwrap();
    let mut parent_seen = false;
    let mut completed = None;
    while let Some(action) = stream.next().await {
        match action {
            iced_runtime::Action::Window(window::Action::Run(id, callback)) => {
                assert_eq!(id, app.main_window);
                parent_seen = true;
                callback(&MissingParent);
            }
            iced_runtime::Action::Output(message @ Message::SetupPicked { .. }) => {
                completed = Some(message)
            }
            _ => {}
        }
    }
    assert!(parent_seen);
    let _ = app.update(completed.expect("missing native handles must complete with failure"));
    assert!(
        matches!(receiver.try_recv().unwrap().request, ipc::Request::FilePicked { request_id, status, path: None, error: Some(_), .. } if request_id == "choose-bank" && status == "failed")
    );
}

#[test]
fn ready_waits_for_an_outstanding_native_sheet_before_restoring_actions_or_lyrics() {
    let (mut app, mut receiver) = starting();
    app.handshake.capabilities.push("file_picker".into());
    app.config.desktop_lyrics.enabled = true;
    phase(&mut app, "wizard");
    let _ = app.setup_picker("soundfont".into(), "bank".into());
    phase(&mut app, "ready");
    assert!(!app.actions_available());
    assert!(app.lyrics.window.is_none());
    assert!(receiver.try_recv().is_err());
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: app.connection_epoch,
        request_id: "bank".into(),
        result: Ok(Some("/tmp/late.sf2".into())),
    });
    assert!(app.actions_available());
    assert!(app.lyrics.window.is_some());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(true)
    ));
    assert!(
        receiver.try_recv().is_err(),
        "cancelled sheet cannot send a late path"
    );
}

#[test]
fn terminal_child_exit_before_disconnect_closes_all_lyric_windows() {
    for closing in [false, true] {
        let (mut app, _) = starting();
        app.config.desktop_lyrics.enabled = true;
        phase(&mut app, "ready");
        let lyrics = app.lyrics.window.unwrap();
        let toolbar = window::Id::unique();
        app.lyrics.toolbar_window = Some(toolbar);
        app.terminal = Some(Terminal::new(2, iced_term::settings::Settings::default()).unwrap());
        app.closing = closing;
        let event = |event| {
            Message::Terminal(iced_term::Event::BackendCall(
                2,
                iced_term::BackendCommand::ProcessAlacrittyEvent(event),
            ))
        };
        let _ = app.update(event(iced_term::AlacrittyEvent::ChildExit(17)));
        assert_eq!(app.exit_code, Some(17));
        // alacritty emits ChildExit with the status, then Exit to shut down its event loop.
        let _ = app.update(event(iced_term::AlacrittyEvent::Exit));
        assert!(!app.running);
        assert!(!app.ipc_connected);
        assert!(
            app.lyrics.window.is_none(),
            "lyric window {lyrics:?} remained after child exit"
        );
        assert!(
            app.lyrics.toolbar_window.is_none(),
            "toolbar {toolbar:?} remained after child exit"
        );
        assert!(
            app.config.desktop_lyrics.enabled,
            "cleanup preserves the saved preference"
        );
    }
}

#[test]
fn setup_keeps_basic_menu_commands_and_shortcuts_available() {
    use iced::keyboard::{Key, Modifiers};
    let (mut app, mut receiver) = starting();
    for stage in ["starting", "wizard", "initial_loading"] {
        phase(&mut app, stage);
        let font = app.config.font_size;
        let _ = app.update(Message::Desktop(desktop::Event::Action(Action::ZoomIn)));
        assert_eq!(
            app.config.font_size,
            font + 2,
            "basic zoom disabled during {stage}"
        );
        let _ = app.update(Message::Native(
            app.main_window,
            iced::Event::Keyboard(iced::keyboard::Event::KeyPressed {
                key: Key::Character("-".into()),
                modified_key: Key::Character("-".into()),
                physical_key: iced::keyboard::key::Physical::Unidentified(
                    iced::keyboard::key::NativeCode::Unidentified,
                ),
                location: iced::keyboard::Location::Standard,
                modifiers: Modifiers::COMMAND,
                text: None,
                repeat: false,
            }),
        ));
        assert_eq!(
            app.config.font_size, font,
            "zoom shortcut disabled during {stage}"
        );
        let _ = app.action(Action::About);
        assert!(matches!(app.dialog, Some(Dialog::About)));
        let _ = app.update(Message::Dismiss);
        assert!(app.dialog.is_none());
        assert!(!app.actions_available(), "basic commands unlocked playback");
        assert!(receiver.try_recv().is_err());
    }
}

#[test]
fn audio_picker_requires_ready_and_returns_selection_for_the_current_request() {
    let (mut app, mut receiver) = starting();
    app.handshake.capabilities.push("file_picker".into());
    let request = |id: &str| {
        ipc::Event::Received(ipc::Incoming::PickFile {
            purpose: "audio".into(),
            request_id: id.into(),
        })
    };
    for name in ["wizard", "initial_loading"] {
        phase(&mut app, name);
        let task = app.ipc_event(2, request("blocked"));
        assert!(iced_runtime::task::into_stream(task).is_none());
        assert!(app.picker.is_none());
    }
    phase(&mut app, "ready");
    while receiver.try_recv().is_ok() {}
    let task = app.ipc_event(2, request("audio-1"));
    assert!(
        iced_runtime::task::into_stream(task).is_some(),
        "ready audio request must open a native chooser"
    );
    assert!(!app.actions_available());
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: app.connection_epoch,
        request_id: "audio-1".into(),
        result: Ok(Some("/tmp/a song 雪.flac".into())),
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request, ipc::Request::FilePicked { purpose, request_id, status, path: Some(path), .. } if purpose == "audio" && request_id == "audio-1" && status == "selected" && path == std::path::Path::new("/tmp/a song 雪.flac"))
    );
    assert!(app.actions_available());
}

#[test]
fn audio_picker_cancellation_and_disconnect_do_not_open_a_track() {
    let (mut app, mut receiver) = starting();
    app.handshake.capabilities.push("file_picker".into());
    phase(&mut app, "ready");
    while receiver.try_recv().is_ok() {}
    let _ = app.setup_picker("audio".into(), "cancel".into());
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch: app.connection_epoch,
        request_id: "cancel".into(),
        result: Ok(None),
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request, ipc::Request::FilePicked { purpose, status, path: None, .. } if purpose == "audio" && status == "cancelled")
    );
    let _ = app.setup_picker("audio".into(), "stale".into());
    let epoch = app.connection_epoch;
    let _ = app.ipc_event(2, ipc::Event::Disconnected("test".into()));
    let (sender, mut replacement) = mpsc::channel(32);
    let _ = app.ipc_event(2, ipc::Event::Connected(sender));
    phase(&mut app, "ready");
    let _ = app.update(Message::SetupPicked {
        session: 2,
        epoch,
        request_id: "stale".into(),
        result: Ok(Some("/tmp/stale.flac".into())),
    });
    assert!(matches!(
        replacement.try_recv().unwrap().request,
        ipc::Request::DesktopLyrics(_)
    ));
    assert!(
        replacement.try_recv().is_err(),
        "old chooser result must not reach the replacement connection"
    );
    assert!(app.actions_available());
}
