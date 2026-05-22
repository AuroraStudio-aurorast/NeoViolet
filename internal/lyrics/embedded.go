package lyrics

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dhowden/tag"
)

func init() {
	RegisterParser("embedded", &embeddedParser{})
}

type embeddedParser struct{}

func (p *embeddedParser) FindSidecar(audioPath string) string {
	return audioPath
}

func (p *embeddedParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	rs, ok := r.(io.ReadSeeker)
	if !ok {
		return nil, nil
	}

	m, err := tag.ReadFrom(rs)
	if err != nil {
		return nil, nil
	}

	// CHECK 1 — USLT (MP3) / LYRICS (Vorbis)
	if lyricsText := m.Lyrics(); lyricsText != "" {
		var lrc lrcParser
		data, err := lrc.Parse(strings.NewReader(lyricsText), sourcePath)
		if err == nil && data != nil {
			return data, nil
		}
		if data := parsePlainText(lyricsText, sourcePath); data != nil {
			return data, nil
		}
	}

	// CHECK 1.5 — TXXX with USLT/lyrics description (ffmpeg workaround, MP3 only)
	if m.FileType() == tag.MP3 {
		raw := m.Raw()
		if lyricsText := extractTXXXLyrics(raw); lyricsText != "" {
			if data := parseAndReturn(lyricsText, sourcePath); data != nil {
				return data, nil
			}
		}
	}

	// CHECK 2 — SYLT (MP3 only)
	if m.FileType() == tag.MP3 {
		raw := m.Raw()
		for _, key := range []string{"SYLT", "SLT"} {
			if rawData, ok := raw[key]; ok {
				if b, ok := rawData.([]byte); ok {
					if data := parseSYLT(b); data != nil {
						return data, nil
					}
				}
			}
		}
	}

	// CHECK 3 — UNSYNCEDLYRICS (Vorbis only)
	if m.Format() == tag.VORBIS {
		raw := m.Raw()
		if rawVal, ok := raw["unsyncedlyrics"]; ok {
			if s, ok := rawVal.(string); ok && s != "" {
				if data := parsePlainText(s, sourcePath); data != nil {
					return data, nil
				}
			}
		}
	}

	return nil, nil
}

// parseAndReturn tries LRC then plain text on lyrics content.
func parseAndReturn(lyricsText, sourcePath string) *LyricsData {
	var lrc lrcParser
	data, err := lrc.Parse(strings.NewReader(lyricsText), sourcePath)
	if err == nil && data != nil {
		return data
	}
	return parsePlainText(lyricsText, sourcePath)
}

// extractTXXXLyrics checks Raw() for TXXX frames with lyrics descriptions.
// ffmpeg writes -metadata lyrics as TXXX with description "USLT".
func extractTXXXLyrics(raw map[string]interface{}) string {
	// Check TXXX and any TXXX_N duplicates.
	keys := []string{"TXXX"}
	for i := 0; ; i++ {
		k := fmt.Sprintf("TXXX_%d", i)
		if _, ok := raw[k]; !ok {
			break
		}
		keys = append(keys, k)
	}
	for _, key := range keys {
		v, ok := raw[key]
		if !ok {
			continue
		}
		c, ok := v.(*tag.Comm)
		if !ok || c == nil {
			continue
		}
		desc := strings.ToLower(strings.TrimSpace(c.Description))
		if desc == "uslt" || desc == "lyrics" || desc == "unsyncedlyrics" {
			return c.Text
		}
	}
	return ""
}

// parsePlainText creates LyricsData from plain text with sequential 5s timestamps.
func parsePlainText(text string, sourcePath string) *LyricsData {
	raw := strings.Split(strings.TrimSpace(text), "\n")
	var lines []LyricLine
	for i, line := range raw {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, LyricLine{
			Time: time.Duration(i) * 5 * time.Second,
			Text: line,
		})
	}
	if len(lines) == 0 {
		return nil
	}
	return &LyricsData{
		Path:  sourcePath,
		Lines: lines,
	}
}

