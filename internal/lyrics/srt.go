package lyrics

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var srtTimeRe = regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})[,.](\d{3})`)

func init() {
	RegisterParser("srt", &srtParser{})
}

type srtParser struct{}

func (p *srtParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".srt")
}

func (p *srtParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read srt: %w", err)
	}

	lyrics := &LyricsData{Path: sourcePath}
	var lines []LyricLine

	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Split entries by blank line (one or more blank lines)
	entries := splitSRTEntries(content)

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		entryLines := strings.SplitN(entry, "\n", 3)
		if len(entryLines) < 2 {
			continue
		}

		// First line should be the numeric index — validate but ignore it
		indexStr := strings.TrimSpace(entryLines[0])
		if _, err := strconv.Atoi(indexStr); err != nil {
			continue
		}

		// Second line should be the time range
		timeLine := strings.TrimSpace(entryLines[1])
		matches := srtTimeRe.FindStringSubmatch(timeLine)
		if matches == nil {
			continue
		}

		start := parseSRTTime(matches[1], matches[2], matches[3], matches[4])
		end := parseSRTTime(matches[5], matches[6], matches[7], matches[8])

		// Remaining lines are the text content
		text := ""
		if len(entryLines) >= 3 {
			text = strings.TrimSpace(entryLines[2])
		}

		if text == "" {
			continue
		}

		lines = append(lines, LyricLine{
			Time: start,
			End:  end,
			Text: text,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid srt entries found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
}

// splitSRTEntries splits SRT content into individual entries separated by blank lines.
func splitSRTEntries(content string) []string {
	// Normalize: ensure we handle Windows/Mac line endings
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Split on one or more blank lines (double newlines with possible whitespace)
	entries := regexp.MustCompile(`\n\s*\n`).Split(content, -1)
	var result []string
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e != "" {
			result = append(result, e)
		}
	}
	return result
}

// parseSRTTime parses HH, MM, SS, mmm into a time.Duration.
func parseSRTTime(h, m, s, ms string) time.Duration {
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	ss, _ := strconv.Atoi(s)
	mss, _ := strconv.Atoi(ms)
	return time.Duration(hh)*time.Hour +
		time.Duration(mm)*time.Minute +
		time.Duration(ss)*time.Second +
		time.Duration(mss)*time.Millisecond
}