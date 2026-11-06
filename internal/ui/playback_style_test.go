package ui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// The visible glyphs, not the component's options, prove where the ramp sits.
func TestPlaybackProgress_TrackColorsStayFixedWhileFilling(t *testing.T) {
	for _, hex := range []string{"#af87ff", "#73adff", "#60d6be", "#f2b35b", "#ef83b1"} {
		cfg := config.DefaultConfig()
		cfg.ProgressBar.Fill = []string{"X", "."}
		a := &accent.Accent{Main: mustColorful(t, hex)}
		bar := newPlaybackProgress(&cfg, a)
		bar.SetWidth(40)
		_, short := warningForegrounds(t, bar.ViewAs(.25))
		_, long := warningForegrounds(t, bar.ViewAs(.75))
		for x := range 10 {
			if !sameRGB(short[x], long[x]) {
				t.Fatalf("%s: filled cell %d changed colour as progress advanced", hex, x)
			}
		}
		bar.SetWidth(40)
		empty := stripANSI(bar.ViewAs(0))
		full := stripANSI(bar.ViewAs(1))
		if empty != strings.Repeat(".", 40) || full != strings.Repeat("X", 40) {
			t.Fatal("custom fill or progress endpoints changed")
		}
		m := setupModel()
		m.Config = &cfg
		m.Accent = a
		m.rebuildProgressBar()
		m.Components.ProgressBar.SetWidth(40)
		if got, want := m.Components.ProgressBar.ViewAs(.75), bar.ViewAs(.75); got != want {
			t.Fatal("accent rebuild uses different progress styles from construction")
		}
	}
}

func TestPlaybackProgress_ExplicitScaledAndPercentageConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProgressBar.Fill = []string{"X", "."}
	cfg.ProgressBar.Scaled = true
	cfg.ProgressBar.ShowPercentage = true
	bar := newPlaybackProgress(&cfg, nil)
	bar.SetWidth(40)
	_, short := warningForegrounds(t, bar.ViewAs(.25))
	_, long := warningForegrounds(t, bar.ViewAs(.75))
	if sameRGB(short[3], long[3]) {
		t.Fatal("explicit scaled config was ignored")
	}
	if !strings.Contains(stripANSI(bar.ViewAs(.25)), "25%") {
		t.Fatal("percentage config was ignored")
	}
}

func TestVolumeAppearance_ThemeSolidFocusAndConfig(t *testing.T) {
	for _, hex := range []string{"#af87ff", "#73adff", "#60d6be", "#f2b35b", "#ef83b1"} {
		m := setupModel()
		m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
		m.Components.VolumeBar.Full = 'X'
		m.Components.VolumeBar.Empty = '.'
		m.Components.VolumeBar.SetWidth(16)
		original := m.Components.VolumeBar.ViewAs(.65)
		var focusFill color.Color
		for _, focus := range []Focus{FocusContent, FocusFooter} {
			m.UI.Focus = focus
			_, bar := volumeAppearance(m)
			frame := bar.ViewAs(.65)
			plain, inks := warningForegrounds(t, frame)
			first := strings.Index(plain, "X")
			if first < 0 {
				t.Fatal("no volume fill")
			}
			for i, c := range plain {
				if c == 'X' && !sameRGB(inks[i], inks[first]) {
					t.Fatal("volume fill is not solid")
				}
				if c == '.' && !sameRGB(inks[i], lipgloss.Color("#45414b")) {
					t.Fatal("empty slots lost their neutral colour")
				}
			}
			if focus == FocusContent {
				focusFill = inks[first]
			} else {
				old := inkLuminance(focusFill)
				now := inkLuminance(inks[first])
				if now <= old {
					t.Fatal("playback focus did not brighten the volume fill")
				}
			}
			for _, level := range []float64{0, .65, 1} {
				text := stripANSI(bar.ViewAs(level))
				if lipgloss.Width(text) != 16 {
					t.Fatal("volume control width changed")
				}
				if level == 0 && strings.Contains(text, "X") {
					t.Fatal("zero volume has filled cells")
				}
				if level == 1 && strings.Contains(text, ".") {
					t.Fatal("full volume has empty cells")
				}
			}
			if !strings.Contains(plain, "65%") {
				t.Fatal("volume percentage disappeared")
			}
		}
		if got := m.Components.VolumeBar.ViewAs(.65); got != original {
			t.Fatal("rendering mutated the stored volume component")
		}
		m.Components.VolumeBar.ShowPercentage = false
		_, bar := volumeAppearance(m)
		if strings.Contains(stripANSI(bar.ViewAs(.65)), "%") {
			t.Fatal("hidden percentage configuration ignored")
		}
		if lipgloss.Width(bar.ViewAs(.65)) != 16 {
			t.Fatal("volume width without percentage changed")
		}
	}
}

func inkLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	rgb := color.RGBA{R: uint8((r >> 8) & 255), G: uint8((g >> 8) & 255), B: uint8((b >> 8) & 255), A: 255}
	return testLuminance(rgb)
}

func TestVolumeAppearance_LeavesGradientBackgroundVisible(t *testing.T) {
	m := setupModel()
	_, bar := volumeAppearance(m)
	// ANSI controls may carry foregrounds, but the volume's background belongs to
	// the complete frame, which paints it after rendering.
	for _, row := range frameBackgrounds(t, bar.ViewAs(.65)) {
		for _, bg := range row {
			if bg != nil {
				t.Fatal("volume paints over the frame gradient")
			}
		}
	}
	if ansi.Strip(bar.ViewAs(.65)) == "" {
		t.Fatal("missing volume control")
	}
}
