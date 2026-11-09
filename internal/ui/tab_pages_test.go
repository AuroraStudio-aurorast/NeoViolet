package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func pageKey(m *Model, code rune, text string) tea.Cmd {
	_, cmd := handleKeyPress(m, tea.KeyPressMsg{Code: code, Text: text})
	return cmd
}

// Catches page overflow into lyrics/footer and the wrong page being dispatched.
func TestTabPagesFitContentBox(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {68, 17}} {
		for _, mode := range []string{config.PanelModeAuto, config.PanelModeOn, config.PanelModeOff} {
			m := lyricFooterModel()
			m.UI.Width, m.UI.Height = size[0], size[1]
			m.panelMode = mode
			for tab, want := range []string{"Music library", "Session queue", "Sound desk", "Settings"} {
				m.UI.ActiveTab = tab
				plan := m.layoutPlan()
				frame := renderContent(m, plan)
				if !strings.Contains(stripANSI(frame), want) {
					t.Errorf("tab %d: missing page %q", tab, want)
				}
				if got := lipgloss.Height(frame); got != plan.ContentHeight {
					t.Errorf("tab %d height %d, want %d", tab, got, plan.ContentHeight)
				}
				for _, row := range strings.Split(frame, "\n") {
					if got := lipgloss.Width(row); got != plan.ContentWidth {
						t.Errorf("tab %d row width %d, want %d", tab, got, plan.ContentWidth)
					}
				}
			}
		}
	}
}

// Catches global n/p/h/l, slash, tab and space stealing input from a page editor.
func TestTabPageEditorOwnsTextAndPaste(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	pageKey(m, 'f', "f")
	for _, r := range "nhl /p music" {
		pageKey(m, r, string(r))
	}
	_, _ = updateDispatcher(m, tea.PasteMsg{Content: " /more audio.flac"})
	if m.UI.ActiveTab != 0 || m.UI.Mode != ModeNormal || m.Audio.IsPlaying {
		t.Fatal("page input escaped to global actions")
	}
	body := stripANSI(renderContent(m, m.layoutPlan()))
	if !strings.Contains(body, "nhl /p music /more audio.flac") {
		t.Fatalf("editor lost input: %q", body)
	}
	pageKey(m, tea.KeyEscape, "")
	pageKey(m, ']', "]")
	if m.UI.ActiveTab != 1 {
		t.Fatal("escape did not release page input")
	}
}

// Sample page controls must not become accidental playback/configuration wiring.
func TestTabPageActionsRemainPresentationOnly(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	before, _ := json.Marshal(m.Config)
	for tab, keys := range map[int]string{0: "a", 1: "ds", 2: "+="} {
		m.UI.ActiveTab = tab
		pageKey(m, tea.KeyEnter, "")
		for _, r := range keys {
			pageKey(m, r, string(r))
		}
		pageKey(m, tea.KeyEscape, "")
	}
	after, _ := json.Marshal(m.Config)
	if string(before) != string(after) || m.Audio.Player != nil || m.pendingLoad != nil {
		t.Fatal("sample action touched real configuration or playback")
	}
	if !m.Info.Visible {
		t.Fatal("unconnected action did not explain its placeholder status")
	}
}

func TestTabPagesSavedCollectionAndSettingsSubview(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	m.UI.ActiveTab = 1
	pageKey(m, 'v', "v")
	pageKey(m, tea.KeyEnter, "")
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "After the Rain") || !strings.Contains(got, "Evening Sketch") {
		t.Fatalf("missing playlist detail: %q", got)
	}
	pageKey(m, tea.KeyEscape, "")
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "Quiet mornings") {
		t.Fatal("escape did not return to collections")
	}
	m.UI.ActiveTab = 3
	pageKey(m, '2', "2")
	pageKey(m, tea.KeyEnter, "")
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "MIDI SoundFont") {
		t.Fatalf("missing file selector: %q", got)
	}
	pageKey(m, tea.KeyEscape, "")
	pageKey(m, '3', "3")
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "Lyrics") {
		t.Fatal("settings group navigation was lost after closing selector")
	}
}
