package ui

import (
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func TestShuffleNextSkipsConsecutiveLoadFailures(t *testing.T) {
	for _, policy := range []string{config.TrackSwitchOnSuccess, config.TrackSwitchImmediate} {
		t.Run(policy, func(t *testing.T) {
			m := loadedModeQueue(t, config.PlaybackShuffle)
			m.Config.TrackSwitchMode = policy
			current := m.session.current
			m.Audio.Player = &endedPlayer{ended: true}
			if m.advanceQueueAtEnd() == nil {
				t.Fatal("EOF did not select a candidate")
			}
			first := m.queueIntent.entry
			_, cmd := m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "missing", Timer: 90})
			if cmd != nil || m.Loading {
				t.Fatal("shuffle retried automatically")
			}
			if m.stepQueue(1) == nil || m.queueIntent.entry == first || m.queueIntent.entry == current {
				t.Fatal("manual next did not try another unplayed candidate")
			}
			_, _ = m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "missing", Timer: 90})
			if m.stepQueue(1) != nil || m.Loading || m.queueIntent != nil {
				t.Fatal("an exhausted round immediately retried failed tracks")
			}
			if m.stepQueue(1) == nil || m.queueIntent.entry == current {
				t.Fatal("manual next could not retry after exhaustion")
			}
			acceptQueueLoad(t, m)
			if m.stepQueue(-1) == nil || m.queueIntent.entry != current {
				t.Fatal("failed loads altered listening history")
			}
		})
	}
}

func TestShuffleNextDoesNotStartIdleQueue(t *testing.T) {
	m := queueFixture(t)
	m.appendQueue(m.pages.visible[0])
	modeCommand(t, m, "mode shuffle")
	if cmd := pageKey(m, '>', ">"); cmd != nil || m.queueIntent != nil || m.Loading {
		t.Fatal("Next started a queue without a current track")
	}
}

type slowRewindPlayer struct{ repeatingPlayer }

func (*slowRewindPlayer) RewindMayBlock() bool { return true }

func TestRepeatOneSlowRewindIsDeferred(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackRepeatOne)
	entry := m.session.current
	p := &slowRewindPlayer{repeatingPlayer: repeatingPlayer{endedPlayer: endedPlayer{ended: true}}}
	m.Audio.Player = p
	cmd := m.advanceQueueAtEnd()
	if cmd == nil || p.seekCalled || !m.Loading {
		t.Fatal("EOF did not defer the slow rewind to a load command")
	}
	if m.advanceQueueAtEnd() != nil {
		t.Fatal("rewind was queued again before it finished")
	}
	modeCommand(t, m, "mode order")
	_, _ = m.Update(cmd())
	if !p.seekCalled || !p.IsPlaying() || m.Audio.Player != p || m.session.current != entry || m.Loading {
		t.Fatal("deferred rewind did not resume the originally selected track")
	}
}

func TestRepeatOneSupersededRewindClosesOldPlayer(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackRepeatOne)
	p := &slowRewindPlayer{repeatingPlayer: repeatingPlayer{endedPlayer: endedPlayer{ended: true}}}
	m.Audio.Player = p
	rewind := m.advanceQueueAtEnd()
	result := rewind()
	if m.stepQueue(1) == nil {
		t.Fatal("Next could not supersede the rewind")
	}
	acceptQueueLoad(t, m)
	selected := m.Audio.Player
	_, _ = m.Update(result)
	if !p.closeCalled || p.playCalled || m.Audio.Player != selected || m.Loading {
		t.Fatal("superseded rewind leaked or replaced the new player")
	}
}

func TestRepeatOnePauseDuringRewind(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackRepeatOne)
	p := &slowRewindPlayer{repeatingPlayer: repeatingPlayer{endedPlayer: endedPlayer{ended: true}}}
	m.Audio.Player = p
	rewind := m.advanceQueueAtEnd()
	if _, err := m.ipcRequest(ipc.Message{Type: "pause"}); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Update(rewind())
	if m.Audio.Player != p || !p.seekCalled || p.IsPlaying() || p.playCalled || m.Loading {
		t.Fatal("pause request did not carry through the rewind")
	}
}
