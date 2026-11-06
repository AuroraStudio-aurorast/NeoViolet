package startup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

func client(t *testing.T, firstRun bool, path string) (*Model, net.Conn, *json.Decoder) {
	t.Helper()
	t.Setenv("TERM_PROGRAM", "neoviolet-gui")
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	cfg := config.DefaultConfig()
	m := New(path, &cfg, firstRun, 0, 0)
	t.Cleanup(m.Close)
	raw, err := os.ReadFile(filepath.Join(os.TempDir(), fmt.Sprintf("neoviolet-ipc-%d", os.Getpid())))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	conn, err := net.Dial("tcp", lines[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(conn, lines[1]); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bufio.NewReader(conn))
	var hello map[string]any
	if err := decoder.Decode(&hello); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.server.Incoming)
	return m, conn, decoder
}
func send(t *testing.T, m *Model, conn net.Conn, raw string) {
	t.Helper()
	if _, err := fmt.Fprintln(conn, raw); err != nil {
		t.Fatal(err)
	}
	m.Update(<-m.server.Incoming)
}
func read(t *testing.T, decoder *json.Decoder, kind string) map[string]any {
	t.Helper()
	for {
		var message map[string]any
		if err := decoder.Decode(&message); err != nil {
			t.Fatal(err)
		}
		if message["type"] == kind {
			return message
		}
	}
}

func TestWizardAuthenticatesBeforeEnteringAlternateScreen(t *testing.T) {
	m, conn, decoder := client(t, true, "")
	if m.server == nil || m.Player() != nil {
		t.Fatal("early IPC must exist before player")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	if m.View().AltScreen {
		t.Fatal("launch environment alone enabled alternate screen")
	}
	if read(t, decoder, "phase")["phase"] != "starting" {
		t.Fatal("wrong initial phase")
	}
	send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker"]}`)
	if !m.View().AltScreen {
		t.Fatal("GUI authentication did not upgrade wizard")
	}
	if read(t, decoder, "phase")["phase"] != "wizard" {
		t.Fatal("wizard phase not reported")
	}
	send(t, m, conn, `{"type":"open","request_id":"blocked","path":"song.flac"}`)
	if read(t, decoder, "result")["status"] != "failed" || m.Player() != nil {
		t.Fatal("pre-ready open was accepted")
	}
}

func TestInitialLoadingAndRuntimeTrackReplacementAreDifferentPhases(t *testing.T) {
	m, conn, decoder := client(t, false, "missing.flac")
	send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker"]}`)
	if read(t, decoder, "phase")["phase"] != "initial_loading" {
		t.Fatal("initial load was ready too early")
	}
	send(t, m, conn, `{"type":"desktop_lyrics","enable":true,"request_id":"blocked"}`)
	if read(t, decoder, "result")["status"] != "failed" {
		t.Fatal("initial-load controls not rejected")
	}
	m.Update(ui.AudioLoadFailedMsg{Generation: 0, Message: "missing file", Timer: 30})
	if read(t, decoder, "phase")["phase"] != "ready" {
		t.Fatal("failed initial load did not recover ready controls")
	}
	m.Update(ui.LoadTrackMsg{Path: "other.flac"})
	if m.phase != "ready" {
		t.Fatal("runtime replacement re-locked the entire GUI")
	}
}

func TestOldPickerCommandCannotEscapeToAReplacementGUI(t *testing.T) {
	m, conn, decoder := client(t, true, "")
	send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker"]}`)
	read(t, decoder, "phase")
	read(t, decoder, "phase")
	m.Update(wizard.PickFileMsg{ID: "obsolete"})
	if m.pickerID != "" {
		t.Fatal("host sent a picker request no longer pending in the wizard")
	}
}

func TestLateSettingsSaveCannotStartPlayerAfterShutdown(t *testing.T) {
	m, _, _ := client(t, true, "")
	m.Close()
	cfg := config.DefaultConfig()
	m.Update(settingsSaved{Config: &cfg})
	if m.Player() != nil {
		t.Fatal("late settings save created a player after shutdown")
	}
}

func TestGUIWaitFallbackAndReconnectPreserveWizardAnswers(t *testing.T) {
	m, conn, decoder := client(t, true, "")
	if !m.awaitGUI {
		t.Fatal("GUI launch should wait for negotiation")
	}
	m.Update(guiWaitExpired{})
	if m.awaitGUI || m.phase != "wizard" || m.setup == nil {
		t.Fatal("missing GUI trapped first-run setup")
	}
	send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":[]}`)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.View().AltScreen || m.native {
		t.Fatal("GUI presentation and native picker capability conflated")
	}
	// A current session replays wizard rather than re-entering first loading.
	m.publishPhase(true)
	if read(t, decoder, "phase")["phase"] != "starting" {
		t.Fatal("missing initial snapshot")
	}
	if read(t, decoder, "phase")["phase"] != "wizard" {
		t.Fatal("missing wizard snapshot")
	}
	if read(t, decoder, "phase")["phase"] != "wizard" {
		t.Fatal("phase replay incorrect")
	}
}

func TestAcceptedSetupPreservesInitialFileSeekAndVolume(t *testing.T) {
	m, _, decoder := client(t, true, "initial.mid")
	m.volume = 35
	m.seek = 9 * time.Second
	cfg := config.DefaultConfig()
	cfg.SoundfontPath = "chosen.sf2"
	cfg.IconTheme = "fallback"
	m.Update(settingsSaved{Config: &cfg})
	if m.Player() == nil || m.phase != "initial_loading" {
		t.Fatal("accepted setup lost original load")
	}
	if m.Player().Config.SoundfontPath != "chosen.sf2" || m.Player().Audio.Volume != 0.35 || m.Player().Config.IconTheme != "fallback" {
		t.Fatal("accepted configuration not used by player")
	}
	// The initial path still starts through the normal load command; no second
	// endpoint or wizard was created when attaching the existing session.
	if read(t, decoder, "phase")["phase"] != "starting" {
		t.Fatal("initial phase missing")
	}
	if read(t, decoder, "phase")["phase"] != "initial_loading" {
		t.Fatal("initial file skipped")
	}
}

func TestReadyOpenCommandUsesAudioPickerBeforeAndAfterWizard(t *testing.T) {
	for _, firstRun := range []bool{false, true} {
		t.Run(fmt.Sprint(firstRun), func(t *testing.T) {
			m, conn, decoder := client(t, firstRun, "")
			send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker","audio_file_picker"]}`)
			if firstRun {
				cfg := config.DefaultConfig()
				m.Update(settingsSaved{Config: &cfg})
			}
			player := m.Player()
			player.Components.CommandInput.SetValue("open")
			player.UI.Mode = ui.ModeCommand
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if player.Error.Message != "" {
				t.Fatalf("picker request rejected: %s", player.Error.Message)
			}
			pick := read(t, decoder, "pick_file")
			if pick["purpose"] != "audio" {
				t.Fatalf("wrong picker: %v", pick)
			}
			path := filepath.Join(t.TempDir(), "song.flac")
			if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(map[string]any{"type": "file_picked", "purpose": "audio", "request_id": pick["request_id"], "status": "selected", "path": path})
			send(t, m, conn, string(raw))
			if !player.Loading {
				t.Fatal("startup host swallowed the audio picker selection")
			}
		})
	}
}
