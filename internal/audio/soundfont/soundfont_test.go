package soundfont

import (
	"bytes"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

// Catches import/loader errors that prevent a complete generated SF2 from sounding.
func TestLoadGeneratedSF2AndRender(t *testing.T) {
	sf, err := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSynthesizer([]*SoundFont{sf}, NewSynthesizerSettings(44100))
	if err != nil {
		t.Fatal(err)
	}
	s.NoteOn(0, 60, 100)
	l, r := make([]float32, 4096), make([]float32, 4096)
	s.Render(l, r)
	var energy float64
	for i := range l {
		x, y := float64(l[i]), float64(r[i])
		if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
			t.Fatalf("nonfinite PCM at %d", i)
		}
		energy += x*x + y*y
	}
	if energy < 0.001 {
		t.Fatalf("generated instrument silent: energy=%g", energy)
	}
}
