package ui

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func playbackModeBoundaryQueue(t *testing.T, mode string, index int) *Model {
	t.Helper()
	m := loadedModeQueue(t, mode)
	if index != 0 {
		m.requestQueueLoad(m.session.entries[index], nil, false)
		acceptQueueLoad(t, m)
	}
	// Start with a seeded bag too, rather than retaining the fixture's random picks.
	// #nosec G404 -- deterministic playback fixture, not a security decision.
	m.session.shuffle = shuffleState{random: rand.New(rand.NewPCG(1, 2))}
	return m
}

func TestPlaybackModeBoundaryModeBeforeEOF(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		index               int
		key                 bool
	}{
		{"order_to_repeat", config.PlaybackOrder, config.PlaybackRepeatOne, 0, true},
		{"order_tail_to_repeat", config.PlaybackOrder, config.PlaybackRepeatOne, 2, true},
		{"repeat_to_order", config.PlaybackRepeatOne, config.PlaybackOrder, 0, false},
		{"repeat_tail_to_order", config.PlaybackRepeatOne, config.PlaybackOrder, 2, false},
		{"order_tail_to_shuffle", config.PlaybackOrder, config.PlaybackShuffle, 2, false},
		{"repeat_tail_to_shuffle", config.PlaybackRepeatOne, config.PlaybackShuffle, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := playbackModeBoundaryQueue(t, tc.before, tc.index)
			current, generation := m.session.current, m.loadGeneration
			p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
			m.Audio.Player = p
			if tc.key {
				_, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
			} else {
				modeCommand(t, m, "mode "+tc.after)
			}
			if m.Config.PlaybackMode != tc.after {
				t.Fatalf("mode = %q, want %q", m.Config.PlaybackMode, tc.after)
			}
			// #nosec G404 -- deterministic playback fixture, not a security decision.
			m.session.shuffle.random = rand.New(rand.NewPCG(1, 2))
			_, _ = m.Update(TickMsg{})

			switch tc.after {
			case config.PlaybackRepeatOne:
				if !p.seekCalled || p.lastSeekPos != 0 || !p.playCalled || !p.IsPlaying() || !m.Audio.IsPlaying {
					t.Fatal("new repeat mode did not restart the completed track")
				}
				if m.Loading || m.queueIntent != nil || m.loadGeneration != generation || m.session.current != current {
					t.Fatal("repeat replaced the current queue entry")
				}
			case config.PlaybackOrder:
				if p.seekCalled || p.playCalled {
					t.Fatal("old repeat mode restarted the track")
				}
				if tc.index == len(m.session.entries)-1 {
					if m.Loading || m.queueIntent != nil || m.Audio.IsPlaying || m.loadGeneration != generation {
						t.Fatal("new order mode did not stop at the queue tail")
					}
				} else if !m.Loading || m.queueIntent == nil || m.queueIntent.entry != m.session.entries[tc.index+1].id {
					t.Fatal("new order mode did not select the sequential successor")
				}
			case config.PlaybackShuffle:
				if !m.Loading || m.queueIntent == nil || m.queueIntent.entry == current || queueIndex(m.session.entries, m.queueIntent.entry) < 0 {
					t.Fatal("new shuffle mode did not select another live entry at the queue tail")
				}
				if p.seekCalled || p.playCalled {
					t.Fatal("old repeat mode restarted the track instead of shuffling")
				}
			}
		})
	}
}

