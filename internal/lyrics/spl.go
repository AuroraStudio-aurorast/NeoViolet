package lyrics

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// splStampFields is the digit grammar of one stamp: the standard gives minutes one
// to three digits, seconds one to two, and the fraction one to six. The line-stamp
// and the in-line marker regex share it so the two grammars cannot drift apart.
const splStampFields = `(\d{1,3}):(\d{1,2})(?:\.(\d{1,6}))?`

var splStampRe = regexp.MustCompile("^" + splStampFields + "$")

// splMarkerRe finds the stamps written inside a line, each of which starts the text
// after it. Both of the standard's bracket forms are read: "[...]" for a marker in
// the middle of the text, and "<...>" for the sugar that lets a marker stand at the
// start of a body, where a square bracket would repeat the line (see cutSPLStamp).
var splMarkerRe = regexp.MustCompile(`\[` + splStampFields + `\]|<` + splStampFields + `>`)

func init() {
	RegisterParser("spl", &splParser{})
}

// splParser parses Salt Player Lyrics (SPL), the reading-friendly format Salt
// Player writes beside a track: https://moriafly.com/standards/spl.html
//
// SPL is based on enhanced LRC but gets its own name and its own ".spl" extension
// instead of being folded into lrcParser, because the two read the same bytes
// differently: SPL takes a stamp inside a line as the next word's start and one at
// the end as the line's end, while LRC takes both as further line stamps and merges
// the lines that share one into translation parts. The cost is a known gap: SPL
// content stored under ".lrc" still reads as LRC.
type splParser struct{}

// FindSidecar looks for ".spl" only, since lrcParser owns ".lrc".
func (p *splParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".spl")
}

// Parse reads an SPL file: "[stamp]text" lines, adjacent leading stamps repeating
// one text, a stamp inside the text starting the word after it, and a stamp with no
// text after it ending a line instead of writing one. A line without a stamp, or one
// carrying the same stamp as the line it follows, is a translation.
func (p *splParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read spl: %w", err)
	}

	// The panel draws the parts after the first the way it draws a translated LRC row.
	lyrics := &Data{Path: sourcePath, TranslationsInParts: true}
	var lines []LyricLine
	// anchor is the first line of the group the last lyric line wrote, which is what a
	// translation without a stamp of its own belongs to. -1 means no lyric line yet.
	anchor := -1

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		inner, body, ok := cutSPLStamp(line)
		if !ok {
			// No stamp: the standard lets a translation leave it out when it sits right below
			// its lyric line, and consecutive lines like this are one
			// multi-line translation. An end marker in between does not
			// interrupt them; it is not a lyric line itself.
			if text := strings.TrimSpace(line); text != "" && anchor >= 0 {
				for i := anchor; i < len(lines); i++ {
					appendPart(&lines[i], text)
				}
			}
			continue
		}

		// "[ti:Title]" is a header, not a stamp. A colon key the header table does not know
		// ("[re:x]") falls through to the stamp parse below, which drops the line; a plain
		// stamp takes that path too, since it has a colon as well.
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

		// Read once per line, after applyHeaderField may have written lyrics.Offset, so
		// "[offset:]" shifts only the lines that follow it (LRC and LYS do the same).
		delta := time.Duration(lyrics.Offset) * time.Millisecond

		// text is the body with the markers cut out; the scanner needs the body itself,
		// since that is where the markers still are.
		trimmed := strings.TrimSpace(body)
		text := splMarkerRe.ReplaceAllString(trimmed, "")
		if text == "" {
			// A stamp with no text after it is the standard's line-end marker: it writes no
			// lyric, it only says where the previous one stops.
			if n := len(lines); n > 0 {
				if end := shiftTime(stamps[len(stamps)-1], delta); end > lines[n-1].Time {
					lines[n-1].End = end
				}
			}
			continue
		}

		// Move the anchor before this line is written, so a translation that follows
		// attaches to it and to every copy of a repeated one.
		anchor = len(lines)
		for _, stamp := range stamps {
			// The markers are read against each line's own start, not once for the text: a
			// repeat that starts after them cannot use them.
			words, end := scanSPLBody(trimmed, stamp)
			at := shiftTime(stamp, delta)
			line := LyricLine{Time: at, Text: text, Words: shiftWords(words, delta)}
			// Drop an end the offset pushed to or below the line's own start: the panel would
			// read that as a line that is never active.
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
	lyrics.Lines = mergeSameTime(lines)
	return lyrics, nil
}

