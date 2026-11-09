package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

type queueEntry struct {
	id    uint64
	track library.Track
}
type sessionQueue struct {
	entries               []queueEntry
	current, detachedNext uint64
	removed               *queueEntry
	removedAt             int
}
type queueLoadIntent struct {
	detachedNext uint64
	generation   int
	entry        uint64
	track        library.Track
	replacement  []queueEntry
	replace      bool
}

func (m *Model) newQueueEntry(track library.Track) queueEntry {
	m.queueSequence++
	return queueEntry{id: m.queueSequence, track: track}
}
func (m *Model) appendQueue(track library.Track) {
	m.session.entries = append(m.session.entries, m.newQueueEntry(track))
	m.Info.Set("Added to session queue", 90)
}
func (m *Model) playLibrarySelection() tea.Cmd {
	p := m.pages
	if len(p.visible) == 0 {
		return nil
	}
	entries := make([]queueEntry, 0, len(p.visible))
	for _, track := range p.visible {
		entries = append(entries, m.newQueueEntry(track))
	}
	index := max(0, min(p.cursors[0], len(entries)-1))
	return m.requestQueueLoad(entries[index], entries, true)
}
func (m *Model) requestQueueLoad(entry queueEntry, replacement []queueEntry, replace bool) tea.Cmd {
	_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: entry.track.Path})
	m.queueIntent = &queueLoadIntent{generation: m.loadGeneration, entry: entry.id, track: entry.track, replacement: replacement, replace: replace}
	return cmd
}
func queueIndex(entries []queueEntry, id uint64) int {
	for i, entry := range entries {
		if entry.id == id {
			return i
		}
	}
	return -1
}
func (m *Model) stepQueue(delta int) tea.Cmd {
	entries, current := m.session.entries, m.session.current
	detachedNext := m.session.detachedNext
	replacement := false
	if intent := m.queueIntent; intent != nil && intent.generation == m.loadGeneration {
		current = intent.entry
		detachedNext = intent.detachedNext
		if intent.replace {
			entries = intent.replacement
			replacement = true
		}
	}
	index := queueIndex(entries, current)
	if index < 0 {
		if delta > 0 && detachedNext != 0 {
			if next := queueIndex(entries, detachedNext); next >= 0 {
				return m.requestQueueLoad(entries[next], nil, false)
			}
		}
		return nil
	}
	next := index + delta
	if next < 0 || next >= len(entries) {
		return nil
	}
	return m.requestQueueLoad(entries[next], entries, replacement)
}
func (m *Model) acceptQueueTrack(path string) {
	intent := m.queueIntent
	m.queueIntent = nil
	if intent != nil && intent.generation == m.loadGeneration && intent.track.Path == path {
		if intent.replace {
			m.session.entries = slices.Clone(intent.replacement)
			m.session.removed = nil
		}
		m.session.current = intent.entry
		m.session.detachedNext = 0
		if queueIndex(m.session.entries, intent.entry) < 0 {
			m.session.current = 0
			m.session.detachedNext = intent.detachedNext
		}
	} else {
		track := library.Track{Path: path, Title: m.Audio.CurrentSong, Artist: m.Audio.Artist, Album: m.Audio.Album, Duration: m.Audio.Duration, Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}
		entry := m.newQueueEntry(track)
		m.session.entries = []queueEntry{entry}
		m.session.current = entry.id
		m.session.detachedNext = 0
		m.session.removed = nil
	}
	if index := queueIndex(m.session.entries, m.session.current); index >= 0 {
		track := &m.session.entries[index].track
		track.Duration = m.Audio.Duration
		if m.Audio.CurrentSong != "" {
			track.Title = m.Audio.CurrentSong
		}
		if m.Audio.Artist != "Unknown Artist" {
			track.Artist = m.Audio.Artist
		}
		track.Album = m.Audio.Album
	}
	m.queueEnded = false
	m.pages.cursors[1] = max(0, queueIndex(m.session.entries, m.session.current))
}
func (m *Model) failQueueLoad() {
	m.queueIntent = nil
	if m.Config.TrackSwitchMode == "immediate" {
		m.session.current = 0
		m.session.detachedNext = 0
	}
}
func (m *Model) removeQueueEntry() {
	q := &m.session
	if len(q.entries) == 0 {
		return
	}
	index := max(0, min(m.pages.cursors[1], len(q.entries)-1))
	entry := q.entries[index]
	q.removed = &entry
	q.removedAt = index
	next := uint64(0)
	if index+1 < len(q.entries) {
		next = q.entries[index+1].id
	}
	if q.current == entry.id {
		q.current = 0
		q.detachedNext = next
	}
	if q.detachedNext == entry.id {
		q.detachedNext = next
	}
	if intent := m.queueIntent; intent != nil && !intent.replace {
		if intent.entry == entry.id || intent.detachedNext == entry.id {
			intent.detachedNext = next
		}
	}
	q.entries = slices.Delete(q.entries, index, index+1)
	m.pages.cursors[1] = max(0, min(index, len(q.entries)-1))
}
func (m *Model) undoQueueRemoval() {
	q := &m.session
	if q.removed == nil {
		return
	}
	index := min(q.removedAt, len(q.entries))
	q.entries = slices.Insert(q.entries, index, *q.removed)
	q.removed = nil
	m.pages.cursors[1] = index
}
func (m *Model) reorderQueue(delta int) {
	entries := m.session.entries
	i := m.pages.cursors[1]
	j := i + delta
	if i >= 0 && i < len(entries) && j >= 0 && j < len(entries) {
		entries[i], entries[j] = entries[j], entries[i]
		m.pages.cursors[1] = j
	}
}
func (m *Model) advanceQueueAtEnd() tea.Cmd {
	if m.Loading || m.Audio.Player == nil {
		return nil
	}
	reporter, ok := m.Audio.Player.(interface{ PlaybackEnd() (bool, error) })
	if !ok {
		return nil
	}
	ended, err := reporter.PlaybackEnd()
	if !ended {
		m.queueEnded = false
		return nil
	}
	if m.queueEndSeekRevision != m.Audio.seekRevision || m.queueEndPlaybackRevision != m.Audio.playbackRevision {
		m.queueEnded = false
	}
	if !m.Audio.IsPlaying || m.queueEnded {
		return nil
	}
	m.queueEndSeekRevision = m.Audio.seekRevision
	m.queueEndPlaybackRevision = m.Audio.playbackRevision
	m.queueEnded = true
	m.Audio.IsPlaying = false
	if err != nil {
		m.Error.Set(fmt.Sprintf("Playback failed: %v", err), 120)
		return nil
	}
	cmd := m.stepQueue(1)
	if cmd == nil {
		m.Info.Set("End of session queue", 90)
	}
	return cmd
}
func trackClock(track library.Track) string {
	if track.Duration <= 0 {
		return "--:--"
	}
	return formatDuration(track.Duration)
}
func queueClock(entries []queueEntry) string {
	var total time.Duration
	for _, entry := range entries {
		if entry.track.Duration <= 0 {
			return "--:--"
		}
		total += entry.track.Duration
	}
	return formatDuration(total)
}

func trackDescription(track library.Track) string {
	if track.Artist == "" && track.Album == "" {
		return "No artist or album tags"
	}
	if track.Artist == "" {
		return track.Album
	}
	if track.Album == "" {
		return track.Artist
	}
	return track.Artist + " / " + track.Album
}
