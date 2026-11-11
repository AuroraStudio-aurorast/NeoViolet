package ui

import (
	"math/rand/v2"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

func TestShufflePendingReplacementDoesNotInventHistory(t *testing.T) {
	m := queueFixture(t)
	m.Config.PlaybackMode = config.PlaybackShuffle
	pageKey(m, tea.KeyEnter, "")
	if m.stepQueue(1) == nil {
		t.Fatal("missing pending replacement navigation")
	}
	acceptQueueLoad(t, m)
	if m.stepQueue(-1) != nil {
		t.Fatal("unheard first selection entered the replacement queue history")
	}
}

func TestShufflePendingNextThenPreviousReturnsCurrent(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	if m.stepQueue(1) == nil || m.stepQueue(-1) == nil || m.queueIntent.entry != first {
		t.Fatal("previous did not return to the currently heard entry while next was loading")
	}
	acceptQueueLoad(t, m)
	if m.stepQueue(-1) != nil {
		t.Fatal("cancelled next added a phantom history entry")
	}
}

func TestShuffleEnteringWithPendingLoadSeedsHeardHistory(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackOrder)
	first := m.session.current
	m.stepQueue(1)
	unheard := m.queueIntent.entry
	modeCommand(t, m, "mode shuffle")
	if m.stepQueue(1) == nil || m.queueIntent.entry == first || m.queueIntent.entry == unheard {
		t.Fatal("shuffle did not advance the pending choice")
	}
	acceptQueueLoad(t, m)
	if m.stepQueue(-1) == nil || m.queueIntent.entry != first {
		t.Fatal("entering shuffle seeded an unheard pending entry as listening history")
	}
}

func TestShuffleRepeatedPendingPreviousWalksHistory(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	second := shuffleNext(t, m)
	third := shuffleNext(t, m)
	m.stepQueue(1)
	for _, want := range []uint64{third, second, first} {
		if m.stepQueue(-1) == nil || m.queueIntent.entry != want {
			t.Fatalf("rapid previous did not advance to heard entry %d", want)
		}
	}
	acceptQueueLoad(t, m)
	if m.session.current != first {
		t.Fatal("rapid previous did not commit its final target")
	}
}

func TestShuffleRapidNextDoesNotRollOverAnUnheardRound(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	for range 10 {
		if m.stepQueue(1) == nil || m.queueIntent.entry == first {
			t.Fatal("rapid next repeated the heard entry before the rest of its round played")
		}
	}
	acceptQueueLoad(t, m)
	seen := map[uint64]bool{first: true, m.session.current: true}
	if seen[shuffleNext(t, m)] {
		t.Fatal("superseded pending choices were lost from the round")
	}
}

func TestShuffleRapidNextAtRoundBoundaryAvoidsLastHeard(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	shuffleNext(t, m)
	last := shuffleNext(t, m)
	for range 10 {
		if m.stepQueue(1) == nil || m.queueIntent.entry == last {
			t.Fatal("provisional round rollover repeated the last heard entry")
		}
	}
}

func TestShufflePreviousDoesNotConsumeAnUnplayedRoundEntry(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	second := shuffleNext(t, m)
	third := shuffleNext(t, m)
	next := shuffleNext(t, m)
	// Return to the preceding round, then forward to the new round's first.
	for range 3 {
		if m.stepQueue(-1) == nil {
			t.Fatal("missing previous history entry")
		}
		acceptQueueLoad(t, m)
	}
	if m.session.current != first {
		t.Fatal("history did not return to the first entry")
	}
	for _, want := range []uint64{second, third, next} {
		if shuffleNext(t, m) != want {
			t.Fatal("forward history was not preserved")
		}
	}
	seen := map[uint64]bool{next: true}
	for range 2 {
		id := shuffleNext(t, m)
		if seen[id] {
			t.Fatal("visiting history consumed an unplayed entry in the new round")
		}
		seen[id] = true
	}
}

func TestShuffleHistoryTargetAlsoReservedRemainsInRound(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	m.session.shuffle.random = rand.New(rand.NewPCG(56, 78)) // #nosec G404 -- repeatable navigation fixture.
	shuffleNext(t, m)
	last := shuffleNext(t, m)
	m.stepQueue(1)
	reserved := m.queueIntent.entry
	// Walk back to whichever preceding-round entry is also reserved.
	for range 3 {
		if m.stepQueue(-1) == nil {
			t.Fatal("missing history entry")
		}
		if m.queueIntent.entry == reserved {
			break
		}
	}
	if m.queueIntent.entry != reserved {
		t.Fatal("did not reach overlapping history entry")
	}
	acceptQueueLoad(t, m)
	if !slices.Contains(m.session.shuffle.bag, reserved) {
		t.Fatal("accepting a history target consumed its unplayed random reservation")
	}
	for m.session.current != last {
		shuffleNext(t, m)
	}
	seen := map[uint64]bool{}
	for range 3 {
		id := shuffleNext(t, m)
		if seen[id] {
			t.Fatal("history consumed its overlapping reservation from the new round")
		}
		seen[id] = true
	}
}

