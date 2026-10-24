package midi

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestSMFResourceLimits(t *testing.T) {
	data := miditest.SMF(0, 120, []byte{0, 0x90, 60, 100, 0, 0x80, 60, 0, 0, 255, 47, 0})
	for _, limits := range []parseLimits{{10, 100, 100}, {1000, 1, 100}} {
		if _, _, e := parse(bytes.NewReader(data), limits); e == nil {
			t.Fatal("SMF resource limit ignored")
		}
	}
	sx := miditest.SMF(0, 120, []byte{0, 240, 4, 0x7E, 0x7F, 9, 247, 0, 255, 47, 0})
	if _, _, e := parse(bytes.NewReader(sx), parseLimits{1000, 100, 3}); e == nil {
		t.Fatal("SysEx limit ignored")
	}
}
func TestCompileTrackEndAndTail(t *testing.T) {
	a, b := []byte{0, 255, 47, 0}, miditest.Event(66790, 255, 47, 0)
	s, _, e := Parse(bytes.NewReader(miditest.SMF(1, 120, a, b)))
	if e != nil {
		t.Fatal(e)
	}
	s.tempos = []tempoEvent{{tempo: 480000}}
	tl, e := s.Compile(44100)
	if e != nil {
		t.Fatal(e)
	}
	if tl.ContentFrames != 11781756 || tl.TotalFrames != 11869956 {
		t.Fatalf("end %+v", tl)
	}
	empty, _, e := Parse(bytes.NewReader(miditest.SMF(0, 120, a)))
	if e != nil {
		t.Fatal(e)
	}
	tl, e = empty.Compile(44100)
	if e != nil || tl.TotalFrames != 0 {
		t.Fatal("empty song has tail")
	}
	if _, e = (&Song{division: 1, end: math.MaxUint64}).Compile(44100); e == nil {
		t.Fatal("duration overflow accepted")
	}
}
func TestPositionConversions(t *testing.T) {
	if DurationFrames(time.Duration(math.MaxInt64), 44100, 100) != 100 || DurationFrames(-1, 44100, 100) != 0 {
		t.Fatal("position clamp overflow")
	}
	if DurationFrames(FramesDuration(11869956, 44100), 44100, 11869956) != 11869956 {
		t.Fatal("duration endpoint lost a frame")
	}
	if FramesDuration(44100, 44100) != time.Second {
		t.Fatal("one-second frame conversion")
	}
}
func TestRepairDoesNotAcceptNewTailEvent(t *testing.T) {
	data := miditest.SMF(0, 120, []byte{0, 0x90, 60, 100, 0, 0xC0, 5, 0, 255, 47, 0})
	// Ends at a valid event boundary; appended program change is not a repair.
	data[21] = 4
	if _, _, e := Parse(bytes.NewReader(data)); e == nil {
		t.Fatal("extra tail event accepted")
	}
}
