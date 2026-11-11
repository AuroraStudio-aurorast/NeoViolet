package midi

import (
	"bytes"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestSongDurationMatchesCompiledPlayback(t *testing.T) {
	data := miditest.SMF(1, 480, append(append(
		miditest.Event(0, 0xff, 0x51, 0x03, 0x07, 0xa1, 0x20),
		miditest.Event(0, 0x90, 60, 100)...),
		miditest.Event(960, 0xff, 0x2f, 0)...))
	song, _, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := song.Compile(44100)
	if err != nil {
		t.Fatal(err)
	}
	got, err := song.Duration(44100)
	if err != nil {
		t.Fatal(err)
	}
	want := FramesDuration(timeline.TotalFrames, 44100)
	if got != want || got != 3*time.Second {
		t.Fatalf("duration = %v, want %v", got, want)
	}
}

func TestSongDurationOfEmptySongIsZero(t *testing.T) {
	song, _, err := Parse(bytes.NewReader(miditest.SMF(0, 480, []byte{0, 0xff, 0x2f, 0})))
	if err != nil {
		t.Fatal(err)
	}
	got, err := song.Duration(44100)
	if err != nil || got != 0 {
		t.Fatalf("duration = %v, err = %v", got, err)
	}
}
