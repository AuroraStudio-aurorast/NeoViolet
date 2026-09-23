package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The payload below holds a bare space, so splitDropFields resolves it to two
// fields that reassemble; TestPasteInCommandModeInsertsEveryPath is the clear case.
func TestPasteInCommandModeInsertsPath(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("open ")
	m.Components.CommandInput.CursorEnd()

	updated, _ := updateDispatcher(m, tea.PasteMsg{Content: "/a/My File.mp3"})
	m = updated.(*Model)
	if got := m.Components.CommandInput.Value(); got != "open /a/My File.mp3" {
		t.Errorf("value = %q", got)
	}
}

// A paste refreshes the candidate list: without syncCompletion the overlay
// keeps showing what the pre-paste line would have completed to.
func TestPasteInCommandModeRefreshesCandidates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("open ")
	m.Components.CommandInput.CursorEnd()

	updated, _ := updateDispatcher(m, tea.PasteMsg{Content: dir + "/"})
	m = updated.(*Model)
	if got, want := m.Components.CommandInput.Value(), "open "+dir+"/"; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
	if len(m.completionCandidates) == 0 {
		t.Error("candidates were not refreshed after the paste")
	}
}

// A dropped file whose name contains a space arrives backslash-escaped (the
// usual macOS terminal form). The line must end up showing the real name, not
// the escaped one: this drives the whole route, split -> unescape -> pick the
// form that exists -> insert.
func TestPasteInCommandModeUnescapesExistingSpacePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My File.mp3")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("open ")
	m.Components.CommandInput.CursorEnd()

	payload := filepath.Join(dir, `My\ File.mp3`)
	updated, _ := updateDispatcher(m, tea.PasteMsg{Content: payload})
	m = updated.(*Model)
	if got, want := m.Components.CommandInput.Value(), "open "+path; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

// A paste can carry several paths, and the scan must not give up on the whole
// payload when the first path is unusable: a valid file later in it still loads.
func TestPasteInNormalModeScansPastAnInvalidPath(t *testing.T) {
	m := setupModel()
	dir := t.TempDir()
	// The extension is playable but the file is not there, so only the
	// existence check can reject it.
	missing := filepath.Join(dir, "missing.mp3")
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: missing + " " + path})
	m = updated.(*Model)
	if !m.Loading || !m.switchingTrack {
		t.Errorf("Loading = %v, switchingTrack = %v; want the second path to load", m.Loading, m.switchingTrack)
	}
	if cmd == nil {
		t.Error("expected a load command")
	}
}

// A single unusable path must be ignored outright: nothing loads, no command is
// returned and no error is written.
func TestPasteInNormalModeIgnoresMissingAudioPath(t *testing.T) {
	m := setupModel()
	// The extension is playable but the file is not there, so only the
	// existence check can reject this payload.
	missing := filepath.Join(t.TempDir(), "missing.mp3")

	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: missing})
	m = updated.(*Model)
	if m.Loading || m.switchingTrack || cmd != nil {
		t.Errorf("Loading = %v, switchingTrack = %v, cmd = %v; want no reaction", m.Loading, m.switchingTrack, cmd)
	}
	if m.Error.Message != "" {
		t.Errorf("error = %q, want none", m.Error.Message)
	}
}

// Dropping two files at once puts both of them on the line, not just the first.
func TestPasteInCommandModeInsertsEveryPath(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "one.mp3")
	second := filepath.Join(dir, "two.mp3")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	m := setupModel()
	m.UI.Mode = ModeCommand
	m.Components.CommandInput.Focus()
	m.Components.CommandInput.SetValue("open ")
	m.Components.CommandInput.CursorEnd()

	updated, _ := updateDispatcher(m, tea.PasteMsg{Content: first + " " + second})
	m = updated.(*Model)
	if got, want := m.Components.CommandInput.Value(), "open "+first+" "+second; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

func TestPasteInNormalModeLoadsAudio(t *testing.T) {
	m := setupModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: path})
	m = updated.(*Model)
	if !m.Loading || !m.switchingTrack {
		t.Errorf("Loading = %v, switchingTrack = %v; want both true", m.Loading, m.switchingTrack)
	}
	if cmd == nil {
		t.Error("expected a load command")
	}
}

// Pasting a non-audio file in normal mode has no effect and writes no error.
func TestPasteInNormalModeIgnoresNonAudio(t *testing.T) {
	m := setupModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: path})
	m = updated.(*Model)
	if m.Loading || m.switchingTrack || cmd != nil {
		t.Errorf("Loading = %v, switchingTrack = %v, cmd = %v; want no reaction", m.Loading, m.switchingTrack, cmd)
	}
	if m.Error.Message != "" {
		t.Errorf("error = %q, want none", m.Error.Message)
	}
}

// Pasting plain text in normal mode is ignored silently.
func TestPasteInNormalModeIgnoresPlainText(t *testing.T) {
	m := setupModel()
	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: "hello world"})
	m = updated.(*Model)
	if m.Loading || cmd != nil || m.Error.Message != "" {
		t.Errorf("plain text paste had side effects: loading=%v cmd=%v err=%q", m.Loading, cmd, m.Error.Message)
	}
}
