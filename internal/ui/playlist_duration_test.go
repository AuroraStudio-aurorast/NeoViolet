package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
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
