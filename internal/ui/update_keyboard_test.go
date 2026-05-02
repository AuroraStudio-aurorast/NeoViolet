package ui

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

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
