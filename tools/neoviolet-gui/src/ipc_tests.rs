use super::*;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

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
    let mut peer = authenticate(stream, &token).await.unwrap();
    write_request(peer.get_mut(), &Request::Open("/tmp/a song\".flac".into()))
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
    let terminal = iced_term::Terminal::new(
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
    let mut peer = tokio::time::timeout(Duration::from_secs(10), discover(&port))
        .await
        .expect("TUI must start IPC without a desktop window")
        .expect("real authentication acknowledgement");
    write_request(peer.get_mut(), &Request::Quit).await.unwrap();
    tokio::time::timeout(Duration::from_secs(5), async {
        while port.exists() {
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .expect("confirmed GUI quit must clean up the TUI IPC endpoint");
    drop(terminal);
}
