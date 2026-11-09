package audio

import (
	"math"

	"github.com/gopxl/beep/v2"
)

var effectFrequencies = [5]float64{80, 250, 1000, 4000, 12000}

type biquadCoefficients struct {
	b0, b1, b2, a1, a2 float64
	active             bool
}
type effectsPreset struct {
	settings       EffectsSettings
	coefficients   [5]biquadCoefficients
	gain, panScale float64
}
type effectsBank struct {
	preset effectsPreset
	delay  [5][2][2]float64
}
type effectsProcessor struct {
	source           beep.Streamer
	current, next    effectsBank
	desired          effectsPreset
	fade, fadeLength int
}

func newEffectsProcessor(source beep.Streamer, rate beep.SampleRate, settings EffectsSettings) *effectsProcessor {
	preset := prepareEffects(settings, rate)
	return &effectsProcessor{source: source, current: effectsBank{preset: preset}, desired: preset, fadeLength: max(1, int(rate)/100)}
}
func prepareEffects(settings EffectsSettings, rate beep.SampleRate) effectsPreset {
	result := effectsPreset{settings: settings, gain: math.Pow(10, settings.PreampDB/20), panScale: 1 / (1 + math.Abs(settings.Pan))}
	for i, frequency := range effectFrequencies {
		gain := settings.Gains[i]
		if gain == 0 || frequency >= float64(rate)/2 {
			continue
		}
		// RBJ peaking EQ, Q=1. Formulae: https://www.w3.org/TR/audio-eq-cookbook/
		amplitude := math.Pow(10, gain/40)
		omega := 2 * math.Pi * frequency / float64(rate)
		alpha := math.Sin(omega) / 2
		cosine := math.Cos(omega)
		a0 := 1 + alpha/amplitude
		result.coefficients[i] = biquadCoefficients{b0: (1 + alpha*amplitude) / a0, b1: -2 * cosine / a0, b2: (1 - alpha*amplitude) / a0, a1: -2 * cosine / a0, a2: (1 - alpha/amplitude) / a0, active: true}
	}
	return result
}
func (b *effectsBank) sample(input [2]float64) [2]float64 {
	cfg := b.preset.settings
	if cfg.Bypass {
		return input
	}
	sample := input
	if cfg.PreampDB != 0 {
		sample[0] *= b.preset.gain
		sample[1] *= b.preset.gain
	}
	for i, c := range b.preset.coefficients {
		if !c.active {
			continue
		}
		for channel := range 2 {
			state := &b.delay[i][channel]
			x := sample[channel]
			y := c.b0*x + state[0]
			state[0] = c.b1*x - c.a1*y + state[1]
			state[1] = c.b2*x - c.a2*y
			sample[channel] = y
		}
	}
	if cfg.Mono {
		average := (sample[0] + sample[1]) / 2
		sample = [2]float64{average, average}
	}
	if cfg.Pan < 0 {
		right := sample[1]
		sample[0] += -cfg.Pan * right
		sample[1] *= 1 + cfg.Pan
	} else if cfg.Pan > 0 {
		left := sample[0]
		sample[0] *= 1 - cfg.Pan
		sample[1] += cfg.Pan * left
	}
	if cfg.Pan != 0 {
		sample[0] *= b.preset.panScale
		sample[1] *= b.preset.panScale
	}
	if cfg.Swap {
		sample[0], sample[1] = sample[1], sample[0]
	}
	return sample
}
func (s *effectsProcessor) Stream(samples [][2]float64) (int, bool) {
	n, ok := s.source.Stream(samples)
	for i, input := range samples[:n] {
		if s.fade == 0 && s.current.preset.settings != s.desired.settings {
			s.next = effectsBank{preset: s.desired}
			s.fade = s.fadeLength
		}
		output := s.current.sample(input)
		if s.fade > 0 {
			next := s.next.sample(input)
			mix := float64(s.fadeLength-s.fade+1) / float64(s.fadeLength)
			for ch := range 2 {
				output[ch] += (next[ch] - output[ch]) * mix
			}
			s.fade--
			if s.fade == 0 {
				s.current = s.next
			}
		}
		samples[i] = output
	}
	return n, ok
}
func (s *effectsProcessor) Err() error { return s.source.Err() }
