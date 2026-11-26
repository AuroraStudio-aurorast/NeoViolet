package ui

import (
	"fmt"
	"image"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
)

// repeatPlayback is read and updated only by the UI while the worker owns audio.
type repeatPlayback struct {
	path  string
	cover image.Image
	play  bool
}

func (m *Model) pendingRewind() *repeatPlayback {
	if m.pendingLoad == nil {
		return nil
	}
	return m.pendingLoad.rewind
}

func (m *Model) hasAudioTrack() bool {
	return m.Audio.Player != nil || m.pendingRewind() != nil
}

type repeatRewoundMsg struct {
	player     audio.AudioPlayer
	load       *audioLoad
	generation int
	err        error
}

// APE decoders may restart a subprocess when seeking. Give the load job
// exclusive ownership so ticks, track switches and shutdown can keep running.
func (m *Model) rewindInBackground() tea.Cmd {
	m.pendingLoad.cancel()
	player := m.Audio.Player
	load := &audioLoad{player: player, rewind: &repeatPlayback{
		path: player.Path(), cover: player.CoverImage(), play: true,
	}}
	m.pendingLoad = load
	m.Audio.Player = nil
	m.Loading, m.switchingTrack, m.loadingTick = true, true, 0
	m.loadGeneration++
	generation := m.loadGeneration
	return func() tea.Msg {
		if !load.take() {
			return nil
		}
		player.Pause()
		err := player.Seek(0)
		return load.deliver(repeatRewoundMsg{player: player, load: load, generation: generation, err: err})
	}
}

func (m *Model) finishRepeatRewind(msg repeatRewoundMsg) {
	if m.exiting || m.pendingLoad != msg.load || m.loadGeneration != msg.generation {
		msg.load.cancel()
		return
	}
	if !msg.load.take() {
		return
	}
	play := msg.load.rewind.play
	m.finishAudioLoad()
	m.Audio.Player = msg.player
	if msg.err != nil {
		m.Error.Set(fmt.Sprintf("Repeat failed: %v", msg.err), 120)
		return
	}
	m.Audio.seekRevision++
	m.Audio.SetVolume(m.Audio.Volume)
	if err := m.Audio.SetPlaying(play); err != nil {
		m.Error.Set(fmt.Sprintf("Repeat failed: %v", err), 120)
	}
	m.Audio.UpdatePosition()
}
