package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
)

func effectsCanvas(width, height, selected int, settings audio.EffectsSettings) *pageCanvas {
	m := setupModel()
	m.pages.effect = selected
	m.pages.effects = settings
	c := newPageCanvas(width, height, m)
	m.pages.renderEffects(c, true, "Session · ready for next track")
	return c
}
func TestEffectsPolishSignedReadoutAndDirectionalPan(t *testing.T) {
	c := effectsCanvas(82, 23, 0, audio.EffectsSettings{Gains: [5]float64{2.5, -3, 0, 6, -12}, Pan: -.3})
	body := stripANSI(c.String())
	for _, want := range []string{"+2.5", "-3.0", "+6.0", "L 30%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing meaningful readout %q", want)
		}
	}
	if strings.Contains(body, "PCM →") {
		t.Fatal("internal pipeline competes with user controls")
	}
}
func TestEffectsPolishFocusStaysLocalToReadout(t *testing.T) {
	c := effectsCanvas(82, 23, 2, audio.EffectsSettings{})
	if len(c.selections) == 0 || len(c.selections) > 2 {
		t.Fatal("focus paints an entire rail rather than the active control")
	}
	if !strings.Contains(stripANSI(c.String()), "[0.0]") {
		t.Fatal("focused numeric readout has no monochrome marker")
	}
}
func TestEffectsPolishNarrowWindowPrioritizesSelectedPane(t *testing.T) {
	eq := stripANSI(effectsCanvas(66, 10, 0, audio.EffectsSettings{}).String())
	if !strings.Contains(eq, "Channels →") || strings.Contains(eq, "Swap L/R") {
		t.Fatal("minimum window compresses both panes")
	}
	channels := stripANSI(effectsCanvas(66, 10, 9, audio.EffectsSettings{Bypass: true}).String())
	for _, want := range []string{"← Equalizer", "Pan", "Mono", "Swap L/R", "Preamp", "Bypass"} {
		if !strings.Contains(channels, want) {
			t.Fatalf("missing narrow control %q", want)
		}
	}
}
func TestEffectsPolishEveryControlRemainsVisibleAcrossBreakpoints(t *testing.T) {
	labels := []string{"80 Hz", "250 Hz", "1 kHz", "4 kHz", "12 kHz", "Pan", "Mono", "Swap L/R", "Preamp", "Bypass"}
	for _, width := range []int{42, 66, 70, 72, 76, 82, 118} {
		for _, height := range []int{10, 12, 13, 14, 15, 16, 17, 18, 21, 23, 32} {
			for selected, label := range labels {
				settings := audio.EffectsSettings{Gains: [5]float64{-12, -6, 0, 6, 12}, Pan: 1, PreampDB: -12, Mono: true, Swap: true, Bypass: true}
				c := effectsCanvas(width, height, selected, settings)
				frame := c.String()
				rows := strings.Split(frame, "\n")
				if len(rows) != height || len(frame) > 64*1024 {
					t.Fatalf("%dx%d overflow", width, height)
				}
				for _, row := range rows {
					if lipgloss.Width(row) != width {
						t.Fatalf("%dx%d row width changed", width, height)
					}
				}
				if !strings.Contains(stripANSI(frame), label) || len(c.selections) == 0 {
					t.Fatalf("%dx%d hides selected %s", width, height, label)
				}
				row := c.selections[len(c.selections)-1].row
				if row >= height-2 {
					t.Fatalf("%dx%d control overlaps footer", width, height)
				}
			}
		}
	}
}

func TestEffectsPanCenterKnobOverwritesCenterTickForBothRailParities(t *testing.T) {
	for width := 10; width <= 80; width++ {
		m := setupModel()
		c := newPageCanvas(width+4, 1, m)
		m.pages.renderEffectRail(c, 0, 1, width, 0, -1, 1, true, "L", "R")
		plain := stripANSI(c.String())
		if strings.Count(plain, "●") != 1 || strings.Contains(plain, "┊") {
			t.Fatalf("width %d left the center tick exposed: %q", width, plain)
		}
	}
}

// Measure the visible rail at an unchanged band, independently of the layout formula.
func effectRailRows(t *testing.T, height int) (int, int, []string) {
	t.Helper()
	c := effectsCanvas(82, height, 0, audio.EffectsSettings{})
	rows := strings.Split(stripANSI(c.String()), "\n")
	first, last := -1, -1
	for y, row := range rows {
		if strings.ContainsRune(row, '│') && strings.Contains(row, "+12") {
			first = y
		}
		if strings.ContainsRune(row, '│') && strings.Contains(row, "-12") {
			last = y
		}
	}
	if first < 0 || last < first {
		t.Fatalf("height %d has no complete scale", height)
	}
	return first, last, rows
}
func TestEffectsEQHeightGrowsUntilRailLimit(t *testing.T) {
	// At these content heights, headers and footer leave 9/14/17 rail rows.
	for _, tc := range []struct{ height, want int }{{18, 9}, {23, 14}, {26, 17}, {32, 17}, {45, 17}, {60, 17}} {
		top, bottom, _ := effectRailRows(t, tc.height)
		if got := bottom - top + 1; got != tc.want {
			t.Fatalf("height %d rail has %d rows, want %d", tc.height, got, tc.want)
		}
	}
}
func TestEffectsCappedEQCentersWholeControlAboveFooter(t *testing.T) {
	for _, height := range []int{27, 28, 32, 33, 45, 60} {
		top, bottom, rows := effectRailRows(t, height)
		// The EQ area starts below the section rule; labels use two rows after the rail.
		above := top - 4
		below := (height - 4) - (bottom + 2)
		if difference := above - below; difference < -1 || difference > 1 {
			t.Fatalf("height %d rail group is not centered: above=%d below=%d", height, above, below)
		}
		if !strings.Contains(rows[bottom+1], "80 Hz") || !strings.Contains(rows[bottom+2], "[0.0]") {
			t.Fatal("labels did not move with rails")
		}
		if !strings.Contains(rows[height-2], "Select") || !strings.Contains(rows[height-1], "Session") {
			t.Fatal("centering overwrote footer")
		}
	}
}
func TestEffectsEQReflowsWhenWindowShrinksAfterCap(t *testing.T) {
	m := setupModel()
	m.pages.effect = 2
	for _, height := range []int{45, 23, 60, 18, 26} {
		c := newPageCanvas(82, height, m)
		m.pages.renderEffects(c, true, "Session")
		frame := c.String()
		if len(c.selections) != 1 || c.selections[0].row >= height-3 {
			t.Fatalf("height %d selection overlaps footer", height)
		}
		if !strings.Contains(stripANSI(frame), "[0.0]") {
			t.Fatal("resize lost selected gain")
		}
	}
}
