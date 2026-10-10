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
// line that starts at each stamp.
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

		text := strings.TrimSpace(body)
		if text == "" {
			continue
		}

		// delta is read once per line, after applyHeaderField above may have
		// written lyrics.Offset, so "[offset:]" shifts only the lines that follow
		// it — the same timing rule LRC and LYS follow.
		delta := time.Duration(lyrics.Offset) * time.Millisecond
		for _, stamp := range stamps {
			at := shiftTime(stamp, delta)
			lines = append(lines, LyricLine{
				Time:  at,
				Text:  text,
				Words: []WordFragment{{Time: at, Text: text}},
			})
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
