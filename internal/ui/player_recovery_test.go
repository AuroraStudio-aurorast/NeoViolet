package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestPlayerRecoveryPathIsValidatedAndSavedByGo(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	path := filepath.Join(t.TempDir(), "neoviolet.exe")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	// #nosec G302 -- private test program must have owner execute permission.
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	value, _ := json.Marshal(path)
	cmd, err := m.ipcRequest(ipc.Message{Type: "set_preference", Key: "gui.neoviolet_path", Value: value})
	if err != nil || cmd == nil {
		t.Fatalf("recovery path: %v", err)
	}
	if m.Config.GUI.NeovioletPath != "" {
		t.Fatal("path changed before successful persistence")
	}
	completeSettingsCmd(t, m, cmd)
	disk, err := config.Load()
	if err != nil || disk.GUI.NeovioletPath != path || disk.SeekStep != 13 || m.pages.draft.SeekStep != 21 || !m.pages.dirty {
		t.Fatal("recovery saved unrelated draft or lost player path")
	}
}

func TestPlayerRecoveryRejectsInvalidOrConflictingPath(t *testing.T) {
	for _, value := range []string{`null`, `42`, `"relative/player"`, `"/does/not/exist/neoviolet.exe"`} {
		m := wiredSettingsModel(t)
		cmd, err := m.ipcRequest(ipc.Message{Type: "set_preference", Key: "gui.neoviolet_path", Value: json.RawMessage(value)})
		if cmd != nil {
			completeSettingsCmd(t, m, cmd)
		}
		if err == nil && !m.Error.Visible {
			t.Fatalf("accepted %s", value)
		}
		if m.Config.GUI.NeovioletPath != "" {
			t.Fatal("invalid path applied")
		}
	}
}

func TestPlayerRecoveryDoesNotOverwriteConflictingProgramDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	index := settingIndex(m, "Player path (next launch)")
	if err := m.pages.applyDraftValue(index, "/user/pending-program"); err != nil {
		t.Fatal(err)
	}
	_, err := m.ipcRequest(ipc.Message{Type: "set_preference", Key: "gui.neoviolet_path", Value: json.RawMessage(`"/recovery/program"`)})
	if err == nil || m.pages.draft.GUI.NeovioletPath != "/user/pending-program" || m.Config.GUI.NeovioletPath != "" {
		t.Fatal("program recovery overwrote user draft")
	}
}
