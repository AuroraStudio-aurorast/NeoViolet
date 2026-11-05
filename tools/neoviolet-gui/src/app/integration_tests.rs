use super::*;

fn connected() -> (App, mpsc::Receiver<ipc::Command>) {
    let mut app = app();
    app.dialog = None;
    app.pending.clear();
    app.handshake = ipc::Handshake {
        version: String::new(),
        protocol: 1,
        capabilities: vec!["state".into(), "results".into(), "playback".into()],
    };
    let (sender, mut receiver) = mpsc::channel(32);
    let _ = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    let initial = receiver.try_recv().unwrap();
    let _ = app.command_result(initial.id, "succeeded".into(), String::new());
    (app, receiver)
}

#[test]
fn lyric_toolbar_follows_snapshots_without_optimistically_changing_playback() {
    let (mut app, mut receiver) = connected();
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        track_id: 2,
        has_track: true,
        playing: true,
        duration: 60.0,
        position: 10.0,
        volume: 0.6,
        ..Default::default()
    });
    assert_eq!(app.lyrics.playback, Some(true));
    let _ = app.update(Message::Lyrics(lyrics_window::Event::PlayPause));
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Pause
    ));
    assert!(app.playback.as_ref().unwrap().playing);
    app.dialog = Some(Dialog::Quit);
    let _ = app.update(Message::Lyrics(lyrics_window::Event::PlayPause));
    assert!(receiver.try_recv().is_err());
}

#[test]
fn obsolete_snapshots_and_results_cannot_change_the_current_session() {
    let (mut app, mut receiver) = connected();
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 3,
        track_id: 5,
        has_track: true,
        title: "Current".into(),
        ..Default::default()
    });
    for (revision, track_id) in [(2, 5), (4, 4)] {
        let _ = app.apply_snapshot(ipc::PlaybackState {
            revision,
            track_id,
            has_track: true,
            title: "Stale".into(),
            ..Default::default()
        });
    }
    assert_eq!(app.title(), "Current");
    let _ = app.send_all(vec![ipc::Request::Open("missing.flac".into())]);
    let old = receiver.try_recv().unwrap();
    let old_epoch = app.connection_epoch;
    let _ = app.update(Message::Ipc(2, ipc::Event::Disconnected("closed".into())));
    assert!(app.playback.is_none());
    assert!(app.lyrics.playback.is_none());
    assert!(app.requests.is_empty());
    let (sender, mut next) = mpsc::channel(4);
    let _ = app.update(Message::Ipc(2, ipc::Event::Connected(sender)));
    let command = next.try_recv().unwrap();
    let notice = app.notice.clone();
    let _ = app.command_result(old.id.clone(), "failed".into(), "old failure".into());
    let _ = app.update(Message::RequestTimeout {
        session: 2,
        epoch: old_epoch,
        id: old.id,
        accepted: false,
    });
    assert_eq!(app.notice, notice);
    assert!(app.requests.contains_key(&command.id));
    // A reconnection starts its revision sequence again.
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        track_id: 5,
        has_track: true,
        title: "Recovered".into(),
        ..Default::default()
    });
    assert_eq!(app.title(), "Recovered");
}

#[test]
fn open_receipts_track_acceptance_and_report_failures_without_blocking_terminal_input() {
    let (mut app, mut receiver) = connected();
    let _ = app.send_all(vec![ipc::Request::Open("broken.flac".into())]);
    let command = receiver.try_recv().unwrap();
    assert_eq!(app.requests.get(&command.id), Some(&false));
    let _ = app.command_result(command.id.clone(), "accepted".into(), String::new());
    let _ = app.update(Message::RequestTimeout {
        session: 2,
        epoch: app.connection_epoch,
        id: command.id.clone(),
        accepted: false,
    });
    assert_eq!(app.requests.get(&command.id), Some(&true));
    let _ = app.command_result(
        command.id.clone(),
        "failed".into(),
        "Audio could not be decoded".into(),
    );
    assert!(!app.requests.contains_key(&command.id));
    assert_eq!(app.notice.as_deref(), Some("Audio could not be decoded"));
    assert!(app.dialog.is_none());
}

