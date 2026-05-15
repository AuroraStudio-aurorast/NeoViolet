package lyrics

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ttmlNamespace = "http://www.w3.org/ns/ttml"

func init() {
	RegisterParser("ttml", &ttmlParser{})
}

type ttmlParser struct{}

func (p *ttmlParser) FindSidecar(audioPath string) string {
	ext := filepath.Ext(audioPath)
	base := audioPath[:len(audioPath)-len(ext)]
	for _, ext := range []string{".ttml", ".xml"} {
		path := base + ext
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

type ttmlTT struct {
	XMLName xml.Name `xml:"tt"`
	Body    ttmlBody `xml:"body"`
	TickRate    int `xml:"http://www.w3.org/ns/ttml#parameter tickRate,attr"`
	FrameRate   int `xml:"http://www.w3.org/ns/ttml#parameter frameRate,attr"`
	FrameRateMul int `xml:"http://www.w3.org/ns/ttml#parameter frameRateMultiplier,attr"`
	SubFrameRate int `xml:"http://www.w3.org/ns/ttml#parameter subFrameRate,attr"`
	tickRate     int
}

type ttmlBody struct {
	Div ttmlDiv `xml:"div"`
}

type ttmlDiv struct {
	Paragraphs []ttmlParagraph `xml:"p"`
}

type ttmlParagraph struct {
	Begin string       `xml:"begin,attr"`
	End   string       `xml:"end,attr"`
	Spans []ttmlSpan   `xml:"span"`
	Text  string       `xml:",chardata"`
}

type ttmlSpan struct {
	Begin string `xml:"begin,attr"`
	Text  string `xml:",chardata"`
}

func (p *ttmlParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	decoder := xml.NewDecoder(r)
	decoder.Strict = false

	var tt ttmlTT
	if err := decoder.Decode(&tt); err != nil {
		return nil, fmt.Errorf("parse ttml: %w", err)
	}

	return tt.toLyricsData(sourcePath)
}

func (tt *ttmlTT) toLyricsData(sourcePath string) (*LyricsData, error) {
	tt.resolveRates()

	tickRate := tt.tickRate
	if tickRate <= 0 {
		tickRate = 1
	}
	frameRate := tt.FrameRate
	if frameRate <= 0 {
		frameRate = 30
	}
	frameRateMul := tt.FrameRateMul
	if frameRateMul <= 0 {
		frameRateMul = 1
	}
	subFrameRate := tt.SubFrameRate
	if subFrameRate <= 0 {
		subFrameRate = 1
	}

	lines := make([]LyricLine, 0, len(tt.Body.Div.Paragraphs))

	for _, para := range tt.Body.Div.Paragraphs {
		paraBegin, err := parseTTMLTime(para.Begin, tickRate, frameRate, frameRateMul, subFrameRate)
		if err != nil {
			continue
		}

		var words []WordFragment
		fullText := strings.Builder{}

		for _, span := range para.Spans {
			spanBegin, spanErr := parseTTMLTime(span.Begin, tickRate, frameRate, frameRateMul, subFrameRate)
			if spanErr == nil {
				words = append(words, WordFragment{Time: spanBegin, Text: span.Text})
			}
			fullText.WriteString(span.Text)
		}

		if fullText.Len() == 0 {
			fullText.WriteString(strings.TrimSpace(para.Text))
		}

		text := strings.TrimSpace(fullText.String())
		if strings.TrimSpace(text) == "" {
			continue
		}

		lines = append(lines, LyricLine{
			Time:  paraBegin,
			Text:  text,
			Words: words,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid ttml paragraphs found")
	}

	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].Time < lines[j].Time
	})

	return &LyricsData{
		Lines: lines,
		Path:  sourcePath,
	}, nil
}

func (tt *ttmlTT) resolveRates() {
	if tt.tickRate > 0 {
		return
	}
	if tt.TickRate > 0 {
		tt.tickRate = tt.TickRate
		return
	}
	if tt.FrameRate > 0 {
		subFrameRate := tt.SubFrameRate
		if subFrameRate <= 0 {
			subFrameRate = 1
		}
		tt.tickRate = tt.FrameRate * subFrameRate
		return
	}
	tt.tickRate = 1
}

func parseTTMLTime(s string, tickRate, frameRate, frameRateMul, subFrameRate int) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Offset-time: number + unit
	if isOffsetTime(s) {
		return parseOffsetTime(s)
	}

	// Clock-time: HH:MM:SS.mmm or HH:MM:SS:ff
	return parseClockTime(s, tickRate, frameRate, frameRateMul, subFrameRate)
}

