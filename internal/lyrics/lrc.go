package lyrics

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	bracketRe = regexp.MustCompile(`\[([^\]]*)\]`)
	wordTagRe = regexp.MustCompile(`<(\d+:\d+(?:\.\d+)?)>([^<]*)`)
)

func FindLRC(audioPath string) string {
	ext := filepath.Ext(audioPath)
	lrcPath := audioPath[:len(audioPath)-len(ext)] + ".lrc"
	if _, err := os.Stat(lrcPath); err == nil {
		return lrcPath
	}
	return ""
}

func ParseFile(path string) (*LyricsData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open lyrics file: %w", err)
	}
	defer f.Close()
	return Parse(f, path)
}

func Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read lyrics: %w", err)
	}

	lyrics := &LyricsData{Path: sourcePath}
	var lines []LyricLine
	offset := 0
	lineNum := 0

	raw := string(data)
	for _, line := range strings.Split(raw, "\n") {
		lineNum++
		line = strings.TrimRight(line, "\r\n\t ")

		// Skip empty or whitespace-only lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Comment lines
		if strings.HasPrefix(line, ";") {
			continue
		}

		// Extract all [xxx] groups
		groups := bracketRe.FindAllStringSubmatch(line, -1)

		// Extract text outside brackets
		text := bracketRe.ReplaceAllString(line, "")

		// Check for word-level timestamps (<mm:ss.xx>) even without bracket tags
		wordMatches := wordTagRe.FindAllStringSubmatch(text, -1)
		hasWordTags := len(wordMatches) > 0

		if len(groups) == 0 && !hasWordTags {
			continue
		}

		// Process each group
		var timestamps []time.Duration
		isValidLine := false

		for _, g := range groups {
			content := g[1]
			switch {
			case isTimestamp(content):
				t, err := parseTimestamp(content)
				if err != nil {
					continue
				}
				timestamps = append(timestamps, t)
				isValidLine = true

			case isMetadata(content):
				key, val, _ := strings.Cut(content, ":")
				key = strings.ToLower(strings.TrimSpace(key))
				val = strings.TrimSpace(val)
				switch key {
				case "ti":
					lyrics.Title = val
				case "ar":
					lyrics.Artist = val
				case "al":
					lyrics.Album = val
				case "au":
					lyrics.Author = val
				case "by":
					lyrics.Creator = val
				case "offset":
					n, err := strconv.Atoi(val)
					if err == nil {
						offset = n
					}
				}

			default:
				// Unknown bracket content - try parsing as timestamp
				if t, err := parseTimestamp(content); err == nil {
					timestamps = append(timestamps, t)
					isValidLine = true
				}
			}
		}

		if !isValidLine && !hasWordTags {
			continue
		}

		// Process word-level timestamps in text
		var words []WordFragment
		wordMatches = wordTagRe.FindAllStringSubmatch(text, -1)
		if len(wordMatches) > 0 {
			var fullText strings.Builder
			lastEnd := 0
			for _, wm := range wordMatches {
				idx := strings.Index(text[lastEnd:], wm[0])
				if idx >= 0 {
					fullText.WriteString(text[lastEnd : lastEnd+idx])
				}
				t, err := parseTimestamp(wm[1])
				if err == nil {
					words = append(words, WordFragment{Time: t, Text: wm[2]})
				}
				fullText.WriteString(wm[2])
				lastEnd += idx + len(wm[0])
			}
			text = fullText.String()
		}

		// If no bracket timestamps but has word tags, use first word tag time
		if len(timestamps) == 0 && len(words) > 0 {
			timestamps = append(timestamps, words[0].Time)
		}

		// Apply offset and create lyric lines
		for _, ts := range timestamps {
			adjusted := ts + time.Duration(offset)*time.Millisecond
			if adjusted < 0 {
				adjusted = 0
			}
			var wc []WordFragment
			if len(words) > 0 {
				wc = make([]WordFragment, len(words))
				for i, w := range words {
					wc[i] = WordFragment{
						Time: w.Time + time.Duration(offset)*time.Millisecond,
						Text: w.Text,
					}
				}
			}
			lines = append(lines, LyricLine{
				Time:  adjusted,
				Text:  text,
				Words: wc,
			})
		}
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid lyric lines found")
	}

	// Merge lines with same timestamp (bilingual support)
	lines = mergeSameTimestamp(lines)

	// Sort by time
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	lyrics.Lines = lines
	return lyrics, nil
}

func isTimestamp(s string) bool {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return false
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return false
	}
	// Seconds part might have extra content after the number (e.g. "28.00:en")
	secStr := parts[1]
	extraParts := strings.SplitN(secStr, ":", 2)
	if _, err := strconv.ParseFloat(extraParts[0], 64); err != nil {
		return false
	}
	return true
}

func isMetadata(s string) bool {
	return strings.Contains(s, ":")
}

func parseTimestamp(s string) (time.Duration, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid timestamp: %s", s)
	}
	mm, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes: %s", parts[0])
	}
	// Seconds part may have extra content after the number (e.g. "28.00:en")
	secStr := strings.SplitN(parts[1], ":", 2)[0]
	secFloat, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %s", secStr)
	}
	ss := int(secFloat)
	ms := int(math.Round((secFloat - float64(ss)) * 1000))
	return time.Duration(mm)*time.Minute + time.Duration(ss)*time.Second + time.Duration(ms)*time.Millisecond, nil
}

func mergeSameTimestamp(lines []LyricLine) []LyricLine {
	if len(lines) == 0 {
		return lines
	}
	merged := make([]LyricLine, 0, len(lines))
	i := 0
	for i < len(lines) {
		texts := []string{lines[i].Text}
		j := i + 1
		for j < len(lines) && lines[j].Time == lines[i].Time {
			texts = append(texts, lines[j].Text)
			j++
		}
		merged = append(merged, LyricLine{
			Time:  lines[i].Time,
			Text:  strings.Join(texts, " | "),
			Words: lines[i].Words,
		})
		i = j
	}
	return merged
}

func (d *LyricsData) CurrentLine(elapsed time.Duration) int {
	if len(d.Lines) == 0 {
		return -1
	}
	if elapsed < d.Lines[0].Time {
		return -1
	}
	idx := sort.Search(len(d.Lines), func(i int) bool {
		return d.Lines[i].Time > elapsed
	})
	return idx - 1
}