func TestShufflePendingDirectSelectionDoesNotRecyclePlayedEntry(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	second := shuffleNext(t, m)
	index := queueIndex(m.session.entries, first)
	m.requestQueueLoad(m.session.entries[index], nil, false)
	for range 5 {
		if m.stepQueue(1) == nil || m.queueIntent.entry == first || m.queueIntent.entry == second {
			t.Fatal("pending direct selection recycled a played entry before the round completed")
		}
	}
}

func TestShuffleContinuesWhenHistoryLeavesOnlyCurrentInBag(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	current := m.session.current
	// A history visit can make the final unplayed entry the one just heard.
	m.session.shuffle.bag = []uint64{current}
	m.session.shuffle.played = []uint64{m.session.entries[1].id, m.session.entries[2].id}
	if m.stepQueue(1) == nil || m.queueIntent.entry == current {
		t.Fatal("shuffle stopped or immediately repeated when only the current entry remained in its bag")
	}
}

func TestShuffleDirectSelectionAfterPreviousBranchesHistory(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	second := shuffleNext(t, m)
	shuffleNext(t, m)
	m.stepQueue(-1)
	acceptQueueLoad(t, m)
	i := queueIndex(m.session.entries, first)
	m.requestQueueLoad(m.session.entries[i], nil, false)
	acceptQueueLoad(t, m)
	if m.stepQueue(-1) == nil || m.queueIntent.entry != second {
		t.Fatal("direct selection inherited a previous history-navigation flag")
	}
}

func TestShuffleSeededHistoryBoundAndReplacement(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	// #nosec G404 -- deterministic shuffle fixture, not a security decision.
	m.session.shuffle.random = rand.New(rand.NewPCG(12, 34))
	for range shuffleHistoryLimit + 10 {
		shuffleNext(t, m)
	}
	if len(m.session.shuffle.history) != shuffleHistoryLimit {
		t.Fatal("shuffle retained an unbounded listening history")
	}
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	if m.stepQueue(-1) != nil {
		t.Fatal("replacing the queue retained history from the previous queue")
	}
}

func shuffleNext(t *testing.T, m *Model) uint64 {
	t.Helper()
	if m.stepQueue(1) == nil || m.queueIntent == nil {
		t.Fatal("shuffle did not request a next entry")
	}
	id := m.queueIntent.entry
	acceptQueueLoad(t, m)
	return id
}

func TestShuffleRoundsDoNotRepeatOrReorderQueue(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	original := slices.Clone(m.session.entries)
	previous := uint64(0)
	for round := range 5 {
		seen := map[uint64]bool{}
		for i := range len(original) {
			id := m.session.current
			if round > 0 || i > 0 {
				m.Audio.Player = &endedPlayer{ended: true}
				if m.advanceQueueAtEnd() == nil {
					t.Fatal("shuffle EOF stopped before selecting another entry")
				}
				id = m.queueIntent.entry
				acceptQueueLoad(t, m)
			}
			if seen[id] || id == previous {
				t.Fatalf("round %d repeats entry %d (previous %d)", round, id, previous)
			}
			seen[id] = true
			previous = id
		}
	}
	for i, entry := range original {
		if m.session.entries[i].id != entry.id {
			t.Fatal("shuffle reordered the visible queue")
		}
	}
}

func TestShufflePreviousAndForwardFollowAcceptedHistory(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	a := m.session.current
	b := shuffleNext(t, m)
	c := shuffleNext(t, m)
	for _, want := range []uint64{b, a} {
		if m.stepQueue(-1) == nil || m.queueIntent.entry != want {
			t.Fatalf("previous did not return accepted history entry %d", want)
		}
		acceptQueueLoad(t, m)
	}
	if m.stepQueue(-1) != nil {
		t.Fatal("previous invented history before the first played entry")
	}
	for _, want := range []uint64{b, c} {
		if got := shuffleNext(t, m); got != want {
			t.Fatalf("forward history = %d, want %d", got, want)
		}
	}
}