func isOffsetTime(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[len(s)-1] < '0' || s[len(s)-1] > '9'
}

func parseOffsetTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	suffixes := []struct {
		suffix string
		unit   time.Duration
	}{
		{"ms", time.Millisecond},
		{"h", time.Hour},
		{"m", time.Minute},
		{"s", time.Second},
		{"f", time.Second},
		{"t", time.Millisecond},
	}

	for _, su := range suffixes {
		if strings.HasSuffix(s, su.suffix) {
			numStr := strings.TrimSuffix(s, su.suffix)
			numStr = strings.TrimSpace(numStr)
			numStr = strings.ReplaceAll(numStr, ",", ".")
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid offset time %q: %w", s, err)
			}
			return time.Duration(val * float64(su.unit)), nil
		}
	}

	return 0, fmt.Errorf("unrecognized offset time: %s", s)
}

func parseClockTime(s string, tickRate, frameRate, frameRateMul, subFrameRate int) (time.Duration, error) {
	s = strings.TrimSpace(s)

	var dur time.Duration

	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 3 {
		// Partial time (MM:SS.mmm)
		if len(parts) == 2 {
			return parseTwoFieldTime(parts[0], parts[1])
		}
		return 0, fmt.Errorf("invalid clock time: %s", s)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q: %w", s, err)
	}
	dur += time.Duration(hours) * time.Hour

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in %q: %w", s, err)
	}
	dur += time.Duration(minutes) * time.Minute

	// Seconds: could be "SS.mmm" or "SS:ff" (frames)
	secPart := parts[2]
	if strings.Contains(secPart, ":") {
		// Frames: SS:ff.subff
		return parseSecondsAndFrames(dur, secPart, frameRate, frameRateMul, subFrameRate)
	}

	return parseSecondsDecimal(dur, secPart)
}

func parseTwoFieldTime(field1, field2 string) (time.Duration, error) {
	// field1 could be "MM", field2 could be "SS.mmm" or "SS"
	minutes, err := strconv.Atoi(field1)
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in two-field time: %w", err)
	}
	dur := time.Duration(minutes) * time.Minute

	secFloat, err := strconv.ParseFloat(strings.ReplaceAll(field2, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in two-field time: %w", err)
	}

	ss := int(secFloat)
	ms := int(math.Round((secFloat - float64(ss)) * 1000))
	dur += time.Duration(ss)*time.Second + time.Duration(ms)*time.Millisecond

	return dur, nil
}

func parseSecondsAndFrames(dur time.Duration, secPart string, frameRate, frameRateMul, subFrameRate int) (time.Duration, error) {
	frameParts := strings.SplitN(secPart, ":", 2)
	seconds, err := strconv.Atoi(frameParts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in frames time: %w", err)
	}
	dur += time.Duration(seconds) * time.Second

	frameFields := frameParts[1]
	subFrames := 0

	if idx := strings.IndexAny(frameFields, ".,"); idx >= 0 {
		subFrameStr := frameFields[idx+1:]
		frameFields = frameFields[:idx]
		sf, err := strconv.Atoi(subFrameStr)
		if err == nil {
			subFrames = sf
		}
	}

	frames, err := strconv.Atoi(frameFields)
	if err != nil {
		return 0, fmt.Errorf("invalid frames in %q: %w", secPart, err)
	}

	if frameRate > 0 && frameRateMul > 0 {
		frameDur := float64(time.Second) * float64(frameRateMul) / float64(frameRate)
		dur += time.Duration(float64(frames)*frameDur + float64(subFrames)*frameDur/float64(subFrameRate))
	} else {
		dur += time.Duration(frames) * time.Second / 30
	}

	return dur, nil
}

func parseSecondsDecimal(dur time.Duration, secStr string) (time.Duration, error) {
	secFloat, err := strconv.ParseFloat(strings.ReplaceAll(secStr, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q: %w", secStr, err)
	}

	ss := int(secFloat)
	ms := int(math.Round((secFloat - float64(ss)) * 1000))
	dur += time.Duration(ss)*time.Second + time.Duration(ms)*time.Millisecond

	return dur, nil
}
