package ui

import (
	"strings"
	"testing"
)

func TestOpenAliasesRaiseCommandLimitForLongURLs(t *testing.T) {
	for _, name := range []string{"open", "load", "e"} {
		t.Run(name, func(t *testing.T) {
			m := commandModeModel(t, name+" ")
			if got := m.Components.CommandInput.CharLimit; got != 4096 {
				t.Fatalf("CharLimit = %d, want 4096 after %s prefix", got, name)
			}
			remoteURL := "https://audio.example.test/track.mp3?token=" + strings.Repeat("x", 300)
			handlePaste(m, remoteURL)
			if got, want := m.Components.CommandInput.Value(), name+" "+remoteURL; got != want {
				t.Fatalf("pasted URL = %q, want %q", got, want)
			}
		})
	}
}

func TestOpenCommandAndAliasesAcceptRemoteAudioURLs(t *testing.T) {
	const remoteURL = "https://audio.example.test/music/track.mp3?token=signed-value"
	for _, name := range []string{"open", "load", "e"} {
		t.Run(name, func(t *testing.T) {
			m := setupModel()
			_, cmd := runOpen(m, invocation{
				Name:  name,
				Parts: []string{name, remoteURL},
				Rest:  remoteURL,
			})
			t.Cleanup(func() {
				if m.pendingLoad != nil {
					m.pendingLoad.cancel()
				}
			})
			if m.Error.Message != "" {
				t.Fatalf("remote URL rejected: %s", m.Error.Message)
			}
			if cmd == nil || !m.Loading {
				t.Fatal("remote URL did not enter the normal audio loading path")
			}
		})
	}
}

func TestOpenCommandRejectsNonHTTPRemoteSources(t *testing.T) {
	m := setupModel()
	_, cmd := runOpen(m, invocation{Rest: "ftp://audio.example.test/track.mp3"})
	if cmd != nil || m.Loading {
		t.Fatal("unsupported URL scheme entered the audio loading path")
	}
	if m.Error.Message == "" {
		t.Fatal("unsupported URL scheme was accepted")
	}
}

func TestIPCOpenAcceptsRemoteAudioURL(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	const remoteURL = "https://audio.example.test/music/track.mp3?token=signed-value"
	requestIPC(t, m, conn, `{"type":"open","request_id":"remote-open","path":"`+remoteURL+`"}`)
	result := readIPC(t, decoder, "result", "remote-open")
	if result["status"] != "accepted" {
		t.Fatalf("IPC open status = %v, want accepted (result: %v)", result["status"], result)
	}
	if !m.Loading {
		t.Fatal("IPC URL did not enter the normal audio loading path")
	}
	t.Cleanup(func() {
		if m.pendingLoad != nil {
			m.pendingLoad.cancel()
		}
	})
}
