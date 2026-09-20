package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDrop(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "My File.mp3")
	if err := os.WriteFile(spaced, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The same name with the space backslash-escaped (the usual terminal form): it
	// does not exist on disk.
	escaped := filepath.Join(dir, `My\ File.mp3`)

	for _, tc := range []struct {
		name  string
		input string
		want  []string
	}{
		{"plain path", "/tmp/a.mp3", []string{"/tmp/a.mp3"}},
		{"trailing space", "/tmp/a.mp3 ", []string{"/tmp/a.mp3"}},
		{"backslash escaped, file exists", escaped, []string{spaced}},
		{"double quoted", `"/tmp/a.mp3"`, []string{"/tmp/a.mp3"}},
		{"single quoted", "'/tmp/a.mp3'", []string{"/tmp/a.mp3"}},
		{"curly quoted", "“/tmp/a.mp3”", []string{"/tmp/a.mp3"}},
		// A quoted path keeps its spaces together: only the quotes are stripped.
		{"quoted path with space", `"` + spaced + `"`, []string{spaced}},
		// An escaped space belongs to the name, a bare one separates two paths.
		{"escaped space then second file", escaped + " " + filepath.Join(dir, "b.mp3"), []string{spaced, filepath.Join(dir, "b.mp3")}},
		{"multiple files", "/tmp/a.mp3\n/tmp/b.mp3", []string{"/tmp/a.mp3", "/tmp/b.mp3"}},
		{"multiple with spaces", "/tmp/a.mp3   /tmp/b.mp3", []string{"/tmp/a.mp3", "/tmp/b.mp3"}},
		{"empty payload", "   ", nil},
		{"quotes only", `""`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDrop(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizeDrop(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("element %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNormalizeDropFileURI(t *testing.T) {
	got := normalizeDrop("file:///tmp/My%20File.mp3")
	if len(got) != 1 || got[0] != "/tmp/My File.mp3" {
		t.Errorf("normalizeDrop(file://) = %v, want [/tmp/My File.mp3]", got)
	}
}

// A real file name containing a backslash must not be destroyed by unescaping
// (legal on Linux/macOS).
func TestNormalizeDropKeepsRealBackslash(t *testing.T) {
	dir := t.TempDir()
	weird := filepath.Join(dir, `a\b.mp3`)
	if err := os.WriteFile(weird, []byte("x"), 0o600); err != nil {
		t.Skipf("cannot create %q: %v", weird, err)
	}
	got := normalizeDrop(weird)
	if len(got) != 1 || got[0] != weird {
		t.Errorf("normalizeDrop(%q) = %v, want the original", weird, got)
	}
}

func TestUnescapeBackslashes(t *testing.T) {
	for _, tc := range [][2]string{
		{`/a/My\ File.mp3`, "/a/My File.mp3"},
		{`/a/back\\slash.mp3`, `/a/back\slash.mp3`},
		{`/a/it\'s.mp3`, "/a/it's.mp3"},
		{`/a/no\escape.mp3`, `/a/no\escape.mp3`},
		{`/plain.mp3`, "/plain.mp3"},
	} {
		if got := unescapeBackslashes(tc[0]); got != tc[1] {
			t.Errorf("unescapeBackslashes(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestInsertPathAtCursor(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	ti.SetValue("open ")
	ti.CursorEnd()

	insertPathAtCursor(m, "/a/My File.mp3")
	if got := ti.Value(); got != "open /a/My File.mp3" {
		t.Fatalf("value = %q", got)
	}
	if got := ti.Position(); got != len("open /a/My File.mp3") {
		t.Errorf("cursor = %d, want the end of the inserted path", got)
	}
}

// A separating space is added unless the rune before the cursor already is one;
// nothing is added at the start of the line.
func TestInsertPathAtCursorSeparator(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput

	ti.SetValue("open")
	ti.CursorEnd()
	insertPathAtCursor(m, "/x.mp3")
	if got := ti.Value(); got != "open /x.mp3" {
		t.Errorf("value = %q, want a separating space", got)
	}

	ti.SetValue("")
	insertPathAtCursor(m, "/y.mp3")
	if got := ti.Value(); got != "/y.mp3" {
		t.Errorf("value = %q, want no leading space", got)
	}
}

// An over-long path is not silently truncated: the input line is left untouched
// when it would exceed the limit.
func TestInsertPathAtCursorRespectsCharLimit(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	ti.SetValue("open /a.mp3")
	long := "/" + strings.Repeat("x", 300) + ".mp3"

	insertPathAtCursor(m, long)
	if got := ti.Value(); got != "open /a.mp3" {
		t.Errorf("value changed to %q, want it untouched", got)
	}
}
