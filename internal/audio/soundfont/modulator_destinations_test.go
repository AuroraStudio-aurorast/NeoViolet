package soundfont

import (
	"math"
	"testing"
)

func TestModulatorDestinations(t *testing.T) {
	// One full-scale CC routes to all supported scalar destinations. Each change
	// must survive route evaluation and leave unrelated shared generators intact.
	for _, tc := range []struct {
		dest   uint16
		amount int16
	}{
		{5, 100}, {6, 100}, {7, 100}, {8, -500}, {9, 50}, {10, 100}, {11, 100}, {13, 50},
		{15, 100}, {16, 100}, {17, 100}, {21, 100}, {22, 100}, {23, 100}, {24, 100}, {25, 100},
		{26, 100}, {27, 100}, {28, 100}, {29, 100}, {30, 100}, {31, 100}, {32, 100}, {33, 100},
		{34, 100}, {35, 100}, {36, 100}, {37, 100}, {38, 100}, {39, 100}, {40, 100}, {48, 100}, {51, 1}, {52, 100},
	} {
		s := modVoice(t, []modulator{{Source: 0x82, Destination: tc.dest, Amount: tc.amount}})
		ir := s.SoundFont[0].Instruments[0].Regions[0]
		before := ir.gs
		s.ProcessMidiMessage(0, 0xB0, 2, 127)
		s.NoteOn(0, 60, 100)
		s.Render(make([]float32, 512), make([]float32, 512))
		v := s.voices.voices[0]
		// Default modulators also feed filter, vibrato, effects and attenuation.
		base := modVoice(t, nil)
		base.NoteOn(0, 60, 100)
		base.Render(make([]float32, 512), make([]float32, 512))
		want := base.voices.voices[0].modValues[tc.dest] + float32(tc.amount)
		if math.Abs(float64(v.modValues[tc.dest]-want)) > 0.01 {
			t.Fatalf("destination%d got%v want%v", tc.dest, v.modValues[tc.dest], want)
		}
		if ir.gs != before {
			t.Fatal("modulation wrote shared generators")
		}
	}
}
func TestLiveEnvelopeRateDoesNotRestartAttack(t *testing.T) {
	s := modVoice(t, []modulator{{Source: 0x82, Destination: 34, Amount: 1200}})
	s.SoundFont[0].Instruments[0].Regions[0].gs[genAttackVolumeEnvelope] = -1200
	s.NoteOn(0, 60, 100)
	s.Render(make([]float32, 1024), make([]float32, 1024))
	v := s.voices.voices[0]
	before := v.volEnv.processedSampleCount
	s.ProcessMidiMessage(0, 0xB0, 2, 127)
	s.Render(make([]float32, 64), make([]float32, 64))
	if v.volEnv.processedSampleCount != before+64 || v.volEnv.attackSlope != 1 {
		t.Fatal("live attack route restarted or ignored")
	}
}

func TestPitchBendIndependentOfScaleTuning(t *testing.T) {
	s := modVoice(t, nil)
	s.SoundFont[0].Instruments[0].Regions[0].gs[genScaleTuning] = 0
	s.NoteOn(0, 60, 127)
	s.ProcessMidiMessage(0, 0xE0, 0, 127)
	s.Render(make([]float32, 64), make([]float32, 64))
	v := s.voices.voices[0]
	if v.oscillator.tune < 1.9 || v.oscillator.tune > 2.01 {
		t.Fatalf("bend scaled by key tracking: %v", v.oscillator.tune)
	}
}
