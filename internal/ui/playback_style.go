package ui

import (
	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func playbackColors(a *accent.Accent) (colorful.Color, colorful.Color) {
	main, _ := colorful.MakeColor(backgroundAccent(a))
	return accent.ProgressColors(main)
}

// Both initial construction and cover changes use the same palette and options.
func newPlaybackProgress(cfg *config.Config, a *accent.Accent) progress.Model {
	low, high := playbackColors(a)
	p := progress.New(
		progress.WithColors(lipgloss.Color(low.Hex()), lipgloss.Color(high.Hex())),
		progress.WithFillCharacters([]rune(cfg.ProgressBar.Fill[0])[0], []rune(cfg.ProgressBar.Fill[1])[0]),
		progress.WithScaled(cfg.ProgressBar.Scaled),
	)
	p.ShowPercentage = cfg.ProgressBar.ShowPercentage
	return p
}

// Style a copy so focus and cover changes never restart the volume animation.
func volumeAppearance(m *Model) (lipgloss.Style, progress.Model) {
	low, high := playbackColors(m.Accent)
	fill := low
	text := lipgloss.Color("#a59dac")
	if m.UI.Focus == FocusFooter {
		fill = high
		text = lipgloss.Color("#e5e1eb")
	}
	bar := m.Components.VolumeBar
	progress.WithColors(lipgloss.Color(fill.Hex()))(&bar)
	bar.EmptyColor = lipgloss.Color("#45414b")
	style := lipgloss.NewStyle().Foreground(text)
	bar.PercentageStyle = style
	return style, bar
}
