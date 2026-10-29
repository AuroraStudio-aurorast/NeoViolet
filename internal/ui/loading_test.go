package ui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

func TestStartupLoadingUsesDefaultPurpleSweepWithoutChangingScreen(t *testing.T) {
	m := setupModel()
	m.Loading = true
	m.Accent = nil
	m.UI.Width, m.UI.Height = 80, 24
	first := m.View()
	if got := ansi.Strip(first.Content); got != "Loading audio" {
		t.Fatalf("startup label = %q", got)
	}
	if first.AltScreen || first.BackgroundColor != nil || first.ForegroundColor != nil || strings.Contains(first.Content, "\n") {
		t.Fatal("startup loading changed the normal screen lifecycle")
	}
	assertWarningInk(t, first.Content, "Loading audio", color.RGBA{R: 175, G: 135, B: 255, A: 255})
	m.loadingTick = 10
	later := m.View()
	if later.Content == first.Content || ansi.Strip(later.Content) != ansi.Strip(first.Content) {
		t.Fatal("startup sweep did not animate stationary text")
	}
	for _, width := range []int{0, 8, 80} {
		m.UI.Width = width
		frame := m.View()
		if strings.Contains(frame.Content, "\n") || width > 0 && lipgloss.Width(frame.Content) > width {
			t.Errorf("startup loading overflows width %d", width)
		}
		if width == 8 && ansi.Strip(frame.Content) != "Loading…" {
			t.Errorf("narrow startup label = %q", ansi.Strip(frame.Content))
		}
	}
}

func TestTrackLoadingHelpStaysReadableThroughoutSweep(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 68, 17
	m.Loading, m.switchingTrack = true, true
	for _, hex := range []string{"#af87ff", "#46536b", "#55ccb4", "#f2b35b", "#ef83b1", "#000000", "#ffffff"} {
		m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
		bg := frameBackground(m)[m.UI.Height-1]
		var first string
		changed := false
		for tick := range 42 {
			m.loadingTick = tick
			row := renderHelp(m)
			if got := ansi.Strip(row); !strings.HasPrefix(got, " Loading audio") {
				t.Fatalf("help label = %q", got)
			}
			if lipgloss.Width(row) != 68 || lipgloss.Height(row) != 1 {
				t.Fatal("loading changed the help row geometry")
			}
			if tick == 0 {
				first = row
			}
			changed = changed || row != first
			plain, inks := warningForegrounds(t, row)
			for i, char := range plain {
				if char == ' ' {
					continue
				}
				if inks[i] == nil {
					t.Fatal("loading character has no foreground")
				}
				fg := color.RGBAModel.Convert(inks[i]).(color.RGBA)
				if contrast := interactionContrast(fg, bg); contrast < 7 {
					t.Fatalf("%s tick %d character %d contrast %.2f below 7", hex, tick, i, contrast)
				}
			}
			for _, background := range frameBackgrounds(t, row)[0] {
				if background != nil {
					t.Fatal("loading row hides the frame gradient")
				}
			}
		}
		if !changed {
			t.Fatal("help loading label did not animate")
		}
	}
	m.UI.Mode = ModeCommand
	if got := renderHelp(m); got != renderCommandLine(m) {
		t.Fatal("loading replaced the active command input")
	}
}
