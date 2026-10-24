package soundfont

import (
	"bytes"
	"math"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/miditest"
)

func TestOscillatorInvalidEffectiveLoop(t *testing.T) {
	sf, err := NewSoundFont(bytes.NewReader(miditest.SF2(&miditest.SF2Options{
		Samples: make([]int16, 32), LoopStart: 8, LoopEnd: 16,
		Instruments: [][]miditest.Generator{{{Operator: 3, Amount: uint16(65520)}, {Operator: 54, Amount: 1}, {Operator: 53, Amount: 0}}},
	})))
	if err != nil {
		t.Fatal(err)
	}
	r := sf.Instruments[0].Regions[0]
	if r.getSampleModes() != loopNoLoop {
		t.Fatal("reverse effective loop was not disabled")
	}
	o := newOscillator(&Synthesizer{SampleRate: 44100})
	o.startByRegion(sf.WaveData, newRegionPair(sf.Presets[0].Regions[0], r))
	b := make([]float32, 64)
	if !o.fillBlock(b, 1) || o.fillBlock(b, 1) {
		t.Fatal("invalid loop did not terminate as one-shot")
	}
}

func TestOscillatorSampleBounds(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end int32
	}{{"negative", -1, 2}, {"long", 0, 4}, {"empty", 1, 1}, {"reverse", 2, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			o := newOscillator(&Synthesizer{SampleRate: 44100})
			o.start([]int16{32767, 16384}, 0, 44100, tc.start, tc.end, 0, 2, 60, 0, 0, 100)
			if o.fillBlock(make([]float32, 64), 1) {
				t.Fatal("invalid sample played")
			}
		})
	}
	o := newOscillator(&Synthesizer{SampleRate: 44100})
	o.start([]int16{16384}, 0, 44100, 0, 1, 0, 1, 60, 0, 0, 100)
	b := make([]float32, 3)
	if !o.fillBlock(b, 0.5) {
		t.Fatal("valid one-point sample ended early")
	}
	if b[0] != 0.5 || b[1] != 0.5 || b[2] != 0 {
		t.Fatalf("last point interpolation escaped region: %v", b)
	}
}

func TestOscillatorMultipleWraps(t *testing.T) {
	o := newOscillator(&Synthesizer{SampleRate: 44100})
	o.start([]int16{0, 16384}, 1, 44100, 0, 2, 0, 2, 60, 0, 0, 100)
	b := make([]float32, 4)
	o.fillBlock(b, 9)
	for i, w := range []float32{0, 0.5, 0, 0.5} {
		if b[i] != w {
			t.Fatalf("wrap %d: got %v want %v", i, b[i], w)
		}
	}
	for _, ratio := range []float64{math.NaN(), math.Inf(1), -1, 0, 1e30} {
		if o.fillBlock(b, ratio) {
			t.Errorf("unsafe ratio %v accepted", ratio)
		}
	}
}

func TestSF2ResourceLimits(t *testing.T) {
	data := miditest.SF2(nil)
	for _, limit := range []loadLimits{{4, 16 << 20}, {1 << 30, 4}} {
		if _, _, err := load(bytes.NewReader(data), limit); err == nil {
			t.Fatal("resource limit ignored")
		}
	}
}
