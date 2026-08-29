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
		if !m.Audio.Player.IsPlaying() {
			_ = m.Audio.Player.Play()
			m.Audio.IsPlaying = true
		}
	case mediactl.CmdPause:
		if m.Audio.Player.IsPlaying() {
			m.Audio.Player.Pause()
			m.Audio.IsPlaying = false
		}
	case mediactl.CmdStop:
		m.Audio.Player.Pause()
		m.Audio.IsPlaying = false
		_ = m.Audio.Player.Seek(0)
	case mediactl.CmdNext:
		// No tracklist — skip forward 10s as fallback
		m.Audio.SeekRelative(10 * time.Second)
	case mediactl.CmdPrev:
		m.Audio.SeekRelative(-10 * time.Second)
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
		m.Audio.SetVolume(msg.Command.Volume)
	}

	return m, nil
}
