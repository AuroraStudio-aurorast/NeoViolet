use super::*;

fn app() -> App {
    App {
        main_window: window::Id::unique(),
        lyrics: lyrics_window::Overlay::default(),
        titlebar: titlebar::State::default(),
        config: config::Config::default(),
        terminal: None,
        running: true,
        dialog: Some(Dialog::Quit),
        version: String::new(),
        window_title: "NeoViolet".into(),
        desktop: None,
        ipc_connected: false,
        session: 2,
        pid: None,
        connection: None,
        pending: VecDeque::from([PathBuf::from("/tmp/song.flac")]),
        arguments: Vec::new(),
        family: "monospace",
        modifiers: Modifiers::empty(),
        exit_code: None,
        closing: false,
        save_settings: false,
        environment: Default::default(),
    }
}

#[test]
fn dismiss_before_handshake_preserves_pending_file() {
    let mut app = app();
    let _ = app.update(Message::Dismiss);
    assert!(app.dialog.is_none());
    assert_eq!(app.pending.front(), Some(&PathBuf::from("/tmp/song.flac")));
}

#[test]
fn authentication_during_modal_does_not_consume_pending_file() {
    let mut app = app();
    let (sender, _receiver) = mpsc::channel(2);
    let _ = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    assert!(app.connection.is_some());
    assert_eq!(app.pending.len(), 1);
}

#[test]
fn obsolete_session_cannot_install_its_connection() {
    let mut app = app();
    let (sender, _receiver) = mpsc::channel(2);
    let _ = app.update(Message::Ipc(1, ipc::Event::Connected(sender)));
    assert!(app.connection.is_none());
}

#[test]
fn restart_discards_initial_flags_and_queued_files_even_if_the_new_launch_fails() {
    let mut app = app();
    app.arguments = launch::arguments(["--seek", "99", "--invalid", "old.flac"].map(String::from));
    app.window_title = "NeoViolet | Old track".into();
    let temporary = tempfile::tempdir().unwrap();
    let config = config::Config {
        neoviolet_path: Some(
            temporary
                .path()
                .join("missing-player")
                .to_string_lossy()
                .into_owned(),
        ),
        ..Default::default()
    };
    let _ = app.restart_with_config(config, None);
    assert_eq!(app.arguments, ["--xdg-config"]);
    assert!(app.pending.is_empty());
    assert_eq!(app.title(), "NeoViolet");
    assert_eq!(app.session, 3);
    assert!(!app.running);
}

#[test]
fn terminal_titles_replace_and_clear_the_window_title() {
    let mut app = app();
    app.dialog = None;
    app.terminal =
        Some(iced_term::Terminal::new(2, iced_term::settings::Settings::default()).unwrap());
    let command = |event| {
        Message::Terminal(iced_term::Event::BackendCall(
            2,
            iced_term::BackendCommand::ProcessAlacrittyEvent(event),
        ))
    };
    let _ = app.update(command(iced_term::AlacrittyEvent::Title(
        "Current Track".into(),
    )));
    assert_eq!(app.title(), "Current Track");
    let _ = app.update(command(iced_term::AlacrittyEvent::ResetTitle));
    assert_eq!(app.title(), "NeoViolet");
}

fn lyric_frame(text: &str) -> crate::lyrics::Frame {
    serde_json::from_value(serde_json::json!({
        "elapsed": 6.0, "title": "Track", "lines": [{"time": 5.0, "text": text}]
    }))
    .unwrap()
}

#[test]
fn secondary_windows_cannot_resize_or_quit_the_main_player() {
    let mut app = app();
    app.dialog = None;
    let overlay = window::Id::unique();
    let settings = window::Id::unique();
    app.lyrics.window = Some(overlay);
    app.lyrics.settings_window = Some(settings);
    let _ = app.update(Message::Native(
        overlay,
        iced::Event::Window(window::Event::Resized(iced::Size::new(880.0, 240.0))),
    ));
    assert_eq!(
        (app.config.window_width, app.config.window_height),
        (1000, 700)
    );
    let _ = app.update(Message::Native(
        settings,
        iced::Event::Window(window::Event::CloseRequested),
    ));
    assert!(app.lyrics.settings_window.is_none());
    assert!(app.config.desktop_lyrics.enabled);
    let _ = app.update(Message::Native(
        overlay,
        iced::Event::Window(window::Event::CloseRequested),
    ));
    assert!(!app.config.desktop_lyrics.enabled);
    assert!(app.lyrics.window.is_none());
    assert!(app.dialog.is_none());
    assert!(app.running);
    let _ = app.update(Message::Native(
        app.main_window,
        iced::Event::Window(window::Event::CloseRequested),
    ));
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
}

#[test]
fn old_sessions_and_disabled_streams_cannot_restore_stale_lyrics() {
    let mut app = app();
    let message = |session, text| {
        Message::Ipc(
            session,
            ipc::Event::Received(ipc::Incoming::Lyrics(lyric_frame(text))),
        )
    };
    let _ = app.update(message(2, "current"));
    let _ = app.update(message(1, "obsolete"));
    assert_eq!(app.lyrics.frame.active_lines()[0].text, "current");
    let _ = app.update(Message::Lyrics(lyrics_window::Event::Visible(false)));
    let _ = app.update(message(2, "late frame"));
    assert!(app.lyrics.frame.lines.is_empty());
    app.config.desktop_lyrics.enabled = true;
    let _ = app.update(message(2, "restored"));
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Received(ipc::Incoming::Lyrics(crate::lyrics::Frame::default())),
    ));
    assert!(app.lyrics.frame.lines.is_empty());
}