#[test]
fn queued_commands_fail_explicitly_when_the_connection_is_full() {
    let (mut app, _receiver) = connected();
    let (sender, mut receiver) = mpsc::channel(1);
    app.connection = Some(sender);
    let _ = app.send_all(vec![ipc::Request::Play, ipc::Request::Pause]);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Play
    ));
    assert_eq!(app.requests.len(), 1);
    assert!(app.notice.is_some());
}

#[test]
fn main_window_leaves_playback_controls_to_the_embedded_terminal() {
    use iced::advanced::{layout, text::paragraph::Plain, widget};
    type Paragraph = iced::advanced::graphics::text::paragraph::Paragraph;
    fn inspect(tree: &widget::Tree, labels: &mut Vec<String>) {
        if let widget::tree::State::Some(state) = &tree.state
            && let Some(paragraph) = state.downcast_ref::<Plain<Paragraph>>()
        {
            labels.push(paragraph.content().to_owned());
        }
        for child in &tree.children {
            inspect(child, labels);
        }
    }
    let (mut app, _receiver) = connected();
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        track_id: 1,
        has_track: true,
        playing: true,
        title: "A song".into(),
        duration: 3672.0,
        position: 65.0,
        volume: 0.5,
        ..Default::default()
    });
    let renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    let mut view = app.view(app.main_window);
    let mut tree = widget::Tree::new(&view);
    let size = iced::Size::new(480.0, 320.0);
    let node = view
        .as_widget_mut()
        .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
    assert!(node.size().width <= 480.0 && node.size().height <= 320.0);
    let mut labels = Vec::new();
    inspect(&tree, &mut labels);
    assert!(
        !labels
            .iter()
            .any(|label| label == "Play" || label == "Pause" || label == "Vol"),
        "native playback controls duplicated the TUI"
    );
    assert!(
        !labels.iter().any(|label| label == "1:05 / 61:12"),
        "native timeline duplicated the TUI"
    );
    assert!(
        labels.iter().any(|label| label == "Starting player…"),
        "terminal area disappeared"
    );
}

#[test]
fn unsent_file_opens_survive_a_full_connection_queue_and_can_be_retried() {
    let (mut app, _receiver) = connected();
    let (sender, mut receiver) = mpsc::channel(1);
    app.connection = Some(sender);
    let _ = app.send_all(vec![
        ipc::Request::Open("first.flac".into()),
        ipc::Request::Open("second.flac".into()),
        ipc::Request::Open("third.flac".into()),
    ]);
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::Open(path) if path==std::path::Path::new("first.flac"))
    );
    assert_eq!(
        app.pending,
        VecDeque::from([PathBuf::from("second.flac"), PathBuf::from("third.flac")])
    );
    let _ = app.update(Message::FlushPending {
        session: 2,
        epoch: app.connection_epoch,
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::Open(path) if path==std::path::Path::new("second.flac"))
    );
    assert_eq!(app.pending, VecDeque::from([PathBuf::from("third.flac")]));
    let _ = app.update(Message::FlushPending {
        session: 2,
        epoch: app.connection_epoch,
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::Open(path) if path==std::path::Path::new("third.flac"))
    );
    assert!(app.pending.is_empty());
}

#[test]
fn newly_selected_file_stays_after_older_unsent_files() {
    let (mut app, _receiver) = connected();
    let (sender, mut receiver) = mpsc::channel(1);
    app.connection = Some(sender);
    app.pending = VecDeque::from([PathBuf::from("older.flac")]);
    let _ = app.open("newest.flac".into());
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::Open(path) if path==std::path::Path::new("older.flac"))
    );
    let _ = app.update(Message::FlushPending {
        session: 2,
        epoch: app.connection_epoch,
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::Open(path) if path==std::path::Path::new("newest.flac"))
    );
    assert!(app.pending.is_empty());
}

#[test]
fn native_quit_asks_the_player_even_when_the_last_snapshot_is_idle() {
    let (mut app, mut receiver) = connected();
    app.handshake=serde_json::from_value(serde_json::json!({"protocol":1,"version":"v9.8.7","capabilities":["results","state","playback","request_quit"]})).unwrap();
    let _ = app.apply_snapshot(ipc::PlaybackState {
        revision: 1,
        ..Default::default()
    });
    let _ = app.action(Action::Quit);
    assert!(
        app.dialog.is_none(),
        "idle close should not show a local confirmation"
    );
    assert!(
        receiver.try_recv().is_ok(),
        "Go must decide from live state"
    );
}

