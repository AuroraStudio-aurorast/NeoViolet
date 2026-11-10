package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestQueueAppendAfterRemovingPlayingTailCanAdvance(t *testing.T) {
	for _, eof := range []bool{false, true} {
		t.Run(map[bool]string{false: "next", true: "end"}[eof], func(t *testing.T) {
			m := queueFixture(t)
			appended := m.pages.visible[1]
			m.pages.visible = m.pages.visible[:1]
			pageKey(m, tea.KeyEnter, "")
			acceptQueueLoad(t, m)
			m.removeQueueEntry()
			m.appendQueue(appended)
			var cmd tea.Cmd
			if eof {
				m.Audio.Player = &endedPlayer{ended: true}
				m.Audio.IsPlaying = true
				cmd = m.advanceQueueAtEnd()
			} else {
				cmd = m.stepQueue(1)
			}
			if cmd == nil || m.queueIntent == nil || m.queueIntent.track.Path != appended.Path {
				t.Fatal("appended track was unreachable after removing the playing tail")
			}
		})
	}
}

func TestQueueUndoPlayingEntryRestoresIdentityAndPrevious(t *testing.T) {
	m := queueFixture(t)
	// Duplicate paths must remain separate entries when one is removed and restored.
	m.pages.visible[1] = m.pages.visible[0]
	m.pages.cursors[0] = 1
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	playing := m.session.current
	previous := m.session.entries[0].id
	m.pages.cursors[1] = 1
	m.removeQueueEntry()
	m.undoQueueRemoval()
	if m.session.current != playing {
		t.Fatalf("undo restored current=%d, want %d", m.session.current, playing)
	}
	if cmd := m.stepQueue(-1); cmd == nil || m.queueIntent.entry != previous {
		t.Fatal("Previous lost the entry preceding restored playback")
	}
}

func TestQueueAppendAfterRemovingPendingTailKeepsSuccessor(t *testing.T) {
	for _, acceptFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "accepted"}[acceptFirst], func(t *testing.T) {
			m := queueFixture(t)
			pageKey(m, tea.KeyEnter, "")
			acceptQueueLoad(t, m)
			tail := m.session.entries[2]
			m.requestQueueLoad(tail, nil, false)
			m.pages.cursors[1] = 2
			m.removeQueueEntry()
			appended := m.pages.visible[0]
			m.appendQueue(appended)
			appendedID := m.session.entries[2].id
			if acceptFirst {
				acceptQueueLoad(t, m)
			}
			if cmd := m.stepQueue(1); cmd == nil || m.queueIntent.entry != appendedID {
				t.Fatal("removed pending tail lost the subsequently appended successor")
			}
		})
	}
}

func TestQueueAppendAfterRemovingAllSuccessorsCanAdvance(t *testing.T) {
	m := queueFixture(t)
	appended := m.pages.visible[2]
	m.pages.visible = m.pages.visible[:2]
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	m.removeQueueEntry()
	m.removeQueueEntry()
	m.appendQueue(appended)
	if cmd := m.stepQueue(1); cmd == nil || m.queueIntent.track.Path != appended.Path {
		t.Fatal("removing successors discarded the detached playback boundary")
	}
}

func TestQueueUndoAfterPlaybackChangesKeepsNewCurrent(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	m.removeQueueEntry()
	if m.stepQueue(1) == nil {
		t.Fatal("missing successor")
	}
	acceptQueueLoad(t, m)
	playing := m.session.current
	m.undoQueueRemoval()
	if m.session.current != playing || m.session.current != m.session.entries[1].id {
		t.Fatal("undo replaced the newly accepted playing identity")
	}
}

func TestQueueAppendToIdleQueueDoesNotCreatePlayingEntry(t *testing.T) {
	m := queueFixture(t)
	m.appendQueue(m.pages.visible[0])
	if m.session.current != 0 || m.stepQueue(1) != nil {
		t.Fatal("appending to an idle queue created a detached playback successor")
	}
}

func TestQueueUndoRemovedSuccessorRestoresDetachedBoundary(t *testing.T) {
	for _, stage := range []string{"playing", "pending", "accepted"} {
		t.Run(stage, func(t *testing.T) {
			m := queueFixture(t)
			pageKey(m, tea.KeyEnter, "")
			acceptQueueLoad(t, m)
			if stage != "playing" {
				m.requestQueueLoad(m.session.entries[1], nil, false)
				m.pages.cursors[1] = 1
			}
			m.removeQueueEntry()
			successor := m.session.entries[m.pages.cursors[1]].id
			m.removeQueueEntry()
			if stage == "accepted" {
				acceptQueueLoad(t, m)
			}
			m.undoQueueRemoval()
			if stage == "pending" {
				acceptQueueLoad(t, m)
			}
			if cmd := m.stepQueue(1); cmd == nil || m.queueIntent.entry != successor {
				t.Fatal("Next skipped the restored successor of detached playback")
			}
		})
	}
}
