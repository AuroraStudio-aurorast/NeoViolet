package startup

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
)

func TestGUIInitialLoadingCentersTheOriginalSweepAfterHandshake(t *testing.T) {
	for _, firstRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("first_run=%v", firstRun), func(t *testing.T) {
			m, conn, _ := client(t, firstRun, "initial.flac")
			if !firstRun && m.View().AltScreen {
				t.Fatal("environment hint alone must not change the startup screen")
			}
			send(t, m, conn, `{"type":"client_hello","client":"neoviolet-gui","protocol":1,"capabilities":["file_picker"]}`)
			if firstRun {
				cfg := config.DefaultConfig()
				m.Update(settingsSaved{Config: &cfg})
			}
			for _, size := range [][2]int{{80, 24}, {100, 34}, {8, 3}, {1, 1}, {80, 24}} {
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				view := m.View()
				if !view.AltScreen || view.BackgroundColor != nil || view.ForegroundColor != nil {
					t.Fatal("GUI loading must use alternate screen without pinning terminal colours")
				}
				lines := strings.Split(ansi.Strip(view.Content), "\n")
				if len(lines) != size[1] {
					t.Fatalf("loading has %d rows, want %d", len(lines), size[1])
				}
				labels := 0
				for y, line := range lines {
					if lipgloss.Width(line) != size[0] {
						t.Fatalf("loading row width %d, want %d", lipgloss.Width(line), size[0])
					}
					text := strings.TrimSpace(line)
					if text == "" {
						continue
					}
					labels++
					want := "Loading audio"
					if size[0] == 8 {
						want = "Loading…"
					}
					if size[0] == 1 {
						want = "…"
					}
					if text != want {
						t.Fatalf("loading label %q, want %q", text, want)
					}
					if y != (size[1]-1)/2 && y != size[1]/2 {
						t.Fatalf("loading is on row %d in %d rows", y, size[1])
					}
					left := len(line) - len(strings.TrimLeft(line, " "))
					balance := size[0] - lipgloss.Width(text) - 2*left
					if balance < -1 || balance > 1 {
						t.Fatalf("loading is not horizontally centered: %q", line)
					}
				}
				if labels != 1 {
					t.Fatalf("found %d loading labels, want 1", labels)
				}
			}
			first := m.View().Content
			for range 10 {
				m.Update(ui.TickMsg{})
			}
			later := m.View().Content
			if first == later || ansi.Strip(first) != ansi.Strip(later) {
				t.Fatal("centering changed or froze the original foreground sweep")
			}
			m.Update(ui.AudioLoadFailedMsg{Generation: 0, Message: "decode failed", Timer: 30})
			if m.phase != "ready" || strings.Contains(ansi.Strip(m.View().Content), "Loading audio") {
				t.Fatal("failed loading did not restore the main player")
			}
			m.Update(ui.LoadTrackMsg{Path: "next.flac"})
			if m.phase != "ready" || m.View().Content != m.Player().View().Content {
				t.Fatal("runtime track replacement was moved to a startup loading screen")
			}
		})
	}
}

func TestStandaloneInitialLoadingStaysInline(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	config.SetXDGConfig(true)
	t.Cleanup(func() { config.SetXDGConfig(false) })
	cfg := config.DefaultConfig()
	m := New("initial.flac", &cfg, false, 0, 0)
	t.Cleanup(m.Close)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := m.View()
	if view.AltScreen || strings.Contains(view.Content, "\n") || ansi.Strip(view.Content) != "Loading audio" {
		t.Fatal("standalone startup loading no longer stays inline")
	}
}
