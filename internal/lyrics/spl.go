package lyrics

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// splStampFields is the digit grammar of a single stamp with each field captured,
// shared by splStampRe below and the in-line marker regex so the two cannot drift
// apart. The standard gives minutes one to three digits, seconds one to two, and
// the fraction one to six.
const splStampFields = `(\d{1,3}):(\d{1,2})(?:\.(\d{1,6}))?`

var splStampRe = regexp.MustCompile("^" + splStampFields + "$")

// splMarkerRe finds the stamps written inside a line body, each of which marks
// where the text after it starts. It shares splStampFields with splStampRe so a
// bracket the standard calls a wrong writing stays text instead of turning into
// a marker: the two grammars cannot drift apart, and a malformed stamp is handled
// in one place, by parseSPLStamp.
var splMarkerRe = regexp.MustCompile(`\[` + splStampFields + `\]`)

func init() {
	RegisterParser("spl", &splParser{})
}

// splParser parses Salt Player Lyrics (SPL), the reading-friendly format Salt
// Player writes beside a track:
//
//	https://moriafly.com/standards/spl.html
//
// SPL is specified as a superset of enhanced LRC, but it gets its own format
// name and its own ".spl" extension instead of being folded into lrcParser,
// because the two disagree about what the same bytes mean. SPL reads a timestamp
// inside a line as the start of the next word and one at the very end as the
// line's end; LRC reads both as further line timestamps and merges the lines
// that share one into translation parts. Teaching either parser both grammars
// would change what an existing .lrc file displays, so each keeps its own
// reading of its own extension. The cost is a known gap: SPL content stored in an
// .lrc file still parses as LRC, which is why Salt Player asks for the .spl
// extension in the first place.
type splParser struct{}

// FindSidecar looks for ".spl" only. See the type comment for why ".lrc" is not
// a fallback here: lrcParser owns that extension.
func (p *splParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".spl")
}

// Parse reads an SPL file. A lyric line is "[stamp]text", and several adjacent
// leading stamps are SPL's repeat syntax: "[05:20.22][05:30.22]text" is one text
// line that starts at each stamp. A stamp inside the text starts the word after
// it, and a stamp with no text after it ends a line instead of writing one.
func (p *splParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read spl: %w", err)
	}

	lyrics := &Data{Path: sourcePath}
	var lines []LyricLine

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		inner, body, ok := cutSPLStamp(line)
		if !ok {
			continue
		}

		// "[ti:Title]" is a header, not a stamp. An unrecognised key such as
		// "[re:x]" reports false and falls through to the stamp parse below, which
		// drops the line rather than misreading it.
		if key, val, isField := lrcField(inner); isField && applyHeaderField(lyrics, key, val) {
			continue
		}

		start, err := parseSPLStamp(inner)
		if err != nil {
			continue
		}

		stamps := []time.Duration{start}
		for {
			next, rest, ok := cutSPLStamp(body)
			if !ok {
				break
			}
			stamp, err := parseSPLStamp(next)
			if err != nil {
				break
			}
			stamps = append(stamps, stamp)
			body = rest
		}

		// delta is read once per line, after applyHeaderField above may have
		// written lyrics.Offset, so "[offset:]" shifts only the lines that follow
		// it — the same timing rule LRC and LYS follow.
		delta := time.Duration(lyrics.Offset) * time.Millisecond

		// The line's text is the body with the markers cut out of it, while the
		// scanner reads the body itself: that is where the markers still are.
		trimmed := strings.TrimSpace(body)
		text := splMarkerRe.ReplaceAllString(trimmed, "")
		if text == "" {
			// A stamp with no text after it is the standard's line-end marker: it
			// writes no lyric of its own, it only says where the previous one stops.
			// With no line before it there is nothing to end.
			if n := len(lines); n > 0 {
				if end := shiftTime(stamps[len(stamps)-1], delta); end > lines[n-1].Time {
					lines[n-1].End = end
				}
			}
			continue
		}

		for _, stamp := range stamps {
			// The word markers are read against each line's own start rather than
			// once for the text: a repeat that starts after them cannot use them, and
			// the standard reads the markers it cannot place as ignored (scanSPLBody).
			words, end := scanSPLBody(trimmed, stamp)
			at := shiftTime(stamp, delta)
			line := LyricLine{Time: at, Text: text, Words: shiftWords(words, delta)}
			// An end that does not survive the shift is dropped rather than stored
			// below the line's own start, which the panel would read as a line that
			// is never active.
			if shifted := shiftTime(end, delta); shifted > at {
				line.End = shifted
			}
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid spl lines found")
	}

	sortLyricLines(lines)
	lyrics.Lines = lines
	return lyrics, nil
}

