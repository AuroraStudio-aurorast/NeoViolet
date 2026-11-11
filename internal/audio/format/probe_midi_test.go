package format

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestProbeMIDIReportsPlaybackDuration(t *testing.T) {
	data := miditest.SMF(1, 480, append(append(
		miditest.Event(0, 0xff, 0x51, 0x03, 0x07, 0xa1, 0x20),
		miditest.Event(0, 0x90, 60, 100)...),
		miditest.Event(960, 0xff, 0x2f, 0)...))
	got := probeTestFile(t, "track.mid", data)
	if got.Format != "MID" || got.Duration != 3*time.Second {
		t.Fatalf("got %+v", got)
	}
}
