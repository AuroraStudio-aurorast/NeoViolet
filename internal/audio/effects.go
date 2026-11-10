package audio

import (
	"fmt"
	"math"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

// EffectsSettings is a session preset. Its zero value is transparent.
type EffectsSettings struct {
	Gains              [5]float64
	PreampDB, Pan      float64
	Mono, Swap, Bypass bool
}

// EffectsController is optional for AudioPlayer implementations.
// Values are copied; callers never receive mutable objects in the audio chain.
type EffectsController interface {
	SetEffects(EffectsSettings) error
	Effects() EffectsSettings
	EffectsSampleRate() beep.SampleRate
}

// Validate rejects invalid values before they reach coefficients or audio samples.
func (s EffectsSettings) Validate() error {
	for _, v := range s.Gains {
		if !finiteRange(v, -12, 12) {
			return fmt.Errorf("EQ gain must be finite and between -12 and 12 dB")
		}
	}
	if !finiteRange(s.PreampDB, -12, 0) {
		return fmt.Errorf("preamp must be finite and between -12 and 0 dB")
	}
	if !finiteRange(s.Pan, -1, 1) {
		return fmt.Errorf("pan must be finite and between -1 and 1")
	}
	return nil
}
func finiteRange(v, low, high float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= low && v <= high
}

// SetEffects prepares coefficients on the control thread and publishes one value
// under the speaker lock. Lock order remains Player.mu -> speaker; no callback
// acquires Player.mu, allocates buffers, or performs I/O.
func (p *Player) SetEffects(settings EffectsSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	preset := prepareEffects(settings, p.effectsSampleRateLocked())
	speaker.Lock()
	p.effectSettings = settings
	if p.processor != nil {
		p.processor.desired = preset
	}
	speaker.Unlock()
	return nil
}

// Effects returns the accepted session settings.
func (p *Player) Effects() EffectsSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.effectSettings
}

// EffectsSampleRate reports the rate at which the output processor runs.
func (p *Player) EffectsSampleRate() beep.SampleRate {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.effectsSampleRateLocked()
}
func (p *Player) effectsSampleRateLocked() beep.SampleRate {
	if p.streamer == nil && !p.isSynthActive() {
		return 44100
	}
	if speakerSampleRate > 0 {
		return speakerSampleRate
	}
	if p.isSynthActive() {
		return p.synthCtrl.Streamer().Format().SampleRate
	}
	if p.format.SampleRate > 0 {
		return p.format.SampleRate
	}
	return 44100
}
