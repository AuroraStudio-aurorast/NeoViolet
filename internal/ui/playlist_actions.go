package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

func (m *Model) queueTracks() []library.Track {
	tracks := make([]library.Track, 0, len(m.session.entries))
	for _, entry := range m.session.entries {
		tracks = append(tracks, entry.track)
	}
	return tracks
}
func (m *Model) editPlaylist(purpose string) tea.Cmd {
	if !m.playlistsReady() {
		return nil
	}
	p := m.pages
	title, value := "Create empty playlist", ""
	var tracks []library.Track
	id := ""
	switch purpose {
	case "playlist_queue":
		title = "Save session queue as playlist"
		tracks = m.queueTracks()
	case "playlist_rename":
		list := m.selectedPlaylist()
		if list == nil {
			return nil
		}
		title = "Rename playlist"
		value = list.Name
		id = list.ID
	}
	cmd := p.openEditor(title, purpose, value, 0)
	p.editor.playlistID = id
	p.editor.tracks = tracks
	return cmd
}
func (m *Model) submitPlaylistName() tea.Cmd {
	if !m.playlistsReady() {
		return nil
	}
	editor := m.pages.editor
	list, err := playlist.New(editor.input.Value(), editor.tracks)
	if err != nil {
		m.Error.Set(err.Error(), 120)
		return nil
	}
	snapshot := slices.Clone(m.playlists.lists)
	if editor.purpose == "playlist_rename" {
		for i := range snapshot {
			if snapshot[i].ID == editor.playlistID {
				snapshot[i].Name = list.Name
				return m.persistPlaylists(snapshot, editor.playlistID, nil)
			}
		}
		m.Error.Set("Playlist no longer exists; cancel and refresh", 120)
		return nil
	}
	snapshot = append(snapshot, list)
	return m.persistPlaylists(snapshot, list.ID, list.Tracks)
}
func (m *Model) playSavedPlaylist(index int) tea.Cmd {
	if !m.playlistsReady() {
		return nil
	}
	list := m.selectedPlaylist()
	if list == nil || len(list.Tracks) == 0 {
		m.Info.Set("This playlist is empty; A adds the session queue", 90)
		return nil
	}
	entries := make([]queueEntry, 0, len(list.Tracks))
	for _, track := range list.Tracks {
		entries = append(entries, m.newQueueEntry(track))
	}
	index = max(0, min(index, len(entries)-1))
	return m.requestQueueLoad(entries[index], entries, true)
}
func (m *Model) appendSavedPlaylist() {
	if m.queueIntent != nil && m.queueIntent.replace && m.queueIntent.generation == m.loadGeneration {
		m.Info.Set("Wait for the new session queue to finish loading, then append", 120)
		return
	}
	if !m.playlistsReady() {
		return
	}
	list := m.selectedPlaylist()
	if list == nil {
		return
	}
	for _, track := range list.Tracks {
		m.appendQueue(track)
	}
	m.Info.Set(fmt.Sprintf("Appended %d tracks to session queue", len(list.Tracks)), 90)
}
func (m *Model) changePlaylist(action string, delta int) tea.Cmd {
	if !m.playlistsReady() {
		return nil
	}
	selected := m.selectedPlaylist()
	if selected == nil {
		return nil
	}
	snapshot := slices.Clone(m.playlists.lists)
	list := &snapshot[m.pages.collection]
	index := m.pages.savedTrack
	var added []library.Track
	switch action {
	case "append":
		tracks := m.queueTracks()
		// Normalize references before adding them to an existing list too.
		captured, err := playlist.New(list.Name, tracks)
		if err != nil {
			m.Error.Set(err.Error(), 120)
			return nil
		}
		if len(tracks) == 0 {
			m.Info.Set("Session queue is empty", 90)
			return nil
		}
		added = captured.Tracks
		// Force a new backing array even when the published list has spare capacity.
		list.Tracks = append(list.Tracks[:len(list.Tracks):len(list.Tracks)], added...)
	case "remove":
		if len(list.Tracks) == 0 {
			return nil
		}
		list.Tracks = slices.Delete(slices.Clone(list.Tracks), index, index+1)
	case "reorder":
		next := index + delta
		if next < 0 || next >= len(list.Tracks) {
			return nil
		}
		list.Tracks = slices.Clone(list.Tracks)
		list.Tracks[index], list.Tracks[next] = list.Tracks[next], list.Tracks[index]
	case "delete":
		snapshot = slices.Delete(snapshot, m.pages.collection, m.pages.collection+1)
	}
	cmd := m.persistPlaylists(snapshot, selected.ID, added)
	if action == "reorder" && m.playlists.save != nil {
		m.playlists.save.cursor = index + delta
	}
	return cmd
}
func (m *Model) updatePlaylistKey(k string) (bool, tea.Cmd) {
	p := m.pages
	if k == "v" {
		p.saved = !p.saved
		p.detail = false
		return true, nil
	}
	if k == "s" {
		return true, m.editPlaylist("playlist_queue")
	}
	if p.saved {
		switch k {
		case "r":
			return true, m.loadPlaylists()
		case "c":
			return true, m.editPlaylist("playlist_create")
		case "e":
			return true, m.editPlaylist("playlist_rename")
		case "P":
			return true, m.playSavedPlaylist(0)
		case "a":
			m.appendSavedPlaylist()
		case "A":
			return true, m.changePlaylist("append", 0)
		case "d":
			if !m.playlistsReady() {
				return true, nil
			}
			if p.detail {
				return true, m.changePlaylist("remove", 0)
			}
			if list := m.selectedPlaylist(); list != nil {
				p.deletePlaylist = list.ID
			}
		case "J", "K":
			if p.detail {
				delta := 1
				if k == "K" {
					delta = -1
				}
				return true, m.changePlaylist("reorder", delta)
			}
		default:
			return false, nil
		}
		return true, nil
	}
	switch k {
	case "d":
		m.removeQueueEntry()
	case "u":
		m.undoQueueRemoval()
	case "J", "K":
		delta := 1
		if k == "K" {
			delta = -1
		}
		m.reorderQueue(delta)
	default:
		return false, nil
	}
	return true, nil
}
func (m *Model) updatePlaylistDelete(msg tea.KeyPressMsg) tea.Cmd {
	id := m.pages.deletePlaylist
	if msg.String() == "esc" || msg.String() == "ctrl+g" || msg.String() == "n" {
		m.pages.deletePlaylist = ""
		return nil
	}
	if msg.String() != "y" {
		return nil
	}
	m.pages.deletePlaylist = ""
	if list := m.selectedPlaylist(); list == nil || list.ID != id {
		return nil
	}
	return m.changePlaylist("delete", 0)
}
