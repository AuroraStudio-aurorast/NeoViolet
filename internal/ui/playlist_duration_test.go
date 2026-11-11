package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

func TestFillPlaylistDurationsForLegacyMIDIEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.mid")
	data := miditest.SMF(1, 480, append(append(
		miditest.Event(0, 0xff, 0x51, 0x03, 0x07, 0xa1, 0x20),
		miditest.Event(0, 0x90, 60, 100)...),
		miditest.Event(960, 0xff, 0x2f, 0)...))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	lists := []playlist.List{{Tracks: []library.Track{{Path: path, Title: "track.mid"}}}}
	fillPlaylistDurations(lists)
	if got := lists[0].Tracks[0].Duration; got != 3*time.Second {
		t.Fatalf("duration = %v, want 3s", got)
	}
}

func TestFillPlaylistDurationsForLegacyTrackerEntry(t *testing.T) {
	if !synth.OpenmptAvailable() {
		t.Skip("libopenmpt not installed")
	}
	data, err := os.ReadFile(filepath.Join("..", "audio", "synth", "testdata", "minimal.mod"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "track.mod")
	// #nosec G703 -- the path is a test temporary directory, not user input.
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	lists := []playlist.List{{Tracks: []library.Track{{Path: path, Title: "track.mod"}}}}
	fillPlaylistDurations(lists)
	if got := lists[0].Tracks[0].Duration; got <= 0 {
		t.Fatalf("duration = %v, want a positive duration", got)
	}
}