#[test]
fn about_version_comes_from_the_authenticated_child_and_resets_on_restart() {
    let mut app = app();
    let hello = serde_json::from_value(serde_json::json!({"version":"v9.8.7"})).unwrap();
    let _ = app.update(Message::Ipc(1, ipc::Event::Handshake(hello)));
    assert!(app.version.is_empty());
    let hello = serde_json::from_value(serde_json::json!({"version":"v9.8.7"})).unwrap();
    let _ = app.update(Message::Ipc(2, ipc::Event::Handshake(hello)));
    assert_eq!(app.version, "NeoViolet version v9.8.7");
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Handshake(ipc::Handshake::default()),
    ));
    assert_eq!(app.version, "unavailable");
}

#[test]
fn closing_stops_file_opens_and_pending_retries() {
    let (mut app, mut receiver) = connected();
    app.pending.push_back("old.flac".into());
    let _ = app.update(Message::ConfirmQuit);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Quit
    ));
    let _ = app.open("new.flac".into());
    let _ = app.update(Message::FlushPending {
        session: 2,
        epoch: app.connection_epoch,
    });
    assert!(
        receiver.try_recv().is_err(),
        "closing session reopened a file"
    );
    assert!(app.pending.is_empty());
}

#[test]
fn idle_quit_response_begins_graceful_shutdown_and_is_idempotent() {
    let (mut app, mut receiver) = connected();
    app.handshake.capabilities.push("request_quit".into());
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Received(ipc::Incoming::Quit { dialog: false }),
    ));
    assert!(app.closing);
    assert!(app.dialog.is_none());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Quit
    ));
    let _ = app.update(Message::ConfirmQuit);
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Received(ipc::Incoming::Quit { dialog: true }),
    ));
    assert!(
        app.dialog.is_none(),
        "late decision reopened confirmation while closing"
    );
    assert!(receiver.try_recv().is_err(), "quit was sent twice");
}

#[test]
fn loaded_player_and_unknown_legacy_state_keep_confirmation() {
    let (mut app, mut receiver) = connected();
    app.handshake.capabilities.push("request_quit".into());
    let _ = app.action(Action::Quit);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::CheckQuit
    ));
    let _ = app.update(Message::Ipc(
        2,
        ipc::Event::Received(ipc::Incoming::Quit { dialog: true }),
    ));
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
    assert!(!app.closing);
    let _ = app.update(Message::Dismiss);
    assert!(app.dialog.is_none());
    assert!(!app.closing);
    app.handshake = Default::default();
    app.playback = None;
    let _ = app.action(Action::Quit);
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
    assert!(receiver.try_recv().is_err());
}

#[test]
fn graceful_quit_is_not_blocked_by_old_receipt_limits() {
    let (mut app, mut receiver) = connected();
    for index in 0..64 {
        app.requests.insert(format!("old-{index}"), false);
    }
    let _ = app.update(Message::ConfirmQuit);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::Quit
    ));
    assert!(
        app.requests.is_empty(),
        "old operations must not interfere with shutdown"
    );
}

#[tokio::test]
async fn close_requests_wait_for_queue_capacity_instead_of_disappearing() {
    use iced::futures::StreamExt;
    for request in [ipc::Request::CheckQuit, ipc::Request::Quit] {
        let (mut app, _receiver) = connected();
        let (sender, mut receiver) = mpsc::channel(1);
        sender
            .try_send(ipc::Command {
                request: ipc::Request::GetState,
                id: String::new(),
            })
            .unwrap();
        app.connection = Some(sender);
        for index in 0..64 {
            app.requests.insert(format!("old-{index}"), false);
        }
        let task = app.send_all(vec![request]);
        assert!(matches!(
            receiver.try_recv().unwrap().request,
            ipc::Request::GetState
        ));
        let mut stream =
            iced_runtime::task::into_stream(task).expect("close must wait for capacity");
        while let Some(action) = stream.next().await {
            if let iced_runtime::Action::Output(message) = action {
                let _ = app.update(message);
            }
        }
        assert!(matches!(
            receiver.try_recv().unwrap().request,
            ipc::Request::CheckQuit | ipc::Request::Quit
        ));
    }
}
