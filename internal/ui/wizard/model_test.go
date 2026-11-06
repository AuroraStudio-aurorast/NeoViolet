package wizard

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func soundfontFocus(t *testing.T, m *Model) {
	t.Helper()
	for range 3 {
		if m.form.GetFocusedField() == m.picker {
			return
		}
		m.form.Update(m.form.NextField()())
	}
	t.Fatal("SoundFont field not reachable")
}

func TestEmbeddedWizardCentersOnlyAfterGUIIsNegotiated(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	if m.View().AltScreen {
		t.Fatal("standalone wizard entered alternate screen")
	}
	m.SetGUI(true, true)
	view := m.View()
	if !view.AltScreen {
		t.Fatal("authenticated GUI did not enable alternate screen")
	}
	if lipgloss.Width(view.Content) != 100 || lipgloss.Height(view.Content) != 34 {
		t.Fatalf("frame %dx%d", lipgloss.Width(view.Content), lipgloss.Height(view.Content))
	}
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if strings.TrimSpace(lines[0]) != "" {
		t.Fatal("form was not vertically centered")
	}
	for _, text := range []string{"Welcome to NeoViolet", "Select your favorite icon theme", "Choose a SoundFont file (optional)", "No file selected."} {
		if !strings.Contains(view.Content, text) {
			t.Fatalf("existing Huh field lost: %s", text)
		}
	}
	for _, size := range [][2]int{{80, 24}, {60, 20}, {100, 34}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Width(m.View().Content) > size[0] || lipgloss.Height(m.View().Content) > size[1] {
			t.Fatal("wizard exceeded terminal dimensions")
		}
	}
}

func TestNativeSoundfontCancelAndValidationPreserveOptionalValue(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	m.SetGUI(true, true)
	soundfontFocus(t, m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	request, ok := cmd().(PickFileMsg)
	if !ok {
		t.Fatal("Enter did not request native picker")
	}
	m.Update(Selection{ID: request.ID, Status: "cancelled"})
	if m.sfPath != "" || m.form.GetFocusedField() != m.picker {
		t.Fatal("cancel changed field or value")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	request = cmd().(PickFileMsg)
	bad := filepath.Join(t.TempDir(), "font.sfz")
	if err := os.WriteFile(bad, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd = m.Update(Selection{ID: request.ID, Status: "selected", Path: bad})
	m.Update(cmd())
	if m.sfPath != "" || m.pending != "" || m.notice == "" {
		t.Fatal("invalid extension was accepted or stuck pending")
	}
	good := filepath.Join(t.TempDir(), "音色.SF2")
	if err := os.WriteFile(good, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	request = cmd().(PickFileMsg)
	_, cmd = m.Update(Selection{ID: request.ID, Status: "selected", Path: good})
	m.Update(cmd())
	if m.sfPath != good {
		t.Fatal("valid Unicode SoundFont path was not applied")
	}
}

func TestNativePickerRepliesCannotOutliveTheirRequest(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	m.SetGUI(true, true)
	soundfontFocus(t, m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	request := cmd().(PickFileMsg)
	m.SetGUI(true, false) // disconnected: preserve form, fall back to terminal picker
	m.Update(Selection{ID: request.ID, Status: "selected", Path: "late.sf2"})
	if m.sfPath != "" {
		t.Fatal("late result changed the field")
	}
	if m.form.State != huh.StateNormal {
		t.Fatal("disconnect completed or aborted the form")
	}
}

func TestResizePreservesTheTerminalBackgroundQuery(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd == nil {
		t.Fatal("resize swallowed the one-time terminal background query")
	}
}

func TestTabSkipsOptionalSoundfontAndCompletesForm(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	soundfontFocus(t, m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for step := 0; cmd != nil && step < 6; step++ {
		message := cmd()
		if batch, ok := message.(tea.BatchMsg); ok {
			if len(batch) != 1 {
				t.Fatalf("unexpected navigation batch: %d", len(batch))
			}
			message = batch[0]()
		}
		_, cmd = m.Update(message)
	}
	cfg, done := m.Result()
	if !done || cfg.SoundfontPath != "" {
		t.Fatal("Tab did not complete optional field with empty SoundFont")
	}
}

func TestCenteredFormKeepsLeftAlignedHuhFooter(t *testing.T) {
	frame := ansi.Strip(centerForm(20, 6, "┃ Title       \n┃ Field       \n\nTab skip"))
	lines := strings.Split(frame, "\n")
	title, footer := -1, -1
	for _, line := range lines {
		if strings.Contains(line, "┃ Title") {
			title = strings.Index(line, "┃")
		}
		if strings.Contains(line, "Tab skip") {
			footer = strings.Index(line, "Tab skip")
		}
	}
	if title != footer {
		t.Fatalf("form and footer alignment diverged: title=%d footer=%d", title, footer)
	}
}

func TestInitialTinyPTYResizeDoesNotPermanentlyHideIconOptions(t *testing.T) {
	t.Setenv("NEOVIOLET_FONT", "Menlo")
	m := New()
	m.SetGUI(true, true)
	drainWizardCommands(t, m, m.Init())
	for _, size := range [][2]int{{80, 4}, {118, 36}} {
		_, cmd := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		drainWizardCommands(t, m, cmd)
	}
	view := ansi.Strip(m.View().Content)
	for _, option := range []string{"Nerd Font:", "Emoji:", "Fallback:"} {
		if !strings.Contains(view, option) {
			t.Fatalf("resizing lost %s", option)
		}
	}
}

func drainWizardCommands(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for step := 0; len(queue) > 0 && step < 100; step++ {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		message := current()
		value := reflect.ValueOf(message)
		if value.IsValid() && value.Kind() == reflect.Slice && value.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
			for i := 0; i < value.Len(); i++ {
				queue = append(queue, value.Index(i).Interface().(tea.Cmd))
			}
		} else {
			_, next := m.Update(message)
			queue = append(queue, next)
		}
	}
	if len(queue) > 0 {
		t.Fatal("wizard commands did not settle")
	}
}
