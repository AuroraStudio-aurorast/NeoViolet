package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestOpenCommandUsesNegotiatedAudioPickerAndLoadsSelection(t *testing.T) {
	for _, name := range []string{"open", "load", "e"} {
		t.Run(name, func(t *testing.T) {
			m, conn, decoder := ipcModel(t)
			requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker","audio_file_picker"]}`)
			setCommand(m, name)
			executeCommand(m)
			if m.Error.Message != "" {
				t.Fatalf("native picker rejected: %s", m.Error.Message)
			}
			pick := readIPC(t, decoder, "pick_file", "")
			if pick["purpose"] != "audio" || pick["request_id"] == "" {
				t.Fatalf("bad picker request: %v", pick)
			}
			path := filepath.Join(t.TempDir(), "a  song 雪.flac")
			if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			reply, _ := json.Marshal(ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: pick["request_id"].(string), Status: "selected", Path: path})
			requestIPC(t, m, conn, string(reply))
			if !m.Loading {
				t.Fatal("native selection did not enter the TUI loading path")
			}
		})
	}
}

func TestAudioPickerCancellationFailureAndStaleRepliesPreserveTrack(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker"]}`)
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	for _, status := range []string{"cancelled", "failed", "selected"} {
		m.Error.Message = ""
		runOpen(m, invocation{})
		if m.Error.Message != "" {
			t.Fatalf("native picker rejected: %s", m.Error.Message)
		}
		pick := readIPC(t, decoder, "pick_file", "")
		id := pick["request_id"].(string)
		requestIPC(t, m, conn, `{"type":"file_picked","purpose":"audio","request_id":"obsolete","status":"selected","path":"/tmp/ignored.flac"}`)
		raw := fmt.Sprintf(`{"type":"file_picked","purpose":"audio","request_id":%q,"status":%q,"path":"/tmp/invalid.txt","error":"Picker unavailable"}`, id, status)
		requestIPC(t, m, conn, raw)
		if m.Loading || m.Audio.Player != player || player.closeCalled {
			t.Fatal("cancelled, failed or invalid selection replaced current track")
		}
		if status == "cancelled" && m.Error.Message != "" {
			t.Fatal("cancellation reported an error")
		}
		if status != "cancelled" && m.Error.Message == "" {
			t.Fatal("picker failure or invalid selection was silent")
		}
	}
}

func TestAudioPickerRequiresCapabilityAndDiscardsDisconnectedRequests(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker"]}`)
	runOpen(m, invocation{})
	if m.pages.browser == nil || m.Error.Message != "" {
		t.Fatal("old GUI must fall back to terminal browser")
	}
	requestIPC(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["audio_file_picker"]}`)
	m.Error.Message = ""
	runOpen(m, invocation{})
	if m.Error.Message != "" {
		t.Fatalf("native picker rejected: %s", m.Error.Message)
	}
	pick := readIPC(t, decoder, "pick_file", "")
	m.Update(ipc.Event{Kind: "disconnected", Session: m.ipcSession})
	m.Update(ipc.Event{Kind: "connected", Session: m.ipcServer.Session()})
	reply := ipc.Message{Type: "file_picked", Purpose: "audio", RequestID: pick["request_id"].(string), Status: "selected", Path: "/tmp/ignored.flac"}
	m.Update(ipc.Event{Kind: "request", Session: m.ipcSession, Message: reply})
	if m.Loading {
		t.Fatal("selection survived disconnect")
	}
	runOpen(m, invocation{})
	if m.pages.browser == nil || m.ipcPickerID != "" {
		t.Fatal("disconnected native picker did not fall back to terminal browser")
	}
}