// parseSYLT parses an ID3v2 SYLT (Synchronized lyrics/text) frame body.
// Only supports content type $01 (lyrics) and timestamp format $01 (absolute ms).
func parseSYLT(data []byte) *LyricsData {
	if len(data) < 6 {
		return nil
	}

	encoding := data[0]
	contentType := data[4]
	timeFormat := data[5]

	// Only support lyrics content type.
	if contentType != 1 {
		return nil
	}

	// Only support absolute milliseconds.
	if timeFormat != 1 {
		return nil
	}

	pos := 6

	// Skip content descriptor (null-terminated).
	pos = skipNullTerminated(data, pos, encoding)
	if pos < 0 {
		return nil
	}

	// Read pairs of (null-terminated text, 4-byte sync).
	var lines []LyricLine
	for pos < len(data) {
		text, next := readNullTerminated(data, pos, encoding)
		if next < 0 || text == "" {
			break
		}
		pos = next

		if pos+4 > len(data) {
			break
		}
		syncMs := binary.BigEndian.Uint32(data[pos:])
		pos += 4

		lines = append(lines, LyricLine{
			Time: time.Duration(syncMs) * time.Millisecond,
			Text: text,
		})
	}

	if len(lines) == 0 {
		return nil
	}
	return &LyricsData{Lines: lines}
}

// skipNullTerminated skips past a null-terminated string at pos.
// Returns the position after the null terminator, or -1 on error.
func skipNullTerminated(data []byte, pos int, encoding byte) int {
	if encoding == 0 {
		// ISO-8859-1: each char is 1 byte, terminated by $00.
		end := bytes.IndexByte(data[pos:], 0)
		if end < 0 {
			return -1
		}
		return pos + end + 1
	}
	if encoding == 1 {
		// UTF-16 with BOM: terminated by $00 $00.
		off := bomAdjust(data[pos:])
		for i := pos + off; i+1 < len(data); i += 2 {
			if data[i] == 0 && data[i+1] == 0 {
				return i + 2
			}
		}
		return -1
	}
	return -1
}

// readNullTerminated reads a null-terminated string at pos.
// Returns the string and the position after the null terminator, or -1 on error.
func readNullTerminated(data []byte, pos int, encoding byte) (string, int) {
	if encoding == 0 {
		end := bytes.IndexByte(data[pos:], 0)
		if end < 0 {
			return "", -1
		}
		return string(data[pos : pos+end]), pos + end + 1
	}
	if encoding == 1 {
		off := bomAdjust(data[pos:])
		for i := pos + off; i+1 < len(data); i += 2 {
			if data[i] == 0 && data[i+1] == 0 {
				text := decodeUTF16(data[pos : i+2])
				return text, i + 2
			}
		}
		return "", -1
	}
	return "", -1
}

// bomAdjust returns the byte offset to skip a UTF-16 BOM if present.
func bomAdjust(data []byte) int {
	if len(data) >= 2 {
		if data[0] == 0xFE && data[1] == 0xFF {
			return 2
		}
		if data[0] == 0xFF && data[1] == 0xFE {
			return 2
		}
	}
	return 0
}

// decodeUTF16 decodes UTF-16 encoded bytes (with optional BOM) to a Go string.
func decodeUTF16(data []byte) string {
	if len(data) < 2 {
		return string(data)
	}
	var endian binary.ByteOrder
	switch {
	case data[0] == 0xFE && data[1] == 0xFF:
		endian = binary.BigEndian
		data = data[2:]
	case data[0] == 0xFF && data[1] == 0xFE:
		endian = binary.LittleEndian
		data = data[2:]
	default:
		endian = binary.BigEndian
	}

	// Align to 2-byte boundary.
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	u16 := make([]uint16, len(data)/2)
	for i := range u16 {
		u16[i] = endian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(u16))
}