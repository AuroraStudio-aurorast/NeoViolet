package soundfont

import (
	"bytes"
	"sync"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestSharedSoundFontConcurrentSynths(t *testing.T) {
	sf, err := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := NewSynthesizer([]*SoundFont{sf}, NewSynthesizerSettings(44100))
			if e != nil {
				t.Error(e)
				return
			}
			if &s.SoundFont[0].WaveData[0] != &sf.WaveData[0] {
				t.Error("PCM copied")
			}
			// #nosec G115 -- loop index is bounded to 0..7.
			s.ProcessMidiMessage(0, 0xB0, 11, int32(15*i))
			s.NoteOn(0, 60, 100)
			s.Render(make([]float32, 128), make([]float32, 128))
		}()
	}
	wg.Wait()
}

func TestPolyphonyStatsAndStealing(t *testing.T) {
	sf, err := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := NewSynthesizerSettings(44100)
	cfg.MaximumPolyphony = 8
	s, err := NewSynthesizer([]*SoundFont{sf}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for k := int32(40); k < 49; k++ {
		s.NoteOn(0, k, 100)
	}
	st := s.Stats()
	if st.ActiveVoices != 8 || st.PeakVoices != 8 || st.StolenVoices != 1 {
		t.Fatalf("stats: %+v", st)
	}
	s.ReleaseAll()
	s.Render(make([]float32, 44100), make([]float32, 44100))
	if s.Stats().ActiveVoices != 0 {
		t.Fatal("released voices retained")
	}
}
