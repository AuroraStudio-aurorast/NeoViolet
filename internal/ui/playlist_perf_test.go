package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

func TestSavedEditsCopyOnlyChangedTrackSequence(t *testing.T) {
	for _, action := range []string{"rename", "reorder", "remove", "append", "delete"} {
		t.Run(action, func(t *testing.T) {
			m := savedFixture(t)
			// Spare capacity catches append and delete accidentally mutating an old snapshot.
			tracks := make([]library.Track, 3, 8)
			copy(tracks, m.pages.visible)
			m.playlists.lists = []playlist.List{
				{ID: "edited", Name: "Edited", Tracks: tracks},
				{ID: "untouched", Name: "Untouched", Tracks: slices.Clone(tracks)},
			}
			before := m.playlists.lists
			wantBefore := playlist.Clone(before)
			var cmd tea.Cmd
			if action == "rename" {
				m.editPlaylist("playlist_rename")
				cmd = namePlaylist(t, m, "Renamed")
			} else {
				m.appendQueue(m.pages.visible[0])
				cmd = m.changePlaylist(action, 1)
			}
			if cmd == nil || m.playlists.save == nil {
				t.Fatal("missing save")
			}
			snapshot := m.playlists.save.snapshot
			unchanged := len(snapshot) - 1
			if &snapshot[unchanged].Tracks[0] != &before[1].Tracks[0] {
				t.Fatal("copied tracks in an unchanged playlist")
			}
			if action == "rename" && &snapshot[0].Tracks[0] != &before[0].Tracks[0] {
				t.Fatal("rename copied an unchanged track sequence")
			}
			for i := range before {
				if before[i].Name != wantBefore[i].Name || !slices.Equal(before[i].Tracks, wantBefore[i].Tracks) {
					t.Fatal("pending edit mutated the published snapshot")
				}
			}
			playlistResult(t, m, cmd)
			if m.playlists.save != nil || m.Error.Visible {
				t.Fatalf("save failed: %s", m.Error.Message)
			}
			for i := range before {
				if before[i].Name != wantBefore[i].Name || !slices.Equal(before[i].Tracks, wantBefore[i].Tracks) {
					t.Fatal("successful edit mutated a retained old snapshot")
				}
			}
			disk, err := playlist.Load(m.playlists.path)
			if err != nil || !slices.EqualFunc(disk, m.playlists.lists, func(a, b playlist.List) bool {
				return a.ID == b.ID && a.Name == b.Name && slices.Equal(a.Tracks, b.Tracks)
			}) {
				t.Fatalf("published state differs from disk: %v", err)
			}
		})
	}
}

func TestSavedFailedEditsPreservePublishedSnapshot(t *testing.T) {
	for _, action := range []string{"rename", "reorder", "remove", "append", "delete"} {
		t.Run(action, func(t *testing.T) {
			m := savedFixture(t)
			list, err := playlist.New("Original", m.pages.visible)
			if err != nil {
				t.Fatal(err)
			}
			m.playlists.lists = []playlist.List{list}
			m.playlists.path = t.TempDir() // Atomic replacement of a directory must fail.
			want := playlist.Clone(m.playlists.lists)
			m.appendQueue(m.pages.visible[0])
			var cmd tea.Cmd
			if action == "rename" {
				m.editPlaylist("playlist_rename")
				cmd = namePlaylist(t, m, "Retry rename")
			} else {
				cmd = m.changePlaylist(action, 1)
			}
			playlistResult(t, m, cmd)
			if !m.Error.Visible || len(m.playlists.lists) != 1 || m.playlists.lists[0].ID != want[0].ID ||
				m.playlists.lists[0].Name != "Original" || !slices.Equal(m.playlists.lists[0].Tracks, want[0].Tracks) {
				t.Fatal("failed edit altered published state")
			}
		})
	}
}

