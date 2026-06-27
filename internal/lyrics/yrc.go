package lyrics

import (
	"fmt"
	"io"
	"regexp"
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
	return findSidecarWithExt(audioPath, ".yrc")
}

func (p *yrcParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := readAllWithLimit(r)
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
		words, text := parseYRCWordTimedLine(body)
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

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
}