// cutSPLStamp splits one leading "[...]" bracket off s and returns its content
// and the remainder. It is deliberately not bracketInner: the repeat loop needs
// the remainder, and an angle bracket must never pass as a line stamp — "<...>"
// marks a position inside a line, so a line that opens with one has no start
// time and is not a lyric line at all.
func cutSPLStamp(s string) (inner, rest string, ok bool) {
	if !strings.HasPrefix(s, "[") {
		return "", s, false
	}
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return "", s, false
	}
	return s[1:end], s[end+1:], true
}

// scanSPLBody splits a line body into the fragments the panel sweeps word by
// word. A stamp inside the body marks where the text after it starts, so every
// fragment runs from its own stamp to the next one; a stamp at the very end of
// the body has no text behind it and is the line's end instead. The standard
// requires the stamps to increase and to stay inside the line, and reads a stamp
// that breaks either rule as ignored — its text joins the fragment before it, as
// if the stamp had not been written at all.
//
// lineStart is the line's own time, and the start of a fragment that precedes the
// first stamp. The returned end is zero when the line carries no end marker and
// therefore lasts until the next line starts.
func scanSPLBody(body string, lineStart time.Duration) (words []WordFragment, end time.Duration) {
	matches := splMarkerRe.FindAllStringSubmatchIndex(body, -1)

	// A stamp at the end of the body is the line's end, not a word marker: it
	// writes no text of its own, so it is cut out of the line as well.
	textEnd := len(body)
	if n := len(matches); n > 0 && matches[n-1][1] == len(body) {
		if stamp, err := parseSPLStamp(markerInner(body, matches[n-1])); err == nil && stamp > lineStart {
			end = stamp
		}
		textEnd = matches[n-1][0]
		matches = matches[:n-1]
	}

	at := lineStart
	var pending strings.Builder
	cut := 0
	for _, loc := range matches {
		pending.WriteString(body[cut:loc[0]])
		cut = loc[1]
		stamp, err := parseSPLStamp(markerInner(body, loc))
		// An ignored marker leaves nothing behind, so the text on both sides of it
		// lands in the same fragment.
		if err != nil || stamp <= at || (end > 0 && stamp >= end) {
			continue
		}
		if text := pending.String(); text != "" {
			words = append(words, WordFragment{Time: at, Text: text})
			pending.Reset()
		}
		at = stamp
	}
	pending.WriteString(body[cut:textEnd])
	if text := pending.String(); text != "" {
		words = append(words, WordFragment{Time: at, Text: text})
	}
	return words, end
}

// markerInner returns the stamp a splMarkerRe match wraps, with the brackets the
// match includes dropped again.
func markerInner(body string, loc []int) string {
	return body[loc[0]+1 : loc[1]-1]
}

// parseSPLStamp parses one SPL stamp. It does not reuse parseTimestamp, which
// reads the seconds as a float: that accepts any fraction length and a third
// colon-separated field, and drops everything after the first field without a
// word. The standard pins a digit count for each field instead, which is what
// splStampRe enforces here.
//
// The fraction is a string of digits rather than a decimal fraction, so it is
// padded on the right: a fraction shorter than three digits counts as having
// zeros omitted from the end, which makes ".5" 500ms and not 5ms or 50ms. Four
// to six digits therefore mean sub-millisecond precision, which is why the
// padding target is microseconds rather than milliseconds.
func parseSPLStamp(s string) (time.Duration, error) {
	m := splStampRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid spl timestamp: %q", s)
	}
	minutes, _ := strconv.Atoi(m[1])
	seconds, _ := strconv.Atoi(m[2])
	// Pad to six digits so the value lands in microseconds; the doc comment
	// explains why the digits are padded rather than scaled.
	fraction := m[3] + strings.Repeat("0", 6-len(m[3]))
	micros, _ := strconv.Atoi(fraction)
	return time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(micros)*time.Microsecond, nil
}
