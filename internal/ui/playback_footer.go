package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func (m *Model) playbackModeDisplay(width int) string {
	mode := m.playbackMode()
	icon, short := m.Icons.Order, "ORD"
	switch mode {
	case config.PlaybackRepeatOne:
		icon, short = m.Icons.RepeatOne, "ONE"
	case config.PlaybackShuffle:
		icon, short = m.Icons.Shuffle, "SHF"
	}
	full := icon + " " + playbackModeLabel(mode)
	if lipgloss.Width(full) <= width {
		return full
	}
	compact := icon + " " + short
	if lipgloss.Width(compact) <= width {
		return compact
	}
	return icon
}

func renderPlaybackControls(m *Model, playIcon string, width int) string {
	white := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	transport := white.Render(m.Icons.Prev + " " + playIcon + " " + m.Icons.Next)
	volumeStyle, volumeModel := volumeAppearance(m)
	volumeLabel := volumeStyle.Render(m.Icons.Volume + " ")
	volume := volumeLabel + volumeModel.ViewAs(m.Audio.Volume)
	// Preserve the configured volume width when possible, shortening the mode
	// label first. An oversized custom bar must yield space to the mode glyph.
	modeRoom := width - lipgloss.Width(transport) - lipgloss.Width(volume) - 4
	mode := m.playbackModeDisplay(modeRoom)
	left := transport + "   " + footerTextStyle.Render(mode)
	volumeRoom := width - lipgloss.Width(left) - 1
	if lipgloss.Width(volume) > volumeRoom {
		volumeModel.SetWidth(max(1, volumeRoom-lipgloss.Width(volumeLabel)))
		volume = volumeLabel + volumeModel.ViewAs(m.Audio.Volume)
	}
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(volume))
	return left + strings.Repeat(" ", gap) + volume
}
