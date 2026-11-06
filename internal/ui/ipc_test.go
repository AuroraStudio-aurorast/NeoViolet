package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

func TestDisconnectedGUIQuitFallsBackToLocalCleanup(t *testing.T) {
	m := setupModel()
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	m.ipcServer = &ipc.Server{}
	_, cmd := runQuit(m, invocation{})
	if cmd == nil {
		t.Fatal("disconnected GUI prevented local quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected graceful quit")
	}
	if !player.closeCalled {
		t.Fatal("player not cleaned up")
	}
}

func ipcModel(t *testing.T) (*Model, net.Conn, *json.Decoder) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	m := setupModel()
	s, err := ipc.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	m.ipcServer = s
	contents, err := os.ReadFile(filepath.Join(os.TempDir(), fmt.Sprintf("neoviolet-ipc-%d", os.Getpid())))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := strings.Split(string(contents), "\n")
	conn, err := net.Dial("tcp", endpoint[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(conn, endpoint[1]); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bufio.NewReader(conn))
	var hello ipc.Message
	if err := decoder.Decode(&hello); err != nil {
		t.Fatal(err)
	}
	m.Update(<-s.Incoming)
	return m, conn, decoder
}

func TestIPCDispatchDoesNotWaitForPlaybackTick(t *testing.T) {
	m, conn, _ := ipcModel(t)
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	m.Audio.IsPlaying = true
	if _, err := fmt.Fprintln(conn, `{"type":"play_pause"}`); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
	if !player.pauseCalled {
		t.Fatal("IPC request waited for playback tick")
	}
}

func readIPC(t *testing.T, decoder *json.Decoder, kind, id string) map[string]any {
	t.Helper()
	for {
		var message map[string]any
		if err := decoder.Decode(&message); err != nil {
			t.Fatalf("waiting for %s %s: %v", kind, id, err)
		}
		if message["type"] == kind && (id == "" || message["request_id"] == id) {
			return message
		}
	}
}

func requestIPC(t *testing.T, m *Model, conn net.Conn, raw string) {
	t.Helper()
	if _, err := fmt.Fprintln(conn, raw); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.ipcServer.Incoming)
}

func TestIPCReportsInvalidCommandAndInitialStateWithoutLyrics(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	state := readIPC(t, decoder, "state", "")["state"].(map[string]any)
	if state["volume"] != 0.0 || state["has_track"] != false {
		t.Fatalf("initial state: %v", state)
	}
	requestIPC(t, m, conn, `{"type":"seek","request_id":"bad"}`)
	result := readIPC(t, decoder, "result", "bad")
	if result["status"] != "failed" || result["error"] == "" {
		t.Fatalf("missing failed result: %v", result)
	}
}

func TestIPCVolumeAndSeekAcceptZeroAndRejectMissingValues(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	m.Audio.Player = &mockPlayer{duration: time.Minute}
	m.Audio.Duration = time.Minute
	for _, tc := range []struct{ raw, id, status string }{
		{`{"type":"set_volume","request_id":"mute","volume":0}`, "mute", "succeeded"},
		{`{"type":"set_volume","request_id":"missing"}`, "missing", "failed"},
		{`{"type":"seek","request_id":"start","position":0}`, "start", "succeeded"},
		{`{"type":"seek","request_id":"past","position":999}`, "past", "succeeded"},
	} {
		requestIPC(t, m, conn, tc.raw)
		result := readIPC(t, decoder, "result", tc.id)
		if result["status"] != tc.status {
			t.Fatalf("%s: %v", tc.id, result)
		}
	}
	if m.Audio.Volume != 0 {
		t.Fatal("zero volume ignored")
	}
	if got := m.Audio.Player.Position(); got != time.Minute {
		t.Fatalf("seek not clamped: %v", got)
	}
}

