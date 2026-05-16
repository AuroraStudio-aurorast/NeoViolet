package lyrics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterParser("lys", &lysParser{})
}

type lysParser struct{}

func (p *lysParser) FindSidecar(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	path := base + ".lys"
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (p *lysParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := io.ReadAll(r)
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

		body := parts[1]
		matches := qrcWordRe.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder
		lineStart := time.Duration(0)

		for _, m := range matches {
			wordText := m[1]
			wordStart, _ := strconv.Atoi(m[2])

			words = append(words, WordFragment{
				Time: time.Duration(wordStart) * time.Millisecond,
				Text: wordText,
			})
			fullText.WriteString(wordText)

			if lineStart == 0 && wordStart > 0 {
				lineStart = time.Duration(wordStart) * time.Millisecond
			}
		}

		text := fullText.String()
		if strings.TrimSpace(text) == "" {
			continue
		}

		lines = append(lines, LyricLine{
			Time:  lineStart,
			Text:  text,
			Words: words,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid lys lines found")
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	lyrics.Lines = lines
	return lyrics, nil
}
