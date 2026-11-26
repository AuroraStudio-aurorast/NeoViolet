package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func playbackModeLabel(mode string) string {
	switch mode {
	case config.PlaybackRepeatOne:
		return "Repeat one"
	case config.PlaybackShuffle:
		return "Shuffle"
	default:
		return "Order"
	}
}

func (m *Model) playbackMode() string {
	switch m.Config.PlaybackMode {
	case config.PlaybackRepeatOne, config.PlaybackShuffle:
		return m.Config.PlaybackMode
	default:
		return config.PlaybackOrder
	}
}

func (m *Model) setPlaybackMode(mode string) {
	changed := m.playbackMode() != mode
	pendingConflict := m.configSave != nil && m.configSave.snapshot.PlaybackMode != mode
	if changed || pendingConflict {
		m.Config.PlaybackMode = mode
		m.modeRevision++
		if changed {
			m.resetShuffle()
		}
		m.volumeSavePending = true
		m.volumeSaveDue = time.Now().Add(volumeSaveDelay)
	}
	if p := m.pages; p != nil {
		p.baseline.PlaybackMode = mode
		if !p.changed["playback_mode"] {
			p.draft.PlaybackMode = mode
		}
		if p.draft.PlaybackMode == mode {
			delete(p.changed, "playback_mode")
		}
		p.dirty = len(p.changed) > 0
		for i := range p.settings {
			if p.settings[i].key == "playback_mode" {
				p.settings[i].value = p.draft.PlaybackMode
				p.settings[i].savedValue = mode
			}
		}
	}
	m.Info.Set("Playback mode: "+playbackModeLabel(mode), 90)
}

func (m *Model) resetShuffle() {
	m.session.shuffle = shuffleState{}
	m.session.shuffleFailed = nil
	current := m.session.current
	if current == 0 {
		current = m.session.detached
	}
	m.session.shuffle.init(current)
	if m.queueIntent != nil {
		m.queueIntent.shuffle = nil
	}
}

func (m *Model) cyclePlaybackMode() {
	mode := config.PlaybackOrder
	switch m.playbackMode() {
	case config.PlaybackOrder:
		mode = config.PlaybackRepeatOne
	case config.PlaybackRepeatOne:
		mode = config.PlaybackShuffle
	}
	m.setPlaybackMode(mode)
}

func runMode(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) == 1 {
		m.Info.Set("Playback mode: "+playbackModeLabel(m.playbackMode()), 90)
		return m, nil
	}
	if len(inv.Parts) == 2 {
		switch inv.Parts[1] {
		case config.PlaybackOrder, config.PlaybackRepeatOne, config.PlaybackShuffle:
			m.setPlaybackMode(inv.Parts[1])
			return m, nil
		}
	}
	m.Error.Set("Usage: mode [order|repeat_one|shuffle]", m.Config.Error.Duration)
	return m, nil
}

func (m *Model) repeatQueueEntry() tea.Cmd {
	index := queueIndex(m.session.entries, m.session.current)
	if index < 0 {
		return m.stepQueue(1)
	}
	if player, ok := m.Audio.Player.(interface{ RewindMayBlock() bool }); ok && player.RewindMayBlock() {
		return m.rewindInBackground()
	}
	m.Audio.Player.Pause()
	if err := m.Audio.SeekPlayer(0); err != nil {
		return m.requestQueueLoad(m.session.entries[index], nil, false)
	}
	if err := m.Audio.SetPlaying(true); err != nil {
		m.Error.Set(fmt.Sprintf("Repeat failed: %v", err), 120)
	}
	return nil
}
