package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

func TestNormalizeDrop(t *testing.T) {
	dir := t.TempDir()
	// A dropped path arrives with every space escaped, the way a terminal hands one
	// over: escaping the whole path here keeps these fixtures honest on machines
	// whose temporary directory contains a space.
	escapeDropped := func(p string) string { return strings.ReplaceAll(p, " ", `\ `) }
	spaced := filepath.Join(dir, "My File.mp3")
	if err := os.WriteFile(spaced, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The same name with every space backslash-escaped (the usual terminal form):
	// this form does not exist on disk.
	escaped := escapeDropped(spaced)
	// The same idea with wide characters: escaping and unescaping have to round-trip
	// a multi-byte name without losing bytes.
	cjk := filepath.Join(dir, "歌 曲.mp3")
	if err := os.WriteFile(cjk, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	escapedCJK := escapeDropped(cjk)
	// A name containing a double quote: the payload escapes it as \", so the
	// escape has to come off again to reach the file that is on disk.
	quotedQuote := filepath.Join(dir, `a"b.mp3`)
	if err := os.WriteFile(quotedQuote, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	escapedQuote := escapeDropped(strings.Replace(quotedQuote, `"`, `\"`, 1))
	// A name with spaces and a shell special character: a macOS terminal escapes
	// both the spaces and the "!" (history expansion), while the comma travels as
	// it is, exactly as it does for a real drop.
	special := filepath.Join(dir, "Taylor Swift,Brendon Urie - ME!.mp3")
	if err := os.WriteFile(special, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	escapedSpecial := escapeDropped(strings.ReplaceAll(special, "!", `\!`))

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
		{"single quoted path with space", "'" + spaced + "'", []string{spaced}},
		{"curly quoted path with space", "“" + spaced + "”", []string{spaced}},
		{"single curly quoted path with space", "‘" + spaced + "’", []string{spaced}},
		// An escaped space belongs to the name, a bare one separates two paths.
		{"escaped space then second file", escaped + " " + filepath.Join(dir, "b.mp3"), []string{spaced, filepath.Join(dir, "b.mp3")}},
		{"escaped CJK space, file exists", escapedCJK, []string{cjk}},
		{"escaped double quote, file exists", escapedQuote, []string{quotedQuote}},
		{"escaped shell special, file exists", escapedSpecial, []string{special}},
		{"multiple files", "/tmp/a.mp3\n/tmp/b.mp3", []string{"/tmp/a.mp3", "/tmp/b.mp3"}},
		{"multiple with spaces", "/tmp/a.mp3   /tmp/b.mp3", []string{"/tmp/a.mp3", "/tmp/b.mp3"}},
		{"empty payload", "   ", nil},
		{"quotes only", `""`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDrop(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizeDrop(%q) = %q, want %q", tc.input, got, tc.want)
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
		{`/a/a\"b.mp3`, `/a/a"b.mp3`},
		{`/a/no\escape.mp3`, `/a/no\escape.mp3`},
		{`/plain.mp3`, "/plain.mp3"},
	} {
		if got := unescapeBackslashes(tc[0]); got != tc[1] {
			t.Errorf("unescapeBackslashes(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

// A backslash that really belongs to a file name must win over the stripped
// reading of it: both forms exist on disk here, and the payload as delivered
// names the one that carries the backslash.
func TestNormalizeDropPrefersTheBackslashThatExists(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "xb.mp3")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	withSlash := filepath.Join(dir, `x\b.mp3`)
	if err := os.WriteFile(withSlash, []byte("x"), 0o600); err != nil {
		t.Skipf("cannot create %q: %v", withSlash, err)
	}

	got := normalizeDrop(withSlash)
	if len(got) != 1 || got[0] != withSlash {
		t.Errorf("normalizeDrop(%q) = %v, want the name that carries the backslash", withSlash, got)
	}
}

// When only the stripped reading is on disk the drop still has to land: the
// escape a terminal added for a shell special character may hide the real name,
// so the form that exists on disk wins.
func TestNormalizeDropStripsEscapesToReachTheFile(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "xb.mp3")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Not on disk: only the stripped reading exists.
	if got := normalizeDrop(filepath.Join(dir, `x\b.mp3`)); len(got) != 1 || got[0] != plain {
		t.Errorf("normalizeDrop(x\\b.mp3) = %v, want %q", got, plain)
	}

	special := filepath.Join(dir, "me!.mp3")
	if err := os.WriteFile(special, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := normalizeDrop(filepath.Join(dir, `me\!.mp3`)); len(got) != 1 || got[0] != special {
		t.Errorf("normalizeDrop(me\\!.mp3) = %v, want %q", got, special)
	}

	// A name that really contains a backslash arrives doubled, so the pair has to
	// be turned back into one backslash to reach the file: keeping the doubled form
	// as it is and merely stripping escapes both miss. This needs its own temporary
	// directory, because the first case here requires the single-backslash name not
	// to exist on disk.
	doubledDir := t.TempDir()
	doubledFile := filepath.Join(doubledDir, `x\b.mp3`)
	if err := os.WriteFile(doubledFile, []byte("x"), 0o600); err != nil {
		t.Skipf("cannot create %q: %v", doubledFile, err)
	}
	// Drops arrive with every space escaped, the way the table fixtures above are
	// built, so a temporary directory containing a space stays one field.
	escapeDropped := func(p string) string { return strings.ReplaceAll(p, " ", `\ `) }
	doubled := escapeDropped(strings.ReplaceAll(doubledFile, `\`, `\\`))
	if got := normalizeDrop(doubled); len(got) != 1 || got[0] != doubledFile {
		t.Errorf("normalizeDrop(%q) = %v, want %q", doubled, got, doubledFile)
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
	if got := ti.Position(); got != len([]rune("open /a/My File.mp3")) {
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

// Dropping into the middle of a line inserts at the cursor and keeps whatever
// sits after it.
func TestInsertPathAtCursorKeepsTextAfterTheCursor(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	ti.SetValue("open  tail")
	ti.SetCursor(5)

	insertPathAtCursor(m, "/x.mp3")

	if got := ti.Value(); got != "open /x.mp3 tail" {
		t.Errorf("value = %q, want %q", got, "open /x.mp3 tail")
	}
	if got := ti.Position(); got != len([]rune("open /x.mp3")) {
		t.Errorf("cursor = %d, want %d", got, len([]rune("open /x.mp3")))
	}
}

// The limit counts runes, not bytes: wide characters that fit the limit must
// still be inserted even though their UTF-8 form is three times as long.
func TestInsertPathAtCursorCountsRunesNotBytes(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	ti.SetValue("open ")
	ti.CursorEnd()

	wide := strings.Repeat("歌", ti.CharLimit-5)
	insertPathAtCursor(m, wide)

	want := "open " + wide
	if got := ti.Value(); got != want {
		t.Fatalf("value has %d runes (%d bytes), want %d runes (%d bytes)",
			len([]rune(got)), len(got), len([]rune(want)), len(want))
	}
	if got := ti.Position(); got != len([]rune(want)) {
		t.Errorf("cursor = %d, want %d runes", got, len([]rune(want)))
	}
}

// The text already on the line counts in runes too: a wide prefix that fits by
// rune count must not be rejected just because its UTF-8 form is longer.
func TestInsertPathAtCursorCountsWidePrefixInRunes(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	ti.SetValue(strings.Repeat("歌", 200))
	ti.CursorEnd()

	insertPathAtCursor(m, strings.Repeat("x", 50))

	want := strings.Repeat("歌", 200) + " " + strings.Repeat("x", 50)
	if got := ti.Value(); got != want {
		t.Fatalf("value has %d runes (%d bytes), want %d runes (%d bytes)",
			len([]rune(got)), len(got), len([]rune(want)), len(want))
	}
	if got := ti.Position(); got != len([]rune(want)) {
		t.Errorf("cursor = %d, want %d runes", got, len([]rune(want)))
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

// A line that is nearly full must not be overfilled either: the guard counts the
// text already on the line, so the insert is refused outright rather than letting
// bubbles truncate the path to the limit and leave a name that does not exist.
// The path on its own stays far below the limit, which is what makes this case
// different from the over-long path above.
func TestInsertPathAtCursorRefusesToOverflowTheLine(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	// Six runes short of the limit; the separating space and the ten runes of path
	// pushed in below take the result past it.
	full := strings.Repeat("a", ti.CharLimit-6)
	ti.SetValue(full)
	ti.CursorEnd()

	insertPathAtCursor(m, strings.Repeat("b", 10))

	if got := ti.Value(); got != full {
		t.Fatalf("value has %d runes, want %d runes: the line was overfilled and truncated",
			len([]rune(got)), len([]rune(full)))
	}
	if got := ti.Position(); got != ti.CharLimit-6 {
		t.Errorf("cursor = %d, want %d", got, ti.CharLimit-6)
	}
}

// The guard counts the whole line, not just the part before the cursor: a line
// six runes short of the limit still refuses the insert when the cursor sits at
// its start, where only the inserted runes themselves fit under the limit.
func TestInsertPathAtCursorCountsTheWholeLine(t *testing.T) {
	m := setupModel()
	m.UI.Mode = ModeCommand
	ti := &m.Components.CommandInput
	full := strings.Repeat("a", ti.CharLimit-6)
	ti.SetValue(full)
	ti.SetCursor(0)

	insertPathAtCursor(m, strings.Repeat("b", 10))

	if got := ti.Value(); got != full {
		t.Fatalf("value has %d runes, want %d runes: the line was overfilled and truncated",
			len([]rune(got)), len([]rune(full)))
	}
	if got := ti.Position(); got != 0 {
		t.Errorf("cursor = %d, want 0", got)
	}
}

// A file name with two consecutive spaces has to survive the escape round trip:
// the escapes belong to the name, so nothing may fold the doubled space into a
// separator or collapse it, and the path handed back has to be the one that is
// really on disk.
func TestNormalizeDropKeepsDoubledSpacesInTheName(t *testing.T) {
	dir := t.TempDir()
	doubled := filepath.Join(dir, "My  File.mp3") // two spaces, one file name
	if err := os.WriteFile(doubled, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Each space escaped, the way a terminal hands a dropped path over.
	escaped := strings.ReplaceAll(doubled, " ", `\ `)

	for _, tc := range []struct{ name, input string }{
		{"escaped", escaped},
		{"quoted", `"` + doubled + `"`},
		{"escaped and quoted", `"` + escaped + `"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDrop(tc.input)
			if len(got) != 1 {
				t.Fatalf("normalizeDrop(%q) = %q, want exactly one path", tc.input, got)
			}
			if got[0] != doubled {
				t.Errorf("path = %q, want %q: both spaces belong to the name", got[0], doubled)
			}
		})
	}

	// The same payload dropped into normal mode loads the file.
	m := setupModel()
	updated, cmd := updateDispatcher(m, tea.PasteMsg{Content: escaped})
	m = updated.(*Model)
	if !m.Loading || !m.switchingTrack || cmd == nil {
		t.Errorf("Loading = %v, switchingTrack = %v, cmd = %v; want the doubled-space file to load",
			m.Loading, m.switchingTrack, cmd)
	}
}

// escapeForDrop mirrors the GUI's escape_for_drop (tools/neoviolet-gui/src/
// drop_paste.rs). It is kept here, not imported, because the TUI must not
// depend on the GUI — but the copy is covered by the same review that keeps
// README's lyrics table in step: the contract test below fails when the two
// programs' escape sets drift apart.
func escapeForDrop(path string) string {
	var b strings.Builder
	for _, r := range path {
		if strings.ContainsRune(" \\'\"\u201c\u201d\u2018\u2019", r) || unicode.IsSpace(r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// A GUI-escaped payload must round-trip to the original name even when the
// file no longer exists on disk (delete-just-before-drop, or the payload
// replayed later). unescapeBackslashes is the exact inverse of escapeForDrop,
// so the de-escaped candidate has to win without any disk tie-break.
func TestNormalizeDropRoundTripsGUIEscapes(t *testing.T) {
	for _, name := range []string{
		"my song.mp3",
		`a\b.mp3`,
		"it's.mp3",
		`a"b.mp3`,
		"quoth\u201c.mp3",
		"quoth\u201d.mp3",
		"left\u2018right.mp3",
		"left\u2019right.mp3",
		"left\u00a0right.mp3", // NBSP — a splitter separator if unescaped
		"left\u3000right.mp3", // ideographic space
		"left\u000bright.mp3", // vertical tab
	} {
		payload := escapeForDrop(name)
		got := normalizeDrop(payload)
		if len(got) != 1 || got[0] != name {
			t.Errorf("normalizeDrop(%q) = %q, want %q", payload, got, name)
		}
	}
}

// A quote that opens but never closes is a broken path, not the start of a
// monster field: the rest of the payload keeps standing on its own, quoted
// reads happen literally, and normalizeDrop's Trim strips the stray quotes of
// the first field.
func TestSplitDropFieldsBreaksAnUnclosedQuote(t *testing.T) {
	payload := `"a'b.mp3 b.mp3 c.mp3`
	fields := splitDropFields(payload)
	if len(fields) != 3 || fields[0] != `"a'b.mp3` || fields[1] != "b.mp3" || fields[2] != "c.mp3" {
		t.Fatalf("splitDropFields(%q) = %q", payload, fields)
	}
	paths := normalizeDrop(payload)
	if len(paths) != 3 || paths[0] != `a'b.mp3` || paths[1] != "b.mp3" || paths[2] != "c.mp3" {
		t.Errorf("normalizeDrop(%q) = %q", payload, paths)
	}
}
