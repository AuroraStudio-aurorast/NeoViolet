package ui

import (
	"math/rand/v2"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

const shuffleHistoryLimit = 1024

// Pending loads own a clone. Only a successful load publishes its navigation
// state, so failed and superseded requests never enter the listening history.
type shuffleState struct {
	bag, played, history, pending []uint64
	cursor                        int
	heard                         uint64
	initialized                   bool
	historyTarget                 bool
	random                        *rand.Rand
}

func (s shuffleState) clone() shuffleState {
	s.bag = slices.Clone(s.bag)
	s.played = slices.Clone(s.played)
	s.history = slices.Clone(s.history)
	s.pending = slices.Clone(s.pending)
	return s
}

func (s *shuffleState) init(current uint64) {
	if s.random == nil {
		// #nosec G404 -- playback order is not security-sensitive.
		s.random = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if !s.initialized {
		s.initialized = true
		s.heard = current
		s.cursor = -1
		if current != 0 {
			s.played = append(s.played, current)
			s.history = append(s.history, current)
			s.cursor = 0
		}
	}
}

func (s *shuffleState) sync(entries []queueEntry, current uint64) {
	s.init(current)
	live := make(map[uint64]bool, len(entries))
	for _, entry := range entries {
		live[entry.id] = true
	}
	s.bag = slices.DeleteFunc(s.bag, func(id uint64) bool { return !live[id] })
	known := make(map[uint64]bool, len(entries))
	for _, ids := range [][]uint64{s.bag, s.played, s.pending} {
		for _, id := range ids {
			known[id] = true
		}
	}
	for _, entry := range entries {
		if !known[entry.id] {
			// Insert additions at a random position without disturbing prior picks.
			i := s.random.IntN(len(s.bag) + 1)
			s.bag = slices.Insert(s.bag, i, entry.id)
		}
	}
}

func (s *shuffleState) next(entries []queueEntry, current uint64, delta int) uint64 {
	s.sync(entries, current)
	for i := s.cursor + delta; i >= 0 && i < len(s.history); i += delta {
		if queueIndex(entries, s.history[i]) >= 0 {
			s.cursor = i
			s.historyTarget = true
			return s.history[i]
		}
	}
	if delta < 0 || len(entries) == 0 {
		return 0
	}
	s.historyTarget = false
	if len(s.bag) == 0 {
		// Exhausting provisional selections does not complete the listening
		// round. Cycle the reserved choices until one actually loads.
		for _, id := range s.pending {
			if queueIndex(entries, id) >= 0 && !slices.Contains(s.played, id) && !slices.Contains(s.bag, id) {
				s.bag = append(s.bag, id)
			}
		}
	}
	if len(s.bag) == 0 {
		s.refill(entries)
	}
	// The first accepted choice of a new round must differ from the last
	// heard track, even when many provisional requests precede acceptance.
	index := slices.IndexFunc(s.bag, func(id uint64) bool { return len(entries) == 1 || id != s.heard })
	if index < 0 {
		for _, id := range s.pending {
			if id != s.heard && queueIndex(entries, id) >= 0 && !slices.Contains(s.played, id) {
				return id
			}
		}
		// A history visit can leave only the just-heard entry in the bag.
		// There is no distinct choice left in this round; begin another one.
		s.refill(entries)
		index = slices.IndexFunc(s.bag, func(id uint64) bool { return id != s.heard })
	}
	id := s.bag[index]
	s.bag = slices.Delete(s.bag, index, index+1)
	if !slices.Contains(s.pending, id) {
		s.pending = append(s.pending, id)
	}
	return id
}

func (s *shuffleState) refill(entries []queueEntry) {
	s.played, s.pending = nil, nil
	s.bag = s.bag[:0]
	for _, entry := range entries {
		s.bag = append(s.bag, entry.id)
	}
	s.random.Shuffle(len(s.bag), func(i, j int) { s.bag[i], s.bag[j] = s.bag[j], s.bag[i] })
}

func (s *shuffleState) accept(entries []queueEntry, id uint64) {
	s.sync(entries, id)
	fromHistory := s.historyTarget
	// Superseded targets remain eligible in this round: they were never heard.
	for _, skipped := range s.pending {
		if (skipped != id || fromHistory) && queueIndex(entries, skipped) >= 0 && !slices.Contains(s.played, skipped) && !slices.Contains(s.bag, skipped) {
			s.bag = append(s.bag, skipped)
		}
	}
	s.pending = nil
	s.heard = id
	if fromHistory {
		return
	}
	s.bag = slices.DeleteFunc(s.bag, func(entry uint64) bool { return entry == id })
	if !slices.Contains(s.played, id) {
		s.played = append(s.played, id)
	}
	if s.cursor >= 0 && s.history[s.cursor] == id {
		return
	}
	s.history = append(s.history[:s.cursor+1], id)
	if len(s.history) > shuffleHistoryLimit {
		s.history = slices.Clone(s.history[len(s.history)-shuffleHistoryLimit:])
	}
	s.cursor = len(s.history) - 1
}

func (m *Model) stepShuffleQueue(delta int) tea.Cmd {
	entries, current, replace := m.session.entries, m.session.current, false
	if current == 0 {
		current = m.session.detached
	}
	state := m.session.shuffle.clone()
	state.init(current)
	pendingNew := false
	if intent := m.queueIntent; intent != nil && intent.generation == m.loadGeneration {
		current = intent.entry
		if intent.replace {
			entries, replace = intent.replacement, true
			state = shuffleState{}
			state.init(0)
		}
		if intent.shuffle != nil {
			state = intent.shuffle.clone()
			pendingNew = !state.historyTarget
		} else {
			state.sync(entries, 0)
			state.bag = slices.DeleteFunc(state.bag, func(id uint64) bool { return id == current })
			if !slices.Contains(state.played, current) {
				state.pending = append(state.pending, current)
			}
			pendingNew = true
		}
	}
	var id uint64
	if delta < 0 && pendingNew && state.cursor >= 0 {
		// The loading target was never heard. Previous first returns to the
		// accepted history cursor instead of stepping past it.
		id = state.history[state.cursor]
		state.historyTarget = true
		if queueIndex(entries, id) < 0 {
			id = state.next(entries, current, delta)
		}
	} else {
		id = state.next(entries, current, delta)
	}
	index := queueIndex(entries, id)
	if index < 0 {
		return nil
	}
	cmd := m.requestQueueLoad(entries[index], entries, replace)
	m.queueIntent.shuffle = &state
	return cmd
}

func (m *Model) acceptShuffleTrack(intent *queueLoadIntent) {
	if m.playbackMode() != config.PlaybackShuffle {
		m.session.shuffle = shuffleState{}
		return
	}
	switch {
	case intent != nil && intent.shuffle != nil:
		m.session.shuffle = *intent.shuffle
	case intent == nil || intent.replace:
		m.session.shuffle = shuffleState{}
	default:
		m.session.shuffle.historyTarget = false
	}
	id := m.session.current
	if id == 0 {
		id = m.session.detached
	}
	m.session.shuffle.accept(m.session.entries, id)
}
