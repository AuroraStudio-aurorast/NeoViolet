package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestGUIPreferencesNegotiationAndExplicitRefresh(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	if _, err := fmt.Fprintln(conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences"]}`); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
	first := readIPC(t, decoder, "preferences", "")
	if first["revision"] != float64(1) {
		t.Fatalf("first preferences: %v", first)
	}
	if _, err := fmt.Fprintln(conn, `{"type":"get_state"}`); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
	next := readIPC(t, decoder, "preferences", "")
	if next["revision"] != float64(2) {
		t.Fatalf("explicit snapshot: %v", next)
	}
}

func TestGUIRuntimePreferenceSavesOnlyRequestedKeyAndPreservesDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	old := m.Config.GUI.FontSize
	cmd, err := m.ipcRequest(ipc.Message{Type: "set_preference", Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil || cmd == nil {
		t.Fatalf("save: %v", err)
	}
	if m.Config.GUI.FontSize != old {
		t.Fatal("changed before successful save")
	}
	completeSettingsCmd(t, m, cmd)
	cfg, err := config.Load()
	if err != nil || cfg.GUI.FontSize != 20 || cfg.SeekStep != 13 {
		t.Fatalf("runtime save wrote draft: %+v %v", cfg, err)
	}
	if !m.pages.dirty || m.pages.draft.SeekStep != 21 || m.pages.draft.GUI.FontSize != 20 {
		t.Fatal("unrelated draft lost or saved value stale")
	}
}

func TestGUIRuntimePreferenceRejectsConflictsAndUnknownKeys(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Terminal font size", "18")
	for _, req := range []ipc.Message{
		{Type: "set_preference", Key: "gui.font_size", Value: json.RawMessage(`20`)},
		{Type: "set_preference", Key: "gui.opacity", Value: json.RawMessage(`0.5`)},
		{Type: "set_preference", Key: "desktop_lyrics.enabled", Value: json.RawMessage(`"yes"`)},
	} {
		if _, err := m.ipcRequest(req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
}

func TestGUISettingsSectionNavigation(t *testing.T) {
	m := wiredSettingsModel(t)
	for _, section := range []string{"gui", "desktop_lyrics"} {
		if _, err := m.ipcRequest(ipc.Message{Type: "open_settings", Section: section}); err != nil {
			t.Fatal(err)
		}
		if got := m.pages.settings[m.pages.cursors[3]].key; got != section+map[string]string{"gui": ".monospace_font", "desktop_lyrics": ".enabled"}[section] {
			t.Fatal(got)
		}
	}
	if _, err := m.ipcRequest(ipc.Message{Type: "open_settings", Section: "bad"}); err == nil {
		t.Fatal("unknown section accepted")
	}
}

func TestGUIRuntimeSavePreservesFormatPickerDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	slices.Reverse(m.pages.formats)
	m.pages.setFormatDraft(slices.Clone(m.pages.formats))
	order := slices.Clone(m.pages.formats)
	enabled := slices.Clone(m.pages.enabled)
	cmd, err := m.saveGUIPreference(ipc.Message{Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil {
		t.Fatal(err)
	}
	completeSettingsCmd(t, m, cmd)
	if !slices.Equal(m.pages.formats, order) || !slices.Equal(m.pages.enabled, enabled) || !slices.Equal(m.pages.draft.Lyrics.FormatPriority, order) {
		t.Fatal("format picker draft reset by an unrelated GUI save")
	}
}

func TestGUIRuntimeSaveDoesNotSwallowExplicitSave(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	cmd, err := m.saveGUIPreference(ipc.Message{Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil {
		t.Fatal(err)
	}
	_ = m.saveSettings(false)
	followup := m.finishSettingsSave(cmd().(configSavedMsg))
	if followup == nil {
		t.Fatal("explicit save was swallowed by runtime save")
	}
	completeSettingsCmd(t, m, followup)
	disk, err := config.Load()
	if err != nil || disk.SeekStep != 21 || disk.GUI.FontSize != 20 || m.pages.dirty {
		t.Fatalf("explicit save lost: %+v %v", disk, err)
	}
}

func TestGUIRuntimeSaveShutdownJoinsQueuedExplicitSave(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	_, err := m.saveGUIPreference(ipc.Message{Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil {
		t.Fatal(err)
	}
	_ = m.saveSettings(false)
	m.Close()
	disk, err := config.Load()
	if err != nil || disk.SeekStep != 21 || disk.GUI.FontSize != 20 {
		t.Fatal("shutdown lost queued save")
	}
}

func TestGUIRuntimeSaveFailureKeepsLiveConfigAndDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Seek step", "21")
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	old := m.Config.GUI.FontSize
	cmd, err := m.saveGUIPreference(ipc.Message{Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil {
		t.Fatal(err)
	}
	completeSettingsCmd(t, m, cmd)
	if m.Config.GUI.FontSize != old || !m.pages.dirty || m.pages.draft.SeekStep != 21 || !m.Error.Visible {
		t.Fatal("failed runtime save changed preferences or lost draft")
	}
}

func TestGUIRuntimeSaveReceiptFollowsDiskWrite(t *testing.T) {
	m, _, decoder := ipcModel(t)
	readIPC(t, decoder, "state", "")
	cmd, err := m.ipcRequest(ipc.Message{Type: "set_preference", RequestID: "font-save", Key: "gui.font_size", Value: json.RawMessage(`20`)})
	if err != nil {
		t.Fatal(err)
	}
	completeSettingsCmd(t, m, cmd)
	receipt := readIPC(t, decoder, "result", "font-save")
	disk, err := config.Load()
	if receipt["status"] != "succeeded" || err != nil || disk.GUI.FontSize != 20 {
		t.Fatalf("receipt did not follow persistence: %v %v", receipt, err)
	}
}
