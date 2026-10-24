package midi

import (
	"bytes"
	"io"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

func testFont(t *testing.T) *soundfont.SoundFont {
	t.Helper()
	s, e := soundfont.NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func timeline(t *testing.T, tracks ...[]byte) *Timeline {
	t.Helper()
	s, _, e := Parse(bytes.NewReader(miditest.SMF(1, 120, tracks...)))
	if e != nil {
		t.Fatal(e)
	}
	tl, e := s.Compile(44100)
	if e != nil {
		t.Fatal(e)
	}
	return tl
}
func renderPCM(t *testing.T, tl *Timeline, sf *soundfont.SoundFont, chunks []int) [][2]float32 {
	t.Helper()
	r, e := NewRenderer(tl, sf)
	if e != nil {
		t.Fatal(e)
	}
	var out [][2]float32
	for step := 0; int64(len(out)) < tl.TotalFrames; step++ {
		n := chunks[step%len(chunks)]
		l, rbuf := make([]float32, n), make([]float32, n)
		n, e = r.Render(l, rbuf)
		if e != nil && e != io.EOF {
			t.Fatal(e)
		}
		for i := 0; i < n; i++ {
			out = append(out, [2]float32{l[i], rbuf[i]})
		}
		if n == 0 {
			break
		}
	}
	return out
}
func TestPortsIsolateNoteOffAndAllSoundOff(t *testing.T) {
	a := []byte{0, 0x90, 60, 100, 24, 0xB0, 120, 0, 0, 255, 47, 0}
	b := []byte{0, 255, 33, 1, 1, 0, 0x90, 60, 100, 120, 0x80, 60, 0, 0, 255, 47, 0}
	sf := testFont(t)
	mix := renderPCM(t, timeline(t, a, b), sf, []int{512})
	ref := renderPCM(t, timeline(t, b), sf, []int{512})
	// After port0 CC120 and its effects decay, port1 still holds the note.
	var energy float64
	for i := 15000; i < 21000; i++ {
		energy += math.Abs(float64(mix[i][0]))
		if math.Abs(float64(ref[i][0])) > 0.0001 && math.Abs(float64(mix[i][0])) < 1e-8 {
			t.Fatal("other port was cut")
		}
	}
	if energy < 1 {
		t.Fatal("port1 went silent")
	}
}
func TestRenderChunkEquivalence(t *testing.T) {
	tr := []byte{0, 0x90, 60, 100, 13, 0xB0, 11, 60, 15, 0xE0, 127, 80, 92, 0x80, 60, 0, 0, 255, 47, 0}
	tl := timeline(t, tr)
	sf := testFont(t)
	a := renderPCM(t, tl, sf, []int{512})
	b := renderPCM(t, tl, sf, []int{1, 63, 64, 65, 512})
	if len(a) != len(b) || int64(len(a)) != tl.TotalFrames {
		t.Fatal("render lengths differ")
	}
	for i := range a {
		for c := range 2 {
			if math.Abs(float64(a[i][c]-b[i][c])) > 1e-7 {
				t.Fatalf("chunk drift at%d", i)
			}
		}
	}
}
func TestPortResourceLimit(t *testing.T) {
	sf := testFont(t)
	tl := &Timeline{SampleRate: 44100, TotalFrames: 100}
	for i := range 33 {
		tl.Events = append(tl.Events, TimedEvent{Event: Event{Port: uint8(i), Status: 0x90, Data1: 60, Data2: 100}})
	}
	if _, e := NewRenderer(tl, sf); e == nil {
		t.Fatal("33 ports accepted")
	}
	tl.Events = tl.Events[:32]
	tl.Events[31].Event.Port = 255
	if _, e := NewRenderer(tl, sf); e != nil {
		t.Fatal(e)
	}
}
