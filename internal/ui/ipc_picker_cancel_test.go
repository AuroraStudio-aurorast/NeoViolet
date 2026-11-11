package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestNativePickerEscapeAcknowledgesLateCompletionAndTimeoutRetry(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'g', Mod: tea.ModCtrl}} {
		t.Run(key.String(), func(t *testing.T) {
			m, conn, decoder := ipcModel(t)
			requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker","settings_file_picker"]}`)
			m.UI.ActiveTab = 0
			m.UI.Focus = FocusContent
			_ = pageKey(m, 'o', "o")
			pick := readIPC(t, decoder, "pick_file", "")
			_, _ = m.Update(key)
			if m.ipcPickerID != "" {
				t.Fatal("Escape did not cancel Go input ownership")
			}
			id := pick["request_id"].(string)
			for _, status := range []string{"selected", "cancelled"} {
				cmd, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: id, Status: status, Path: "/tmp/ignored.flac"})
				if err != nil || cmd != nil {
					t.Fatal("cancelled chooser started work")
				}
				if got := readIPC(t, decoder, "result", id)["status"]; got != "cancelled" {
					t.Fatal("GUI cannot release the late chooser:", got)
				}
				if m.Loading || m.pages.dirty {
					t.Fatal("cancelled reply changed track or preferences")
				}
			}
		})
	}
}

func TestLateCancelledPickerDoesNotReleaseNewPicker(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker","settings_file_picker"]}`)
	m.UI.ActiveTab = 0
	m.UI.Focus = FocusContent
	_ = pageKey(m, 'o', "o")
	old := readIPC(t, decoder, "pick_file", "")["request_id"].(string)
	_ = pageKey(m, tea.KeyEscape, "")
	_ = pageKey(m, 'o', "o")
	current := readIPC(t, decoder, "pick_file", "")["request_id"].(string)
	_, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: old, Status: "selected", Path: "/tmp/ignored.flac"})
	if err != nil {
		t.Fatal(err)
	}
	if readIPC(t, decoder, "result", old)["status"] != "cancelled" {
		t.Fatal("old chooser not acknowledged")
	}
	if m.ipcPickerID != current || m.Loading {
		t.Fatal("old reply released or replaced new picker")
	}
}

func TestPickerQuitCancellationAcknowledgesLateNativeResult(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker","settings_file_picker"]}`)
	m.UI.ActiveTab = 0
	m.UI.Focus = FocusContent
	_ = pageKey(m, 'o', "o")
	id := readIPC(t, decoder, "pick_file", "")["request_id"].(string)
	if err := m.requestGUIQuit(); err != nil {
		t.Fatal(err)
	}
	_, err := m.ipcRequest(ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: id, Status: "cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	if readIPC(t, decoder, "result", id)["status"] != "cancelled" {
		t.Fatal("Quit cleared picker without acknowledging late cancellation")
	}
}
