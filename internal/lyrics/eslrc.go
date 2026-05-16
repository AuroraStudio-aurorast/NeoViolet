package lyrics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func init() {
	RegisterParser("eslrc", &eslrcParser{})
}

type eslrcParser struct{}

func (p *eslrcParser) FindSidecar(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	path := base + ".eslrc"
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (p *eslrcParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read eslrc: %w", err)
	}

	lyrics := &LyricsData{Path: sourcePath}
	var lines []LyricLine

	for _, rawLine := range strings.Split(string(data), "\n") {
		rawLine = strings.TrimRight(rawLine, "\r\n\t ")
		if strings.TrimSpace(rawLine) == "" {
			continue
		}

		groups := bracketRe.FindAllStringSubmatchIndex(rawLine, -1)
		if len(groups) == 0 {
			continue
		}

		firstContent := rawLine[groups[0][2]:groups[0][3]]
		lineStart, err := parseTimestamp(firstContent)
		if err != nil {
			continue
		}

		var words []WordFragment
		var fullText strings.Builder
		hasWord := false

		for i := 0; i < len(groups); i++ {
			bracketStart := groups[i][0]
			bracketEnd := groups[i][1]

			prevEnd := 0
			if i > 0 {
				prevEnd = groups[i-1][1]
			}
			textBetween := rawLine[prevEnd:bracketStart]

			if i == 0 {
				continue
			}

			bracketContent := rawLine[groups[i][2]:groups[i][3]]

			wordDuration, wordErr := parseTimestamp(bracketContent)
			if wordErr == nil && wordDuration > 0 {
				words = append(words, WordFragment{
					Time: wordDuration,
					Text: textBetween,
				})
				hasWord = true
			}

			fullText.WriteString(textBetween)
			_ = bracketEnd
		}

		lastEnd := groups[len(groups)-1][1]
		if lastEnd < len(rawLine) {
			tail := rawLine[lastEnd:]
			fullText.WriteString(tail)
		}

		text := fullText.String()
		if !hasWord {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
		}

		lines = append(lines, LyricLine{
			Time:  lineStart,
			Text:  text,
			Words: words,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid eslrc lines found")
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	lyrics.Lines = lines
	return lyrics, nil
}