// mergeSameTime folds the lines that share one stamp into a single line whose Parts
// are its display rows. The standard recognises a translation by its timestamp and
// allows the two lines to be written apart, so they are grouped over the whole file
// rather than run by run the way LRC's mergeSameTimestamp does. The line written
// first governs the merged one; the later ones only add a row.
func mergeSameTime(lines []LyricLine) []LyricLine {
	if len(lines) < 2 {
		return lines
	}

	out := lines[:0]
	for i := 0; i < len(lines); {
		main := lines[i]
		next := i + 1
		for next < len(lines) && lines[next].Time == main.Time {
			appendPart(&main, lines[next].Text)
			next++
		}
		out = append(out, main)
		i = next
	}
	return out
}

// cutSPLStamp splits one leading "[...]" bracket off s and returns its content and
// the remainder. It is not bracketInner: the repeat loop needs the remainder, and a
// line that opens with "<...>" has no start time and is not a lyric line at all.
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

// scanSPLBody splits a line body into the fragments the panel sweeps word by word.
// A stamp marks where the text after it starts, so a fragment runs from its own
// stamp to the next one; a stamp at the very end has no text behind it and is the
// line's end instead. The standard requires the stamps to increase and to stay
// inside the line, and reads a stamp that breaks either rule as ignored: its text
// joins the fragment before it, as if the stamp had not been written.
//
// lineStart is the line's own time, and the start of any text ahead of the first
// marker, so a marker at the start of the body holds the row back from lighting up.
// A returned end of zero means the line lasts until the next line starts.
func scanSPLBody(body string, lineStart time.Duration) (words []WordFragment, end time.Duration) {
	matches := splMarkerRe.FindAllStringSubmatchIndex(body, -1)

	// A stamp at the end of the body is the line's end, not a word marker: it writes
	// no text of its own, so it is cut out of the text as well.
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
		// An ignored marker leaves nothing behind, so the text on both sides of it lands
		// in the same fragment.
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

// markerInner returns the stamp a splMarkerRe match wraps, without the brackets.
func markerInner(body string, loc []int) string {
	return body[loc[0]+1 : loc[1]-1]
}

// appendPart adds one display row to a line: further rows are translations, and Text
// keeps carrying all of them joined the way LRC and the TTML adapter join theirs.
func appendPart(line *LyricLine, text string) {
	if line.Parts == nil {
		line.Parts = []string{line.Text}
	}
	line.Parts = append(line.Parts, text)
	line.Text = strings.Join(line.Parts, " | ")
}

// parseSPLStamp parses one stamp. It does not reuse parseTimestamp, which reads the
// seconds as a float and so accepts any fraction length and a third colon-separated
// field, where the standard pins a digit count for each field.
//
// The fraction is a string of digits rather than a decimal fraction, so it is padded
// on the right: ".5" is 500ms and not 5ms or 50ms. Four to six digits therefore mean
// sub-millisecond precision, which is why the padding target is microseconds.
func parseSPLStamp(s string) (time.Duration, error) {
	m := splStampRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid spl timestamp: %q", s)
	}
	minutes, _ := strconv.Atoi(m[1])
	seconds, _ := strconv.Atoi(m[2])
	fraction := m[3] + strings.Repeat("0", 6-len(m[3]))
	micros, _ := strconv.Atoi(fraction)
	return time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(micros)*time.Microsecond, nil
}
