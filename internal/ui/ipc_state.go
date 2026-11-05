package ui

import (
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func (m *Model) pushIPCState(force bool) {
	if m.ipcServer == nil || m.ipcSession == 0 || m.exiting {
		return
	}
	state := ipc.PlaybackState{TrackID: m.activeGeneration, Title: m.Audio.CurrentSong, Artist: m.Audio.Artist, Album: m.Audio.Album,
		HasTrack: m.Audio.Player != nil, Playing: m.Audio.IsPlaying, Loading: m.Loading, DesktopLyrics: m.DesktopLyricsEnabled,
		Duration: m.Audio.Duration.Seconds(), Position: m.Audio.Elapsed.Seconds(), Volume: m.Audio.Volume}
	if m.Audio.Player != nil {
		state.Path = m.Audio.Player.Path()
	}
	if dir, err := config.Dir(); err == nil {
		state.ConfigDir = dir
	}
	last, discrete := m.ipcLastState, state
	last.Position, discrete.Position = 0, 0
	now := time.Now()
	if !force && m.Audio.seekRevision == m.ipcLastSeekRevision && (state == m.ipcLastState || (last == discrete && now.Sub(m.ipcLastPush) < 250*time.Millisecond)) {
		return
	}
	m.ipcRevision++
	frame := state
	frame.Revision = m.ipcRevision
	if err := m.ipcServer.SendTo(m.ipcSession, ipc.Message{Type: "state", State: &frame}); err == nil {
		m.ipcLastState = state
		m.ipcLastSeekRevision = m.Audio.seekRevision
		m.ipcLastPush = now
	}
}
