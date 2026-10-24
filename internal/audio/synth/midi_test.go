package synth

import (
	"bytes"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
)

func testMIDIPlayer(t *testing.T) *MidiPlayer {
	t.Helper()
	sf, e := soundfont.NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if e != nil {
		t.Fatal(e)
	}
	s, _, e := midi.Parse(bytes.NewReader(miditest.SMF(0, 120, []byte{0, 0x90, 60, 100, 0x81, 0x70, 0x80, 60, 0, 0, 255, 47, 0})))
	if e != nil {
		t.Fatal(e)
	}
	tl, e := s.Compile(44100)
	if e != nil {
		t.Fatal(e)
	}
	p, e := newMIDIPlayer(func() (midiRenderer, error) { return midi.NewRenderer(tl, sf) }, tl.TotalFrames, 44100)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}
func TestMidiPositionFrames(t *testing.T) {
	p := testMIDIPlayer(t)
	p.SetVolume(0)
	if e := p.Play(); e != nil {
		t.Fatal(e)
	}
	p.Stream(make([][2]float64, 44100))
	if p.Position() != time.Second {
		t.Fatalf("position=%v", p.Position())
	}
}
func TestMidiPauseDoesNotAdvance(t *testing.T) {
	p := testMIDIPlayer(t)
	_ = p.Play()
	p.Stream(make([][2]float64, 128))
	before := p.Position()
	p.Pause()
	out := make([][2]float64, 256)
	p.Stream(out)
	if p.Position() != before {
		t.Fatal("pause advanced")
	}
	for _, v := range out {
		if v != [2]float64{} {
			t.Fatal("pause audible")
		}
	}
}
func TestMidiEOFIsReusableSilence(t *testing.T) {
	p := testMIDIPlayer(t)
	_ = p.Play()
	b := make([][2]float64, 512)
	for range 400 {
		p.Stream(b)
	}
	if p.IsPlaying() {
		t.Fatal("EOF still playing")
	}
	if n, ok := p.Stream(b); n != len(b) || !ok {
		t.Fatal("EOF detached stream")
	}
	p.Stop()
	if e := p.Play(); e != nil {
		t.Fatal(e)
	}
	p.Stream(b)
	if p.Position() == 0 {
		t.Fatal("replay did not start")
	}
}
func TestMidiStopThenPlayStartsAtZero(t *testing.T) {
	a, b := testMIDIPlayer(t), testMIDIPlayer(t)
	_ = a.Play()
	a.Stream(make([][2]float64, 2048))
	a.Stop()
	_ = a.Play()
	_ = b.Play()
	x, y := make([][2]float64, 1024), make([][2]float64, 1024)
	a.Stream(x)
	b.Stream(y)
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("stop leaked state at%d", i)
		}
	}
}
