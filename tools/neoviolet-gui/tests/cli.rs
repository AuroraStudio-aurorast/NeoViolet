use std::process::Command;

#[test]
fn gui_help_exits_without_creating_settings_or_starting_the_player() {
    for option in ["--help", "-h"] {
        let temporary = tempfile::tempdir().unwrap();
        let configuration = temporary.path().join("configuration");
        let output = Command::new(env!("CARGO_BIN_EXE_neoviolet-gui"))
            .args(["--seek", "25", option])
            .env("XDG_CONFIG_HOME", &configuration)
            .env_remove("DISPLAY")
            .env_remove("WAYLAND_DISPLAY")
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        let help = String::from_utf8(output.stdout).unwrap();
        assert!(help.starts_with("NeoViolet GUI "));
        assert!(help.contains("Usage:"));
        assert!(help.contains("config.json"));
        assert!(help.contains("--vol PERCENT"));
        assert!(
            !configuration.exists(),
            "help must not load or create configuration"
        );
    }
}
