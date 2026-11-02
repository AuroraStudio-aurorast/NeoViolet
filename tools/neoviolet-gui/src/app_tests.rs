use super::*;

fn app() -> App {
    App {
        config: config::Config::default(),
        terminal: None,
        running: true,
        dialog: Some(Dialog::Quit),
        version: String::new(),
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