#[test]
fn lyric_payloads_update_even_while_the_main_window_has_a_modal() {
    let mut app = app();
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Received(ipc::Incoming::Lyrics(lyric_frame("current"))),
    ));
    assert_eq!(app.lyrics.frame.active_lines()[0].text, "current");
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
    assert_eq!(app.pending.len(), 1);
    let _ = app.update(Message::Ipc(2, ipc::Event::Disconnected("closed".into())));
    assert!(app.lyrics.frame.lines.is_empty());
}

#[test]
fn a_late_window_open_does_not_replace_the_current_overlay() {
    let mut app = app();
    let old = window::Id::unique();
    let current = window::Id::unique();
    app.lyrics.window = Some(current);
    let _ = app.update(Message::Lyrics(lyrics_window::Event::Opened(old)));
    assert_eq!(app.lyrics.window, Some(current));
    assert!(app.config.desktop_lyrics.enabled);
}

#[tokio::test]
async fn quit_from_a_secondary_window_reveals_the_main_confirmation() {
    use iced::futures::StreamExt;
    let mut app = app();
    app.dialog = None;
    let task = app.update(Message::Desktop(desktop::Event::Action(Action::Quit)));
    let actions = iced_runtime::task::into_stream(task)
        .expect("quit must reveal the main window")
        .collect::<Vec<_>>()
        .await;
    assert!(matches!(actions.as_slice(), [
        iced_runtime::Action::Window(window::Action::Minimize(id, false)),
        iced_runtime::Action::Window(window::Action::GainFocus(focus)),
    ] if *id == app.main_window && *focus == app.main_window));
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
}

#[tokio::test]
async fn authentication_subscribes_to_lyrics_without_opening_pending_files_during_a_modal() {
    use iced::futures::StreamExt;
    let mut app = app();
    let (sender, mut receiver) = mpsc::channel(4);
    let task = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    let _ = iced_runtime::task::into_stream(task)
        .unwrap()
        .collect::<Vec<_>>()
        .await;
    assert!(matches!(
        receiver.try_recv().unwrap(),
        ipc::Request::DesktopLyrics(true)
    ));
    assert!(receiver.try_recv().is_err());
    assert_eq!(app.pending.len(), 1);
}

#[tokio::test]
async fn reusing_the_overlay_reconciles_native_lock_state_after_settings_reload() {
    use iced::futures::StreamExt;
    let mut overlay = lyrics_window::Overlay::default();
    let id = window::Id::unique();
    overlay.window = Some(id);
    let mut preferences = crate::lyrics::Preferences::default();
    for locked in [false, true] {
        preferences.locked = locked;
        let (task, _) = overlay.update(lyrics_window::Event::Visible(true), &mut preferences);
        let mut actions = iced_runtime::task::into_stream(task)
            .expect("an existing window must apply the current passthrough setting");
        #[cfg(target_os = "macos")]
        {
            assert!(
                matches!(actions.next().await, Some(iced_runtime::Action::Window(
                window::Action::Run(window, _)
            )) if window == id)
            );
            continue;
        }
        #[cfg(not(target_os = "macos"))]
        {
            let actions = actions.collect::<Vec<_>>().await;
            if locked {
                assert!(matches!(actions.as_slice(), [iced_runtime::Action::Window(
                window::Action::EnableMousePassthrough(window)
            )] if *window == id));
            } else {
                assert!(matches!(actions.as_slice(), [iced_runtime::Action::Window(
                window::Action::DisableMousePassthrough(window)
            )] if *window == id));
            }
        }
    }
}

#[cfg(not(target_os = "windows"))]
#[tokio::test]
async fn closing_a_menu_restores_the_terminal_widget_focus() {
    use iced::advanced::widget::operation::Focusable;
    use iced::futures::StreamExt;
    #[derive(Default)]
    struct Focus(bool);
    impl Focusable for Focus {
        fn is_focused(&self) -> bool {
            self.0
        }
        fn focus(&mut self) {
            self.0 = true;
        }
        fn unfocus(&mut self) {
            self.0 = false;
        }
    }
    for action in ["escape", "dismiss", "copy"] {
        let mut app = app();
        app.dialog = None;
        app.terminal = Some(Terminal::new(2, settings::Settings::default()).unwrap());
        let target = app.terminal.as_ref().unwrap().widget_id().clone();
        let _ = app.update(Message::Titlebar(titlebar::Message::ToggleMenu));
        let message = match action {
            "escape" => Message::Native(
                app.main_window,
                iced::Event::Keyboard(iced::keyboard::Event::KeyPressed {
                    key: Key::Named(Named::Escape),
                    modified_key: Key::Named(Named::Escape),
                    physical_key: iced::keyboard::key::Physical::Code(
                        iced::keyboard::key::Code::Escape,
                    ),
                    location: iced::keyboard::Location::Standard,
                    modifiers: Modifiers::empty(),
                    text: None,
                    repeat: false,
                }),
            ),
            "copy" => Message::Titlebar(titlebar::Message::Action(Action::Copy)),
            _ => Message::Titlebar(titlebar::Message::Dismiss),
        };
        let task = app.update(message);
        let mut focus = Focus::default();
        if let Some(mut actions) = iced_runtime::task::into_stream(task) {
            while let Some(action) = actions.next().await {
                if let iced_runtime::Action::Widget(mut operation) = action {
                    operation.focusable(Some(&target), iced::Rectangle::default(), &mut focus);
                }
            }
        }
        assert!(!app.titlebar.visible());
        assert!(
            focus.0,
            "{action}: terminal must receive widget focus when the menu closes"
        );
    }
}
