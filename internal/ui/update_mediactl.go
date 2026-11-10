package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

func handleMediaCtlReady(m *Model, msg MediaCtlReadyMsg) (tea.Model, tea.Cmd) {
	m.MediaCtl = msg.Controller
	logger.Debug("Media control layer initialized")
	return m, nil
}

func handleMediaCtlCmd(m *Model, msg MediaCtlMsg) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		return m, nil
	}

	switch msg.Command.Type {
	case mediactl.CmdPlayPause:
		m.togglePlayback()
	case mediactl.CmdPlay:
		if err := m.Audio.SetPlaying(true); err != nil {
			m.Error.Set(err.Error(), m.Config.Error.Duration)
		}
	case mediactl.CmdPause:
		if err := m.Audio.SetPlaying(false); err != nil {
			m.Error.Set(err.Error(), m.Config.Error.Duration)
		}
	case mediactl.CmdStop:
		m.Audio.Player.Pause()
		m.Audio.IsPlaying = false
		_ = m.Audio.Player.Seek(0)
	case mediactl.CmdNext:
		return m, m.stepQueue(1)
	case mediactl.CmdPrev:
		return m, m.stepQueue(-1)
	case mediactl.CmdSeek:
		// MPRIS Seek offset is in microseconds
		offset := time.Duration(msg.Command.Value) * time.Microsecond
		m.Audio.SeekRelative(offset)
		// Push new position to NowPlaying immediately so Control Center
		// doesn't briefly show the old position before the next tick.
		if m.MediaCtl != nil {
			m.MediaCtl.Update(m.buildPlayState())
		}
	case mediactl.CmdSetPosition:
		// MPRIS SetPosition is absolute position in microseconds
		pos := time.Duration(msg.Command.Value) * time.Microsecond
		_ = m.Audio.SeekPlayer(pos)
		if m.MediaCtl != nil {
			m.MediaCtl.Update(m.buildPlayState())
		}
	case mediactl.CmdSetVolume:
		m.setVolume(msg.Command.Volume)
	}

	return m, nil
}