func TestPlaybackModeBoundaryPendingEOFLoadSurvivesModeChange(t *testing.T) {
	for _, switchMode := range []string{config.TrackSwitchOnSuccess, config.TrackSwitchImmediate} {
		t.Run(switchMode, func(t *testing.T) {
			for _, tc := range []struct {
				name, before, after string
				index               int
			}{
				{"order_to_repeat", config.PlaybackOrder, config.PlaybackRepeatOne, 0},
				{"shuffle_to_repeat", config.PlaybackShuffle, config.PlaybackRepeatOne, 2},
				{"shuffle_to_order", config.PlaybackShuffle, config.PlaybackOrder, 0},
				{"order_to_shuffle_at_tail", config.PlaybackOrder, config.PlaybackShuffle, 1},
				{"repeat_reload_to_order", config.PlaybackRepeatOne, config.PlaybackOrder, 0},
				{"repeat_reload_to_shuffle", config.PlaybackRepeatOne, config.PlaybackShuffle, 2},
			} {
				t.Run(tc.name, func(t *testing.T) {
					m := playbackModeBoundaryQueue(t, tc.before, tc.index)
					m.Config.TrackSwitchMode = switchMode
					current, initialGeneration := m.session.current, m.loadGeneration
					p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
					if tc.before == config.PlaybackRepeatOne {
						p.seekError = errors.New("not seekable")
					}
					m.Audio.Player = p
					_, _ = m.Update(TickMsg{})
					if !m.Loading || m.queueIntent == nil || m.pendingLoad == nil || m.loadGeneration != initialGeneration+1 {
						t.Fatal("EOF did not start exactly one queue load")
					}
					intent, load, generation := m.queueIntent, m.pendingLoad, m.loadGeneration
					selected, path := intent.entry, intent.track.Path
					if tc.before == config.PlaybackRepeatOne {
						if selected != current {
							t.Fatal("repeat fallback did not reload the completed entry")
						}
					} else if selected == current {
						t.Fatal("EOF did not select another queue entry")
					}

					modeCommand(t, m, "mode "+tc.after)
					// #nosec G404 -- deterministic playback fixture, not a security decision.
					m.session.shuffle.random = rand.New(rand.NewPCG(3, 4))
					for range 3 {
						_, _ = m.Update(TickMsg{})
						if !m.Loading || m.queueIntent != intent || m.queueIntent.entry != selected || m.queueIntent.track.Path != path || m.pendingLoad != load || m.loadGeneration != generation {
							t.Fatal("mode change or repeated EOF tick replaced the pending load")
						}
					}

					// Deliver the captured load owner too, so a cancelled request cannot
					// appear to succeed merely because its generation stayed unchanged.
					accepted := &repeatingPlayer{endedPlayer: endedPlayer{mockPlayer: mockPlayer{path: path, duration: time.Second}}}
					_, _ = m.Update(AudioLoadedMsg{Player: accepted, Path: path, Generation: generation, load: load})
					if m.Loading || m.queueIntent != nil || m.session.current != selected || m.Audio.Player != accepted || !accepted.IsPlaying() || !m.Audio.IsPlaying {
						t.Fatal("mode change cancelled or retargeted the selected EOF load")
					}
					if m.loadGeneration != generation || m.Config.PlaybackMode != tc.after {
						t.Fatal("accepting the selected load changed its generation or the new mode")
					}

					accepted.playCalled = false
					accepted.playing, accepted.ended = false, true
					_, _ = m.Update(TickMsg{})
					switch tc.after {
					case config.PlaybackRepeatOne:
						if !accepted.seekCalled || accepted.lastSeekPos != 0 || !accepted.playCalled || !accepted.IsPlaying() || !m.Audio.IsPlaying {
							t.Fatal("new mode did not repeat the selected track at its own EOF")
						}
						if m.Loading || m.queueIntent != nil || m.session.current != selected || m.loadGeneration != generation {
							t.Fatal("repeat at subsequent EOF changed the selected entry")
						}
					case config.PlaybackOrder:
						index := queueIndex(m.session.entries, selected)
						if index == len(m.session.entries)-1 {
							if m.Loading || m.queueIntent != nil || m.Audio.IsPlaying || m.loadGeneration != generation {
								t.Fatal("new order mode did not stop after the selected tail entry")
							}
						} else if !m.Loading || m.queueIntent == nil || m.queueIntent.entry != m.session.entries[index+1].id {
							t.Fatal("new order mode did not advance from the selected entry")
						}
					case config.PlaybackShuffle:
						if !m.Loading || m.queueIntent == nil || m.queueIntent.entry == selected || queueIndex(m.session.entries, m.queueIntent.entry) < 0 {
							t.Fatal("new shuffle mode did not advance after the selected tail entry")
						}
					}
					if tc.after != config.PlaybackRepeatOne && (accepted.seekCalled || accepted.playCalled) {
						t.Fatal("old mode repeated the selected track at its subsequent EOF")
					}
				})
			}
		})
	}
}

