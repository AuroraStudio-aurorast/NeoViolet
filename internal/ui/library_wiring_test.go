package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

func queueFixture(t *testing.T) *Model {
	t.Helper()
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 0
	tracks := []library.Track{}
	for _, name := range []string{"A.wav", "B.wav", "C.wav"} {
		tracks = append(tracks, library.Track{Path: filepath.Join(t.TempDir(), name), Title: name, Format: "WAV", Duration: time.Second})
	}
	m.installLibrary(tracks)
	return m
}
func acceptQueueLoad(t *testing.T, m *Model) {
	t.Helper()
	intent := m.queueIntent
	if intent == nil {
		t.Fatal("missing queue intent")
	}
	player := &mockPlayer{path: intent.track.Path, duration: time.Second}
	_, _ = m.Update(AudioLoadedMsg{Player: player, Path: intent.track.Path, Generation: m.loadGeneration})
}
func TestLibraryQueueCommitsOnlyAcceptedLoadAndChainsPendingNext(t *testing.T) {
	m := queueFixture(t)
	cmd := pageKey(m, tea.KeyEnter, "")
	if cmd == nil || m.session.current != 0 || len(m.session.entries) != 0 {
		t.Fatal("request prematurely committed queue")
	}
	acceptQueueLoad(t, m)
	if len(m.session.entries) != 3 || m.session.current != m.session.entries[0].id {
		t.Fatal("accepted selection did not install visible sequence")
	}
	pageKey(m, '>', ">")
	pageKey(m, '>', ">")
	if m.queueIntent.track.Title != "C.wav" {
		t.Fatal("pending Next restarted from accepted track")
	}
	acceptQueueLoad(t, m)
	if m.session.current != m.session.entries[2].id {
		t.Fatal("loaded target did not become current entry")
	}
}
func TestLibraryScanStaleResultAndRescanDoNotResetQueue(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	previous := m.session.current
	m.scanGeneration = 2
	_, _ = m.Update(libraryScannedMsg{generation: 1, result: library.Result{Tracks: []library.Track{{Title: "stale"}}}})
	if m.libraryTracks[0].Title == "stale" {
		t.Fatal("stale scan installed")
	}
	m.installLibrary([]library.Track{})
	if m.session.current != previous || len(m.session.entries) != 3 {
		t.Fatal("rescan rewrote queue")
	}
}
func TestLibraryMediaKeysUseQueueNotSeek(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	player := m.Audio.Player.(*mockPlayer)
	_, cmd := handleMediaCtlCmd(m, MediaCtlMsg{Command: mediactl.Command{Type: mediactl.CmdNext}})
	if cmd == nil || m.queueIntent.track.Title != "B.wav" || player.seekCalled {
		t.Fatal("media Next did not navigate queue")
	}
}
func TestLibraryScanCancellationDoesNotReplaceExistingSnapshot(t *testing.T) {
	m := queueFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.scanGeneration = 3
	m.scanCancel = cancel
	_, _ = m.Update(libraryScannedMsg{generation: 3, err: ctx.Err()})
	if len(m.libraryTracks) != 3 {
		t.Fatal("canceled scan discarded previous snapshot")
	}
}

type endedPlayer struct {
	mockPlayer
	ended bool
	err   error
}

