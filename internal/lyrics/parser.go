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

type LyricParser interface {
	FindSidecar(audioPath string) string
	Parse(r io.Reader, sourcePath string) (*LyricsData, error)
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
