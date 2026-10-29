package ui

import (
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

const animLoadingCycle = 1400 * time.Millisecond

// renderAnimLoading sweeps a narrow foreground highlight across a stationary
// label. The existing tick drives it; leaving room at both ends gives the
// highlight time to leave before the next pass, without reversing direction.
func renderAnimLoading(m *Model, plan layoutPlan) string {
	if plan.ContentInnerW <= 0 || plan.ContentInnerH <= 0 {
		return ""
	}
	const text = "Loading animation"
	rate := 30
	if m.Config != nil && m.Config.TickRate > 0 {
		rate = m.Config.TickRate
	}
	cycleTicks := max(1, int(animLoadingCycle*time.Duration(rate)/time.Second))
	phase := float64(m.loadingTick%cycleTicks) / float64(cycleTicks)
	center := phase*float64(len(text)+11) - 3

	base, _ := colorful.Hex("#aaa2b2")
	prompt, _ := colorful.MakeColor(newInteractionPalette(m.Accent).prompt)
	bright := prompt.BlendLab(colorful.Color{R: 1, G: 1, B: 1}, .45)
	var label strings.Builder
	for x, char := range text {
		weight := max(0, 1-math.Abs(float64(x)-center)/3)
		ink := paletteRGB(base.BlendLab(bright, weight).Clamped())
		label.WriteString(lipgloss.NewStyle().Foreground(ink).Render(string(char)))
	}
	line := ansi.Truncate(label.String(), plan.ContentInnerW, "")
	return lipgloss.Place(plan.ContentInnerW, plan.ContentInnerH, lipgloss.Center, lipgloss.Center, line)
}
