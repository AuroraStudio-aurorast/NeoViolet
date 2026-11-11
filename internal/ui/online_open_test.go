package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRemoteLoadCancellationInterruptsStalledBody(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	m := setupModel()
	cmd := m.startAudioLoad(server.URL + "/audio.mp3")
	finished := make(chan tea.Msg, 1)
	go func() { finished <- cmd() }()
	<-started
	m.pendingLoad.cancel()
	select {
	case result := <-finished:
		if result != nil {
			t.Fatalf("cancelled load delivered %T", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled remote load remained blocked on the response body")
	}
}

func TestOpenLongURLHistoryRecallPreservesWholeCommand(t *testing.T) {
	m := commandModeModel(t, "")
	command := "open https://audio.example.test/track.mp3?token=" + strings.Repeat("x", 350)
	m.CommandHistory = []string{command}
	m.historyIndex = 1
	handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if got := m.Components.CommandInput.Value(); got != command {
		t.Fatalf("recalled %d characters, want %d", len(got), len(command))
	}
}

func TestLongURLPasteWithoutTrailingCommandSpace(t *testing.T) {
	url := "https://audio.example.test/a.mp3?token=" + strings.Repeat("x", 300)
	for _, name := range []string{"open", "load", "e"} {
		for _, whole := range []bool{false, true} {
			m := commandModeModel(t, name)
			payload := url
			if whole {
				m = commandModeModel(t, "")
				payload = name + " " + url
			}
			handlePaste(m, payload)
			if got, want := m.Components.CommandInput.Value(), name+" "+url; got != want {
				t.Errorf("%s whole=%v: pasted %q, want full %d-character command", name, whole, got, len(want))
			}
		}
	}
}

func TestLongURLPasteAfterCommandCompletion(t *testing.T) {
	m := commandModeModel(t, "op")
	cycleCompletion(m, 1)
	url := "https://audio.example.test/a.mp3?token=" + strings.Repeat("x", 300)
	handlePaste(m, url)
	if got, want := m.Components.CommandInput.Value(), "open "+url; got != want {
		t.Fatalf("pasted %q, want full completed command", got)
	}
}

type remoteDisplayPlayer struct{ mockPlayer }

func (*remoteDisplayPlayer) Title() string { return "audio.example.test/track.mp3" }

func TestRemoteLoadedTrackSkipsLocalLyricSidecars(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	if err := logger.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	m, _ := currentTrackModel()
	m.Config.Lyrics.Enabled = true
	m.Config.Lyrics.FormatPriority = []string{"embedded", "lrc"}
	source := "https://audio.example.test/track.mp3?token=private-success-secret"
	player := &remoteDisplayPlayer{mockPlayer{path: source}}
	handleAudioLoaded(m, AudioLoadedMsg{Player: player, Path: source, Generation: m.loadGeneration})
	// #nosec G304 -- reads only the logger fixture in this test's private temp directory.
	contents, err := os.ReadFile(filepath.Join(dir, "neoviolet.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "private-success-secret") {
		t.Fatalf("local sidecar lookup leaked remote source: %s", contents)
	}
}

func TestRemoteQueueFormatOmitsQueryAndKeepsSource(t *testing.T) {
	m := setupModel()
	source := "https://audio.example.test/track.mp3?token=private-secret"
	m.acceptQueueTrack(source)
	track := m.session.entries[0].track
	if track.Path != source {
		t.Fatal("operational source was redacted")
	}
	if track.Format != "MP3" {
		t.Fatalf("queue format = %q, want MP3", track.Format)
	}
}
