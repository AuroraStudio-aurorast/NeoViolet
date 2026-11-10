package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

type settingsLyricsMsg struct {
	generation int
	revision   uint64
	data       *lyrics.Data
	err        error
}

func (m *Model) loadSettingsLyrics() tea.Cmd {
	m.invalidateLyricRequests()
	revision, generation := m.settingsLyricsRevision, m.activeGeneration
	path := m.Audio.Player.Path()
	order := slices.Clone(m.Config.Lyrics.FormatPriority)
	return func() tea.Msg {
		data, err := lyrics.FindAndParse(path, order)
		return settingsLyricsMsg{generation: generation, revision: revision, data: data, err: err}
	}
}
func (m *Model) applySettingsLyrics(msg settingsLyricsMsg) tea.Cmd {
	if m.exiting || msg.generation != m.activeGeneration || msg.revision != m.settingsLyricsRevision || !m.Config.Lyrics.Enabled {
		return nil
	}
	if msg.err != nil {
		m.Info.Visible = false
		m.Error.Set(fmt.Sprintf("Lyrics: %v", msg.err), m.Config.Error.Duration)
		return nil
	}
	if msg.data != nil {
		m.Audio.Lyrics = msg.data
		m.Audio.ShowLyrics = true
		m.Audio.LyricIndex = -1
		m.Audio.UpdateLyricIndex()
		return nil
	}
	if m.Audio.Player != nil {
		return m.maybeFetchLyrics(m.Audio.Player.Path())
	}
	return nil
}

// Old requests keep their own session cache. They can still populate the disk
// cache, but cannot leave pending markers or results in the new request epoch.
// Provider cooldown is separate and deliberately survives this invalidation.
func (m *Model) invalidateLyricRequests() {
	m.settingsLyricsRevision++
	m.LyricsFetching = false
	m.fetchCache = fetch.NewCache()
}
