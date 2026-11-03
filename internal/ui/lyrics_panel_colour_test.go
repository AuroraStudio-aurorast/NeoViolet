package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

func TestPanelColours_StayNeutralAcrossAccents(t *testing.T) {
	for _, hex := range []string{"", "#af87ff", "#60b5a6", "#df9b67", "#b86782", "#485b80"} {
		t.Run(hex, func(t *testing.T) {
			m := panelTranslationModel(t, []string{"The city settles into blue", "城市渐渐沉入蓝色"})
			if hex != "" {
				m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
			}
			if got := panelCurrentStyle().GetForeground(); got != lipgloss.Color("#ffffff") {
				t.Errorf("original colour = %v, want white", got)
			}
			if got := lyricTranslationStyle().GetForeground(); got != lipgloss.Color("#c8c8c8") {
				t.Errorf("translation colour = %v, want #c8c8c8", got)
			}
			ramp := newLyricRamp()
			for _, progress := range []float64{0.25, 0.5, 0.75, 1} {
				r, g, b, _ := ramp.styleFor(progress).GetForeground().RGBA()
				if r != g || g != b {
					t.Errorf("progress %.2f has coloured foreground %d/%d/%d", progress, r, g, b)
				}
			}
			if got := ramp.styleFor(0).GetForeground(); got != lipgloss.Color("245") {
				t.Errorf("unsung colour = %v, want existing ANSI 245", got)
			}
			wantCountdown := lipgloss.Color(accentOrDefault(m.Accent, "141"))
			if got := panelCountdownStyle(m).GetForeground(); got != wantCountdown {
				t.Errorf("countdown colour = %v, want %v", got, wantCountdown)
			}
			if !panelCurrentStyle().GetBold() || lyricTranslationStyle().GetBold() {
				t.Error("original must stay bold and translation must stay regular")
			}
		})
	}
}

func TestPanelColours_KeepLyricsLeftAligned(t *testing.T) {
	m := panelTranslationModel(t, []string{"The city settles into blue", "城市渐渐沉入蓝色"})
	plan := m.layoutPlan()
	for _, hex := range []string{"#af87ff", "#60b5a6"} {
		m.Accent = &accent.Accent{Main: mustColorful(t, hex)}
		frame := ansi.Strip(renderLyricsPanel(m, plan))
		for _, text := range []string{"The city settles into blue", "城市渐渐沉入蓝色"} {
			found := false
			for _, line := range strings.Split(frame, "\n") {
				if strings.Contains(line, text) {
					found = true
					if !strings.HasPrefix(line, "│ "+text) {
						t.Errorf("lyric moved from its left inset: %q", line)
					}
				}
			}
			if !found {
				t.Errorf("missing lyric %q", text)
			}
		}
	}
}
