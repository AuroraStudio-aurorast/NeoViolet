package audio

import (
	"errors"
	"math"
	"testing"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
)

type effectSource struct {
	frame     [2]float64
	remaining int
	err       error
}

func (s *effectSource) Stream(out [][2]float64) (int, bool) {
	n := len(out)
	if s.remaining >= 0 {
		n = min(n, s.remaining)
		s.remaining -= n
	}
	for i := range out[:n] {
		out[i] = s.frame
	}
	return n, n > 0
}
func (s *effectSource) Err() error { return s.err }
func effectSamples(t *testing.T, cfg EffectsSettings, frame [2]float64) [2]float64 {
	t.Helper()
	processor := newEffectsProcessor(&effectSource{frame: frame, remaining: -1}, 44100, cfg)
	var out [1][2]float64
	processor.Stream(out[:])
	return out[0]
}

// Characterizes v2.1.1's behavior; it is unsuitable for the real-time processor.
func TestEffectsUpstreamEqualizerCharacterization(t *testing.T) {
	source := &effectSource{frame: [2]float64{.25, -.5}, remaining: -1}
	eq := effects.NewEqualizer(source, 44100, effects.MonoEqualizerSections{{F0: 1000, Bf: 1000}})
	out := make([][2]float64, 64)
	eq.Stream(out)
	if !math.IsNaN(out[0][0]) {
		t.Fatal("upstream zero-gain behavior changed; reassess workaround")
	}
	eq = effects.NewEqualizer(source, 44100, effects.MonoEqualizerSections{{F0: 1000, Bf: 1000, G: 6, GB: 3}})
	if allocations := testing.AllocsPerRun(10, func() { eq.Stream(out) }); allocations == 0 {
		t.Fatal("upstream allocation behavior changed; reassess workaround")
	}
}
func TestEffectsNeutralBypassAndChannels(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  EffectsSettings
		want [2]float64
	}{
		{name: "neutral", want: [2]float64{.25, -.5}},
		{name: "preamp", cfg: EffectsSettings{PreampDB: -6.020599913279624}, want: [2]float64{.125, -.25}},
		{name: "mono", cfg: EffectsSettings{Mono: true}, want: [2]float64{-.125, -.125}},
		{name: "swap", cfg: EffectsSettings{Swap: true}, want: [2]float64{-.5, .25}},
		{name: "pan-left", cfg: EffectsSettings{Pan: -1}, want: [2]float64{-.125, 0}},
		{name: "pan-right", cfg: EffectsSettings{Pan: 1}, want: [2]float64{0, -.125}},
		{name: "mono-before-pan", cfg: EffectsSettings{Mono: true, Pan: 1}, want: [2]float64{0, -.125}},
		{name: "bypass", cfg: EffectsSettings{PreampDB: -12, Gains: [5]float64{12, 12, 12, 12, 12}, Mono: true, Swap: true, Pan: 1, Bypass: true}, want: [2]float64{.25, -.5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := effectSamples(t, tc.cfg, [2]float64{.25, -.5})
			for ch := range 2 {
				if math.Abs(got[ch]-tc.want[ch]) > 1e-12 {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}
func TestEffectsOnlyWritesValidSamplesAndPreservesErrors(t *testing.T) {
	want := errors.New("source failed")
	source := &effectSource{frame: [2]float64{.2, .4}, remaining: 1, err: want}
	processor := newEffectsProcessor(source, 44100, EffectsSettings{Mono: true, Gains: [5]float64{0, 0, 6}})
	out := [][2]float64{{0, 0}, {123, 456}, {321, 654}}
	n, ok := processor.Stream(out)
	if n != 1 || !ok || out[1] != [2]float64{123, 456} || out[2] != [2]float64{321, 654} {
		t.Fatal("processor modified outside valid source samples")
	}
	if n, ok = processor.Stream(out); n != 0 || ok || !errors.Is(processor.Err(), want) {
		t.Fatal("processor lost EOF/error")
	}
}
func TestEffectsValidationRejectsNonfiniteAndOutOfRange(t *testing.T) {
	for _, cfg := range []EffectsSettings{{Pan: math.NaN()}, {Pan: math.Inf(1)}, {Pan: 1.1}, {PreampDB: 1}, {PreampDB: -13}, {Gains: [5]float64{13}}, {Gains: [5]float64{0, math.Inf(-1)}}} {
		p := NewPlayer()
		if err := p.SetEffects(cfg); err == nil {
			t.Fatalf("accepted invalid effects %#v", cfg)
		}
		if p.Effects() != (EffectsSettings{}) {
			t.Fatal("invalid edit changed accepted settings")
		}
	}
}
func TestEffectsOutputSampleContract(_ *testing.T) { var _ beep.Streamer = (*effectsProcessor)(nil) }
