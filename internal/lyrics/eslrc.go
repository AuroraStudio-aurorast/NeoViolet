package lyrics

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func init() {
	RegisterParser("eslrc", &eslrcParser{})
}

type eslrcParser struct{}

func (p *eslrcParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".eslrc")
}

func (p *eslrcParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read eslrc: %w", err)
	}

	lyrics := &Data{Path: sourcePath}
	var lines []LyricLine

	for _, rawLine := range strings.Split(string(data), "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		groups := bracketRe.FindAllStringSubmatchIndex(rawLine, -1)
		if len(groups) == 0 {
			continue
		}

		firstContent := rawLine[groups[0][2]:groups[0][3]]

		// Metadata headers share the "[key:value]" shape with LRC. They have to
		// be handled before parseTimestamp, which rejects them as malformed and
		// would skip the line before this branch could run.
		if key, val, isField := lrcField(firstContent); isField {
			if applyHeaderField(lyrics, key, val) {
				continue
			}
		}

		lineStart, err := parseTimestamp(firstContent)
		if err != nil {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder
		// In ESLRC each fragment's timestamp is the bracket that **follows**
		// it, i.e. its end; the fragment's start is the previous boundary.
		// "[00:00.000]" is a carry marker meaning "same as the last known
		// boundary" (that is what spaces carry).
		prevBoundary := lineStart
		timed := false

		for i := 1; i < len(groups); i++ {
			if text := rawLine[groups[i-1][1]:groups[i][0]]; text != "" {
				words = append(words, WordFragment{Time: prevBoundary, Text: text})
				fullText.WriteString(text)
			}
			if b, bErr := parseTimestamp(rawLine[groups[i][2]:groups[i][3]]); bErr == nil && b > 0 {
				prevBoundary = b
				timed = true
			}
		}

		lastEnd := groups[len(groups)-1][1]
		if lastEnd < len(rawLine) {
			if tail := rawLine[lastEnd:]; tail != "" {
				words = append(words, WordFragment{Time: prevBoundary, Text: tail})
				fullText.WriteString(tail)
			}
		}

		text := fullText.String()
		delta := time.Duration(lyrics.Offset) * time.Millisecond
		if !timed {
			// No word timestamps at all (an LRC-shaped file): keep the old
			// behaviour exactly — trimmed text, no Words, unbounded. The offset
			// still shifts Time (same delta as the word-bracket path), or a mixed
			// file with [offset:] would sort on a mixed basis and misorder lines.
			if text = strings.TrimSpace(text); text == "" {
				continue
			}
			lines = append(lines, LyricLine{Time: shiftTime(lineStart, delta), Text: text})
			continue
		}

		line := LyricLine{
			Time:  shiftTime(lineStart, delta),
			Text:  text,
			Words: shiftWords(words, delta),
		}
		if prevBoundary > lineStart {
			line.End = shiftTime(prevBoundary, delta)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid eslrc lines found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
}
