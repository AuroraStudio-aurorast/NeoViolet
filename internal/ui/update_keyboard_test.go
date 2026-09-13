package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

func TestMain(m *testing.M) {
	testMode = true
	os.Exit(m.Run())
}

func setupModel() *Model {
	return NewModel("", &config.Config{
		CommandHistory: config.CommandHistoryConfig{Max: 50},
		Error:          config.ErrorConfig{Duration: 90},
		VolumeStep:     0.1,
		ProgressBar:    config.ProgressBarConfig{Fill: []string{"a", "b"}},
		VolumeBar:      config.VolumeBarConfig{Fill: []string{"c", "d"}, Width: 10, ShowPercentage: true},
	})
}

func setCommand(m *Model, cmd string) {
	m.Components.CommandInput.SetValue(cmd)
	m.UI.Mode = ModeCommand
}

func TestExecuteCommand_quit(t *testing.T) {
	m := setupModel()
	setCommand(m, "quit")
	_, cmd := executeCommand(m)
	if cmd == nil {
		t.Error("expected non-nil Cmd for quit")
	}
}

func TestExecuteCommand_quitShort(t *testing.T) {
	m := setupModel()
	setCommand(m, "q")
	_, cmd := executeCommand(m)
	if cmd == nil {
		t.Error("expected non-nil Cmd for q")
	}
}

func TestExecuteCommand_quitBang(t *testing.T) {
	m := setupModel()
	setCommand(m, "q!")
	_, cmd := executeCommand(m)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd for q!")
	}
	if m.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", m.ExitCode)
	}
}

func TestExecuteCommand_empty(t *testing.T) {
	m := setupModel()
	setCommand(m, "")
	_, cmd := executeCommand(m)
	if cmd != nil {
		t.Error("expected nil Cmd for empty command")
	}
}

func TestExecuteCommand_p(t *testing.T) {
	m := setupModel()
	setCommand(m, "p")
	_, cmd := executeCommand(m)
	if m.UI.Mode != ModeNormal {
		t.Error("mode should return to normal after command")
	}
	_ = cmd // toggle returns nil Cmd
}

func TestExecuteCommand_vol(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol 0.5")
	_, cmd := executeCommand(m)
	if cmd != nil {
		t.Error("expected nil Cmd for vol")
	}
	if m.Audio.Volume != 0.5 {
		t.Errorf("Volume = %f, want 0.5", m.Audio.Volume)
	}
}

func TestExecuteCommand_volNoArg(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol")
	_, cmd := executeCommand(m)
	if cmd != nil {
		t.Error("expected nil Cmd for vol without arg")
	}
	if !m.Error.Visible {
		t.Error("expected error for vol without arg")
	}
}

func TestExecuteCommand_volOutOfRange(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol 1.5")
	executeCommand(m)
	if !m.Error.Visible {
		t.Error("expected error for vol out of range")
	}
}

func TestExecuteCommand_volNegative(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol -0.5")
	executeCommand(m)
	if !m.Error.Visible {
		t.Error("expected error for vol negative")
	}
}

func TestExecuteCommand_seekAbsolute(t *testing.T) {
	mp := &mockPlayer{duration: 200 * time.Second}
	m := setupModel()
	m.Audio.Player = mp

	setCommand(m, "seek 30")
	executeCommand(m)

	if !mp.seekCalled || mp.lastSeekPos != 30*time.Second {
		t.Errorf("Seek not called with 30s, got %v", mp.lastSeekPos)
	}
}

func TestExecuteCommand_seekTimestamp(t *testing.T) {
	mp := &mockPlayer{duration: 200 * time.Second}
	m := setupModel()
	m.Audio.Player = mp

	setCommand(m, "seek 1:30")
	executeCommand(m)

	if mp.lastSeekPos != 90*time.Second {
		t.Errorf("Seek called with %v, want 90s", mp.lastSeekPos)
	}
}

func TestExecuteCommand_seekRelative(t *testing.T) {
	mp := &mockPlayer{position: 60 * time.Second, duration: 200 * time.Second}
	m := setupModel()
	m.Audio.Player = mp

	setCommand(m, "seek +30")
	executeCommand(m)

	if mp.lastSeekPos != 90*time.Second {
		t.Errorf("Seek called with %v, want 90s", mp.lastSeekPos)
	}
}

