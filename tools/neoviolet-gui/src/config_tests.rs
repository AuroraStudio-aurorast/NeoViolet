use super::*;

#[test]
fn bootstrap_reads_go_preferences_and_local_geometry_separately() {
    let root = tempfile::tempdir().unwrap();
    std::fs::write(root.path().join("config.json"), r#"{"gui":{"font_size":18,"monospace_font":"Test Mono"},"desktop_lyrics":{"enabled":false,"font_size":28,"position":{"x":900,"y":900}}}"#).unwrap();
    std::fs::write(root.path().join("gui-state.json"), r#"{"main_window":{"width":729,"height":556},"desktop_lyrics":{"position":{"x":299,"y":710}}}"#).unwrap();
    let (cfg, warning) = Config::load_from(root.path());
    assert!(warning.is_none(), "{warning:?}");
    assert_eq!(cfg.font_size, 18);
    assert_eq!(cfg.monospace_font, "Test Mono");
    assert_eq!((cfg.window_width, cfg.window_height), (729, 556));
    assert!(!cfg.desktop_lyrics.enabled);
    assert_eq!(cfg.desktop_lyrics.font_size, 28);
    assert_eq!(cfg.desktop_lyrics.position.unwrap().x, 299.0);
}

#[test]
fn gui_save_never_writes_preferences_or_legacy_toml() {
    let root = tempfile::tempdir().unwrap();
    let original = r#"{"gui":{"font_size":18},"unknown":"keep"}"#;
    std::fs::write(root.path().join("config.json"), original).unwrap();
    let (mut cfg, _) = Config::load_from(root.path());
    cfg.font_size = 24;
    cfg.window_width = 810;
    cfg.desktop_lyrics.position = Some(crate::lyrics::Position { x: 21.0, y: 32.0 });
    cfg.save_to(root.path()).unwrap();
    assert_eq!(
        std::fs::read_to_string(root.path().join("config.json")).unwrap(),
        original
    );
    assert!(!root.path().join("neoviolet_gui.toml").exists());
    let data: serde_json::Value =
        serde_json::from_str(&std::fs::read_to_string(root.path().join("gui-state.json")).unwrap())
            .unwrap();
    assert_eq!(data["main_window"]["width"], 810);
    assert!(data.get("font_size").is_none());
    assert!(data["desktop_lyrics"].get("enabled").is_none());
}

#[test]
fn missing_config_bootstrap_is_read_only_and_defaults_match_go() {
    let root = tempfile::tempdir().unwrap();
    let (cfg, warning) = Config::load_from(root.path());
    assert!(warning.is_none());
    assert_eq!(cfg.font_size, 14);
    assert!(cfg.desktop_lyrics.enabled);
    assert_eq!(cfg.desktop_lyrics.font_size, 32);
    assert_eq!(std::fs::read_dir(root.path()).unwrap().count(), 0);
}

#[test]
fn malformed_local_state_does_not_discard_authoritative_preferences() {
    let root = tempfile::tempdir().unwrap();
    std::fs::write(
        root.path().join("config.json"),
        r#"{"gui":{"font_size":20}}"#,
    )
    .unwrap();
    std::fs::write(root.path().join("gui-state.json"), "broken").unwrap();
    let (cfg, warning) = Config::load_from(root.path());
    assert_eq!(cfg.font_size, 20);
    assert!(warning.is_some());
}
