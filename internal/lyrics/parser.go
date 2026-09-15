package lyrics

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxLyricSize is the maximum lyric file size we'll read into memory (1 MB).
// Files larger than this are rejected with ErrLyricTooLarge to prevent OOM.
const maxLyricSize = 1 * 1024 * 1024

// ErrLyricTooLarge is returned when a lyric file exceeds maxLyricSize.
var ErrLyricTooLarge = errors.New("lyrics file too large (>1MB)")

// LyricParser parses lyric content from a sidecar file or reader.
type LyricParser interface {
	FindSidecar(audioPath string) string
	Parse(r io.Reader, sourcePath string) (*Data, error)
}

// findSidecarWithExt checks if a sidecar file exists with the given extension
// next to the audio file. The first matching extension is returned.
func findSidecarWithExt(audioPath string, exts ...string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	for _, candidate := range exts {
		path := base + candidate
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// sortLyricLines sorts lyric lines by Time in ascending order.
func sortLyricLines(lines []LyricLine) {
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})
}

// parseWordTimedLine extracts word fragments from a lyric body using the given regex,
// building both the full text and word-level fragments.
func parseWordTimedLine(body string, wordRe *regexp.Regexp) (words []WordFragment, fullText string) {
	matches := wordRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, ""
	}
	var sb strings.Builder
	for _, m := range matches {
		// Generic: each regex must capture (wordText, startMs) in any order.
		// We use m[1] and m[2] as convention; subclasses define their own regex.
		wordStart, _ := strconv.Atoi(m[2])
		words = append(words, WordFragment{
			Time: time.Duration(wordStart) * time.Millisecond,
			Text: m[1],
		})
		sb.WriteString(m[1])
	}
	return words, sb.String()
}

// parseQRCWordTimedLine parses QRC-format word fragments.
// QRC format: text(startMs,durationMs)
func parseQRCWordTimedLine(body string) (words []WordFragment, fullText string) {
	return parseWordTimedLine(body, qrcWordRe)
}

// parseYRCWordTimedLine parses YRC-format word fragments.
// YRC format: (startMs,durationMs,flag)text
func parseYRCWordTimedLine(body string) (words []WordFragment, fullText string) {
	matches := yrcWordRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, ""
	}
	var sb strings.Builder
	for _, m := range matches {
		wordStart, _ := strconv.Atoi(m[1])
		_ = m[2] // duration, unused
		_ = m[3] // flag, unused
		wordText := m[4]
		words = append(words, WordFragment{
			Time: time.Duration(wordStart) * time.Millisecond,
			Text: wordText,
		})
		sb.WriteString(wordText)
	}
	return words, sb.String()
}

// readAllWithLimit reads from r up to maxLyricSize+1 bytes.
func readAllWithLimit(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxLyricSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxLyricSize {
		return nil, ErrLyricTooLarge
	}
	return data, nil
}

// lrcField splits the inner content of one "[key:value]" bracket into a
// lowercased key and its trimmed value. ok is false when the content is not
// shaped like a metadata field, so callers can fall through to their own
// header handling (QRC's "[1000,2000]" must not be mistaken for metadata).
func lrcField(content string) (key, val string, ok bool) {
	k, v, found := strings.Cut(content, ":")
	if !found {
		return "", "", false
	}
	k = strings.ToLower(strings.TrimSpace(k))
	if k == "" {
		return "", "", false
	}
	return k, strings.TrimSpace(v), true
}

// applyHeaderField writes one LRC-style metadata field into d and reports
// whether the key was a recognised metadata field with a usable value.
// A malformed offset is not a header field: it reports false so callers keep
// parsing the line as a lyric line.
func applyHeaderField(d *Data, key, val string) bool {
	switch key {
	case "ti":
		d.Title = val
	case "ar":
		d.Artist = val
	case "al":
		d.Album = val
	case "au":
		d.Author = val
	case "by":
		d.Creator = val
	case "offset":
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		d.Offset = n
	default:
		return false
	}
	return true
}

