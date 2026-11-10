use super::*;

#[test]
fn startup_phase_and_soundfont_requests_are_recognized() {
    for wire in [
        r#"{"type":"phase","phase":"wizard"}"#,
        r#"{"type":"pick_file","purpose":"soundfont","request_id":"wizard-1"}"#,
    ] {
        let message: Incoming = serde_json::from_str(wire).unwrap();
        assert!(
            !matches!(message, Incoming::Other),
            "startup messages must be dispatched"
        );
    }
}

#[tokio::test]
async fn startup_handshake_announces_gui_capability_before_any_command() {
    for supports_startup in [false, true] {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let server = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut peer = BufReader::new(stream);
            let mut line = String::new();
            peer.read_line(&mut line).await.unwrap();
            assert_eq!(line, "token\n");
            let capabilities = if supports_startup {
                vec!["startup", "file_picker"]
            } else {
                vec![]
            };
            let hello =
                serde_json::json!({"type":"hello", "protocol":1, "capabilities":capabilities});
            peer.get_mut()
                .write_all(format!("{hello}\n").as_bytes())
                .await
                .unwrap();
            line.clear();
            peer.read_line(&mut line).await.unwrap();
            let first: serde_json::Value = serde_json::from_str(&line).unwrap();
            if supports_startup {
                assert_eq!(
                    first,
                    serde_json::json!({"type":"client_hello", "client":"neoviolet-gui", "protocol":1, "capabilities":["file_picker","audio_file_picker","settings_preferences","settings_file_picker"]})
                );
                line.clear();
                peer.read_line(&mut line).await.unwrap();
                assert_eq!(
                    serde_json::from_str::<serde_json::Value>(&line).unwrap()["type"],
                    "get_state"
                );
            } else {
                assert_eq!(first["type"], "get_state");
            }
        });
        let stream = TcpStream::connect(address).await.unwrap();
        let (mut peer, _) = authenticate(stream, "token").await.unwrap();
        write_request(peer.get_mut(), &Request::GetState, "")
            .await
            .unwrap();
        tokio::time::timeout(Duration::from_secs(3), server)
            .await
            .unwrap()
            .unwrap();
    }
}

#[tokio::test]
async fn picker_replies_keep_status_request_and_optional_fields_on_the_wire() {
    for purpose in ["soundfont", "audio", "library_directory", "player_binary"] {
        for (status, path, error) in [
            ("selected", Some(PathBuf::from("/tmp/a bank 雪.sf2")), None),
            ("cancelled", None, None),
            ("failed", None, Some("Window unavailable".to_string())),
        ] {
            let request = Request::FilePicked {
                purpose: purpose.into(),
                request_id: "pick-1".into(),
                status: status.into(),
                path: path.clone(),
                error: error.clone(),
            };
            let mut output = Vec::new();
            write_request(&mut output, &request, "").await.unwrap();
            let wire: serde_json::Value = serde_json::from_slice(&output).unwrap();
            assert_eq!(wire["type"], "file_picked");
            assert_eq!(wire["purpose"], purpose);
            assert_eq!(wire["request_id"], "pick-1");
            assert_eq!(wire["status"], status);
            assert_eq!(
                wire.get("path").cloned(),
                path.map(|value| serde_json::json!(value))
            );
            assert_eq!(
                wire.get("error").cloned(),
                error.map(|value| serde_json::json!(value))
            );
        }
    }
}