func TestPlaybackModeBoundaryModeAfterRepeatRestart(t *testing.T) {
	for _, tc := range []struct {
		name, after string
		index       int
	}{
		{"order_advances", config.PlaybackOrder, 0},
		{"order_tail_stops", config.PlaybackOrder, 2},
		{"shuffle_tail_advances", config.PlaybackShuffle, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := playbackModeBoundaryQueue(t, config.PlaybackRepeatOne, tc.index)
			current, generation := m.session.current, m.loadGeneration
			p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
			m.Audio.Player = p
			_, _ = m.Update(TickMsg{})
			if !p.seekCalled || !p.playCalled || !p.IsPlaying() || !m.Audio.IsPlaying || m.Loading {
				t.Fatal("repeat did not start a fresh pass before the mode change")
			}
			p.seekCalled, p.playCalled, p.pauseCalled = false, false, false
			modeCommand(t, m, "mode "+tc.after)
			// #nosec G404 -- deterministic playback fixture, not a security decision.
			m.session.shuffle.random = rand.New(rand.NewPCG(1, 2))
			_, _ = m.Update(TickMsg{})
			if m.Audio.Player != p || !p.IsPlaying() || !m.Audio.IsPlaying || p.seekCalled || p.playCalled || p.pauseCalled || m.Loading || m.queueIntent != nil || m.loadGeneration != generation || m.session.current != current {
				t.Fatal("mode change interrupted or restarted the repeated pass")
			}

			p.playing, p.ended = false, true
			_, _ = m.Update(TickMsg{})
			if p.seekCalled || p.playCalled {
				t.Fatal("old repeat mode restarted the completed second pass")
			}
			if tc.after == config.PlaybackOrder && tc.index == len(m.session.entries)-1 {
				if m.Loading || m.queueIntent != nil || m.Audio.IsPlaying || m.loadGeneration != generation {
					t.Fatal("order mode did not stop at the second pass's tail EOF")
				}
			} else {
				if !m.Loading || m.queueIntent == nil || m.queueIntent.entry == current {
					t.Fatal("new mode did not advance at the second pass's EOF")
				}
				if tc.after == config.PlaybackOrder && m.queueIntent.entry != m.session.entries[tc.index+1].id {
					t.Fatal("order mode did not select the sequential successor")
				}
			}
		})
	}
}

func TestPlaybackModeBoundaryModeAfterTailStopDoesNotReprocessEOF(t *testing.T) {
	for _, mode := range []string{config.PlaybackRepeatOne, config.PlaybackShuffle} {
		t.Run(mode, func(t *testing.T) {
			m := playbackModeBoundaryQueue(t, config.PlaybackOrder, 2)
			current, generation := m.session.current, m.loadGeneration
			p := &repeatingPlayer{endedPlayer: endedPlayer{ended: true}}
			m.Audio.Player = p
			_, _ = m.Update(TickMsg{})
			if m.Loading || m.queueIntent != nil || m.Audio.IsPlaying || m.loadGeneration != generation {
				t.Fatal("sequential tail did not stop before the mode change")
			}
			if mode == config.PlaybackRepeatOne {
				_, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
			} else {
				modeCommand(t, m, "mode "+mode)
			}
			// #nosec G404 -- deterministic playback fixture, not a security decision.
			m.session.shuffle.random = rand.New(rand.NewPCG(1, 2))
			for range 3 {
				_, _ = m.Update(TickMsg{})
				if m.Config.PlaybackMode != mode || m.Audio.Player != p || p.seekCalled || p.playCalled || p.pauseCalled || p.IsPlaying() || m.Audio.IsPlaying || m.Loading || m.queueIntent != nil || m.loadGeneration != generation || m.session.current != current {
					t.Fatal("new mode reprocessed the handled tail EOF or restarted playback")
				}
			}
		})
	}
}