// shiftTime returns t + delta, clamped at zero.
func shiftTime(t, delta time.Duration) time.Duration {
	if delta == 0 {
		return t
	}
	if shifted := t + delta; shifted > 0 {
		return shifted
	}
	return 0
}

// shiftWords returns words with every Time shifted by delta, clamped at zero.
// The input slice is left untouched.
func shiftWords(words []WordFragment, delta time.Duration) []WordFragment {
	if delta == 0 || len(words) == 0 {
		return words
	}
	out := make([]WordFragment, len(words))
	for i, w := range words {
		out[i] = WordFragment{Time: shiftTime(w.Time, delta), Text: w.Text}
	}
	return out
}

// wordTimedRe describes one word-timed body format: the regex plus the capture
// group holding each piece. QRC puts the text before its timestamp and YRC
// after it, so the group indexes have to travel with the regex.
type wordTimedRe struct {
	re       *regexp.Regexp
	text     int
	start    int
	duration int
}

// wordTimedScan is the result of one scan over a word-timed body.
type wordTimedScan struct {
	// Start is the Time of Words[0]: lineStart when the body opens with untimed
	// text, otherwise the first timed word's own start.
	Start time.Duration
	// End is the greatest timed (start + duration) seen, or 0 when no
	// timestamp carried a duration past lineStart.
	End time.Duration
	// Text is every fragment's text in order; it is always exactly the
	// concatenation of Words, which is what the panel's karaoke needs (C6).
	Text string
	// Words keeps untimed leading/trailing text as ordinary fragments too, so
	// that Text and Words cannot disagree.
	Words []WordFragment
}

// scanWordTimed scans a word-timed body (QRC, YRC and LYS share this).
//
// Text outside the matches — a leading fragment before the first timestamp and
// a trailing one after the last — is kept as a fragment whose Time is the last
// known boundary. Dropping it (the previous behaviour, which also made QRC and
// YRC disagree with each other) would leave Words unable to tile Text, and the
// panel silently degrades to whole-line highlighting when a line does not tile.
func scanWordTimed(body string, groups wordTimedRe, lineStart time.Duration) wordTimedScan {
	var scan wordTimedScan

	locs := groups.re.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		if text := strings.TrimSpace(body); text != "" {
			scan.Start = lineStart
			scan.Text = text
			scan.Words = []WordFragment{{Time: lineStart, Text: text}}
		}
		return scan
	}

	var sb strings.Builder
	boundary := lineStart
	timed := false

	if head := body[:locs[0][0]]; head != "" {
		scan.Words = append(scan.Words, WordFragment{Time: boundary, Text: head})
		sb.WriteString(head)
	}

	for _, loc := range locs {
		text := body[loc[2*groups.text]:loc[2*groups.text+1]]
		startMs, _ := strconv.Atoi(body[loc[2*groups.start]:loc[2*groups.start+1]])
		durMs, _ := strconv.Atoi(body[loc[2*groups.duration]:loc[2*groups.duration+1]])

		startAt := time.Duration(startMs) * time.Millisecond
		if text != "" {
			scan.Words = append(scan.Words, WordFragment{Time: startAt, Text: text})
			sb.WriteString(text)
		}
		// Degenerate (0,0) tuples must not drag the boundary backwards.
		if endAt := startAt + time.Duration(durMs)*time.Millisecond; endAt > boundary {
			boundary = endAt
			timed = true
		}
	}

	if tail := body[locs[len(locs)-1][1]:]; tail != "" {
		scan.Words = append(scan.Words, WordFragment{Time: boundary, Text: tail})
		sb.WriteString(tail)
	}

	scan.Text = sb.String()
	if len(scan.Words) == 0 {
		return wordTimedScan{}
	}
	scan.Start = scan.Words[0].Time
	if timed {
		scan.End = boundary
	}
	return scan
}
