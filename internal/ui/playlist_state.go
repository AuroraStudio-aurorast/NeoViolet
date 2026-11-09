package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

type playlistState struct {
	lists           []playlist.List
	path, err       string
	loaded, loading bool
	generation      uint64
	unavailable     map[string]string
	save            *playlistSaveJob
}
type playlistsLoadedMsg struct {
	generation  uint64
	path        string
	lists       []playlist.List
	unavailable map[string]string
	err         error
}
type playlistSaveJob struct {
	snapshot       []playlist.List
	editor         *pageEditor
	selectID       string
	run            func() playlistSavedMsg
	cursor         int
	quit           bool
	settingsQuit   bool
	originalID     string
	originalCursor int
}
type playlistSavedMsg struct {
	job         *playlistSaveJob
	unavailable map[string]string
	err         error
}

func (m *Model) loadPlaylists() tea.Cmd {
	state := &m.playlists
	if state.save != nil {
		m.Info.Set("Saving playlist…", 90)
		return nil
	}
	state.generation++
	state.loading = true
	generation := state.generation
	return func() tea.Msg {
		dir, err := config.Dir()
		var path string
		var lists []playlist.List
		if err == nil {
			path = filepath.Join(dir, "playlists.json")
			lists, err = playlist.Load(path)
		}
		return playlistsLoadedMsg{generation: generation, path: path, lists: lists, unavailable: playlistAvailability(lists), err: err}
	}
}
func (m *Model) applyPlaylists(msg playlistsLoadedMsg) {
	state := &m.playlists
	if m.exiting || msg.generation != state.generation {
		return
	}
	state.loading = false
	state.loaded = true
	state.path = msg.path
	if msg.err != nil {
		state.err = msg.err.Error()
		return
	}
	selected := ""
	if list := m.selectedPlaylist(); list != nil {
		selected = list.ID
	}
	state.err = ""
	state.lists = msg.lists
	state.unavailable = msg.unavailable
	m.selectPlaylist(selected)
}
func playlistAvailability(lists []playlist.List) map[string]string {
	unavailable := map[string]string{}
	checked := map[string]bool{}
	for _, list := range lists {
		for _, track := range list.Tracks {
			path := track.Path
			if checked[path] || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
				continue
			}
			checked[path] = true
			info, err := os.Stat(path)
			if err != nil {
				unavailable[path] = "Missing or unreadable"
			} else if !info.Mode().IsRegular() {
				unavailable[path] = "Not a regular file"
			}
		}
	}
	return unavailable
}
func (m *Model) playlistsReady() bool {
	state := &m.playlists
	switch {
	case state.save != nil:
		m.Info.Set("Saving playlist…", 90)
	case !state.loaded || state.loading:
		m.Info.Set("Loading playlists…", 90)
	case state.err != "":
		m.Error.Set("Playlists unavailable; repair playlists.json and press r to retry", 120)
	default:
		return true
	}
	return false
}
func (m *Model) selectedPlaylist() *playlist.List {
	if len(m.playlists.lists) == 0 {
		return nil
	}
	index := max(0, min(m.pages.collection, len(m.playlists.lists)-1))
	return &m.playlists.lists[index]
}
func (m *Model) selectPlaylist(id string) {
	p := m.pages
	for i, list := range m.playlists.lists {
		if list.ID == id {
			p.collection = i
			break
		}
	}
	p.collection = max(0, min(p.collection, len(m.playlists.lists)-1))
	count := 0
	if list := m.selectedPlaylist(); list != nil {
		count = len(list.Tracks)
	}
	p.savedTrack = max(0, min(p.savedTrack, count-1))
}
func (m *Model) persistPlaylists(snapshot []playlist.List, selected string) tea.Cmd {
	if !m.playlistsReady() {
		return nil
	}
	job := &playlistSaveJob{snapshot: playlist.Clone(snapshot), selectID: selected, editor: m.pages.editor, cursor: m.pages.savedTrack}
	if list := m.selectedPlaylist(); list != nil {
		job.originalID = list.ID
	}
	job.originalCursor = m.pages.savedTrack
	m.playlists.save = job
	path := m.playlists.path
	m.Info.Set("Saving playlist…", 90)
	job.run = sync.OnceValue(func() playlistSavedMsg {
		err := playlist.Save(path, job.snapshot)
		msg := playlistSavedMsg{job: job, err: err}
		if err == nil {
			msg.unavailable = playlistAvailability(job.snapshot)
		}
		return msg
	})
	return func() tea.Msg { return job.run() }
}
func (m *Model) finishPlaylistSave(msg playlistSavedMsg) tea.Cmd {
	state := &m.playlists
	if m.exiting || state.save != msg.job {
		return nil
	}
	state.save = nil
	m.Info.Visible = false
	if msg.err != nil {
		if m.configSave != nil {
			m.configSave.quit = false
		}
		m.Error.Set(fmt.Sprintf("Playlist save failed: %v", msg.err), 180)
		return nil
	}
	selected, cursor := msg.job.selectID, msg.job.cursor
	currentID := ""
	if list := m.selectedPlaylist(); list != nil {
		currentID = list.ID
	}
	if currentID != msg.job.originalID || m.pages.savedTrack != msg.job.originalCursor {
		selected, cursor = currentID, m.pages.savedTrack
	}
	state.lists = msg.job.snapshot
	state.unavailable = msg.unavailable
	m.pages.savedTrack = cursor
	m.selectPlaylist(selected)
	if msg.job.editor != nil && m.pages.editor == msg.job.editor {
		m.pages.editor = nil
	}
	m.Info.SetSuccess("Playlist saved", 90)
	if msg.job.quit {
		if msg.job.settingsQuit {
			return m.quitAfterSavedSettings()
		}
		_, cmd := runQuit(m, invocation{})
		return cmd
	}
	return nil
}
func (m *Model) flushPlaylistSave() {
	if job := m.playlists.save; job != nil {
		msg := job.run()
		job.quit = false
		_ = m.finishPlaylistSave(msg)
	}
}
