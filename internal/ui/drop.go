package ui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// normalizeDrop turns one paste payload into the paths it carries.
//
// Terminals disagree about how a dropped file is presented (macOS Terminal.app
// and iTerm2 backslash-escape spaces and shell special characters, WezTerm can
// quote or escape and appends a trailing space, some sources send a file://
// URI), so the rules below normalise defensively and then let the filesystem
// decide: the form that exists on disk wins over a transformed one. That keeps a
// file name that really contains a backslash intact.
func normalizeDrop(content string) []string {
	fields := splitDropFields(content)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		path := strings.Trim(field, `'"“”‘’`)
		if path == "" {
			continue
		}
		if strings.HasPrefix(path, "file://") {
			if decoded, ok := pathFromFileURI(path); ok {
				path = decoded
			}
		}
		unescaped := unescapeBackslashes(path)
		stripped := stripAnyEscape(path)
		// A payload without a backslash either transform can act on has a single
		// form, so there is nothing to look up.
		if unescaped == path && stripped == path {
			out = append(out, path)
			continue
		}
		// The de-escaped form first: it is the exact inverse of the escaping
		// dropping terminals add, so an unmatched round trip still ends at the
		// right name. The disk then decides among these three; when nothing
		// exists, pickExisting falls back to the first candidate — which is
		// why the de-escaped form, not the raw one, must come first.
		out = append(out, pickExisting(unescaped, stripped, path))
	}
	return out
}

// splitDropFields splits a paste payload into the fields a terminal may have
// packed into it. Whitespace separates them, except inside a quoted span and
// except when a backslash escapes it: macOS terminals escape the spaces of a
// dropped path, so those spaces belong to the file name rather than to the
// separator between two paths.
//
// A quote that opens but never closes is a broken path, not a monster field:
// a file name may hold one literal quote (a candidate the Trim in
// normalizeDrop strips), and an unbalanced quote must not glue every remaining
// field onto its tail. The pass that reports an unclosed span is therefore
// re-run with quote handling off, so each whitespace-separated run stands on
// its own.
func splitDropFields(s string) []string {
	fields, closed := splitDropFieldsRaw(s, true)
	if closed {
		return fields
	}
	fields, _ = splitDropFieldsRaw(s, false) // quotes read literally: re-scan
	return fields
}

// splitDropFieldsRaw is splitDropFields's scan pass. honorQuotes only decides
// whether dropQuote may open a span: quotes are literal characters when it is
// false. closed reports that every opened quote was closed.
func splitDropFieldsRaw(s string, honorQuotes bool) (fields []string, closed bool) {
	var b strings.Builder
	var quote rune
	escaped := false
	closed = true
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			b.WriteRune(r)
			escaped = true
			continue
		}
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
				closed = true
			}
			continue
		}
		if honorQuotes {
			if closing, ok := dropQuote(r); ok {
				b.WriteRune(r)
				quote = closing
				closed = false
				continue
			}
		}
		if unicode.IsSpace(r) {
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	if quote != 0 {
		closed = false
	}
	return fields, closed
}

// dropQuote reports whether r opens a quoted span and, if so, the rune that
// closes it. The curly forms come from terminals that quote a dropped path.
func dropQuote(r rune) (rune, bool) {
	switch r {
	case '"':
		return '"', true
	case '\'':
		return '\'', true
	case '“':
		return '”', true
	case '‘':
		return '’', true
	}
	return 0, false
}

// pickExisting prefers the form that exists on disk, falling back to the first
// candidate when neither does (the caller reports the failure).
func pickExisting(forms ...string) string {
	for _, form := range forms {
		if form == "" {
			continue
		}
		if _, err := os.Stat(form); err == nil {
			return form
		}
	}
	if len(forms) == 0 {
		return ""
	}
	return forms[0]
}

// pathFromFileURI converts a file:// URI to a filesystem path. url.Parse has
// already percent-decoded the path, so "%20" arrives as a space.
func pathFromFileURI(uri string) (string, bool) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
		return "", false
	}
	return parsed.Path, true
}

// unescapeBackslashes drops the backslash of every escape the payload may
// carry: ASCII and Unicode whitespace alike (the same set as
// `unicode.IsSpace`), backslashes, and the six quote characters
// `splitDropFields` treats as syntax. This is exactly the set the GUI escapes
// in `drop_paste.rs` (escapeForDrop), so a GUI-escaped payload round-trips to
// the original path even when neither form exists on disk; the round trip is
// pinned by TestNormalizeDropRoundTripsGUIEscapes. Any other backslash
// sequence is left alone so ordinary file names survive.
func unescapeBackslashes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			c := rune(s[i+1])
			if dropEscapes(c) {
				i++ // leave the escape behind; the loop writes c
			} else if r, size := utf8.DecodeRuneInString(s[i+1:]); size > 0 && dropEscapes(r) {
				b.WriteRune(r)
				i += 1 + size
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// dropEscapes reports whether r is a character a dropping source backslash-
// escapes: any whitespace (ASCII and Unicode alike) plus backslash and the six
// quote characters the splitter treats as syntax. Unescaped restorations below
// rely on the exact same set as the GUI's escape_for_drop, so the two must be
// kept in step; the contract test ties them together.
func dropEscapes(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '\\', '\'', '"', '\u201c', '\u201d', '\u2018', '\u2019':
		return true
	}
	return false
}

// stripAnyEscape drops the backslash of every escape a terminal may have added
// for a shell special character such as "!", leaving doubled backslashes alone
// because unescapeBackslashes resolves those. It is only a candidate form: the
// caller adopts it when that exact name exists on disk, so a file name that
// really contains a backslash still wins.
func stripAnyEscape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == '\\' {
				b.WriteByte(s[i])
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			i++ // drop the escape backslash itself
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// insertPathAtCursor writes p into the command line at the cursor, adding a
// separating space when it would otherwise glue onto the previous word.
//
// A path that would exceed the input limit leaves the line untouched rather
// than silently truncating it into something that does not exist.
func insertPathAtCursor(m *Model, p string) {
	ti := &m.Components.CommandInput
	runes := []rune(ti.Value())
	pos := ti.Position()
	if pos > len(runes) {
		pos = len(runes)
	}
	sep := ""
	if pos > 0 && runes[pos-1] != ' ' {
		sep = " "
	}
	inserted := []rune(sep + p)
	if len(runes)+len(inserted) > ti.CharLimit {
		return
	}

	updated := string(runes[:pos]) + string(inserted) + string(runes[pos:])
	ti.SetValue(updated)
	ti.SetCursor(pos + len(inserted))
	syncCompletion(m)
}

// handlePaste routes one bracketed-paste payload (a dropped file arrives here
// too). In command mode the paths go into the line at the cursor; in normal
// mode an audio file starts playing and anything else is ignored in silence.
func handlePaste(m *Model, content string) (tea.Model, tea.Cmd) {
	paths := normalizeDrop(content)
	if len(paths) == 0 {
		return m, nil
	}

	if m.UI.Mode == ModeCommand {
		for _, p := range paths {
			// Inserting a path refreshes the candidate state itself, so the
			// overlay cannot keep showing what the pre-paste line completed to.
			insertPathAtCursor(m, p)
		}
		return m, nil
	}

	for _, p := range paths {
		if !isValidAudioPath(p) {
			continue
		}
		if !playableExt(filepath.Ext(p)) {
			continue
		}
		return handleLoadTrack(m, LoadTrackMsg{Path: p})
	}
	return m, nil
}
