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

// propertyToAgent maps an LYS line property onto the agent id of its duet-view
// side. The property is the Lyricify Syllable table of (background vocal, duet
// view):
//
//	0 = unset/unset   1 = unset/left   2 = unset/right
//	3 = no/unset      4 = no/left      5 = no/right
//	6 = yes/unset     7 = yes/left     8 = yes/right
//
// so the view side is property%3 and only it names a performer. The
// background-vocal flag is a role, not an identity: reading 6 and 8 as "channel"
// numbers invented the agents v3 and v4 for singers that do not exist. LYS
// carries no performer names at all, so these ids are everything the display has
// to work with.
//
// unset (0, 3, 6) means the line sits on no side. For a single-singer file that
// is the whole file, and for a duet it is the default performer - both read as
// v1, which is also how the TTML files for these songs assign their lines. A
// property outside 0..8 is malformed data whose meaning we cannot know, so it
// yields no agent rather than a guessed side.
func propertyToAgent(property int) string {
	if property < 0 || property > 8 {
		return ""
	}
	if property%3 == 2 { // duet view: right
		return "v2"
	}
	return "v1"
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

		// Parse the line property [N] (e.g. [0], [2], [6]).
		propertyStr := strings.TrimPrefix(parts[0], "[")

		body := parts[1]

		// LYS headers are "[property]"; "[ti:Title]" is not a property, and
		// applying it as metadata is harmless when the file has none.
		if key, val, hasField := lrcField(propertyStr); hasField {
			if applyHeaderField(lyrics, key, val) {
				continue
			}
		}
		property, _ := strconv.Atoi(propertyStr)

		// LYS bodies are shaped like QRC's: text(startMs,durationMs), so they
		// share qrcGroups. There is no duration in the header, so End comes
		// from the last word's end.
		scan := scanWordTimed(body, qrcGroups, 0)
		if strings.TrimSpace(scan.Text) == "" {
			continue
		}

		// delta is read at line-construction time (after applyHeaderField above
		// may have written lyrics.Offset), so [offset:] only shifts lines parsed
		// after it — the same timing rule as LRC/ESLRC.
		delta := time.Duration(lyrics.Offset) * time.Millisecond
		line := LyricLine{
			Time:  shiftTime(scan.Start, delta),
			Text:  scan.Text,
			Words: shiftWords(scan.Words, delta),
			Agent: propertyToAgent(property),
		}
		// scan.End == 0 is the unbounded sentinel (no duration crossed
		// lineStart), not a timestamp: shifting it by a non-zero delta would
		// make End == Time and the line would never activate.
		if scan.End > 0 {
			line.End = shiftTime(scan.End, delta)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid lys lines found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines

	return lyrics, nil
}
