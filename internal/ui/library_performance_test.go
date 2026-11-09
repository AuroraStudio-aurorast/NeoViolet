package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

func TestLibraryProgressInstallsRowsBeforeCompletionAndRejectsStaleGeneration(t *testing.T) {
	m := wiredSettingsModel(t)
	m.scanGeneration = 2
	m.scanning = true
	tracks := []library.Track{{Path: "/music/a.wav", Title: "A"}}
	_, _ = m.Update(libraryProgressMsg{event: libraryScanEvent{generation: 1, tracks: tracks}})
	if len(m.libraryTracks) != 0 {
		t.Fatal("stale partial snapshot installed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan libraryScanEvent)
	_, cmd := m.Update(libraryProgressMsg{event: libraryScanEvent{generation: 2, tracks: tracks}, ctx: ctx, events: events})
	if len(m.libraryTracks) != 1 || !m.scanning || cmd == nil {
		t.Fatal("partial snapshot not available during scan")
	}
}
func TestLibraryRescanUsesRebuildableDiskCache(t *testing.T) {
	m := wiredSettingsModel(t)
	m.Config.Library.Roots = []string{t.TempDir()}
	path := filepath.Join(m.Config.Library.Roots[0], "cached.wav")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	// A successful custom probe lets this test distinguish reuse from failed reprobe.
	result, err := library.Scan(context.Background(), m.Config.Library.Roots, func(string) bool { return true }, func(context.Context, string) (library.Track, error) {
		return library.Track{Title: "Cached title", Format: "WAV"}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(filepath.Dir(cacheDir), "library.json")
	if err = library.SaveCache(context.Background(), store, result.Cache); err != nil {
		t.Fatal(err)
	}
	cmd := m.rescanLibrary()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if len(m.libraryTracks) != 1 || m.libraryTracks[0].Title != "Cached title" || len(m.scanIssues) != 0 {
		t.Fatal("unchanged cache was not reused")
	}
	if err = os.WriteFile(store, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = m.rescanLibrary()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if len(m.libraryTracks) != 1 || len(m.scanIssues) != 1 {
		t.Fatal("corrupt cache not rebuilt from source")
	}
	if m.scanCancel != nil {
		m.scanCancel()
	}
}
func TestLibraryEmptyFilterReusesVisibleCapacityAndPreservesSelection(t *testing.T) {
	m := queueFixture(t)
	m.pages.cursors[0] = 1
	m.refreshLibraryView()
	before := &m.pages.visible[0]
	m.refreshLibraryView()
	if &m.pages.visible[0] != before || m.pages.cursors[0] != 1 {
		t.Fatal("empty filter recreated slice or lost selection")
	}
	m.pages.filter = "b.WAV"
	m.refreshLibraryView()
	if len(m.pages.visible) != 1 || m.pages.visible[0].Title != "B.wav" {
		t.Fatal("normalized filtering changed")
	}
	m.pages.filter = ""
	m.refreshLibraryView()
	if len(m.pages.visible) != 3 || m.pages.cursors[0] != 1 {
		t.Fatal("clearing filter lost selected identity")
	}
}

func TestLibraryPartialSnapshotsKeepSelectedFormatIdentity(t *testing.T) {
	m := queueFixture(t)
	m.pages.scope = 1
	m.refreshLibraryView()
	m.installLibrary([]library.Track{{Path: "/a.flac", Title: "FLAC", Format: "FLAC"}, {Path: "/a.wav", Title: "WAV", Format: "WAV"}})
	if m.pages.scopes[m.pages.scope] != "WAV" || len(m.pages.visible) != 1 || m.pages.visible[0].Format != "WAV" {
		t.Fatal("snapshot shifted selected format")
	}
	m.installLibrary([]library.Track{{Path: "/a.flac", Title: "FLAC", Format: "FLAC"}})
	if m.pages.scope != 0 {
		t.Fatal("disappeared format selected unrelated scope")
	}
}

func TestLibraryRescanRetainsFilteredSelectionUntilFinalSnapshot(t *testing.T) {
	m := wiredSettingsModel(t)
	root := t.TempDir()
	m.Config.Library.Roots = []string{root}
	for _, name := range []string{"a.wav", "b.wav"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tracks := []library.Track{{Path: filepath.Join(root, "a.wav"), Title: "A", Artist: "Artist", Format: "WAV"}, {Path: filepath.Join(root, "b.wav"), Title: "B", Artist: "Artist", Format: "WAV"}}
	m.installLibrary(tracks)
	m.pages.filter = "artist"
	m.refreshLibraryView()
	m.pages.cursors[0] = 1
	cmd := m.rescanLibrary()
	msg := cmd()
	if partial, ok := msg.(libraryProgressMsg); ok && partial.event.tracks != nil {
		t.Fatal("rescan published placeholders over filtered results")
	}
	_, _ = m.Update(msg)
	if len(m.pages.visible) != 2 || m.pages.cursors[0] != 1 {
		t.Fatal("rescan progress lost filtered selection")
	}
	if m.scanCancel != nil {
		m.scanCancel()
	}
}
