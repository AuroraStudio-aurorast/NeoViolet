package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := expandTilde("~/x.mp3"); got != filepath.Join(home, "x.mp3") {
		t.Errorf("expandTilde(~/x.mp3) = %q", got)
	}
	if got := expandTilde("~"); got != home {
		t.Errorf("expandTilde(~) = %q", got)
	}
	// "~user" and a path without a leading "~" are returned unchanged.
	if got := expandTilde("~bob/x.mp3"); got != "~bob/x.mp3" {
		t.Errorf("expandTilde(~bob/x.mp3) = %q", got)
	}
	if got := expandTilde("a/~/b"); got != "a/~/b" {
		t.Errorf("expandTilde(a/~/b) = %q", got)
	}
}

func TestRunOpenKeepsConsecutiveSpaces(t *testing.T) {
	m := setupModel()
	// A relative path keeps the typed command short: the command input truncates
	// past ti.CharLimit, so a long absolute path would be cut and this test would
	// fail for an unrelated reason.
	t.Chdir(t.TempDir())
	name := "My  File.mp3"
	if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	setCommand(m, "open "+name)
	executeCommand(m)
	if m.Error.Message != "" {
		t.Fatalf("error = %q, want none", m.Error.Message)
	}
	// handleLoadTrack does not record the path on the model; entering the
	// loading state is what proves the path reached it.
	if !m.Loading || !m.switchingTrack {
		t.Errorf("Loading = %v, switchingTrack = %v; want both true", m.Loading, m.switchingTrack)
	}
}

func TestRunOpenExpandsTilde(t *testing.T) {
	m := setupModel()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "song.mp3")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	setCommand(m, "open ~/song.mp3")
	executeCommand(m)
	if m.Error.Message != "" {
		t.Fatalf("error = %q, want none", m.Error.Message)
	}
	if !m.Loading || !m.switchingTrack {
		t.Errorf("Loading = %v, switchingTrack = %v; want both true", m.Loading, m.switchingTrack)
	}
}

// A dragged file arrives as a path followed by a trailing space, so :open must
// not hand the whitespace to the filesystem.
func TestRunOpenDropsTrailingSpace(t *testing.T) {
	m := setupModel()
	t.Chdir(t.TempDir())
	name := "trail.mp3"
	if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	setCommand(m, "open "+name+" ")
	executeCommand(m)
	if m.Error.Message != "" {
		t.Fatalf("error = %q, want none", m.Error.Message)
	}
	if !m.Loading || !m.switchingTrack {
		t.Errorf("Loading = %v, switchingTrack = %v; want both true", m.Loading, m.switchingTrack)
	}
}

func TestRunOpenWithoutArgumentPrintsUsage(t *testing.T) {
	m := setupModel()
	setCommand(m, "open")
	executeCommand(m)
	if got := m.Error.Message; got != "Usage: open <path>" {
		t.Errorf("Error = %q, want %q", got, "Usage: open <path>")
	}
}

func TestExpandTildeFallsBackWhenHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	if got := expandTilde("~/x.mp3"); got != "~/x.mp3" {
		t.Errorf("expandTilde(~/x.mp3) = %q, want the input unchanged", got)
	}
}
