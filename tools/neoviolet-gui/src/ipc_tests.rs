use super::*;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

#[tokio::test]
async fn lyric_stream_frames_are_not_discarded_as_unknown_messages() {
    let payload = b"{\"type\":\"lyrics\",\"elapsed\":12.3,\"title\":\"Song\",\"lines\":[{\"time\":10,\"end\":15,\"text\":\"Hello\",\"parts\":[\"Hello\",\"Additional line\"],\"words\":[{\"time\":10,\"text\":\"Hello\"}],\"agent\":\"a\",\"agent_name\":\"Voice A\"}]}\n";
    let mut reader = BufReader::new(&payload[..]);
    let frame = read_frame(&mut reader).await.unwrap().unwrap();
    let Incoming::Lyrics(frame) = frame else {
        panic!("lyric frame was not decoded");
    };
    assert_eq!(frame.elapsed, 12.3);
    assert_eq!(frame.title, "Song");
    assert_eq!(frame.lines[0].additional_lines(), ["Additional line"]);
    assert_eq!(frame.lines[0].agent, "a");
    assert_eq!(frame.lines[0].agent_name, "Voice A");
}

#[tokio::test]
async fn overlay_commands_use_existing_tui_protocol_and_newline_framing() {
    let mut output = Vec::new();
    write_request(&mut output, &Request::DesktopLyrics(true), "")
        .await
        .unwrap();
    write_request(&mut output, &Request::PlayPause, "")
        .await
        .unwrap();
    write_request(&mut output, &Request::DesktopLyrics(false), "")
        .await
        .unwrap();
    let frames: Vec<serde_json::Value> = String::from_utf8(output)
        .unwrap()
        .lines()
        .map(|line| serde_json::from_str(line).unwrap())
        .collect();
    assert_eq!(
        frames,
        [
            serde_json::json!({"type":"desktop_lyrics","enable":true}),
            serde_json::json!({"type":"play_pause"}),
            serde_json::json!({"type":"desktop_lyrics","enable":false}),
        ]
    );
}

#[test]
fn discovery_rejects_remote_addresses_and_malformed_tokens() {
    let token = "a".repeat(64);
    assert!(parse_endpoint(&format!("127.0.0.1:1234\n{token}")).is_ok());
    assert!(parse_endpoint(&format!("192.0.2.1:1234\n{token}")).is_err());
    assert!(parse_endpoint("127.0.0.1:1234\nnot-a-token").is_err());
}

#[tokio::test]
async fn authentication_requires_server_acknowledgement() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let token = "b".repeat(64);
    let server_token = token.clone();
    let server = tokio::spawn(async move {
        let (socket, _) = listener.accept().await.unwrap();
        let mut peer = BufReader::new(socket);
        let mut line = String::new();
        peer.read_line(&mut line).await.unwrap();
        assert_eq!(line.trim(), server_token);
        peer.get_mut()
            .write_all(b"{\"type\":\"hello\"}\n")
            .await
            .unwrap();
        line.clear();
        peer.read_line(&mut line).await.unwrap();
        let value: serde_json::Value = serde_json::from_str(&line).unwrap();
        assert_eq!(value["type"], "open");
        assert_eq!(value["path"], "/tmp/a song\".flac");
    });
    let stream = tokio::net::TcpStream::connect(address).await.unwrap();
    let (mut peer, _) = authenticate(stream, &token).await.unwrap();
    write_request(
        peer.get_mut(),
        &Request::Open("/tmp/a song\".flac".into()),
        "",
    )
    .await
    .unwrap();
    server.await.unwrap();
}

#[tokio::test]
async fn frames_survive_partial_reads_and_unknown_messages() {
    let (mut sender, receiver) = tokio::io::duplex(256);
    let writer = tokio::spawn(async move {
        sender.write_all(b"{\"type\":\"qu").await.unwrap();
        tokio::task::yield_now().await;
        sender
            .write_all(b"it\",\"dialog\":true}\n{\"type\":\"future\"}\n")
            .await
            .unwrap();
    });
    let mut reader = BufReader::new(receiver);
    assert!(matches!(
        read_frame(&mut reader).await.unwrap(),
        Some(Incoming::Quit { dialog: true })
    ));
    assert!(matches!(
        read_frame(&mut reader).await.unwrap(),
        Some(Incoming::Other)
    ));
    assert!(read_frame(&mut reader).await.unwrap().is_none());
    writer.await.unwrap();
}

#[tokio::test]
async fn truncated_frames_are_errors() {
    let mut reader = BufReader::new(&b"{\"type\":\"hello\"}"[..]);
    assert!(read_frame(&mut reader).await.is_err());
}