func (p *endedPlayer) PlaybackEnd() (bool, error) { return p.ended, p.err }
func TestLibraryQueueAdvancesOnceAndStopsOnError(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	p := &endedPlayer{mockPlayer: mockPlayer{playing: true}, ended: true}
	m.Audio.Player = p
	m.Audio.IsPlaying = true
	cmd := m.advanceQueueAtEnd()
	if cmd == nil || m.queueIntent.track.Title != "B.wav" {
		t.Fatal("EOF did not request next")
	}
	generation := m.loadGeneration
	if m.advanceQueueAtEnd() != nil || m.loadGeneration != generation {
		t.Fatal("EOF requested twice")
	}
	m.pendingLoad.cancel()
	m.Loading = false
	m.queueIntent = nil
	m.queueEnded = false
	m.Audio.IsPlaying = true
	p.err = errors.New("decode failed")
	if m.advanceQueueAtEnd() != nil || !m.Error.Visible {
		t.Fatal("decoder error skipped to next track")
	}
}
func TestLibraryQueueFailureAndSupersededLoadKeepAcceptedIdentity(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	accepted := m.session.current
	pageKey(m, '>', ">")
	older := m.loadGeneration
	pageKey(m, '>', ">")
	stale := &mockPlayer{}
	_, _ = m.Update(AudioLoadedMsg{Generation: older, Player: stale, Path: "stale.wav"})
	if !stale.closeCalled || m.session.current != accepted {
		t.Fatal("superseded result changed queue")
	}
	_, _ = m.Update(AudioLoadFailedMsg{Generation: m.loadGeneration, Message: "bad", Timer: 90})
	if m.queueIntent != nil || m.session.current != accepted || m.Audio.Player == nil {
		t.Fatal("failed replacement changed accepted queue")
	}
}
func TestLibraryQueueDuplicateIdentityRemovalAndUndo(t *testing.T) {
	m := queueFixture(t)
	track := m.pages.visible[0]
	m.appendQueue(track)
	m.appendQueue(track)
	if m.session.entries[0].id == m.session.entries[1].id {
		t.Fatal("duplicate paths share queue identity")
	}
	m.session.current = m.session.entries[0].id
	m.pages.cursors[1] = 0
	m.removeQueueEntry()
	if m.session.current != 0 || m.session.detachedNext != m.session.entries[0].id {
		t.Fatal("removing current entry lost next identity")
	}
	m.undoQueueRemoval()
	if len(m.session.entries) != 2 {
		t.Fatal("undo failed")
	}
}
func TestLibraryRootsStayStructuredAndTriggerScanAfterSave(t *testing.T) {
	m := wiredSettingsModel(t)
	first := t.TempDir()
	second := filepath.Join(t.TempDir(), "space, 雨")
	if err := os.Mkdir(second, 0700); err != nil {
		t.Fatal(err)
	}
	field := settingIndex(m, "Music folders")
	if err := m.pages.addLibraryRoot(field, first); err != nil {
		t.Fatal(err)
	}
	if err := m.pages.addLibraryRoot(field, second); err != nil {
		t.Fatal(err)
	}
	if len(m.Config.Library.Roots) != 0 || len(m.pages.draft.Library.Roots) != 2 {
		t.Fatal("roots draft aliased applied config")
	}
	cmd := pageKey(m, 's', "s")
	_, scan := m.Update(cmd())
	if len(m.Config.Library.Roots) != 2 || m.Config.Library.Roots[1] != second || scan == nil || !m.scanning {
		t.Fatal("saved roots did not start scan")
	}
	if m.scanCancel != nil {
		m.scanCancel()
	}
}

