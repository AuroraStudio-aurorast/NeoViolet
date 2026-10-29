package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestCommandDiscovery_OpensWithRegisteredCommands(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusFooter
	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: ':', Text: ":"})

	if m.UI.Mode != ModeCommand || m.UI.SavedFocus != FocusFooter {
		t.Fatal("command mode did not preserve the original focus")
	}
	if len(m.completionCandidates) == 0 {
		t.Fatal("entering command mode did not show any commands")
	}
	for _, spec := range commands {
		found := false
		for _, c := range m.completionCandidates {
			if c.Value == spec.Name {
				found = true
				if !strings.Contains(c.Desc, spec.Desc) || !strings.Contains(c.Desc, spec.Args) {
					t.Errorf("command %q has incomplete help: %q", spec.Name, c.Desc)
				}
			}
		}
		if !found {
			t.Errorf("registered command %q is missing", spec.Name)
		}
	}
	if m.completionIndex != -1 || m.Components.CommandInput.Value() != "" {
		t.Fatal("opening the list selected or inserted a command")
	}
}

func TestCommandDiscovery_EmptyEnterDoesNotExecute(t *testing.T) {
	m := commandModeModel(t, "")
	m.UI.SavedFocus = FocusFooter
	if len(m.completionCandidates) == 0 {
		t.Fatal("empty command input has no passive list")
	}
	_, cmd := handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.Error.Visible || m.Info.Visible || len(m.CommandHistory) != 0 {
		t.Fatal("empty Enter executed a command or recorded history")
	}
	if m.UI.Mode != ModeNormal || m.UI.Focus != FocusFooter || len(m.completionCandidates) != 0 {
		t.Fatal("empty Enter did not return to the original region and dismiss the list")
	}
}

func TestCommandDiscovery_CyclesOnlyAfterExplicitSelection(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, "anim"},
		{"next", tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}, "anim"},
		{"previous", tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, "wq!"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := commandModeModel(t, "")
			_, cmd := handleCommandModeKeyPress(m, tt.key)
			if got := m.Components.CommandInput.Value(); got != tt.want {
				t.Fatalf("selected command = %q, want %q", got, tt.want)
			}
			if cmd != nil || m.UI.Mode != ModeCommand || m.completionIndex < 0 {
				t.Fatal("selection executed a command or failed to select it")
			}
		})
	}
}

func TestCommandDiscovery_FiltersAndRestoresOnBackspace(t *testing.T) {
	m := commandModeModel(t, "")
	for _, r := range "vo" {
		handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := value(t, m.completionCandidates); strings.Join(got, ",") != "vol" {
		t.Fatalf("filtered commands = %v, want [vol]", got)
	}
	for range 2 {
		handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if len(m.completionCandidates) <= 1 || m.completionIndex != -1 {
		t.Fatal("clearing the input did not restore the passive command list")
	}
	if m.Components.CommandInput.CurrentSuggestion() != "" {
		t.Fatal("clearing the input left stale ghost text")
	}

	m.UI.SavedFocus = FocusFooter
	handleCommandModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.UI.Mode != ModeNormal || m.UI.Focus != FocusFooter || len(m.completionCandidates) != 0 {
		t.Fatal("Escape did not dismiss discovery and restore focus")
	}
}

func TestCommandDiscovery_OverlayFitsExistingFooter(t *testing.T) {
	for _, size := range []struct{ w, h int }{{68, 17}, {80, 24}, {120, 32}} {
		m := setupModel()
		handleResize(m, tea.WindowSizeMsg{Width: size.w, Height: size.h})
		before := strings.Split(renderMainView(m).Content, "\n")
		handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: ':', Text: ":"})
		plan := m.layoutPlan()
		if got := completionRows(m, plan); got != 5 {
			t.Fatalf("%dx%d: discovery rows = %d, want 5", size.w, size.h, got)
		}
		frame := renderMainView(m).Content
		if lipgloss.Width(frame) != size.w || lipgloss.Height(frame) != size.h {
			t.Fatalf("%dx%d: discovery changed the frame size", size.w, size.h)
		}
		after := strings.Split(frame, "\n")
		for y := 0; y < completionTop(plan, 5); y++ {
			if after[y] != before[y] {
				t.Fatalf("%dx%d: discovery changed row %d above the footer overlay", size.w, size.h, y)
			}
		}
	}
}