func TestExecuteCommand_seekInvalid(t *testing.T) {
	mp := &mockPlayer{duration: 200 * time.Second}
	m := setupModel()
	m.Audio.Player = mp

	setCommand(m, "seek abc")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for invalid seek")
	}
}

func TestExecuteCommand_unknown(t *testing.T) {
	m := setupModel()
	setCommand(m, "foobar")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for unknown command")
	}
}

func TestExecuteCommand_lrc_status_no_lyrics(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc")
	executeCommand(m)

	if !m.Info.Visible {
		t.Error("expected status message for lrc without args")
	}
}

func TestExecuteCommand_lrc_status_showing(t *testing.T) {
	m := setupModel()
	m.Audio.Lyrics = &lyrics.Data{}
	m.Audio.ShowLyrics = true
	setCommand(m, "lrc")
	executeCommand(m)

	if !m.Info.Visible {
		t.Error("expected status message")
	}
	if m.Audio.Lyrics == nil {
		t.Error("lyrics should not be nil")
	}
}

func TestExecuteCommand_lrc_off(t *testing.T) {
	m := setupModel()
	m.Audio.Lyrics = &lyrics.Data{}
	m.Audio.ShowLyrics = true
	setCommand(m, "lrc off")
	executeCommand(m)

	if m.Audio.ShowLyrics {
		t.Error("ShowLyrics should be false after lrc off")
	}
	if m.Audio.Lyrics == nil {
		t.Error("Lyrics data should be preserved in memory after lrc off")
	}
}

func TestExecuteCommand_lrc_off_idempotent(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc off")
	executeCommand(m)

	if m.Audio.ShowLyrics {
		t.Error("ShowLyrics should be false")
	}
}

func TestExecuteCommand_lrc_on_no_player(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc on")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for lrc on with no player")
	}
}

func TestExecuteCommand_lrc_on_no_lyrics(t *testing.T) {
	m := setupModel()
	m.Audio.Player = &mockPlayer{path: "/nonexistent/song.mp3"}
	setCommand(m, "lrc on")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error when no lyrics file exists")
	}
}

func TestExecuteCommand_lrc_on_already_loaded(t *testing.T) {
	m := setupModel()
	m.Audio.Lyrics = &lyrics.Data{}
	m.Audio.ShowLyrics = false
	setCommand(m, "lrc on")
	executeCommand(m)

	if !m.Audio.ShowLyrics {
		t.Error("ShowLyrics should be true after lrc on")
	}
	if m.Error.Visible {
		t.Errorf("unexpected error: %s", m.Error.Message)
	}
}

func TestExecuteCommand_lrc_unknown_subcmd(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc foo")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for unknown lrc subcommand")
	}
}

func TestExecuteCommand_lrc_switch_no_format(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc switch")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for lrc switch without format")
	}
}

func TestExecuteCommand_lrc_switch_no_player(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc switch lrc")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for lrc switch with no player")
	}
}

func TestExecuteCommand_lrc_switch_no_file(t *testing.T) {
	m := setupModel()
	m.Audio.Player = &mockPlayer{path: "/nonexistent/song.mp3"}
	setCommand(m, "lrc switch lrc")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error when no lrc file exists for switch")
	}
}

func TestExecuteCommand_lyric_alias(t *testing.T) {
	m := setupModel()
	m.Audio.ShowLyrics = true
	setCommand(m, "lyric off")
	executeCommand(m)

	if m.Audio.ShowLyrics {
		t.Error("ShowLyrics should be false after lyric off")
	}
}

func TestExecuteCommand_lyrics_alias(t *testing.T) {
	m := setupModel()
	m.Audio.ShowLyrics = true
	setCommand(m, "lyrics off")
	executeCommand(m)

	if m.Audio.ShowLyrics {
		t.Error("ShowLyrics should be false after lyrics off")
	}
}

