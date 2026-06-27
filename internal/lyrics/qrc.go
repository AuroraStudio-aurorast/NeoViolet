package lyrics

import (
	"fmt"
	"io"
	"regexp"
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
	return findSidecarWithExt(audioPath, ".qrc")
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
		words, text := parseQRCWordTimedLine(body)
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

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
}