func TestShuffleRapidNextAndFailureDoNotCommitSkippedHistory(t *testing.T) {
	for _, mode := range []string{config.TrackSwitchOnSuccess, config.TrackSwitchImmediate} {
		t.Run(mode, func(t *testing.T) {
			m := loadedModeQueue(t, config.PlaybackShuffle)
			m.Config.TrackSwitchMode = mode
			first := m.session.current
			if m.stepQueue(1) == nil {
				t.Fatal("missing first pending selection")
			}
			skipped := m.queueIntent.entry
			oldGeneration := m.loadGeneration
			if m.stepQueue(1) == nil || m.queueIntent.entry == skipped || m.queueIntent.entry == first {
				t.Fatal("rapid next did not advance its pending random selection")
			}
			target := m.queueIntent.entry
			stale := &mockPlayer{}
			_, _ = m.Update(AudioLoadedMsg{Player: stale, Generation: oldGeneration})
			if !stale.closeCalled || m.queueIntent.entry != target {
				t.Fatal("stale load changed the pending random selection")
			}
			acceptQueueLoad(t, m)
			if m.stepQueue(-1) == nil || m.queueIntent.entry != first {
				t.Fatal("skipped pending track entered accepted history")
			}
			acceptQueueLoad(t, m)
			if shuffleNext(t, m) != target || shuffleNext(t, m) != skipped {
				t.Fatal("skipped pending track was lost from this shuffle round")
			}
		})
	}
	t.Run("failed request", func(t *testing.T) {
		m := loadedModeQueue(t, config.PlaybackShuffle)
		first := m.session.current
		m.stepQueue(1)
		_, _ = m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "failed", Timer: 90})
		if m.session.current != first || m.stepQueue(-1) != nil {
			t.Fatal("failed request modified accepted history")
		}
		seen := map[uint64]bool{first: true}
		for range 2 {
			id := shuffleNext(t, m)
			if seen[id] {
				t.Fatal("failed request consumed a round entry")
			}
			seen[id] = true
		}
	})
}

func TestShuffleLiveQueueChanges(t *testing.T) {
	m := loadedModeQueue(t, config.PlaybackShuffle)
	first := m.session.current
	m.pages.cursors[1] = 1
	removed := m.session.entries[1].id
	m.removeQueueEntry()
	m.undoQueueRemoval()
	m.appendQueue(library.Track{Path: "D.wav", Title: "D"})
	added := m.session.entries[len(m.session.entries)-1].id
	m.pages.cursors[1] = 0
	m.reorderQueue(1)
	seen := map[uint64]bool{first: true}
	for range 3 {
		id := shuffleNext(t, m)
		if seen[id] {
			t.Fatal("queue edits repeated an entry in the same round")
		}
		seen[id] = true
	}
	if !seen[removed] || !seen[added] {
		t.Fatal("undo or append was omitted from the current round")
	}
	// Removing the playing item leaves audio detached but must not stop shuffle.
	m.pages.cursors[1] = queueIndex(m.session.entries, m.session.current)
	m.removeQueueEntry()
	if id := shuffleNext(t, m); queueIndex(m.session.entries, id) < 0 {
		t.Fatal("shuffle selected a removed entry")
	}
}

func TestShuffleDuplicatePathsRemainSeparateEntries(t *testing.T) {
	m := queueFixture(t)
	m.Config.PlaybackMode = config.PlaybackShuffle
	m.pages.visible[1] = m.pages.visible[0]
	m.pages.visible = m.pages.visible[:2]
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	first := m.session.current
	if next := shuffleNext(t, m); next == first || m.Audio.Player.Path() != m.session.entries[0].track.Path {
		t.Fatal("shuffle collapsed distinct queue entries sharing a file path")
	}
}

func TestShuffleSingleEntryAndMediaNavigation(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		m := queueFixture(t)
		m.Config.PlaybackMode = config.PlaybackShuffle
		m.pages.visible = m.pages.visible[:1]
		pageKey(m, tea.KeyEnter, "")
		acceptQueueLoad(t, m)
		id := m.session.current
		if shuffleNext(t, m) != id || len(m.session.entries) != 1 {
			t.Fatal("single-entry shuffle did not repeat the same entry")
		}
	})
	t.Run("media keys", func(t *testing.T) {
		m := loadedModeQueue(t, config.PlaybackShuffle)
		first := m.session.current
		_, cmd := handleMediaCtlCmd(m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdNext}})
		if cmd == nil || m.queueIntent.entry == first {
			t.Fatal("media next ignored shuffle")
		}
		acceptQueueLoad(t, m)
		_, cmd = handleMediaCtlCmd(m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdPrev}})
		if cmd == nil || m.queueIntent.entry != first {
			t.Fatal("media previous ignored shuffle history")
		}
	})
}