func TestCommandHistory(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol 0.5")
	executeCommand(m)

	if len(m.CommandHistory) != 1 || m.CommandHistory[0] != "vol 0.5" {
		t.Errorf("CommandHistory = %v, want [vol 0.5]", m.CommandHistory)
	}

	// Same command should move to top (back of slice)
	setCommand(m, "vol 0.5")
	executeCommand(m)
	if len(m.CommandHistory) != 1 {
		t.Errorf("duplicate should not increase history length: %d", len(m.CommandHistory))
	}
}

func TestCommandHistoryMax(t *testing.T) {
	m := setupModel()
	m.Config.CommandHistory.Max = 3

	for i := 0; i < 5; i++ {
		setCommand(m, "cmd")
		executeCommand(m)
		// Reset for next command
		m.Error.Visible = false
		m.Error.Message = ""
	}

	if len(m.CommandHistory) > 3 {
		t.Errorf("CommandHistory length %d exceeds max 3", len(m.CommandHistory))
	}
}

func TestCommandModeNormalExit(t *testing.T) {
	m := setupModel()
	setCommand(m, "vol 0.5")
	executeCommand(m)

	if m.UI.Mode != ModeNormal {
		t.Error("mode should return to ModeNormal after command execution")
	}
}

func TestExecuteCommand_whitespaceOnly(t *testing.T) {
	m := setupModel()
	setCommand(m, "   ")
	_, cmd := executeCommand(m)
	if cmd != nil {
		t.Error("expected nil Cmd for whitespace-only command")
	}
}

func TestExecuteCommand_noPlayerSeek(t *testing.T) {
	m := setupModel()
	setCommand(m, "seek 30")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected error for seek without player")
	}
}

// historyTestSetup creates a temp XDG_CONFIG_HOME, enables XDG config paths,
// disables testMode so persistence actually hits disk,
// and returns the neoviolet config directory path for the test.
func historyTestSetup(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	config.SetXDGConfig(true)
	oldTestMode := testMode
	testMode = false
	t.Cleanup(func() {
		config.SetXDGConfig(false)
		testMode = oldTestMode
	})
	return filepath.Join(tmpDir, "neoviolet")
}

func TestHistoryPersistence(t *testing.T) {
	cfgDir := historyTestSetup(t)
	m := setupModel()
	setCommand(m, "vol 0.5")
	executeCommand(m)

	// #nosec G304 -- history file path is derived from the test config dir.
	data, err := os.ReadFile(filepath.Join(cfgDir, "history.txt"))
	if err != nil {
		t.Fatal("history.txt should exist after command execution:", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 || lines[0] != "vol 0.5" {
		t.Errorf("history.txt = %q, want [vol 0.5]", lines)
	}
}

func TestHistoryLoad(t *testing.T) {
	cfgDir := historyTestSetup(t)

	// Pre-write a history file
	historyContent := "seek 30\nvol 0.5\n"
	historyPath := filepath.Join(cfgDir, "history.txt")
	// #nosec G301 -- test fixture dir is world-readable, not a security concern.
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	// #nosec G306 -- test fixture written to a private temp dir.
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatal(err)
	}

	m := setupModel()
	if len(m.CommandHistory) != 2 {
		t.Fatalf("CommandHistory length = %d, want 2", len(m.CommandHistory))
	}
	if m.CommandHistory[0] != "seek 30" {
		t.Errorf("CommandHistory[0] = %q, want %q", m.CommandHistory[0], "seek 30")
	}
	if m.CommandHistory[1] != "vol 0.5" {
		t.Errorf("CommandHistory[1] = %q, want %q", m.CommandHistory[1], "vol 0.5")
	}
}

func TestHistoryMaxRespected(t *testing.T) {
	cfgDir := historyTestSetup(t)
	m := setupModel()
	m.Config.CommandHistory.Max = 3

	for i := 0; i < 5; i++ {
		cmd := "cmd"
		setCommand(m, cmd)
		executeCommand(m)
		// Reset for next command
		m.Error.Visible = false
		m.Error.Message = ""
	}

	// Verify in-memory cap
	if len(m.CommandHistory) > 3 {
		t.Errorf("CommandHistory length %d exceeds max 3", len(m.CommandHistory))
	}

	// Verify file cap
	// #nosec G304 -- history file path is derived from the test config dir.
	data, err := os.ReadFile(filepath.Join(cfgDir, "history.txt"))
	if err != nil {
		t.Fatal("history.txt should exist:", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 3 {
		t.Errorf("history.txt has %d lines, want ≤3", len(lines))
	}
}

func TestExecuteCommand_lrc_panel_modes(t *testing.T) {
	cases := []struct {
		arg  string
		want string
	}{
		{"on", config.PanelModeOn},
		{"off", config.PanelModeOff},
		{"auto", config.PanelModeAuto},
	}

	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			m := setupModel()
			setCommand(m, "lrc panel "+tc.arg)
			executeCommand(m)

			if m.panelMode != tc.want {
				t.Errorf("panelMode = %q, want %q", m.panelMode, tc.want)
			}
			if !m.Info.Visible {
				t.Error("expected a status message")
			}
		})
	}
}

