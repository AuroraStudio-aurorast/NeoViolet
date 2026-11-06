package ui

import (
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"testing"
)

func TestGUIQuitPolicyUsesTheLiveGoState(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		loaded, playing, loading, confirm bool
	}{
		{name: "idle"},
		{name: "paused", loaded: true, confirm: true},
		{name: "playing", loaded: true, playing: true, confirm: true},
		{name: "loading", loading: true, confirm: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, conn, decoder := ipcModel(t)
			if tc.loaded {
				m.Audio.Player = &mockPlayer{playing: tc.playing}
			}
			m.Loading = tc.loading
			requestIPC(t, m, conn, `{"type":"request_quit","request_id":"close"}`)
			decision := readIPC(t, decoder, "quit", "")
			if decision["dialog"] != tc.confirm {
				t.Fatalf("decision: %v", decision)
			}
			if m.exiting {
				t.Fatal("decision exited before GUI acknowledgement")
			}
		})
	}
}

func TestTUIQuitUsesTheSameConfirmationPolicy(t *testing.T) {
	m, _, decoder := ipcModel(t)
	_, cmd := runQuit(m, invocation{})
	if cmd != nil {
		t.Fatal("GUI should coordinate graceful shutdown")
	}
	decision := readIPC(t, decoder, "quit", "")
	if decision["dialog"] != false {
		t.Fatalf("idle TUI quit still confirms: %v", decision)
	}
	// A paused track remains a loaded track.
	m.Audio.Player = &mockPlayer{}
	_, cmd = runQuit(m, invocation{})
	if cmd != nil {
		t.Fatal("loaded track should wait for GUI confirmation")
	}
	decision = readIPC(t, decoder, "quit", "")
	if decision["dialog"] != true {
		t.Fatalf("paused track did not confirm: %v", decision)
	}
	m.Update(ipc.Event{Kind: "request", Session: m.ipcSession, Message: ipc.Message{Type: "quit"}})
	if !m.exiting {
		t.Fatal("confirmed shutdown did not clean up")
	}
}

func TestQuitDecisionCannotTargetAReplacementSession(t *testing.T) {
	m, _, _ := ipcModel(t)
	m.ipcSession++ // The model still holds the originating, now obsolete generation.
	if err := m.requestGUIQuit(); err == nil {
		t.Fatal("quit decision escaped its originating session")
	}
}
