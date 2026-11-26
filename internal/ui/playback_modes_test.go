package ui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

type repeatingPlayer struct {
	endedPlayer
	seekError, playError error
}

func (p *repeatingPlayer) Seek(pos time.Duration) error {
	if p.seekError != nil {
		return p.seekError
	}
	p.ended = false
	return p.mockPlayer.Seek(pos)
}

func (p *repeatingPlayer) Play() error {
	if p.playError != nil {
		return p.playError
	}
	return p.mockPlayer.Play()
}

func loadedModeQueue(t *testing.T, mode string) *Model {
	t.Helper()
	m := queueFixture(t)
	m.Config.PlaybackMode = mode
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	return m
}

func TestRepeatOneRestartsAtEOFWithoutReplacingQueue(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackRepeatOne)
	id, generation := m.session.current, m.loadGeneration
	p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
	m.Audio.Player = p
	if cmd := m.advanceQueueAtEnd(); cmd != nil {
		t.Fatal("seekable repeat unnecessarily reloaded the track")
	}
	if !p.IsPlaying() || !p.seekCalled || p.lastSeekPos != 0 || m.session.current != id || len(m.session.entries) != 3 {
		t.Fatal("repeat did not restart the current queue entry")
	}
	if m.loadGeneration != generation || m.queueIntent != nil || !m.Audio.IsPlaying {
		t.Fatal("repeat changed the load generation or left stale playback state")
	}
	m.advanceQueueAtEnd()
	p.ended = true
	if cmd := m.advanceQueueAtEnd(); cmd != nil || !p.IsPlaying() {
		t.Fatal("a second natural end did not repeat")
	}
}

func TestRepeatOneManualKeysStillNavigate(t *testing.T) {
	for _, media := range []bool{false, true} {
		m := loadedModeQueue(t, config.PlaybackRepeatOne)
		var cmd tea.Cmd
		if media {
			_, cmd = handleMediaCtlCmd(m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdNext}})
		} else {
			cmd = pageKey(m, '>', ">")
		}
		if cmd == nil || m.queueIntent.entry != m.session.entries[1].id {
			t.Fatal("manual next was trapped in single-track repeat")
		}
		acceptQueueLoad(t, m)
		if m.stepQueue(-1) == nil || m.queueIntent.entry != m.session.entries[0].id {
			t.Fatal("manual previous was trapped in single-track repeat")
		}
	}
}

func TestRepeatOneSeekFailureReloadsSameEntry(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackRepeatOne)
	id := m.session.current
	m.Audio.Player = &repeatingPlayer{endedPlayer: endedPlayer{ended: true}, seekError: errors.New("not seekable")}
	if m.advanceQueueAtEnd() == nil || m.queueIntent.entry != id || m.queueIntent.replace {
		t.Fatal("repeat fallback did not reload the same entry")
	}
	if m.advanceQueueAtEnd() != nil {
		t.Fatal("repeat started a second load while loading")
	}
	_, _ = m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "unavailable", Timer: 90})
	if m.advanceQueueAtEnd() != nil || !m.Error.Visible {
		t.Fatal("failed repeat entered an automatic retry loop")
	}
}

func TestRepeatOneErrorsAndDetachedTrackDoNotLoop(t *testing.T) {
	t.Run("decoder error", func(t *testing.T) {
		m := loadedModeQueue(t, config.PlaybackRepeatOne)
		p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true, err: errors.New("decoder failed")}}
		m.Audio.Player = p
		if m.advanceQueueAtEnd() != nil || p.seekCalled || !m.Error.Visible {
			t.Fatal("decoder error was treated as a natural completion")
		}
	})
	t.Run("play failure", func(t *testing.T) {
		m := loadedModeQueue(t, config.PlaybackRepeatOne)
		m.Audio.Player = &repeatingPlayer{endedPlayer: endedPlayer{ended: true}, playError: errors.New("cannot play")}
		if m.advanceQueueAtEnd() != nil || !m.Error.Visible || m.Audio.IsPlaying {
			t.Fatal("repeat start failure was not reported")
		}
	})
	t.Run("removed current", func(t *testing.T) {
		m := loadedModeQueue(t, config.PlaybackRepeatOne)
		m.removeQueueEntry()
		p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
		m.Audio.Player = p
		if m.advanceQueueAtEnd() == nil || p.seekCalled || m.queueIntent.entry != m.session.entries[0].id {
			t.Fatal("repeat resurrected a removed entry instead of advancing")
		}
	})
}
