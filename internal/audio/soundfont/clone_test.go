package soundfont

import (
	"bytes"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

// Missing an effect buffer, oscillator phase, private modValues binding or
// partially consumed block changes this continuation or makes the clones alias.
func TestSynthCloneExactIndependentContinuation(t *testing.T) {
	sf, err := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		t.Fatal(err)
	}
	settings := NewSynthesizerSettings(44100)
	s, err := NewSynthesizer([]*SoundFont{sf}, settings)
	if err != nil {
		t.Fatal(err)
	}
	s.ProcessMidiMessage(0, 0xB0, 91, 127)
	s.ProcessMidiMessage(0, 0xB0, 93, 100)
	s.NoteOn(0, 60, 110)
	s.NoteOn(0, 64, 97)
	s.Render(make([]float32, 1337), make([]float32, 1337))
	s.ProcessMidiMessage(0, 0xB0, 64, 127)
	s.NoteOff(0, 60)
	s.ProcessMidiMessage(0, 0xE0, 70, 80)
	s.Render(make([]float32, 777), make([]float32, 777))
	a, b := s.Clone(), s.Clone()
	if a.SoundFont[0] != sf || &a.voices.voices[0].oscillator.data[0] != &s.voices.voices[0].oscillator.data[0] {
		t.Fatal("clone copied immutable sample data")
	}
	if a.StateBytes() <= 0 {
		t.Fatal("missing snapshot memory accounting")
	}
	for step := range 16 {
		for _, x := range []*Synthesizer{s, a, b} {
			x.ProcessMidiMessage(0, 0xB0, 11, int32(20+step*5))
			x.ProcessMidiMessage(0, 0xB0, 10, int32(30+step*3))
			if step == 3 {
				x.ProcessMidiMessage(0, 0xB0, 64, 0)
				x.NoteOff(0, 64)
			}
			if step == 6 {
				x.NoteOn(0, 67, 99)
			}
		}
		l, r := make([]float32, 1001), make([]float32, 1001)
		s.Render(l, r)
		// Rendering source first must not advance either snapshot's buffers.
		for _, x := range []*Synthesizer{a, b} {
			al, ar := make([]float32, len(l)), make([]float32, len(r))
			x.Render(al, ar)
			for i := range l {
				if al[i] != l[i] || ar[i] != r[i] {
					t.Fatalf("clone continuation differs step%d frame%d", step, i)
				}
			}
		}
	}
	a.Reset()
	a.NoteOn(0, 72, 127)
	want := b.Clone()
	s.Render(make([]float32, 800), make([]float32, 800))
	x, y, u, v := make([]float32, 800), make([]float32, 800), make([]float32, 800), make([]float32, 800)
	b.Render(x, y)
	want.Render(u, v)
	for i := range x {
		if x[i] != u[i] || y[i] != v[i] {
			t.Fatal("source/reset mutated saved state")
		}
	}
}
