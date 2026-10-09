package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// The interface is black from edge to edge: every cell of a frame carries the
// colour, and the terminal's own default is set to the same colour for the cells
// no frame draws. One says nothing to the terminal, the other nothing to the
// frame, so it takes both.
var (
	// uiBackground is the background the whole interface is painted with.
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
func paintBackground(frame string) string {
	if frame == "" {
		return frame
	}

	var b strings.Builder
	b.Grow(len(frame) + len(frame)/8)
	b.WriteString(backgroundSGR)
	for i := 0; i < len(frame); {
		if frame[i] == '\n' {
			b.WriteByte('\n')
			b.WriteString(backgroundSGR)
			i++
			continue
		}
		if end, clears := sgrSequence(frame, i); end > i {
			b.WriteString(frame[i:end])
			if clears {
				b.WriteString(backgroundSGR)
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

// clearsBackground reports whether an SGR parameter list leaves the pen with no
// background: empty or all-zero parameters are a full reset, and 49 restores the
// terminal's default. A list that sets a background of its own -- the completion
// overlay's rows do -- is left alone.
func clearsBackground(params string) bool {
	for _, p := range strings.Split(params, ";") {
		if i := strings.IndexByte(p, ':'); i >= 0 {
			p = p[:i] // sub-parameters all belong to the one attribute
		}
		if p != "" && p != "0" && p != "49" {
			return false
		}
	}
	return true
}
