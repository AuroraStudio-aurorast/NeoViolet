package soundfont

import (
	"bytes"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func modVoice(t *testing.T, mods []modulator) *Synthesizer {
	t.Helper()
	sf, e := NewSoundFont(bytes.NewReader(miditest.SF2(nil)))
	if e != nil {
		t.Fatal(e)
	}
	sf.Instruments[0].Regions[0].modulators = mergeModulators(defaultModulators, mods)
	cfg := NewSynthesizerSettings(44100)
	cfg.EnableReverbAndChorus = false
	s, e := NewSynthesizer([]*SoundFont{sf}, cfg)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestDefaultModulatorsNoDoubleApply(t *testing.T) {
	s := modVoice(t, []modulator{{Source: 0x502, Destination: 48, Amount: 0}})
	s.NoteOn(0, 60, 32)
	s.Render(make([]float32, 512), make([]float32, 512))
	a := s.voices.voices[0].currentMixGainLeft
	s.Reset()
	s.NoteOn(0, 60, 127)
	s.Render(make([]float32, 512), make([]float32, 512))
	b := s.voices.voices[0].currentMixGainLeft
	if math.Abs(float64(a-b)) > 1e-6 {
		t.Fatalf("disabled velocity attenuation still contributes %g vs %g", a, b)
	}
}
func TestLiveModulatorInputs(t *testing.T) {
	s := modVoice(t, []modulator{{Source: 0x81, Destination: 17, Amount: 500}})
	s.NoteOn(0, 60, 100)
	s.Render(make([]float32, 512), make([]float32, 512))
	v := s.voices.voices[0]
	if math.Abs(float64(v.currentMixGainLeft-v.currentMixGainRight)) > 0.001 {
		t.Fatal("initial pan unbalanced")
	}
	s.ProcessMidiMessage(0, 0xB0, 1, 127)
	s.Render(make([]float32, 512), make([]float32, 512))
	if v.currentMixGainLeft > 1e-6 || v.currentMixGainRight < 0.001 {
		t.Fatal("live pan route ignored")
	}
	s.ProcessMidiMessage(0, 0xB0, 11, 0)
	s.Render(make([]float32, 512), make([]float32, 512))
	s.ProcessMidiMessage(0, 0xB0, 11, 127)
	s.Render(make([]float32, 512), make([]float32, 512))
	if s.Stats().ActiveVoices == 0 {
		t.Fatal("temporary attenuation killed held voice")
	}
}
