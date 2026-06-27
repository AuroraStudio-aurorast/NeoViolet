package lyrics

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
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
	return findSidecarWithExt(audioPath, ".ttml", ".xml")
}

type ttmlTT struct {
	XMLName      xml.Name `xml:"tt"`
	Head         ttmlHead `xml:"head"`
	Body         ttmlBody `xml:"body"`
	TickRate     int      `xml:"http://www.w3.org/ns/ttml#parameter tickRate,attr"`
	FrameRate    int      `xml:"http://www.w3.org/ns/ttml#parameter frameRate,attr"`
	FrameRateMul int      `xml:"http://www.w3.org/ns/ttml#parameter frameRateMultiplier,attr"`
	SubFrameRate int      `xml:"http://www.w3.org/ns/ttml#parameter subFrameRate,attr"`
	XMLLang      string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	tickRate     int
}

type ttmlHead struct {
	Metadata ttmlMetadata `xml:"metadata"`
}

type ttmlMetadata struct {
	Agents []ttmlAgent `xml:"http://www.w3.org/ns/ttml#metadata agent"`
	AMLLs  []ttmlAMLL  `xml:"http://www.example.com/ns/amll meta"`
}

type ttmlAgent struct {
	ID   string `xml:"http://www.w3.org/XML/1998/namespace id,attr"`
	Type string `xml:"type,attr"`
}

type ttmlAMLL struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

type ttmlBody struct {
	Div ttmlDiv `xml:"div"`
}

type ttmlDiv struct {
	Paragraphs []ttmlParagraph `xml:"p"`
	XMLLang    string          `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
}

type ttmlParagraph struct {
	Begin string     `xml:"begin,attr"`
	End   string     `xml:"end,attr"`
	Agent string     `xml:"http://www.w3.org/ns/ttml#metadata agent,attr"`
	Spans []ttmlSpan `xml:"span"`
	Text  string     `xml:",chardata"`
}

type ttmlSpan struct {
	Begin   string `xml:"begin,attr"`
	Text    string `xml:",chardata"`
	Role    string `xml:"http://www.w3.org/ns/ttml#metadata role,attr"`
	XMLLang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
}

func (p *ttmlParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	decoder := xml.NewDecoder(r)
	decoder.Strict = true

	var tt ttmlTT
	if err := decoder.Decode(&tt); err != nil {
		return nil, fmt.Errorf("parse ttml: %w", err)
	}

	return tt.toLyricsData(sourcePath)
}

func (tt *ttmlTT) toLyricsData(sourcePath string) (*LyricsData, error) {
	tt.resolveRates()

	lang := tt.Body.Div.XMLLang
	if lang == "" {
		lang = tt.XMLLang
	}
	needsSpace := !isCJKLang(lang)

	// Auto-detect: if no xml:lang, check first rune of first paragraph text
	if lang == "" {
		needsSpace = !tt.isCJKContent()
	}

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

	// Parse head metadata: agents and AMLL properties
	agents := make(map[string]string)
	props := make(map[string]string)
	var artists []string

	for _, m := range tt.Head.Metadata.AMLLs {
		if m.Key == "artists" {
			artists = append(artists, m.Value)
		}
		props[m.Key] = m.Value
	}

	// Map agents to artists in order
	for i, a := range tt.Head.Metadata.Agents {
		if i < len(artists) {
			agents[a.ID] = artists[i]
		} else {
			agents[a.ID] = strings.ToUpper(a.ID)
		}
	}

	// Build LyricsData metadata from properties
	lyrics := &LyricsData{
		Path:       sourcePath,
		Agents:     agents,
		Properties: props,
	}
	if title, ok := props["musicName"]; ok {
		lyrics.Title = title
	}
	if len(artists) > 0 {
		lyrics.Artist = artists[0]
	}
	if album, ok := props["album"]; ok {
		lyrics.Album = album
	}

	lines := make([]LyricLine, 0, len(tt.Body.Div.Paragraphs))

	for _, para := range tt.Body.Div.Paragraphs {
		paraBegin, err := parseTTMLTime(para.Begin, tickRate, frameRate, frameRateMul, subFrameRate)
		if err != nil {
			continue
		}

		paraEnd, _ := parseTTMLTime(para.End, tickRate, frameRate, frameRateMul, subFrameRate)

		var words []WordFragment
		fullText := strings.Builder{}

		for _, span := range para.Spans {
			if span.Role == "x-translation" {
				continue
			}
			spanBegin, spanErr := parseTTMLTime(span.Begin, tickRate, frameRate, frameRateMul, subFrameRate)
			if spanErr == nil {
				words = append(words, WordFragment{Time: spanBegin, Text: span.Text})
			}
			if needsSpace && fullText.Len() > 0 {
				fullText.WriteByte(' ')
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
			End:   paraEnd,
			Text:  text,
			Words: words,
			Agent: para.Agent,
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid ttml paragraphs found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
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

	if isOffsetTime(s) {
		return parseOffsetTime(s)
	}

	if !strings.Contains(s, ":") {
		secFloat, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid bare seconds %q: %w", s, err)
		}
		ss := int(secFloat)
		ms := int(math.Round((secFloat - float64(ss)) * 1000))
		return time.Duration(ss)*time.Second + time.Duration(ms)*time.Millisecond, nil
	}

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

func isCJKLang(lang string) bool {
	lang = strings.ToLower(lang)
	for _, prefix := range []string{"zh", "ja", "ko"} {
		if strings.HasPrefix(lang, prefix) {
			return true
		}
	}
	return false
}

func (tt *ttmlTT) isCJKContent() bool {
	for _, para := range tt.Body.Div.Paragraphs {
		for _, span := range para.Spans {
			if span.Role == "x-translation" {
				continue
			}
			for _, r := range span.Text {
				if isCJKRune(r) {
					return true
				}
				return false
			}
		}
		if strings.TrimSpace(para.Text) != "" {
			for _, r := range para.Text {
				if isCJKRune(r) {
					return true
				}
				return false
			}
		}
	}
	return false
}

func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}