func TestExecuteCommand_lrc_panel_invalidMode(t *testing.T) {
	m := setupModel()
	setCommand(m, "lrc panel sideways")
	executeCommand(m)

	if !m.Error.Visible {
		t.Error("expected an error for an unknown panel mode")
	}
	if m.panelMode != "" {
		t.Errorf("panelMode = %q, want it unchanged", m.panelMode)
	}
}

func TestExecuteCommand_lrc_panel_reportsState(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 24
	setCommand(m, "lrc panel")
	executeCommand(m)

	if !m.Info.Visible {
		t.Fatal("expected a status message")
	}
	if !strings.Contains(m.Info.Message, "shown") {
		t.Errorf("status = %q, want it to report the panel as shown at 100 columns", m.Info.Message)
	}
	if !strings.Contains(m.Info.Message, "32") {
		t.Errorf("status = %q, want it to report the panel width", m.Info.Message)
	}
}

// Design D3: the command is session-scoped, so the config value is untouched.
func TestExecuteCommand_lrc_panel_doesNotPersist(t *testing.T) {
	m := setupModel()
	before := m.Config.Lyrics.Panel

	setCommand(m, "lrc panel off")
	executeCommand(m)

	if m.Config.Lyrics.Panel != before {
		t.Errorf("config panel changed: %+v, want %+v", m.Config.Lyrics.Panel, before)
	}
}

// With the panel hidden because the terminal is narrow, the status must say so
// instead of leaving the user wondering why nothing appeared.
func TestExecuteCommand_lrc_panel_reportsHidden(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 80, 24
	setCommand(m, "lrc panel")
	executeCommand(m)

	if !strings.Contains(m.Info.Message, "hidden") {
		t.Errorf("status = %q, want it to report the panel as hidden", m.Info.Message)
	}
}

// A terminal that is too short shows the resize warning instead of the frame,
// so the status must not claim the panel is visible.
func TestExecuteCommand_lrc_panel_reportsTooSmall(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 10
	setCommand(m, "lrc panel")
	executeCommand(m)

	if !strings.Contains(m.Info.Message, "too small") {
		t.Errorf("status = %q, want it to report the terminal as too small", m.Info.Message)
	}
	if strings.Contains(m.Info.Message, "shown") {
		t.Errorf("status = %q must not claim the panel is shown", m.Info.Message)
	}
}

func TestExecuteCommand_lrc_panel_reportsLyricsOff(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 24
	m.Audio.ShowLyrics = false
	setCommand(m, "lrc panel")
	executeCommand(m)

	if !strings.Contains(m.Info.Message, "lyrics are off") {
		t.Errorf("status = %q, want it to report lyrics as off", m.Info.Message)
	}
}

// mode=off can never show the panel, so the status must spell out that the
// panel is hidden rather than leaving the mode name to imply it.
func TestExecuteCommand_lrc_panel_reportsOff(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 200, 24
	setCommand(m, "lrc panel off")
	executeCommand(m)

	if !strings.Contains(m.Info.Message, "off (hidden)") {
		t.Errorf("status = %q, want it to report the panel as off", m.Info.Message)
	}
}