func TestIPCOpenTracksCancellationFailureAndSuccess(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	path := filepath.Join(t.TempDir(), "track.flac")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := func(id string) string {
		data, _ := json.Marshal(map[string]string{"type": "open", "request_id": id, "path": path})
		return string(data)
	}
	requestIPC(t, m, conn, raw("one"))
	if readIPC(t, decoder, "result", "one")["status"] != "accepted" {
		t.Fatal("open not accepted")
	}
	requestIPC(t, m, conn, raw("two"))
	if readIPC(t, decoder, "result", "one")["status"] != "cancelled" {
		t.Fatal("replacement not cancelled")
	}
	readIPC(t, decoder, "result", "two")
	m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "decode failed", Timer: 10})
	if readIPC(t, decoder, "result", "two")["status"] != "failed" {
		t.Fatal("load failure not reported")
	}
	requestIPC(t, m, conn, raw("three"))
	readIPC(t, decoder, "result", "three")
	m.Config.Accent.AutoAccent = boolPtr(false)
	m.Config.Lyrics.Enabled = false
	m.Update(AudioLoadedMsg{Generation: m.loadGeneration, Path: path, Player: &mockPlayer{duration: time.Minute}})
	if readIPC(t, decoder, "result", "three")["status"] != "succeeded" {
		t.Fatal("load success not reported")
	}
	state := readIPC(t, decoder, "state", "")["state"].(map[string]any)
	if state["title"] != "track.flac" || state["playing"] != true || state["loading"] != false {
		t.Fatalf("loaded state: %v", state)
	}
}

func TestPlaybackInputsKeepActualStateAndVolumePersistenceConsistent(t *testing.T) {
	m, _, _ := ipcModel(t)
	player := &mockPlayer{duration: time.Minute, playing: true}
	m.Audio.Player = player
	m.Audio.Duration = time.Minute
	m.Update(MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdSetVolume, Volume: 0.25}})
	if m.Audio.Volume != 0.25 || m.Components.VolumeBar.Percent() != 0.25 || m.Config.DefaultVolume != 0.25 {
		t.Fatal("system volume diverged from player controls")
	}
	m.Update(SeekMsg{Position: 5 * time.Second})
	if m.Audio.Elapsed != 5*time.Second {
		t.Fatalf("seek snapshot is stale: %v", m.Audio.Elapsed)
	}
}

func TestFailedPlayDoesNotReportPlayingThroughOtherInputs(t *testing.T) {
	m := setupModel()
	player := &switchPlayer{playErr: errors.New("audio device unavailable")}
	m.Audio.Player = player
	m.togglePlayback()
	if m.Audio.IsPlaying {
		t.Fatal("failed play reported as playing")
	}
}

func TestIPCRejectsCommandsFromAnObsoleteConnection(t *testing.T) {
	m, _, decoder := ipcModel(t)
	readIPC(t, decoder, "state", "")
	player := &mockPlayer{playing: true}
	m.Audio.Player = player
	m.Audio.IsPlaying = true
	m.Update(ipc.Event{Kind: "request", Session: m.ipcSession + 1, Message: ipc.Message{Type: "pause"}})
	if player.pauseCalled {
		t.Fatal("obsolete command affected player")
	}
}

func TestIPCSeekPushesUpdatedSnapshotImmediately(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	readIPC(t, decoder, "state", "")
	m.Audio.Player = &mockPlayer{duration: time.Minute}
	m.Audio.Duration = time.Minute
	m.pushIPCState(true)
	readIPC(t, decoder, "state", "")
	requestIPC(t, m, conn, `{"type":"seek","request_id":"jump","position":15}`)
	readIPC(t, decoder, "result", "jump")
	state := readIPC(t, decoder, "state", "")["state"].(map[string]any)
	if state["position"] != 15.0 {
		t.Fatalf("stale seek snapshot: %v", state)
	}
}

func TestIPCSeekRejectsDurationOverflow(t *testing.T) {
	m, conn, decoder := ipcModel(t)
	m.Audio.Player = &mockPlayer{duration: time.Minute}
	requestIPC(t, m, conn, `{"type":"seek","request_id":"overflow","position":9223372036.854776}`)
	result := readIPC(t, decoder, "result", "overflow")
	if result["status"] != "failed" {
		t.Fatalf("overflow accepted: %v", result)
	}
}
