package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// windowsSeparators makes the path rules read like Windows' for one test. The CI
// test job runs on Linux and macOS only, so the platform cannot be switched with
// a build tag: the separator set is the seam those rules are read through.
func windowsSeparators(t *testing.T) {
	t.Helper()
	old := pathSeparators
	pathSeparators = `\/`
	t.Cleanup(func() { pathSeparators = old })
}

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

// Windows starts an expanded path with either separator, so "~\x" reaches the
// same file as "~/x" there.
func TestExpandTildeWindowsSeparator(t *testing.T) {
	windowsSeparators(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := expandTilde(`~\x.mp3`), filepath.Join(home, "x.mp3"); got != want {
		t.Errorf("expandTilde(~\\x.mp3) = %q, want %q", got, want)
	}
	// A trailing separator is the home directory itself, exactly as "~/" is.
	if got := expandTilde(`~\`); got != home {
		t.Errorf("expandTilde(~\\) = %q, want %q", got, home)
	}
}

// Off Windows a backslash is an ordinary character in a file name, so the tilde
// rules must not read it as a separator: that would expand a file really named
// "~\x.mp3" into the home directory.
func TestExpandTildeLeavesBackslashAloneOffWindows(t *testing.T) {
	if os.IsPathSeparator('\\') {
		t.Skip("this host reads a backslash as a separator")
	}
	if got := expandTilde(`~\x.mp3`); got != `~\x.mp3` {
		t.Errorf("expandTilde(~\\x.mp3) = %q, want the input unchanged", got)
	}
}

// A completed prefix keeps the separator style it was typed with, so a Windows
// path is never handed back half-converted; only a prefix naming no directory yet
// falls back to the platform's own separator.
func TestPathSepKeepsTheTypedStyle(t *testing.T) {
	windowsSeparators(t)
	for _, tc := range []struct{ prefix, want string }{
		{`C:\Users\me\`, `\`},
		{`C:\Users\me`, `\`},
		{`C:/Users/me`, `/`},
		{`C:\Users/me`, `/`}, // the separator typed last wins
		{"Music", `\`},       // nothing typed yet: the platform default
		{"", `\`},
	} {
		if got := pathSep(tc.prefix); got != tc.want {
			t.Errorf("pathSep(%q) = %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

func TestEndsWithPathSepOnWindows(t *testing.T) {
	windowsSeparators(t)
	for _, tc := range []struct {
		s    string
		want bool
	}{
		{`C:\Users\me\`, true},
		{`C:\Users\me/`, true},
		{"Music/", true},
		{`C:\Users\me`, false},
		{"", false},
	} {
		if got := endsWithPathSep(tc.s); got != tc.want {
			t.Errorf("endsWithPathSep(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// The host's own reading is the one that ships: a slash separates everywhere, a
// backslash only on Windows, where a backslash-separated path is a path and off
// Windows it is a file name.
func TestHostSeparators(t *testing.T) {
	if !endsWithPathSep("/a/b/") {
		t.Error(`endsWithPathSep("/a/b/") = false, want true`)
	}
	if got := pathSep("/a/b"); got != "/" {
		t.Errorf(`pathSep("/a/b") = %q, want "/"`, got)
	}
	if os.IsPathSeparator('\\') {
		return // Windows: the backslash readings are pinned by the injected tests
	}
	if endsWithPathSep(`C:\Users\me\`) {
		t.Error(`endsWithPathSep("C:\\Users\\me\\") = true off Windows, want false`)
	}
	if got := pathSep(`C:\Users\me`); got != "/" {
		t.Errorf(`pathSep("C:\\Users\\me") = %q off Windows, want "/"`, got)
	}
}
