package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestNativeSettingsPickersChangeOnlyDraft(t *testing.T) {
	for _, tc := range []struct{ label, purpose, name string }{
		{"MIDI SoundFont", "soundfont", "orchestra.sf2"},
		{"Music folders", "library_directory", "music"},
		{"Player path (next launch)", "player_binary", "neoviolet.exe"},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			m, conn, decoder := ipcModel(t)
			cfg := config.DefaultConfig()
			m.Config = &cfg
			m.pages.initSettings(cfg)
			requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker","settings_file_picker"]}`)
			m.UI.ActiveTab = 3
			m.UI.Focus = FocusContent
			m.pages.cursors[3] = settingIndex(m, tc.label)
			_ = pageKey(m, tea.KeyEnter, "")
			if tc.purpose == "library_directory" {
				_ = pageKey(m, 'a', "a")
			}
			if m.pages.browser != nil {
				t.Fatal("GUI must host the chooser")
			}
			pick := readIPC(t, decoder, "pick_file", "")
			if pick["purpose"] != tc.purpose {
				t.Fatal(pick)
			}
			path := filepath.Join(t.TempDir(), tc.name)
			if tc.purpose == "library_directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.purpose == "player_binary" {
				// #nosec G302 -- private test fixture needs owner execute permission for player validation.
				if err := os.Chmod(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := m.Config.Clone()
			cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: tc.purpose, RequestID: pick["request_id"].(string), Status: "selected", Path: path})
			if err != nil || cmd == nil {
				t.Fatalf("selection must validate off Update: %v", err)
			}
			if m.pages.dirty {
				t.Fatal("unvalidated selection applied synchronously")
			}
			_, _ = m.Update(cmd())
			if !m.pages.dirty || m.Config.SoundfontPath != before.SoundfontPath || m.Config.GUI.NeovioletPath != before.GUI.NeovioletPath || len(m.Config.Library.Roots) != 0 {
				t.Fatal("selection did not remain in draft")
			}
			switch tc.purpose {
			case "soundfont":
				if m.pages.draft.SoundfontPath != path {
					t.Fatal("SoundFont lost")
				}
			case "library_directory":
				if len(m.pages.draft.Library.Roots) != 1 || m.pages.draft.Library.Roots[0] != path || !m.pages.rootEditor {
					t.Fatal("folder draft lost")
				}
			case "player_binary":
				if m.pages.draft.GUI.NeovioletPath != path {
					t.Fatal("binary lost")
				}
			}
		})
	}
}

func TestNativePickerCancelAndFailedFallbackKeepDraft(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	field := settingIndex(m, "MIDI SoundFont")
	for _, status := range []string{"cancelled", "failed"} {
		m.UI.ActiveTab = 3
		m.UI.Focus = FocusContent
		m.pages.cursors[3] = field
		_ = pageKey(m, tea.KeyEnter, "")
		pick := readIPC(t, decoder, "pick_file", "")
		cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: pick["request_id"].(string), Status: status})
		if err != nil {
			t.Fatal(err)
		}
		if status == "failed" {
			if cmd == nil || m.pages.browser == nil {
				t.Fatal("native failure needs a terminal browser fallback")
			}
			browserResult(t, m, cmd)
			_ = pageKey(m, tea.KeyEscape, "")
		}
		if m.pages.dirty {
			t.Fatal("cancel/failure changed draft")
		}
	}
}

func TestNativePickerRejectsStaleValidationAfterDisconnect(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.pages.cursors[3] = settingIndex(m, "MIDI SoundFont")
	_ = pageKey(m, tea.KeyEnter, "")
	pick := readIPC(t, decoder, "pick_file", "")
	path := filepath.Join(t.TempDir(), "bank.sf2")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: pick["request_id"].(string), Status: "selected", Path: path})
	if err != nil || cmd == nil {
		t.Fatal("missing validation")
	}
	msg := cmd()
	m.Update(ipc.Event{Kind: "disconnected", Session: m.ipcSession})
	m.Update(msg)
	if m.pages.dirty || m.pages.draft.SoundfontPath != "" {
		t.Fatal("disconnected chooser edited draft")
	}
}

func TestNativeAudioPickerHomeAndGUIEntryShareProtocol(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker","settings_file_picker"]}`)
	m.UI.ActiveTab = 0
	m.UI.Focus = FocusContent
	_ = pageKey(m, 'o', "o")
	first := readIPC(t, decoder, "pick_file", "")
	if first["purpose"] != "audio" {
		t.Fatal(first)
	}
	cancelled, _ := json.Marshal(ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: first["request_id"].(string), Status: "cancelled"})
	requestIPC(t, m, conn, string(cancelled))
	requestIPC(t, m, conn, `{"type":"open_picker","purpose":"audio","request_id":"menu"}`)
	pick := readIPC(t, decoder, "pick_file", "")
	if pick["purpose"] != "audio" {
		t.Fatal(pick)
	}
	if readIPC(t, decoder, "result", "menu")["status"] != "succeeded" {
		t.Fatal("GUI picker request rejected")
	}
}