func TestSavedEditsReuseAvailabilityUntilExplicitRefresh(t *testing.T) {
	for _, action := range []string{"rename", "reorder", "remove", "delete"} {
		t.Run(action, func(t *testing.T) {
			m := savedFixture(t)
			dir := t.TempDir()
			missing, removed := filepath.Join(dir, "missing.wav"), filepath.Join(dir, "removed.wav")
			if err := os.WriteFile(removed, nil, 0600); err != nil {
				t.Fatal(err)
			}
			lists := []playlist.List{
				{ID: "edit", Name: "Edit", Tracks: slices.Clone(m.pages.visible)},
				{ID: "keep", Name: "Keep", Tracks: []library.Track{{Path: missing}, {Path: removed}}},
			}
			if err := playlist.Save(m.playlists.path, lists); err != nil {
				t.Fatal(err)
			}
			playlistResult(t, m, m.loadPlaylists())
			if err := os.WriteFile(missing, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(removed); err != nil {
				t.Fatal(err)
			}
			var cmd tea.Cmd
			if action == "rename" {
				m.editPlaylist("playlist_rename")
				cmd = namePlaylist(t, m, "Renamed")
			} else {
				cmd = m.changePlaylist(action, 1)
			}
			playlistResult(t, m, cmd)
			if m.playlists.unavailable[missing] == "" || m.playlists.unavailable[removed] != "" {
				t.Fatal("ordinary edit refreshed existing file availability")
			}
			m.pages.saved = true
			playlistResult(t, m, pageKey(m, 'r', "r"))
			if m.playlists.unavailable[missing] != "" || m.playlists.unavailable[removed] == "" {
				t.Fatal("explicit refresh did not refresh existing files")
			}
		})
	}
}

func TestSavedAddsCheckOnlyNewPathsAndPublishAvailabilityAfterSuccess(t *testing.T) {
	for _, action := range []string{"append", "queue"} {
		t.Run(action, func(t *testing.T) {
			m := savedFixture(t)
			dir := t.TempDir()
			existing, missing, added := filepath.Join(dir, "existing.wav"), filepath.Join(dir, "missing.wav"), filepath.Join(dir, "added.wav")
			if err := os.WriteFile(existing, nil, 0600); err != nil {
				t.Fatal(err)
			}
			lists := []playlist.List{{ID: "old", Name: "Old", Tracks: []library.Track{{Path: existing}, {Path: missing}}}}
			if err := playlist.Save(m.playlists.path, lists); err != nil {
				t.Fatal(err)
			}
			playlistResult(t, m, m.loadPlaylists())
			oldAvailability := m.playlists.unavailable
			if err := os.Remove(existing); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(missing, nil, 0600); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{existing, missing, added, added, "https://example.com/song"} {
				m.appendQueue(library.Track{Path: path})
			}
			var cmd tea.Cmd
			if action == "queue" {
				m.editPlaylist("playlist_queue")
				cmd = namePlaylist(t, m, "New queue")
			} else {
				cmd = m.changePlaylist("append", 0)
			}
			if cmd == nil {
				t.Fatal("missing save")
			}
			msg := cmd()
			if oldAvailability[added] != "" || len(oldAvailability) != 1 {
				t.Fatal("worker mutated availability before successful publication")
			}
			_, _ = m.Update(msg)
			if m.playlists.unavailable[existing] != "" || m.playlists.unavailable[missing] == "" ||
				m.playlists.unavailable[added] == "" || len(m.playlists.unavailable) != 2 {
				t.Fatalf("added paths were not checked independently: %#v", m.playlists.unavailable)
			}
		})
	}
}

func TestSavedReaddedPathReplacesRemovedAvailability(t *testing.T) {
	m := savedFixture(t)
	path := filepath.Join(t.TempDir(), "restored.wav")
	lists := []playlist.List{{ID: "saved", Name: "Saved", Tracks: []library.Track{{Path: path}}}}
	if err := playlist.Save(m.playlists.path, lists); err != nil {
		t.Fatal(err)
	}
	playlistResult(t, m, m.loadPlaylists())
	playlistResult(t, m, m.changePlaylist("remove", 0))
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	m.appendQueue(library.Track{Path: path})
	playlistResult(t, m, m.changePlaylist("append", 0))
	if m.playlists.unavailable[path] != "" {
		t.Fatal("readded available path retained its removed reference's missing status")
	}
}

func TestSavedConcurrentWriterRejectsFurtherMutations(t *testing.T) {
	m := savedFixture(t)
	list, err := playlist.New("Original", m.pages.visible)
	if err != nil {
		t.Fatal(err)
	}
	m.playlists.lists = []playlist.List{list}
	before := playlist.Clone(m.playlists.lists)
	cmd := m.changePlaylist("reorder", 1)
	if cmd == nil {
		t.Fatal("missing save")
	}
	results := make(chan tea.Msg, 2)
	for range 2 {
		go func() { results <- cmd() }()
	}
	for _, action := range []string{"append", "remove", "reorder", "delete"} {
		if blocked := m.changePlaylist(action, 1); blocked != nil {
			t.Fatalf("allowed %s during pending write", action)
		}
	}
	if m.editPlaylist("playlist_rename") != nil || m.loadPlaylists() != nil {
		t.Fatal("allowed rename or reload during pending write")
	}
	first, second := (<-results).(playlistSavedMsg), (<-results).(playlistSavedMsg)
	if first.job != second.job || first.err != nil || second.err != nil {
		t.Fatal("duplicate commands did not share a successful writer")
	}
	if !slices.Equal(m.playlists.lists[0].Tracks, before[0].Tracks) {
		t.Fatal("writer changed published tracks before completion")
	}
	_, _ = m.Update(first)
	_, _ = m.Update(second)
	if m.playlists.save != nil || m.playlists.lists[0].Tracks[0].Title != "B.wav" {
		t.Fatal("writer completion did not publish the single reorder")
	}
}
