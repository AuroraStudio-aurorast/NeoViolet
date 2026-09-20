package ui

import (
	"net/url"
	"os"
	"strings"
	"unicode"
)

// normalizeDrop turns one paste payload into the paths it carries.
//
// Terminals disagree about how a dropped file is presented (macOS Terminal.app
// and iTerm2 backslash-escape spaces, WezTerm can quote or escape and appends a
// trailing space, some sources send a file:// URI), so the rules below
// normalise defensively and then let the filesystem decide: the form that
// exists on disk wins over a transformed one. That keeps a file name that
// really contains a backslash intact.
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
		if unescaped == path {
			out = append(out, path)
			continue
		}
		out = append(out, pickExisting(path, unescaped))
	}
	return out
}

// splitDropFields splits a paste payload into the fields a terminal may have
// packed into it. Whitespace separates them, except inside a quoted span and
// except when a backslash escapes it: macOS terminals escape the spaces of a
// dropped path, so those spaces belong to the file name rather than to the
// separator between two paths.
func splitDropFields(s string) []string {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
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
			}
			continue
		}
		if closing, ok := dropQuote(r); ok {
			b.WriteRune(r)
			quote = closing
			continue
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
	return fields
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

// unescapeBackslashes turns the escapes shells and terminals add for spaces,
// backslashes and quotes into their literal characters. Any other backslash
// sequence is left alone so ordinary file names survive.
func unescapeBackslashes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if c := s[i+1]; c == ' ' || c == '\\' || c == '\'' {
				i++
			}
		}
		b.WriteByte(s[i])
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
