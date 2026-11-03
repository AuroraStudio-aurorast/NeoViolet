use super::*;

fn app() -> App {
    App {
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
