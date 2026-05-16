package lyrics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var yrcWordRe = regexp.MustCompile(`\((\d+),(\d+),(\d+)\)([^(]*)`)

func init() {
	RegisterParser("yrc", &yrcParser{})
}

type yrcParser struct{}

func (p *yrcParser) FindSidecar(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	path := base + ".yrc"
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (p *yrcParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read yrc: %w", err)
	}

	lyrics := &LyricsData{Path: sourcePath}
	var lines []LyricLine

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "]", 2)
		if len(parts) < 2 {
			continue
		}

		header := strings.TrimPrefix(parts[0], "[")
		headerParts := strings.SplitN(header, ",", 2)
		if len(headerParts) < 1 {
			continue
		}
		lineStart, err := strconv.Atoi(strings.TrimSpace(headerParts[0]))
		if err != nil {
			continue
		}

		body := parts[1]
		matches := yrcWordRe.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder

		for _, m := range matches {
			wordStart, _ := strconv.Atoi(m[1])
			_ = m[2] // duration, unused
			_ = m[3] // flag, unused
			wordText := m[4]

			words = append(words, WordFragment{
				Time: time.Duration(wordStart) * time.Millisecond,
				Text: wordText,
			})
			fullText.WriteString(wordText)
		}

		text := fullText.String()
		if text == "" {
			continue
		}

		lines = append(lines, LyricLine{
			Time:  time.Duration(lineStart) * time.Millisecond,
			Text:  text,
			Words: words,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid yrc lines found")
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	lyrics.Lines = lines
	return lyrics, nil
}
