package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// The terminal default stays black for cells cleared by the terminal itself.
// Each complete frame also paints every cell with its screen row's background.
var (
	// uiBackground is the terminal default, including cells cleared by resize.
	uiBackground = lipgloss.Color("#000000")

	// uiForeground is what text drawn without a colour of its own comes out as.
	// Without it a light terminal would draw the footer's song line dark on
	// black, i.e. invisibly.
	uiForeground = lipgloss.Color("#FFFFFF")

	// backgroundSGR is uiBackground as a sequence, derived from the colour so the
	// two cannot drift apart. Truecolour on purpose: the renderer downgrades it
	// to what the terminal negotiated, where lipgloss's ambient profile would
	// drop it entirely under NO_COLOR.
	backgroundSGR = sgrBackground(uiBackground)
)

// sgrBackground returns the truecolour "set background" sequence for c.
func sgrBackground(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

// paintBackground re-asserts the background wherever the pen loses it: at the
// start of the frame, after every line break, and after every sequence that turns
// the background off. A span ends with a full reset, which would otherwise leave
// the rest of that line -- and any line with no escapes at all, of which there
// are some -- showing the terminal's default instead of the interface's.
//
// The sequences it walks past are zero-width, so the frame keeps its text, its
// size and its layout.
func paintBackground(frame string, rows []color.RGBA) string {
	if frame == "" {
		return frame
	}

	row := 0
	rowSGR := func() string {
		if row < len(rows) {
			return sgrBackground(rows[row])
		}
		return backgroundSGR
	}
	background := rowSGR()
	var b strings.Builder
	b.Grow(len(frame) + len(frame)/8)
	b.WriteString(background)
	for i := 0; i < len(frame); {
		if frame[i] == '\n' {
			b.WriteByte('\n')
			row++
			background = rowSGR()
			b.WriteString(background)
			i++
			continue
		}
		if end, clears := sgrSequence(frame, i); end > i {
			b.WriteString(frame[i:end])
			if clears {
				b.WriteString(background)
			}
			i = end
			continue
		}
		b.WriteByte(frame[i])
		i++
	}
	return b.String()
}

// sgrSequence reports whether a select-graphic-rendition sequence starts at i,
// and whether it leaves the pen without a background. end is the byte after the
// sequence, or i when there is none.
func sgrSequence(frame string, i int) (end int, clears bool) {
	if i+2 > len(frame) || frame[i] != 0x1b || frame[i+1] != '[' {
		return i, false
	}
	j := i + 2
	for j < len(frame) && frame[j] >= 0x20 && frame[j] <= 0x3f {
		j++
	}
	if j >= len(frame) || frame[j] != 'm' {
		return i, false
	}
	return j + 1, clearsBackground(frame[i+2 : j])
}

// clearsBackground follows the final background effect of a mixed SGR list.
// Colour components belong to 38/48/58, even when their values are 0 or 49.
func clearsBackground(params string) bool {
	fields := strings.Split(params, ";")
	clears := false
	for i := 0; i < len(fields); i++ {
		field, _, sub := strings.Cut(fields[i], ":")
		n, err := strconv.Atoi(field)
		if field == "" {
			n, err = 0, nil
		}
		if err != nil {
			continue
		}
		switch {
		case n == 0, n == 49:
			clears = true
		case n == 48, n >= 40 && n <= 47, n >= 100 && n <= 107:
			clears = false
		}
		if sub || (n != 38 && n != 48 && n != 58) || i+1 >= len(fields) {
			continue
		}
		switch fields[i+1] {
		case "5":
			i += min(2, len(fields)-i-1)
		case "2":
			i += min(4, len(fields)-i-1)
		}
	}
	return clears
}
