package ui

import (
	"image/color"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestFrameBackground_PanelKeepsNormalizedRamp(t *testing.T) {
	m := sized(t, panelSongs(t, 0, 40, 20), 120, 30)
	m.Accent = &accent.Accent{Main: mustColorful(t, "#46536b")}
	if !m.layoutPlan().PanelShown {
		t.Fatal("test needs Panel")
	}
	got, want := frameBackground(m), backgroundGradient(m.Accent, m.UI.Height)
	for y, c := range got {
		if c != want[y] {
			t.Errorf("Panel row %d darkened", y)
		}
	}
	assertFrameBackgrounds(t, m)
}

func TestFrameBackground_OneLineDarkensOnlyLyricNeighbourhood(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {68, 17}} {
		m := sized(t, lyricFooterModel(), size[0], size[1])
		m.panelMode = config.PanelModeOff
		m.Accent = &accent.Accent{Main: mustColorful(t, "#46536b")}
		rows, plain := frameBackground(m), backgroundGradient(m.Accent, m.UI.Height)
		lyricY := m.UI.Height - 3
		if testLuminance(rows[lyricY]) >= testLuminance(plain[lyricY]) {
			t.Fatal("One-line lyric was not darkened")
		}
		for y, c := range rows {
			if math.Abs(float64(y-lyricY)) > 1 && c != plain[y] {
				t.Errorf("row %d outside lyric neighbourhood changed", y)
			}
		}
		// Read the actual rendered cells, not just the palette generator.
		cells := frameBackgrounds(t, m.View().Content)
		main := color.RGBA{R: 70, G: 83, B: 107, A: 255}
		original := (testLuminance(main) + .05) / .05
		for x, c := range cells[lyricY] {
			if c == nil {
				t.Fatalf("lyric cell %d has no background", x)
			}
			r, g, b, _ := c.RGBA()
			bg := color.RGBA{R: uint8((r >> 8) & 255), G: uint8((g >> 8) & 255), B: uint8((b >> 8) & 255), A: 255}
			contrast := (testLuminance(main) + .05) / (testLuminance(bg) + .05)
			if contrast < original*.95 {
				t.Fatalf("lyric cell %d retains only %.3f original contrast", x, contrast/original)
			}
		}
		assertFrameBackgrounds(t, m)
	}
}

func TestFrameBackground_HiddenLyricsDoNotDarken(t *testing.T) {
	m := sized(t, lyricFooterModel(), 80, 24)
	m.panelMode = config.PanelModeOff
	m.Accent = &accent.Accent{Main: mustColorful(t, "#46536b")}
	m.Audio.ShowLyrics = false
	m.LyricsFetching = false
	rows, want := frameBackground(m), backgroundGradient(m.Accent, m.UI.Height)
	for y, c := range rows {
		if c != want[y] {
			t.Errorf("hidden lyric row %d changed", y)
		}
	}
}

func TestFrameBackground_AutoPanelFallbackAndModeSwitch(t *testing.T) {
	m := sized(t, panelSongs(t, 0, 40, 20), 120, 30)
	m.Accent = &accent.Accent{Main: mustColorful(t, "#265c5a")}
	for _, width := range []int{120, 80, 120} {
		m = sized(t, m, width, 30)
		rows, plain := frameBackground(m), backgroundGradient(m.Accent, m.UI.Height)
		dimmed := rows[27] != plain[27]
		if dimmed == m.layoutPlan().PanelShown {
			t.Errorf("width %d: Panel=%v darkened=%v", width, m.layoutPlan().PanelShown, dimmed)
		}
		assertFrameBackgrounds(t, m)
	}
	m.panelMode = config.PanelModeOff
	m.LyricsFetching = true
	m.Audio.ShowLyrics = false
	if rows, plain := frameBackground(m), backgroundGradient(m.Accent, m.UI.Height); rows[27] == plain[27] {
		t.Error("visible fetching row must get contrast protection")
	}
}
