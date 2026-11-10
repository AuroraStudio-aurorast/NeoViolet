package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

func savedFixture(t *testing.T) *Model {
	t.Helper()
	m := queueFixture(t)
	cmd := m.loadPlaylists()
	if cmd == nil {
		t.Fatal("missing load")
	}
	_, _ = m.Update(cmd())
	m.UI.ActiveTab = 1
	return m
}
func playlistResult(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing playlist command")
	}
	_, _ = m.Update(cmd())
}
func namePlaylist(t *testing.T, m *Model, name string) tea.Cmd {
	t.Helper()
	if m.pages.editor == nil {
		t.Fatal("missing name editor")
	}
	m.pages.editor.input.SetValue(name)
	return pageKey(m, tea.KeyEnter, "")
}
func TestSavedQueueCapturePublishesOnlyAfterDiskSuccess(t *testing.T) {
	m := savedFixture(t)
	track := m.pages.visible[0]
	m.appendQueue(track)
	m.appendQueue(track)
	pageKey(m, 's', "s")
	m.session.entries[0].track.Title = "Changed later"
	cmd := namePlaylist(t, m, "Evening, 雨")
	if len(m.playlists.lists) != 0 || m.playlists.save == nil {
		t.Fatal("published before save")
	}
	playlistResult(t, m, cmd)
	got, err := playlist.Load(m.playlists.path)
	if err != nil || len(got) != 1 || len(got[0].Tracks) != 2 || got[0].Tracks[0].Title != "A.wav" {
		t.Fatalf("did not save independent capture: %#v %v", got, err)
	}
	if m.pages.editor != nil {
		t.Fatal("editor remained after success")
	}
	// A fresh model reads persisted lists even with an empty library.
	other := NewModel("", m.Config)
	playlistResult(t, other, other.loadPlaylists())
	if len(other.playlists.lists) != 1 || other.playlists.lists[0].Name != "Evening, 雨" {
		t.Fatal("restart lost list")
	}
}
func TestSavedCreateRenameReorderRemoveAndConfirmDelete(t *testing.T) {
	m := savedFixture(t)
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	playlistResult(t, m, namePlaylist(t, m, "Empty"))
	id := m.playlists.lists[0].ID
	pageKey(m, 'e', "e")
	playlistResult(t, m, namePlaylist(t, m, "Renamed"))
	if m.playlists.lists[0].ID != id {
		t.Fatal("rename changed identity")
	}
	pageKey(m, tea.KeyEnter, "")
	if !m.pages.detail {
		t.Fatal("detail not opened")
	}
	m.appendQueue(m.pages.visible[0])
	m.appendQueue(m.pages.visible[1])
	playlistResult(t, m, pageKey(m, 'A', "A"))
	playlistResult(t, m, pageKey(m, 'J', "J"))
	if m.playlists.lists[0].Tracks[0].Title != "B.wav" {
		t.Fatal("saved order unchanged")
	}
	playlistResult(t, m, pageKey(m, 'd', "d"))
	if len(m.playlists.lists[0].Tracks) != 1 {
		t.Fatal("entry not removed")
	}
	pageKey(m, tea.KeyEscape, "")
	pageKey(m, 'd', "d")
	pageKey(m, tea.KeyEscape, "")
	if len(m.playlists.lists) != 1 {
		t.Fatal("cancel deleted list")
	}
	pageKey(m, 'd', "d")
	playlistResult(t, m, pageKey(m, 'y', "y"))
	disk, err := playlist.Load(m.playlists.path)
	if err != nil || len(disk) != 0 {
		t.Fatal("deletion not persisted")
	}
}
func TestSavedPlaybackAndAppendUseIndependentQueueIdentities(t *testing.T) {
	m := savedFixture(t)
	list, _ := playlist.New("A", m.pages.visible)
	m.playlists.lists = []playlist.List{list}
	m.appendQueue(m.pages.visible[0])
	previous := m.session.entries[0].id
	m.session.current = previous
	pageKey(m, 'v', "v")
	pageKey(m, 'a', "a")
	if len(m.session.entries) != 4 || m.session.current != previous || m.session.entries[1].id == previous {
		t.Fatal("append replaced audio or reused identities")
	}
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, tea.KeyDown, "")
	cmd := pageKey(m, tea.KeyEnter, "")
	if cmd == nil || m.queueIntent.track.Title != "B.wav" || len(m.session.entries) != 4 {
		t.Fatal("detail selection did not request transactional queue load")
	}
	acceptQueueLoad(t, m)
	if len(m.session.entries) != 3 || m.session.current != m.session.entries[1].id {
		t.Fatal("saved sequence not accepted")
	}
}
func TestSavedWriteFailureRetainsOldStateAndNameForRetry(t *testing.T) {
	m := savedFixture(t)
	m.playlists.path = t.TempDir()
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	playlistResult(t, m, namePlaylist(t, m, "Retry me"))
	if len(m.playlists.lists) != 0 || m.pages.editor == nil || m.pages.editor.input.Value() != "Retry me" || !m.Error.Visible {
		t.Fatal("failed save discarded name or published changes")
	}
	m.playlists.path = filepath.Join(t.TempDir(), "playlists.json")
	playlistResult(t, m, pageKey(m, tea.KeyEnter, ""))
	if len(m.playlists.lists) != 1 {
		t.Fatal("retry failed")
	}
}
func TestSavedCorruptStoreBlocksMutationAndRefreshRecovers(t *testing.T) {
	m := savedFixture(t)
	if err := os.MkdirAll(filepath.Dir(m.playlists.path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.playlists.path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	playlistResult(t, m, m.loadPlaylists())
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	if m.pages.editor != nil || m.playlists.err == "" {
		t.Fatal("unreadable store editable")
	}
	data, _ := os.ReadFile(m.playlists.path)
	if string(data) != "{" {
		t.Fatal("corrupt file overwritten")
	}
	if err := os.Remove(m.playlists.path); err != nil {
		t.Fatal(err)
	}
	playlistResult(t, m, pageKey(m, 'r', "r"))
	pageKey(m, 'c', "c")
	playlistResult(t, m, namePlaylist(t, m, "Recovered"))
	if len(m.playlists.lists) != 1 {
		t.Fatal("refresh failed to recover")
	}
}
func TestSavedStaleLoadsAndWritesCannotOverwriteNewerState(t *testing.T) {
	m := savedFixture(t)
	old := m.loadPlaylists()
	current := m.loadPlaylists()
	oldMsg := old()
	playlistResult(t, m, current)
	m.playlists.lists = []playlist.List{{ID: "current", Name: "Current"}}
	_, _ = m.Update(oldMsg)
	if len(m.playlists.lists) != 1 {
		t.Fatal("stale read replaced current data")
	}
	pageKey(m, 'v', "v")
	pageKey(m, 'e', "e")
	cmd := namePlaylist(t, m, "New")
	msg := cmd()
	_, _ = m.Update(msg)
	m.playlists.lists[0].Name = "Later"
	_, _ = m.Update(msg)
	if m.playlists.lists[0].Name != "Later" {
		t.Fatal("duplicate write completion reapplied")
	}
}
func TestSavedMissingReferenceSurvivesRefreshAndFailedPlayback(t *testing.T) {
	m := savedFixture(t)
	list, _ := playlist.New("Missing", []library.Track{{Path: filepath.Join(t.TempDir(), "gone.wav"), Title: "Gone"}})
	if err := playlist.Save(m.playlists.path, []playlist.List{list}); err != nil {
		t.Fatal(err)
	}
	playlistResult(t, m, m.loadPlaylists())
	pageKey(m, 'v', "v")
	pageKey(m, tea.KeyEnter, "")
	body := stripANSI(renderContent(m, m.layoutPlan()))
	if !strings.Contains(body, "Missing") {
		t.Fatal("missing reference not marked")
	}
	m.appendQueue(m.pages.visible[0])
	previous := m.session.entries[0].id
	m.session.current = previous
	cmd := pageKey(m, tea.KeyEnter, "")
	playlistResult(t, m, cmd)
	if len(m.playlists.lists[0].Tracks) != 1 || m.session.current != previous || len(m.session.entries) != 1 {
		t.Fatal("missing playback destroyed store or queue")
	}
}
func TestSavedQuitWaitsForWriterAndFailedSaveCancelsQuit(t *testing.T) {
	m := savedFixture(t)
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	cmd := namePlaylist(t, m, "Pending")
	if !m.confirmSettingsQuit() || m.exiting {
		t.Fatal("quit did not defer pending write")
	}
	_, quit := m.Update(cmd())
	if quit == nil || !m.exiting {
		t.Fatal("successful save did not complete deferred quit")
	}
	m = savedFixture(t)
	m.playlists.path = t.TempDir()
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	cmd = namePlaylist(t, m, "Failed")
	if !m.confirmSettingsQuit() {
		t.Fatal("quit not deferred")
	}
	playlistResult(t, m, cmd)
	if m.exiting || m.pages.editor == nil {
		t.Fatal("failed save quit without retaining retry")
	}
}
func TestSavedScrollingAndMinimumLayouts(t *testing.T) {
	m := savedFixture(t)
	m.pages.saved = true
	for range 25 {
		list, _ := playlist.New("Collection 雨", []library.Track{{Path: "/missing.wav", Title: "Track 雨"}})
		m.playlists.lists = append(m.playlists.lists, list)
	}
	pageKey(m, 'G', "G")
	if m.pages.collection != 24 {
		t.Fatal("collection cursor clamped to sample length")
	}
	for _, size := range [][2]int{{120, 30}, {80, 24}, {68, 17}} {
		m.UI.Width, m.UI.Height = size[0], size[1]
		for _, detail := range []bool{false, true} {
			m.pages.detail = detail
			plan := m.layoutPlan()
			frame := renderContent(m, plan)
			if !strings.Contains(stripANSI(frame), "Collection") {
				t.Fatal("selected list not visible")
			}
			if lipgloss.Height(frame) != plan.ContentHeight {
				t.Fatal("height overflow")
			}
			for _, row := range strings.Split(frame, "\n") {
				if lipgloss.Width(row) != plan.ContentWidth {
					t.Fatal("width overflow")
				}
			}
		}
	}
}

func TestSavedReorderFollowsMovedTrackAfterSuccessfulSave(t *testing.T) {
	m := savedFixture(t)
	list, _ := playlist.New("Order", m.pages.visible)
	m.playlists.lists = []playlist.List{list}
	m.pages.saved = true
	m.pages.detail = true
	cmd := pageKey(m, 'J', "J")
	if m.pages.savedTrack != 0 {
		t.Fatal("cursor moved before disk success")
	}
	playlistResult(t, m, cmd)
	if m.pages.savedTrack != 1 {
		t.Fatal("cursor did not follow reordered track")
	}
	playlistResult(t, m, pageKey(m, 'd', "d"))
	if m.playlists.lists[0].Tracks[0].Title != "B.wav" || len(m.playlists.lists[0].Tracks) != 2 {
		t.Fatal("remove targeted wrong track after reorder")
	}
}
func TestSavedTransientQueueCannotBePersisted(t *testing.T) {
	m := savedFixture(t)
	m.appendQueue(library.Track{Path: "stdin", Title: "stdin"})
	pageKey(m, 's', "s")
	if cmd := namePlaylist(t, m, "Pipe"); cmd != nil || m.playlists.save != nil || !m.Error.Visible {
		t.Fatal("stdin saved as durable track")
	}
	if _, err := os.Stat(m.playlists.path); !os.IsNotExist(err) {
		t.Fatal("invalid reference created store")
	}
}

func TestSavedCleanupFinishesWriterEvenBeforeCommandStarts(t *testing.T) {
	m := savedFixture(t)
	pageKey(m, 'v', "v")
	pageKey(m, 'c', "c")
	cmd := namePlaylist(t, m, "Shutdown")
	path := m.playlists.path
	m.Close()
	disk, err := playlist.Load(path)
	if err != nil || len(disk) != 1 || disk[0].Name != "Shutdown" {
		t.Fatalf("shutdown lost write: %v %v", disk, err)
	}
	_, _ = m.Update(cmd())
	if m.playlists.save != nil {
		t.Fatal("late completion resurrected job")
	}
}

func TestSavedAppendDoesNotReportSuccessThenDisappearDuringQueueReplacement(t *testing.T) {
	m := savedFixture(t)
	list, _ := playlist.New("A", m.pages.visible)
	m.playlists.lists = []playlist.List{list}
	m.pages.saved = true
	m.appendQueue(m.pages.visible[0])
	pageKey(m, 'P', "P")
	pageKey(m, 'a', "a")
	if len(m.session.entries) != 1 || !strings.Contains(m.Info.Message, "Wait") {
		t.Fatal("append reported success against queue that will be replaced")
	}
	acceptQueueLoad(t, m)
	pageKey(m, 'a', "a")
	if len(m.session.entries) != 6 {
		t.Fatal("append did not resume after accepted load")
	}
}

func TestSavedWriteCompletionPreservesNewerNavigation(t *testing.T) {
	m := savedFixture(t)
	first, _ := playlist.New("First", m.pages.visible)
	second, _ := playlist.New("Second", m.pages.visible)
	m.playlists.lists = []playlist.List{first, second}
	m.pages.saved = true
	m.pages.detail = true
	cmd := pageKey(m, 'J', "J")
	pageKey(m, tea.KeyEscape, "")
	pageKey(m, tea.KeyDown, "")
	playlistResult(t, m, cmd)
	if m.pages.collection != 1 || m.selectedPlaylist().Name != "Second" {
		t.Fatal("save completion overrode newer collection selection")
	}
}
func TestSavedPendingReplacementAlsoRejectsHomeAppend(t *testing.T) {
	m := savedFixture(t)
	m.UI.ActiveTab = 0
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'a', "a")
	if len(m.session.entries) != 0 || !strings.Contains(m.Info.Message, "Wait") {
		t.Fatal("home append silently queued into soon-to-be-replaced session")
	}
	acceptQueueLoad(t, m)
	pageKey(m, 'a', "a")
	if len(m.session.entries) != 4 {
		t.Fatal("Home append did not resume after load")
	}
}

func TestSavedWriteFailureCancelsConcurrentSaveAndQuit(t *testing.T) {
	for _, playlistFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "settings-first", true: "playlist-first"}[playlistFirst], func(t *testing.T) {
			m := savedFixture(t)
			list, _ := playlist.New("Existing", m.pages.visible)
			m.playlists.lists = []playlist.List{list}
			m.pages.saved = true
			m.playlists.path = t.TempDir()
			playlistCmd := m.changePlaylist("remove", 0)
			settingsCmd := m.saveSettings(true)
			if playlistFirst {
				playlistResult(t, m, playlistCmd)
				_, quit := m.Update(settingsCmd())
				if quit != nil {
					t.Fatal("settings quit after failed playlist save")
				}
			} else {
				_, quit := m.Update(settingsCmd())
				if quit != nil {
					t.Fatal("settings quit before pending playlist save finished")
				}
				playlistResult(t, m, playlistCmd)
			}
			if m.exiting || !m.Error.Visible || len(m.playlists.lists[0].Tracks) != 3 {
				t.Fatal("failed playlist write was lost on save-and-quit")
			}
		})
	}
}
