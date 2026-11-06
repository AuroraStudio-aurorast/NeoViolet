package soundfont

import (
	"bytes"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func BenchmarkSynthClone(b *testing.B) {
	font, err := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		b.Fatal(err)
	}
	settings := NewSynthesizerSettings(44100)
	settings.MaximumPolyphony = 256
	s, err := NewSynthesizer([]*SoundFont{font}, settings)
	if err != nil {
		b.Fatal(err)
	}
	s.NoteOn(0, 60, 100)
	s.Render(make([]float32, 1337), make([]float32, 1337))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		c := s.Clone()
		if c.blockRead != s.blockRead {
			b.Fatal("block cursor changed")
		}
	}
}

func BenchmarkSF2LoadPCM(b *testing.B) {
	data := miditest.SF2(&miditest.SF2Options{Samples: make([]int16, 8<<20)})
	b.SetBytes(16 << 20)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		sf, err := NewSoundFont(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		if len(sf.WaveData) < 8<<20 {
			b.Fatal("missing PCM")
		}
	}
}
