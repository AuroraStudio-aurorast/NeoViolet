package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format"
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
	events := make(chan libraryScanEvent, 8)
	m.scanning = true
	m.scanDone, m.scanTotal = 0, 0
	return func() tea.Msg {
		go func() {
			defer close(events)
			decoder := format.NewDecoder()
			result, err := library.Scan(ctx, roots, func(path string) bool { return playableExt(filepath.Ext(path)) },
				func(ctx context.Context, path string) (library.Track, error) {
					info, err := decoder.Probe(ctx, path)
					return library.Track{Title: info.Title, Artist: info.Artist, Album: info.Album, Duration: info.Duration, Format: info.Format}, err
				}, func(done, total int) {
					select {
					case events <- libraryScanEvent{generation: generation, done: done, total: total}:
					case <-ctx.Done():
					default:
					}
				})
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
	m.scanDone, m.scanTotal = msg.event.done, msg.event.total
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
	m.libraryTracks = slices.Clone(tracks)
	scopes := []string{"All tracks"}
	artists, albums := map[string]bool{}, map[string]bool{}
	for _, t := range tracks {
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
	m.pages.scope = max(0, min(m.pages.scope, len(scopes)-1))
	m.refreshLibraryView()
}
func (m *Model) refreshLibraryView() {
	p := m.pages
	selected := ""
	if p.cursors[0] >= 0 && p.cursors[0] < len(p.visible) {
		selected = p.visible[p.cursors[0]].Path
	}
	p.visible = []library.Track{}
	if len(p.scopes) == 0 {
		p.scopes = []string{"All tracks"}
	}
	query := strings.ToLower(p.filter)
	for _, t := range m.libraryTracks {
		if p.scope > 0 && t.Format != p.scopes[p.scope] {
			continue
		}
		if !strings.Contains(strings.ToLower(t.Title+" "+t.Artist+" "+t.Album), query) {
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
