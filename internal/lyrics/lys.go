package lyrics

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterParser("lys", &lysParser{})
}

type lysParser struct{}

// channelToAgent maps LYS channel numbers to agent IDs matching TTML conventions.
// Channel 0 is lead vocal (v1), 2 is duet (v2), 6 and 8 are backing vocals (v3, v4).
var channelToAgent = map[int]string{
	0: "v1",
	2: "v2",
	6: "v3",
	8: "v4",
}

func (p *lysParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".lys")
}

func (p *lysParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read lys: %w", err)
	}

	lyrics := &LyricsData{Path: sourcePath}
	var lines []LyricLine

	for _, rawLine := range strings.Split(string(data), "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		parts := strings.SplitN(rawLine, "]", 2)
		if len(parts) < 2 {
			continue
		}

		// Parse channel prefix [N] (e.g. [0], [2], [6])
		channelStr := strings.TrimPrefix(parts[0], "[")
		channel, _ := strconv.Atoi(channelStr)

		body := parts[1]
		matches := qrcWordRe.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder
		lineStart := time.Duration(0)
		lineEnd := time.Duration(0)

		for _, m := range matches {
			wordText := m[1]
			wordStart, _ := strconv.Atoi(m[2])
			wordDuration, _ := strconv.Atoi(m[3])

			startDur := time.Duration(wordStart) * time.Millisecond
			endDur := startDur + time.Duration(wordDuration)*time.Millisecond

			words = append(words, WordFragment{
				Time: startDur,
				Text: wordText,
			})
			fullText.WriteString(wordText)

			if lineStart == 0 && wordStart > 0 {
				lineStart = startDur
			}
			if endDur > lineEnd {
				lineEnd = endDur
			}
		}

		text := fullText.String()
		if strings.TrimSpace(text) == "" {
			continue
		}

		agent := channelToAgent[channel]

		lines = append(lines, LyricLine{
			Time:  lineStart,
			End:   lineEnd,
			Text:  text,
			Words: words,
			Agent: agent,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid lys lines found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines

	return lyrics, nil
}
