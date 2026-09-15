package lyrics

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func init() {
	RegisterParser("lys", &lysParser{})
}

type lysParser struct{}

// channelToAgent maps LYS channel numbers to agent IDs matching TTML conventions.
// Channel 0 is lead vocal (v1), 2 is duet (v2), 6 and 8 are backing vocals (v3, v4).
var channelToAgent = map[int]string{
	0: "v1",
	2: "v2",
	6: "v3",
	8: "v4",
}

func (p *lysParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".lys")
}

func (p *lysParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read lys: %w", err)
	}

	lyrics := &Data{Path: sourcePath}
	var lines []LyricLine

	for _, rawLine := range strings.Split(string(data), "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		parts := strings.SplitN(rawLine, "]", 2)
		if len(parts) < 2 {
			continue
		}

		// Parse channel prefix [N] (e.g. [0], [2], [6]).
		channelStr := strings.TrimPrefix(parts[0], "[")

		body := parts[1]

		// LYS headers are "[channel]"; "[ti:Title]" is not a channel, and
		// applying it as metadata is harmless when the file has none.
		if key, val, hasField := lrcField(channelStr); hasField {
			if applyHeaderField(lyrics, key, val) {
				continue
			}
		}
		channel, _ := strconv.Atoi(channelStr)

		// LYS bodies are shaped like QRC's: text(startMs,durationMs), so they
		// share qrcGroups. There is no duration in the header, so End comes
		// from the last word's end.
		scan := scanWordTimed(body, qrcGroups, 0)
		if strings.TrimSpace(scan.Text) == "" {
			continue
		}

		// delta is read at line-construction time (after applyHeaderField above
		// may have written lyrics.Offset), so [offset:] only shifts lines parsed
		// after it — the same timing rule as LRC/QRC/YRC.
		delta := time.Duration(lyrics.Offset) * time.Millisecond
		lines = append(lines, LyricLine{
			Time:  shiftTime(scan.Start, delta),
			End:   shiftTime(scan.End, delta),
			Text:  scan.Text,
			Words: shiftWords(scan.Words, delta),
			Agent: channelToAgent[channel],
		})
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid lys lines found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines

	return lyrics, nil
}
