package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

const loadingCycle = 1400 * time.Millisecond

func renderFetchingLyrics(m *Model, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#b4b4b4"))
	text := ansi.Truncate(renderLoadingText(m, "Fetching lyrics", style), width, "")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text)
}

// renderAnimLoading sweeps a narrow foreground highlight across a stationary
// label. The existing tick drives it; leaving room at both ends gives the
// highlight time to leave before the next pass, without reversing direction.
func renderAnimLoading(m *Model, plan layoutPlan) string {
	if plan.ContentInnerW <= 0 || plan.ContentInnerH <= 0 {
		return ""
	}
	text := renderLoadingText(m, "Loading animation", lipgloss.NewStyle().Foreground(lipgloss.Color("#a4a4a4")))
	line := ansi.Truncate(text, plan.ContentInnerW, "")
	return lipgloss.Place(plan.ContentInnerW, plan.ContentInnerH, lipgloss.Center, lipgloss.Center, line)
}

// renderLoadingText shares the foreground sweep while callers own its placement
// and base text style. A bright base keeps status text readable between passes.
func renderLoadingText(m *Model, text string, style lipgloss.Style) string {
	rate := 30
	if m.Config != nil && m.Config.TickRate > 0 {
		rate = m.Config.TickRate
	}
	cycleTicks := max(1, int(loadingCycle*time.Duration(rate)/time.Second))
	phase := float64(m.loadingTick%cycleTicks) / float64(cycleTicks)
	center := phase*float64(len(text)+11) - 3

	base, _ := colorful.MakeColor(style.GetForeground())
	prompt, _ := colorful.MakeColor(newInteractionPalette(m.Accent).prompt)
	bright := prompt.BlendLab(colorful.Color{R: 1, G: 1, B: 1}, .45)
	var label strings.Builder
	for x, char := range text {
		weight := max(0, 1-math.Abs(float64(x)-center)/3)
		ink := paletteRGB(base.BlendLab(bright, weight).Clamped())
		label.WriteString(style.Foreground(ink).Render(string(char)))
	}
	return label.String()
}
