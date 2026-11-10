package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

type libraryScannedMsg struct {
	generation uint64
	result     library.Result
	err        error
}
type libraryScanEvent struct {
	generation  uint64
	done, total int
	final       *libraryScannedMsg
	tracks      []library.Track
}

func (m *Model) rescanLibrary() tea.Cmd {
	if m.scanCancel != nil {
		m.scanCancel()
	}
	m.scanGeneration++
	if len(m.Config.Library.Roots) == 0 {
		m.scanning = false
		m.scanIssues = []string{}
		m.installLibrary([]library.Track{})
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.scanCancel = cancel
	roots := slices.Clone(m.Config.Library.Roots)
	generation := m.scanGeneration
	events := make(chan libraryScanEvent, 2)
	m.scanning = true
	m.scanDone, m.scanTotal = 0, 0
	empty := len(m.libraryTracks) == 0
	return func() tea.Msg {
		go func() {
			defer close(events)
			cachePath := ""
			var cached *library.Cache
			if dir, err := config.CacheDir(); err == nil {
				cachePath = filepath.Join(filepath.Dir(dir), "library.json")
				cached, _ = library.LoadCache(cachePath)
			}
			publish := func(tracks []library.Track) {
				select {
				case events <- libraryScanEvent{generation: generation, tracks: tracks}:
				case <-ctx.Done():
				default:
				}
			}
			restored := empty && cached.MatchesRoots(roots) && len(cached.Entries) > 0
			if restored {
				publish(cached.Tracks())
			}
			var partial func([]library.Track)
			// Existing results remain selectable under artist/album filters.
			// Only a first scan without a snapshot needs placeholder updates.
			if empty && !restored {
				partial = publish
			}
			decoder := format.NewDecoder()
			extensions := map[string]bool{}
			include := func(path string) bool {
				ext := strings.ToLower(filepath.Ext(path))
				allowed, ok := extensions[ext]
				if !ok {
					allowed = playableExt(ext)
					extensions[ext] = allowed
				}
				return allowed
			}
			result, err := library.Scan(ctx, roots, include,
				func(ctx context.Context, path string) (library.Track, error) {
					info, err := decoder.Probe(ctx, path)
					return library.Track{Title: info.Title, Artist: info.Artist, Album: info.Album, Duration: info.Duration, Format: info.Format}, err
				}, func(done, total int) {
					select {
					case events <- libraryScanEvent{generation: generation, done: done, total: total}:
					case <-ctx.Done():
					default:
					}
				}, library.ScanOptions{Cache: cached, Publish: partial})
			if err == nil && cachePath != "" {
				_ = library.SaveCache(ctx, cachePath, result.Cache)
			}
			select {
			case events <- libraryScanEvent{generation: generation, final: &libraryScannedMsg{generation: generation, result: result, err: err}}:
			case <-ctx.Done():
			}
		}()
		return readLibraryEvent(ctx, events)
	}
}
func readLibraryEvent(ctx context.Context, events <-chan libraryScanEvent) tea.Msg {
	select {
	case <-ctx.Done():
		return nil
	case event, ok := <-events:
		if !ok {
			return nil
		}
		if event.final != nil {
			return *event.final
		}
		return libraryProgressMsg{event: event, ctx: ctx, events: events}
	}
}

type libraryProgressMsg struct {
	event  libraryScanEvent
	ctx    context.Context
	events <-chan libraryScanEvent
}

func (m *Model) applyLibraryProgress(msg libraryProgressMsg) tea.Cmd {
	if m.exiting || msg.event.generation != m.scanGeneration {
		return nil
	}
	if msg.event.tracks != nil {
		m.installLibrary(msg.event.tracks)
	} else {
		m.scanDone, m.scanTotal = msg.event.done, msg.event.total
	}
	return func() tea.Msg { return readLibraryEvent(msg.ctx, msg.events) }
}
func (m *Model) applyLibraryScan(msg libraryScannedMsg) tea.Cmd {
	if m.exiting || msg.generation != m.scanGeneration {
		return nil
	}
	m.scanning = false
	if msg.err != nil {
		if msg.err != context.Canceled {
			m.Error.Set(fmt.Sprintf("Library scan: %v", msg.err), 90)
		}
		return nil
	}
	m.scanIssues = msg.result.Issues
	m.installLibrary(msg.result.Tracks)
	if len(m.scanIssues) > 0 {
		m.Info.Set(fmt.Sprintf("Library: %d files, %d scan issues (r to retry)", len(m.libraryTracks), len(m.scanIssues)), 120)
	}
	return nil
}
func (m *Model) installLibrary(tracks []library.Track) {
	selectedScope := "All tracks"
	if m.pages.scope >= 0 && m.pages.scope < len(m.pages.scopes) {
		selectedScope = m.pages.scopes[m.pages.scope]
	}
	m.libraryTracks = slices.Clone(tracks)
	m.librarySearch = make([]string, len(tracks))
	scopes := []string{"All tracks"}
	artists, albums := map[string]bool{}, map[string]bool{}
	for i, t := range tracks {
		// Normalize once per snapshot, not once per track for every filter edit.
		m.librarySearch[i] = strings.ToLower(t.Title + " " + t.Artist + " " + t.Album)
		if t.Format != "" && !slices.Contains(scopes, t.Format) {
			scopes = append(scopes, t.Format)
		}
		if t.Artist != "" {
			artists[t.Artist] = true
		}
		if t.Album != "" {
			albums[t.Album] = true
		}
	}
	slices.Sort(scopes[1:])
	m.pages.scopes = scopes
	m.pages.artists, m.pages.albums = len(artists), len(albums)
	m.pages.scope = max(0, slices.Index(scopes, selectedScope))
	m.refreshLibraryView()
}
func (m *Model) refreshLibraryView() {
	p := m.pages
	selected := ""
	if p.cursors[0] >= 0 && p.cursors[0] < len(p.visible) {
		selected = p.visible[p.cursors[0]].Path
	}
	p.visible = p.visible[:0]
	if len(p.scopes) == 0 {
		p.scopes = []string{"All tracks"}
	}
	query := strings.ToLower(p.filter)
	for i, t := range m.libraryTracks {
		if p.scope > 0 && t.Format != p.scopes[p.scope] {
			continue
		}
		if query != "" && !strings.Contains(m.librarySearch[i], query) {
			continue
		}
		p.visible = append(p.visible, t)
	}
	p.cursors[0] = max(0, min(p.cursors[0], len(p.visible)-1))
	for i, t := range p.visible {
		if t.Path == selected {
			p.cursors[0] = i
			break
		}
	}
}
