package ui

import (
	"image/color"
	"math"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

type interactionPalette struct {
	main     color.RGBA
	command  color.RGBA
	selected color.RGBA
	prompt   color.RGBA
}

func newInteractionPalette(a *accent.Accent) interactionPalette {
	main := backgroundAccent(a)
	p := interactionPalette{
		main:     main,
		command:  interactionTone(main, .018, 1),
		selected: interactionTone(main, .045, 1),
	}
	p.prompt = interactionPrompt(main, p.command)
	return p
}

// interactionTone retains the normalized background's hue/chroma policy but
// uses the lightness of an opaque input surface. Lab values use a 1/100 scale.
func interactionTone(main color.RGBA, y, chromaScale float64) color.RGBA {
	light := 1.16*math.Cbrt(y) - .16
	if y <= 216.0/24389 {
		light = (24389.0 / 27) * y / 100
	}
	return accentTone(main, light, chromaScale)
}

func interactionPrompt(main, bg color.RGBA) color.RGBA {
	c, _ := colorful.MakeColor(main)
	light, a, b := c.Lab()
	if chroma := math.Hypot(a, b); chroma > .45 {
		a, b = a*.45/chroma, b*.45/chroma
	}
	for light = max(.68, light); light <= .95; light += .01 {
		fg := paletteRGB(colorful.Lab(light, a, b).Clamped())
		if interactionContrast(fg, bg) >= 4.5 {
			return fg
		}
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}

func interactionContrast(a, b color.RGBA) float64 {
	x, y := backgroundLuminance(a), backgroundLuminance(b)
	return (max(x, y) + .05) / (min(x, y) + .05)
}

type completionStyles struct{ text, description lipgloss.Style }

func (p interactionPalette) completion(selected bool) completionStyles {
	if selected {
		return completionStyles{
			text:        lipgloss.NewStyle().Foreground(p.prompt).Bold(true),
			description: lipgloss.NewStyle().Foreground(lipgloss.Color("#969696")),
		}
	}
	return completionStyles{
		text:        lipgloss.NewStyle().Foreground(lipgloss.Color("#d9d9d9")),
		description: lipgloss.NewStyle().Foreground(lipgloss.Color("#969696")),
	}
}

// applyInputStyles updates only when the palette changes. Reapplying SetStyles
// on every frame would restart the virtual cursor and prevent it from blinking.
func (p interactionPalette) applyInputStyles(ti *textinput.Model) {
	styles := ti.Styles()
	if styles.Focused.Text.GetBackground() == p.command && styles.Cursor.Color == p.prompt {
		return
	}
	text := lipgloss.NewStyle().Foreground(lipgloss.Color("#eeeef2")).Background(p.command)
	ghost := lipgloss.NewStyle().Foreground(lipgloss.Color("#a8a8b4")).Background(p.command)
	prompt := lipgloss.NewStyle().Foreground(p.prompt).Background(p.command)
	state := textinput.StyleState{Text: text, Suggestion: ghost, Prompt: prompt, Placeholder: ghost}
	styles.Focused, styles.Blurred = state, state
	styles.Cursor.Color = p.prompt
	ti.SetStyles(styles)
}

func interactionRow(row string, width int, bg color.RGBA) string {
	row = clampRowWidth(row, width)
	if pad := width - lipgloss.Width(row); pad > 0 {
		row += strings.Repeat(" ", pad)
	}
	return paintBackground(row, []color.RGBA{bg})
}

func renderInteractionError(m *Model) string {
	return renderStatus(m.Error, m.UI.Width, true)
}