func TestLibraryDirectoryPickerAddsCommaPathToDraft(t *testing.T) {
	m := wiredSettingsModel(t)
	parent := t.TempDir()
	dir := filepath.Join(parent, "music, 雨")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	index := settingIndex(m, "Music folders")
	m.pages.cursors[3] = index
	pageKey(m, tea.KeyEnter, "")
	if !m.pages.rootEditor {
		t.Fatal("root editor did not open")
	}
	browserResult(t, m, m.openFileBrowser("directory", index, parent))
	browserResult(t, m, pageKey(m, tea.KeyEnter, ""))
	browserResult(t, m, pageKey(m, 's', "s"))
	if m.pages.browser != nil || len(m.pages.draft.Library.Roots) != 1 || m.pages.draft.Library.Roots[0] != dir || len(m.Config.Library.Roots) != 0 {
		t.Fatal("directory selection was not structured draft data")
	}
	pageKey(m, tea.KeyEscape, "")
	cmd := pageKey(m, 's', "s")
	_, _ = m.Update(cmd())
	if m.scanCancel != nil {
		m.scanCancel()
	}
	loaded, err := config.Load()
	if err != nil || len(loaded.Library.Roots) != 1 || loaded.Library.Roots[0] != dir {
		t.Fatal("roots lost after restart")
	}
}
func TestLibraryScanProgressCompletesAfterChangingTab(t *testing.T) {
	m := wiredSettingsModel(t)
	root := t.TempDir()
	m.Config.Library.Roots = []string{root}
	if err := os.WriteFile(filepath.Join(root, "track.wav"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := m.rescanLibrary()
	m.UI.ActiveTab = 2
	m.UI.Focus = FocusFooter
	for cmd != nil {
		msg := cmd()
		_, cmd = m.Update(msg)
	}
	if m.scanning || len(m.libraryTracks) != 1 || len(m.scanIssues) != 1 {
		t.Fatal("scan result was lost on a different page")
	}
}

func TestLibraryRemovingPendingTargetRetainsSuccessorAfterAcceptance(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	pageKey(m, '>', ">")
	m.pages.cursors[1] = 1
	m.removeQueueEntry()
	acceptQueueLoad(t, m)
	if m.session.current != 0 || m.session.detachedNext != m.session.entries[1].id {
		t.Fatal("pending removal lost successor")
	}
	if m.stepQueue(1) == nil || m.queueIntent.track.Title != "C.wav" {
		t.Fatal("cannot advance after removed target accepted")
	}
}
func TestLibraryDraftRevertedPathsAndIntegersAreClean(t *testing.T) {
	m := wiredSettingsModel(t)
	i := settingIndex(m, "Seek step")
	if err := m.pages.setDraftField(i, "13"); err != nil {
		t.Fatal(err)
	}
	if m.pages.dirty {
		t.Fatal("unchanged integer remained dirty")
	}
	i = settingIndex(m, "Music folders")
	if err := m.pages.addLibraryRoot(i, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if m.pages.settings[i].value != "1 folders" {
		t.Fatal("folder count does not display typed roots")
	}
	if err := m.pages.applyDraftValue(i, []string{}); err != nil {
		t.Fatal(err)
	}
	if m.pages.dirty {
		t.Fatal("empty roots differ from empty baseline")
	}
}
func TestLibraryQueueCanFinishAgainAfterRewind(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	p := &endedPlayer{mockPlayer: mockPlayer{playing: true}, ended: true}
	m.Audio.Player = p
	m.Audio.IsPlaying = true
	m.queueEnded = true
	p.ended = false
	_ = m.advanceQueueAtEnd()
	p.ended = true
	if m.advanceQueueAtEnd() == nil {
		t.Fatal("rewound playback could not advance at its second end")
	}
}

func TestLibraryNextAfterDeletingPendingTargetUsesItsSuccessor(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	pageKey(m, '>', ">")
	m.pages.cursors[1] = 1
	m.removeQueueEntry()
	if m.stepQueue(1) == nil || m.queueIntent.track.Title != "C.wav" {
		t.Fatal("pending removed target blocked Next")
	}
}

func TestLibraryCompletionRearmsOnReplayWithoutAnIntermediateTick(t *testing.T) {
	m := queueFixture(t)
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	p := &endedPlayer{mockPlayer: mockPlayer{playing: false}, ended: true}
	m.Audio.Player = p
	m.queueEnded = true
	m.Audio.IsPlaying = false
	if err := m.Audio.SetPlaying(true); err != nil {
		t.Fatal(err)
	}
	if m.advanceQueueAtEnd() == nil {
		t.Fatal("fast replay ended before the next tick and kept the previous EOF guard")
	}
}

func TestLibraryAcceptedPlaybackFillsUnknownQueueDuration(t *testing.T) {
	m := queueFixture(t)
	m.pages.visible[0].Duration = 0
	pageKey(m, tea.KeyEnter, "")
	acceptQueueLoad(t, m)
	if m.session.entries[0].track.Duration != time.Second {
		t.Fatal("accepted duration was not projected into queue")
	}
}
