package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
)

// animWarningPalette is foreground-only: both animation gates retain the
// existing frame background, including the GUI's default canvas.
type animWarningPalette struct {
	title   lipgloss.Style
	caution lipgloss.Style
	body    lipgloss.Style
	muted   lipgloss.Style
	risk    lipgloss.Style
	key     lipgloss.Style
}

func newAnimWarningPalette(a *accent.Accent) animWarningPalette {
	// The normalized ramp never exceeds this reference lightness. Checking
	// against its brightest endpoint also covers every warning row above it.
	key := interactionPrompt(backgroundAccent(a), backgroundReference)
	return animWarningPalette{
		title:   lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb4b4")).Bold(true),
		caution: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffd08a")).Bold(true),
		body:    lipgloss.NewStyle().Foreground(lipgloss.Color("#eeeef2")),
		muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("#b5b5be")),
		risk:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb4b4")),
		key:     lipgloss.NewStyle().Foreground(key).Bold(true),
	}
}

func (p animWarningPalette) keys(compact bool) string {
	between, end := " Cancel animation    ", " Play anyway"
	if compact {
		between = " Cancel   "
	}
	return p.key.Render("Esc") + p.body.Render(between) +
		p.key.Render("Enter") + p.body.Render(end)
}

func (p animWarningPalette) battery(percent, below int, compact bool) string {
	end := fmt.Sprintf("  /  warning below %d%%", below)
	if compact {
		end = fmt.Sprintf(", below %d%%; the animation may drain the battery faster.", below)
	}
	charge := lipgloss.NewStyle().Foreground(p.caution.GetForeground())
	return p.body.Render("Battery at ") + charge.Render(fmt.Sprintf("%d%%", percent)) + p.muted.Render(end)
}

// measurements colours only the three existing numeric fields, preserving
// their labels, spacing and formatting. Flags come from raw readings, so a
// rounded percentage cannot hide a value that exceeded the actual threshold.
func (p animWarningPalette) measurements(row string, exceeded [3]bool) string {
	var ranges [3][2]int
	end := len(row)
	for i := len(ranges) - 1; i >= 0; i-- {
		start := strings.LastIndexByte(row[:end], ' ') + 1
		ranges[i] = [2]int{start, end}
		end = start
		for end > 0 && row[end-1] == ' ' {
			end--
		}
	}
	var out strings.Builder
	start := 0
	for i, field := range ranges {
		out.WriteString(p.body.Render(row[start:field[0]]))
		style := p.body
		if exceeded[i] {
			style = p.risk
		}
		out.WriteString(style.Render(row[field[0]:field[1]]))
		start = field[1]
	}
	out.WriteString(p.body.Render(row[start:]))
	return out.String()
}
