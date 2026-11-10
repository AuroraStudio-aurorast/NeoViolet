package ui

import (
	"strings"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestSettingsGUIFeedbackWaitsForCurrentAppliedRevision(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	frame := readIPC(t, decoder, "preferences", "")
	revision := uint64(frame["revision"].(float64))
	if !strings.Contains(m.Info.Message, "updating GUI") {
		t.Fatal("saved config was reported as applied before GUI acknowledgment:", m.Info.Message)
	}
	if _, err := m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: revision - 1, Status: "applied"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Info.Message, "updating GUI") {
		t.Fatal("stale acknowledgment was accepted")
	}
	if _, err := m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: revision, Status: "applied"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Info.Message, "GUI updated") {
		t.Fatal(m.Info.Message)
	}
}

func TestSettingsGUIApplyFailureDoesNotUndoSavedPreferences(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	frame := readIPC(t, decoder, "preferences", "")
	_, err := m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: uint64(frame["revision"].(float64)), Status: "failed", Error: "renderer unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	disk, err := config.Load()
	if err != nil || disk.GUI.FontSize != 20 || m.Config.GUI.FontSize != 20 || m.pages.dirty {
		t.Fatal("GUI failure rolled back a successful save")
	}
	if !m.Error.Visible || !strings.Contains(m.Info.Message, "saved") {
		t.Fatal("save and application failure were not distinguished")
	}
}

func TestSettingsGUIOfflineExplainsNextLaunch(t *testing.T) {
	m := wiredSettingsModel(t)
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	if !strings.Contains(m.Info.Message, "next GUI launch") {
		t.Fatal(m.Info.Message)
	}
}

func TestSettingsGUIFeedbackDoesNotReplaceNewSaveInProgress(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	frame := readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Seek step", "21")
	next := m.saveSettings(false)
	before := m.Info.Message
	_, err := m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: uint64(frame["revision"].(float64)), Status: "applied"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Info.Message != before || m.configSave == nil {
		t.Fatal("old GUI result replaced in-progress save")
	}
	completeSettingsCmd(t, m, next)
}

func TestSettingsGUIFeedbackTracksRefreshAndRestartRequired(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	old := uint64(readIPC(t, decoder, "preferences", "")["revision"].(float64))
	_, _ = m.ipcRequest(ipc.Message{Type: "get_state"})
	current := uint64(readIPC(t, decoder, "preferences", "")["revision"].(float64))
	_, _ = m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: old, Status: "applied"})
	if !strings.Contains(m.Info.Message, "updating GUI") {
		t.Fatal("superseded snapshot acknowledged")
	}
	_, err := m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: current, Status: "restart_required", RestartRequired: []string{"gui.neoviolet_path"}})
	if err != nil || !strings.Contains(m.Info.Message, "reopen GUI") {
		t.Fatal(m.Info.Message, err)
	}
}

func TestSettingsGUIFeedbackFromDisconnectedSessionCannotReplaceNotice(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	frame := readIPC(t, decoder, "preferences", "")
	m.Update(ipc.Event{Kind: "disconnected", Session: m.ipcSession})
	before := m.Info.Message
	_, _ = m.ipcRequest(ipc.Message{Type: "preferences_applied", Revision: uint64(frame["revision"].(float64)), Status: "failed", Error: "stale"})
	if m.Info.Message != before || m.Error.Message == "stale" {
		t.Fatal("previous session changed save status")
	}
}

func TestSettingsGUIDisconnectDoesNotReplaceNewSaveInProgress(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	cfg := config.DefaultConfig()
	*m.Config = cfg
	m.pages.initSettings(cfg)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_preferences","preferences_feedback"]}`)
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Terminal font size", "20")
	completeSettingsCmd(t, m, m.saveSettings(false))
	readIPC(t, decoder, "preferences", "")
	editRealSetting(t, m, "Seek step", "21")
	next := m.saveSettings(false)
	before := m.Info.Message
	m.Update(ipc.Event{Kind: "disconnected", Session: m.ipcSession})
	if m.Info.Message != before {
		t.Fatal("disconnect replaced new save progress")
	}
	completeSettingsCmd(t, m, next)
}
