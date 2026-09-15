package lyrics

import (
	"fmt"
	"io"
	"math"
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

func init() {
	RegisterParser("lrc", &lrcParser{})
}

type lrcParser struct{}

func (p *lrcParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".lrc")
}

func (p *lrcParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read lyrics: %w", err)
	}

	lyrics := &Data{Path: sourcePath}
	var lines []LyricLine

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r\n\t ")

		if strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(line, ";") {
			continue
		}

		groups := bracketRe.FindAllStringSubmatch(line, -1)
		text := bracketRe.ReplaceAllString(line, "")

		wordMatches := wordTagRe.FindAllStringSubmatch(text, -1)
		hasWordTags := len(wordMatches) > 0

		if len(groups) == 0 && !hasWordTags {
			continue
		}

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
				key, val, hasField := lrcField(content)
				if !hasField {
					continue
				}
				applyHeaderField(lyrics, key, val)

			default:
				if t, err := parseTimestamp(content); err == nil {
					timestamps = append(timestamps, t)
					isValidLine = true
				}
			}
		}

		if !isValidLine && !hasWordTags {
			continue
		}

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

		if len(timestamps) == 0 && len(words) > 0 {
			timestamps = append(timestamps, words[0].Time)
		}

		delta := time.Duration(lyrics.Offset) * time.Millisecond
		for _, ts := range timestamps {
			adjusted := shiftTime(ts, delta)
			var wc []WordFragment
			if len(words) > 0 {
				wc = shiftWords(words, delta)
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

	lines = mergeSameTimestamp(lines)

	sortLyricLines(lines)

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
			Parts: partsOrNil(texts),
		})
		i = j
	}
	return merged
}

// partsOrNil returns texts as display parts, or nil when the event has only one
// text: a single text stays a plain line, so Parts != nil always means "this
// event really has several display rows".
func partsOrNil(texts []string) []string {
	if len(texts) < 2 {
		return nil
	}
	return texts
}

// CurrentLine returns the index of the line active at elapsed, or -1 if none.
func (d *Data) CurrentLine(elapsed time.Duration) int {
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

// ParseLRC parses standard LRC text from r. It is the exported entry point for
// code outside this package (e.g. online lyric fetch) that receives raw LRC.
func ParseLRC(r io.Reader) (*Data, error) {
	return (&lrcParser{}).Parse(r, "")
}
