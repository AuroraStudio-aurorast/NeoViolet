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

var qrcWordRe = regexp.MustCompile(`([^(]+)\((\d+),(\d+)\)`)

func init() {
	RegisterParser("qrc", &qrcParser{})
}

type qrcParser struct{}

func (p *qrcParser) FindSidecar(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	path := base + ".qrc"
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (p *qrcParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read qrc: %w", err)
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
		matches := qrcWordRe.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder

		for _, m := range matches {
			wordText := m[1]
			wordStart, _ := strconv.Atoi(m[2])

			words = append(words, WordFragment{
				Time: time.Duration(wordStart) * time.Millisecond,
				Text: wordText,
			})
			fullText.WriteString(wordText)
		}

		text := fullText.String()
		if strings.TrimSpace(text) == "" {
			continue
		}

		lines = append(lines, LyricLine{
			Time:  time.Duration(lineStart) * time.Millisecond,
			Text:  text,
			Words: words,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid qrc lines found")
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	lyrics.Lines = lines
	return lyrics, nil
}