#[cfg(unix)]
#[tokio::test]
#[ignore = "requires the built TUI; run make test/gui-ipc"]
async fn live_tui_acknowledges_and_exits_on_gui_request() {
    let player =
        std::env::var("NEOVIOLET_TEST_PLAYER").expect("make test/gui-ipc provides the player");
    let directory = tempfile::tempdir().unwrap();
    let config = directory.path().join("neoviolet");
    std::fs::create_dir_all(&config).unwrap();
    std::fs::write(config.join("config.json"), "{}").unwrap();
    let mut terminal = iced_term::Terminal::new(
        700,
        iced_term::settings::Settings {
            backend: iced_term::settings::BackendSettings {
                program: player,
                args: vec!["--xdg-config".into()],
                env: std::collections::HashMap::from([
                    ("TERM_PROGRAM".into(), "neoviolet-gui".into()),
                    ("TERM".into(), "xterm-256color".into()),
                    ("NEOVIOLET_FONT".into(), "monospace".into()),
                    (
                        "XDG_CONFIG_HOME".into(),
                        directory.path().to_string_lossy().into_owned(),
                    ),
                    (
                        "XDG_CACHE_HOME".into(),
                        directory.path().to_string_lossy().into_owned(),
                    ),
                ]),
                working_directory: Some(directory.path().into()),
            },
            ..Default::default()
        },
    )
    .expect("start real TUI in a PTY without a GUI window");
    let pid = terminal.child_process_id().unwrap();
    let port = std::env::temp_dir().join(format!("neoviolet-ipc-{pid}"));
    let (mut peer, hello) = tokio::time::timeout(Duration::from_secs(10), discover(&port))
        .await
        .expect("TUI must start IPC without a desktop window")
        .expect("real authentication acknowledgement");
    assert!(
        !hello.version.trim().is_empty(),
        "the running player must identify its build"
    );
    assert!(hello.supports("request_quit"));
    write_request(peer.get_mut(), &Request::DesktopLyrics(true), "")
        .await
        .unwrap();
    live_message(&mut peer, |frame| matches!(frame, Incoming::Lyrics(_))).await;
    write_request(peer.get_mut(), &Request::DesktopLyrics(false), "")
        .await
        .unwrap();
    write_request(peer.get_mut(), &Request::DesktopLyrics(true), "")
        .await
        .unwrap();
    live_message(&mut peer, |frame| matches!(frame, Incoming::Lyrics(_))).await;
    write_request(peer.get_mut(), &Request::SetVolume(0.0), "mute")
        .await
        .unwrap();
    let result = live_message(
        &mut peer,
        |frame| matches!(frame,Incoming::Result{request_id,..} if request_id=="mute"),
    )
    .await;
    assert!(matches!(result,Incoming::Result{status,..} if status=="succeeded"));
    let state = live_message(
        &mut peer,
        |frame| matches!(frame,Incoming::State{state} if state.volume==0.0),
    )
    .await;
    assert!(matches!(state,Incoming::State{state} if !state.has_track && state.config_dir==config));
    let broken = directory.path().join("broken.flac");
    std::fs::write(&broken, b"not audio").unwrap();
    write_request(peer.get_mut(), &Request::Open(broken.clone()), "broken")
        .await
        .unwrap();
    let accepted = live_message(
        &mut peer,
        |frame| matches!(frame,Incoming::Result{request_id,..} if request_id=="broken"),
    )
    .await;
    assert!(matches!(accepted,Incoming::Result{status,..} if status=="accepted"));
    let failed = live_message(
        &mut peer,
        |frame| matches!(frame,Incoming::Result{request_id,..} if request_id=="broken"),
    )
    .await;
    assert!(
        matches!(failed,Incoming::Result{status,error,..} if status=="failed" && !error.is_empty())
    );
    // Exercise the actual command through the PTY, including cancellation and
    // loading the selected path through the startup host rather than GUI state.
    let mut previous_pick = String::new();
    for status in ["cancelled", "selected"] {
        terminal.handle(iced_term::Command::ProxyToBackend(
            iced_term::BackendCommand::Write(b":open\r".to_vec()),
        ));
        let pick = live_message(
            &mut peer,
            |frame| matches!(frame, Incoming::PickFile { purpose, .. } if purpose == "audio"),
        )
        .await;
        let Incoming::PickFile { request_id, .. } = pick else {
            unreachable!()
        };
        assert_ne!(request_id, previous_pick);
        previous_pick = request_id.clone();
        write_request(
            peer.get_mut(),
            &Request::FilePicked {
                purpose: "audio".into(),
                request_id,
                status: status.into(),
                path: (status == "selected").then(|| broken.clone()),
                error: None,
            },
            "",
        )
        .await
        .unwrap();
        if status == "selected" {
            live_message(
                &mut peer,
                |frame| matches!(frame, Incoming::State { state } if state.loading),
            )
            .await;
            live_message(
                &mut peer,
                |frame| matches!(frame, Incoming::State { state } if !state.loading),
            )
            .await;
        }
    }
    drop(peer);
    let (mut recovered, hello) = tokio::time::timeout(Duration::from_secs(5), discover(&port))
        .await
        .unwrap()
        .unwrap();
    assert!(hello.supports("state") && hello.supports("results"));
    let state = live_message(&mut recovered, |frame| {
        matches!(frame, Incoming::State { .. })
    })
    .await;
    assert!(matches!(state,Incoming::State{state} if state.volume==0.0 && !state.loading));
    write_request(recovered.get_mut(), &Request::Play, "empty")
        .await
        .unwrap();
    let failed = live_message(
        &mut recovered,
        |frame| matches!(frame,Incoming::Result{request_id,..} if request_id=="empty"),
    )
    .await;
    assert!(matches!(failed,Incoming::Result{status,..} if status=="failed"));
    write_request(recovered.get_mut(), &Request::CheckQuit, "close")
        .await
        .unwrap();
    let decision = live_message(&mut recovered, |frame| {
        matches!(frame, Incoming::Quit { .. })
    })
    .await;
    assert!(
        matches!(decision, Incoming::Quit { dialog: false }),
        "idle player should not ask for confirmation"
    );
    assert!(
        port.exists(),
        "Go must remain alive until confirmed cleanup is requested"
    );
    write_request(recovered.get_mut(), &Request::Quit, "quit")
        .await
        .unwrap();
    tokio::time::timeout(Duration::from_secs(5), async {
        while port.exists() {
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .expect("confirmed GUI quit must clean up the TUI IPC endpoint");
    drop(terminal);
}

#[tokio::test]
async fn playback_snapshots_and_results_are_recognized() {
    let payload = b"{\"type\":\"state\",\"state\":{\"revision\":1,\"track_id\":0,\"has_track\":false,\"playing\":false,\"loading\":false,\"volume\":0,\"position\":0,\"duration\":0}}\n{\"type\":\"result\",\"request_id\":\"mute\",\"status\":\"failed\",\"error\":\"invalid volume\"}\n";
    let mut reader = BufReader::new(&payload[..]);
    assert!(!matches!(
        read_frame(&mut reader).await.unwrap(),
        Some(Incoming::Other)
    ));
    assert!(!matches!(
        read_frame(&mut reader).await.unwrap(),
        Some(Incoming::Other)
    ));
}

#[tokio::test]
async fn incompatible_protocol_is_rejected_before_commands_are_sent() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let server = tokio::spawn(async move {
        let (socket, _) = listener.accept().await.unwrap();
        let mut peer = BufReader::new(socket);
        let mut token = String::new();
        peer.read_line(&mut token).await.unwrap();
        peer.get_mut()
            .write_all(b"{\"type\":\"hello\",\"protocol\":999}\n")
            .await
            .unwrap();
    });
    let socket = tokio::net::TcpStream::connect(address).await.unwrap();
    assert!(authenticate(socket, "token").await.is_err());
    server.await.unwrap();
}

#[cfg(unix)]
async fn live_message(
    peer: &mut BufReader<TcpStream>,
    accept: impl Fn(&Incoming) -> bool,
) -> Incoming {
    tokio::time::timeout(Duration::from_secs(5), async {
        loop {
            let message = read_frame(peer)
                .await
                .unwrap()
                .expect("player closed before expected message");
            if accept(&message) {
                return message;
            }
        }
    })
    .await
    .expect("player must deliver expected state or result")
}

#[tokio::test]
async fn cross_language_fixture_keeps_snapshot_metadata_and_zero_command_values() {
    let fixture: serde_json::Value =
        serde_json::from_str(include_str!("../../../internal/ipc/testdata/protocol.json")).unwrap();
    let mut payload = serde_json::to_vec(&fixture["state"]).unwrap();
    payload.push(b'\n');
    let mut reader = BufReader::new(payload.as_slice());
    let Some(Incoming::State { state }) = read_frame(&mut reader).await.unwrap() else {
        panic!("snapshot discarded")
    };
    assert_eq!(state.revision, 7);
    assert_eq!(state.track_id, 3);
    assert_eq!(state.title, "雪");
    assert_eq!(state.position, 0.0);
    assert_eq!(state.volume, 0.0);
    assert!(!state.playing);
    let commands = [
        (Request::SetVolume(0.0), "mute"),
        (Request::Seek(0.0), "rewind"),
        (Request::Open("/music/a \"song\" 雪.flac".into()), "open"),
    ];
    for (index, (request, id)) in commands.into_iter().enumerate() {
        let mut data = Vec::new();
        write_request(&mut data, &request, id).await.unwrap();
        let actual: serde_json::Value = serde_json::from_slice(&data).unwrap();
        for (key, expected) in fixture["requests"][index].as_object().unwrap() {
            if key == "volume" || key == "position" {
                assert_eq!(actual[key].as_f64(), expected.as_f64());
            } else {
                assert_eq!(&actual[key], expected);
            }
        }
    }
}

#[tokio::test]
async fn connection_stream_reauthenticates_without_replaying_old_commands() {
    use iced::futures::StreamExt;
    let directory = tempfile::tempdir().unwrap();
    let endpoint = directory.path().join("endpoint");
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let token = "c".repeat(64);
    tokio::fs::write(
        &endpoint,
        format!("{}\n{token}", listener.local_addr().unwrap()),
    )
    .await
    .unwrap();
    let server = tokio::spawn(async move {
        for attempt in 0..2 {
            let (socket, _) = listener.accept().await.unwrap();
            let mut peer = BufReader::new(socket);
            let mut input = String::new();
            peer.read_line(&mut input).await.unwrap();
            assert_eq!(input.trim(), token);
            peer.get_mut()
                .write_all(b"{\"type\":\"hello\",\"protocol\":1,\"capabilities\":[\"results\"]}\n")
                .await
                .unwrap();
            input.clear();
            if attempt == 0 {
                peer.read_line(&mut input).await.unwrap();
                let request: serde_json::Value = serde_json::from_str(&input).unwrap();
                assert_eq!(request["request_id"], "once");
            } else {
                assert!(
                    tokio::time::timeout(Duration::from_millis(150), peer.read_line(&mut input))
                        .await
                        .is_err(),
                    "old command replayed after reconnect"
                );
            }
        }
    });
    let events = events(endpoint);
    iced::futures::pin_mut!(events);
    let mut old = None;
    let mut connections = 0;
    tokio::time::timeout(Duration::from_secs(5), async {
        while let Some(event) = events.next().await {
            if let Event::Connected(sender) = event {
                connections += 1;
                if connections == 1 {
                    sender
                        .send(Command {
                            request: Request::PlayPause,
                            id: "once".into(),
                        })
                        .await
                        .unwrap();
                    old = Some(sender);
                } else {
                    break;
                }
            }
        }
    })
    .await
    .unwrap();
    assert_eq!(connections, 2);
    assert!(
        old.unwrap()
            .try_send(Command {
                request: Request::PlayPause,
                id: "late".into()
            })
            .is_err()
    );
    server.await.unwrap();
}

#[cfg(unix)]
async fn first_run_wizard(can_pick: bool) {
    let player = std::env::var("NEOVIOLET_TEST_PLAYER").expect("make test/gui-ipc provides player");
    let directory = tempfile::tempdir().unwrap();
    let mut terminal = iced_term::Terminal::new(
        710,
        iced_term::settings::Settings {
            backend: iced_term::settings::BackendSettings {
                program: player,
                args: vec!["--xdg-config".into()],
                env: std::collections::HashMap::from([
                    ("TERM_PROGRAM".into(), "neoviolet-gui".into()),
                    ("TERM".into(), "xterm-256color".into()),
                    ("NEOVIOLET_FONT".into(), "Menlo".into()),
                    (
                        "XDG_CONFIG_HOME".into(),
                        directory.path().to_string_lossy().into_owned(),
                    ),
                    (
                        "XDG_CACHE_HOME".into(),
                        directory.path().to_string_lossy().into_owned(),
                    ),
                ]),
                working_directory: Some(directory.path().into()),
            },
            ..Default::default()
        },
    )
    .unwrap();
    let pid = terminal.child_process_id().unwrap();
    let endpoint = std::env::temp_dir().join(format!("neoviolet-ipc-{pid}"));
    let (mut peer, hello) = tokio::time::timeout(Duration::from_secs(10), discover(&endpoint))
        .await
        .unwrap()
        .unwrap();
    assert!(hello.supports("startup") && hello.supports("file_picker"));
    live_message(&mut peer, |message| {
        matches!(
            message,
            Incoming::Phase {
                phase: StartupPhase::Wizard
            }
        )
    })
    .await;
    write_request(
        peer.get_mut(),
        &Request::Open("ignored.flac".into()),
        "blocked",
    )
    .await
    .unwrap();
    let rejected = live_message(
        &mut peer,
        |message| matches!(message,Incoming::Result{request_id,..} if request_id=="blocked"),
    )
    .await;
    assert!(matches!(rejected,Incoming::Result{status,..} if status=="failed"));
    // Wait for Huh's initial focus command; PTY keystrokes then exercise real
    // navigation instead of calling a testing-only completion hook.
    tokio::time::sleep(Duration::from_millis(200)).await;
    terminal.handle(iced_term::Command::ProxyToBackend(
        iced_term::BackendCommand::Write(b"\t".to_vec()),
    ));
    tokio::time::sleep(Duration::from_millis(100)).await;
    if can_pick {
        terminal.handle(iced_term::Command::ProxyToBackend(
            iced_term::BackendCommand::Write(b"\r".to_vec()),
        ));
        let request = live_message(&mut peer, |message| {
            matches!(message, Incoming::PickFile { .. })
        })
        .await;
        let Incoming::PickFile { request_id, .. } = request else {
            unreachable!()
        };
        write_request(
            peer.get_mut(),
            &Request::FilePicked {
                purpose: "soundfont".into(),
                request_id,
                status: "cancelled".into(),
                path: None,
                error: None,
            },
            "",
        )
        .await
        .unwrap();
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    terminal.handle(iced_term::Command::ProxyToBackend(
        iced_term::BackendCommand::Write(b"\t".to_vec()),
    ));
    live_message(&mut peer, |message| {
        matches!(
            message,
            Incoming::Phase {
                phase: StartupPhase::Ready
            }
        )
    })
    .await;
    let configuration = directory.path().join("neoviolet/config.json");
    let saved: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(configuration).unwrap()).unwrap();
    assert_eq!(saved["soundfont_path"], "");
    write_request(peer.get_mut(), &Request::Quit, "")
        .await
        .unwrap();
    tokio::time::timeout(Duration::from_secs(5), async {
        while endpoint.exists() {
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .unwrap();
}

#[cfg(unix)]
#[tokio::test]
#[ignore = "requires built player; make test/gui-ipc"]
async fn live_tui_first_run_allows_cancel_and_optional_skip() {
    first_run_wizard(true).await;
}

#[tokio::test]
async fn startup_fixture_preserves_phase_and_scoped_picker_reply() {
    let fixture: serde_json::Value =
        serde_json::from_str(include_str!("../../../internal/ipc/testdata/protocol.json")).unwrap();
    let mut payload = serde_json::to_vec(&fixture["phase"]).unwrap();
    payload.push(b'\n');
    let mut reader = BufReader::new(payload.as_slice());
    assert!(matches!(
        read_frame(&mut reader).await.unwrap(),
        Some(Incoming::Phase {
            phase: StartupPhase::Wizard
        })
    ));
    let mut payload = serde_json::to_vec(&fixture["pick_file"]).unwrap();
    payload.push(b'\n');
    let mut reader = BufReader::new(payload.as_slice());
    assert!(
        matches!(read_frame(&mut reader).await.unwrap(),Some(Incoming::PickFile{purpose,request_id}) if purpose=="soundfont" && request_id=="soundfont-7")
    );
    let mut output = Vec::new();
    write_request(
        &mut output,
        &Request::FilePicked {
            purpose: "soundfont".into(),
            request_id: "soundfont-7".into(),
            status: "cancelled".into(),
            path: None,
            error: None,
        },
        "",
    )
    .await
    .unwrap();
    let actual: serde_json::Value = serde_json::from_slice(&output).unwrap();
    assert_eq!(actual, fixture["file_picked"]);
}

#[tokio::test]
async fn handshake_library_version_is_optional_and_preserves_runtime_value() {
    let fixture: serde_json::Value =
        serde_json::from_str(include_str!("../../../internal/ipc/testdata/protocol.json")).unwrap();
    for payload in [
        fixture["hello"].clone(),
        serde_json::json!({"type":"hello","version":"dev"}),
    ] {
        let expected_library = payload
            .get("libopenmpt_version")
            .and_then(|value| value.as_str())
            .unwrap_or("");
        let mut data = serde_json::to_vec(&payload).unwrap();
        data.push(b'\n');
        let mut reader = BufReader::new(data.as_slice());
        let Some(Incoming::Hello(hello)) = read_frame(&mut reader).await.unwrap() else {
            panic!("handshake not recognized")
        };
        assert_eq!(hello.version, "dev");
        assert_eq!(hello.libopenmpt_version, expected_library);
    }
}