func TestNativePickerBadSelectionDoesNotMutateDraft(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.pages.cursors[3] = settingIndex(m, "MIDI SoundFont")
	_ = pageKey(m, tea.KeyEnter, "")
	pick := readIPC(t, decoder, "pick_file", "")
	cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: pick["request_id"].(string), Status: "selected", Path: "/missing.txt"})
	if err != nil || cmd == nil {
		t.Fatal("expected asynchronous validation")
	}
	_, _ = m.Update(cmd())
	if m.pages.dirty || !m.Error.Visible || m.ipcPickerID != "" {
		t.Fatal("invalid selection changed draft or retained input ownership")
	}
}

func TestNativePickerQuitRejectsLateValidation(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.pages.cursors[3] = settingIndex(m, "MIDI SoundFont")
	_ = pageKey(m, tea.KeyEnter, "")
	pick := readIPC(t, decoder, "pick_file", "")
	path := filepath.Join(t.TempDir(), "bank.sf2")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: pick["request_id"].(string), Status: "selected", Path: path})
	if err != nil || cmd == nil {
		t.Fatal("missing validation")
	}
	msg := cmd()
	if err := m.requestGUIQuit(); err != nil {
		t.Fatal(err)
	}
	m.Update(msg)
	if m.pages.dirty {
		t.Fatal("quit accepted a late setting selection")
	}
}

func TestNativePickerInputOwnershipAndEscape(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.pages.cursors[3] = settingIndex(m, "MIDI SoundFont")
	_ = pageKey(m, tea.KeyEnter, "")
	pick := readIPC(t, decoder, "pick_file", "")
	old := m.pages.cursors[3]
	_ = pageKey(m, tea.KeyDown, "")
	if m.pages.cursors[3] != old {
		t.Fatal("chooser did not own input")
	}
	_ = pageKey(m, tea.KeyEscape, "")
	if m.ipcPickerID != "" {
		t.Fatal("escape did not invalidate selection")
	}
	cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: pick["request_id"].(string), Status: "selected", Path: "/tmp/ignored.sf2"})
	if err != nil || cmd != nil || m.pages.dirty {
		t.Fatal("cancelled chooser resurrected")
	}
}

func TestPlayerBrowserOffersOnlyExecutableFiles(t *testing.T) {
	m := wiredSettingsModel(t)
	dir := browserFixture(t)
	program := filepath.Join(dir, "neoviolet.exe")
	if err := os.WriteFile(program, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	// #nosec G302 -- private test fixture needs owner execute permission for player validation.
	if err := os.Chmod(program, 0700); err != nil {
		t.Fatal(err)
	}
	field := settingIndex(m, "Player path (next launch)")
	browserResult(t, m, m.openFilePicker("player_binary", field, dir))
	b := m.pages.browser
	found := false
	for i, e := range b.entries {
		if !e.directory {
			if e.path != program {
				t.Fatal("non-program offered:", e.path)
			}
			found = true
			b.cursor = i
		}
	}
	if !found {
		t.Fatal("executable missing")
	}
	browserResult(t, m, pageKey(m, tea.KeyEnter, ""))
	if m.Config.GUI.NeovioletPath != "" || m.pages.draft.GUI.NeovioletPath != program {
		t.Fatal("terminal fallback did not edit draft")
	}
	completeSettingsCmd(t, m, pageKey(m, 's', "s"))
	if m.Config.GUI.NeovioletPath != program {
		t.Fatal("program path not saved")
	}
}

func TestNativeValidationCancellationAcknowledgesAndRejectsLateResult(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["settings_file_picker"]}`)
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.pages.cursors[3] = settingIndex(m, "MIDI SoundFont")
	_ = pageKey(m, tea.KeyEnter, "")
	pick := readIPC(t, decoder, "pick_file", "")
	id := pick["request_id"].(string)
	path := filepath.Join(t.TempDir(), "bank.sf2")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: id, Status: "selected", Path: path})
	if err != nil || cmd == nil {
		t.Fatal("missing validation")
	}
	late := cmd()
	_, err = m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "soundfont", RequestID: id, Status: "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	if readIPC(t, decoder, "result", id)["status"] != "cancelled" {
		t.Fatal("cancellation wasn't acknowledged")
	}
	m.Update(late)
	if m.pages.dirty || m.ipcPickerID != "" {
		t.Fatal("late validated selection changed draft")
	}
}
