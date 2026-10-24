package soundfont

import (
	"bytes"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestModulatorReadAndBounds(t *testing.T) {
	data := miditest.SF2(&miditest.SF2Options{SampleModes: 1, LoopEnd: 1024, InstrumentMods: [][]miditest.Modulator{{{Source: 0x81, Destination: 17, Amount: -400}}}})
	sf, e := NewSoundFont(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewSynthesizer([]*SoundFont{sf}, NewSynthesizerSettings(44100))
	if e != nil {
		t.Fatal(e)
	}
	s.ProcessMidiMessage(0, 0xB0, 1, 127)
	s.NoteOn(0, 60, 100)
	s.Render(make([]float32, 512), make([]float32, 512))
	if s.voices.voices[0].instrumentPan != -40 {
		t.Fatal("signed file modulator not applied")
	}
	bad := mutateSF2(data, "ibag", 2, []byte{127, 127})
	if _, e = NewSoundFont(bytes.NewReader(bad)); e == nil {
		t.Fatal("invalid modulator index accepted")
	}
}
func TestStereoRegionsPreserved(t *testing.T) {
	data := miditest.SF2(&miditest.SF2Options{SampleModes: 1, LoopEnd: 1024, InstrumentZones: [][][]miditest.Generator{{{{Operator: 17, Amount: 65036}, {Operator: 54, Amount: 1}, {Operator: 53, Amount: 0}}, {{Operator: 17, Amount: 500}, {Operator: 54, Amount: 1}, {Operator: 53, Amount: 0}}}}})
	sf, e := NewSoundFont(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	cfg := NewSynthesizerSettings(44100)
	cfg.EnableReverbAndChorus = false
	s, e := NewSynthesizer([]*SoundFont{sf}, cfg)
	if e != nil {
		t.Fatal(e)
	}
	s.NoteOn(0, 60, 100)
	l, r := make([]float32, 512), make([]float32, 512)
	s.Render(l, r)
	if s.Stats().ActiveVoices != 2 {
		t.Fatal("stereo regions changed count")
	}
	energy := float64(0)
	for i := range l {
		energy += float64(l[i] * l[i])
		if math.Abs(float64(l[i]-r[i])) > 1e-7 {
			t.Fatal("stereo side missing")
		}
	}
	if energy < 0.01 {
		t.Fatal("stereo silent")
	}
}
func TestControllerResetAndRPN(t *testing.T) {
	s := modVoice(t, nil)
	send := func(cc, v int32) { s.ProcessMidiMessage(0, 0xB0, cc, v) }
	send(101, 0)
	send(100, 0)
	send(6, 12)
	send(38, 50)
	if s.channels[0].getPitchBendRange() != 12.5 {
		t.Fatal("RPN bend range")
	}
	send(101, 127)
	send(100, 127)
	send(6, 1)
	if s.channels[0].getPitchBendRange() != 12.5 {
		t.Fatal("RPN null changed range")
	}
	s.SelectPreset(0, 8, 39, false)
	s.ProcessMidiMessage(0, 0xD0, 100, 0)
	s.ProcessMidiMessage(0, 0xA0, 60, 88)
	send(64, 127)
	send(121, 0)
	c := s.channels[0]
	if c.bankNumber != 8 || c.patchNumber != 39 || c.pressure != 0 || c.polyPressure[60] != 0 || c.holdPedal {
		t.Fatal("CC121 state wrong")
	}
}
func TestSamePortRepeatedKeyNoteOff(t *testing.T) {
	s := modVoice(t, nil)
	s.NoteOn(0, 60, 100)
	s.NoteOn(0, 60, 100)
	s.NoteOff(0, 60)
	s.Render(make([]float32, 44100), make([]float32, 44100))
	if s.Stats().ActiveVoices != 0 {
		t.Fatal("repeated key retained voice")
	}
}
func TestUnsupportedModulatorDiagnostics(t *testing.T) {
	data := miditest.SF2(&miditest.SF2Options{InstrumentMods: [][]miditest.Modulator{{{Source: 0x7F, Destination: 0x8000, Amount: 500}}}})
	_, d, e := Load(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, x := range d {
		if x.Code == "unsupported_modulator" {
			found = true
		}
	}
	if !found {
		t.Fatal("unsupported route silently discarded")
	}
}
