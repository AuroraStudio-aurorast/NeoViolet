package ui

import (
	"fmt"
	"math"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
)

func (m *Model) applyEffectsTo(player audio.AudioPlayer) error {
	controller, ok := player.(audio.EffectsController)
	if !ok {
		if m.pages.effects == (audio.EffectsSettings{}) {
			return nil
		}
		return fmt.Errorf("effects unavailable for this playback backend")
	}
	return controller.SetEffects(m.pages.effects)
}
func (m *Model) applyEffectEdit(candidate audio.EffectsSettings) {
	if candidate == m.pages.effects {
		return
	}
	if err := candidate.Validate(); err != nil {
		m.Error.Set(err.Error(), 120)
		return
	}
	if player := m.Audio.Player; player != nil {
		controller, ok := player.(audio.EffectsController)
		if !ok {
			m.Error.Set("Effects unavailable for this playback backend", 120)
			return
		}
		if err := controller.SetEffects(candidate); err != nil {
			m.Error.Set(fmt.Sprintf("Effects rejected: %v", err), 120)
			return
		}
	}
	m.pages.effects = candidate
}
func (m *Model) effectsStatus() string {
	status := "Session · ready for next track"
	if controller, ok := m.Audio.Player.(audio.EffectsController); ok {
		rate := controller.EffectsSampleRate()
		status = "Session · applied"
		if m.pages.effects.Bypass {
			status = "Session · bypassed"
		}
		if rate > 0 && rate <= 24000 {
			status += fmt.Sprintf(" · EQ below %.4g kHz", float64(rate)/2000)
		}
	} else if m.Audio.Player != nil {
		status = "Session · effects unavailable"
	}
	return status
}
func (m *Model) adjustEffect(delta float64) {
	candidate := m.pages.effects
	switch m.pages.effect {
	case 0, 1, 2, 3, 4:
		candidate.Gains[m.pages.effect] = max(-12, min(12, candidate.Gains[m.pages.effect]+delta*.5))
	case 5:
		candidate.Pan = math.Round(max(-1, min(1, candidate.Pan+delta*.1))*10) / 10
	case 6:
		candidate.Mono = !candidate.Mono
	case 7:
		candidate.Swap = !candidate.Swap
	case 8:
		candidate.PreampDB = max(-12, min(0, candidate.PreampDB+delta*.5))
	case 9:
		candidate.Bypass = !candidate.Bypass
	}
	m.applyEffectEdit(candidate)
}
func (m *Model) resetEffect() {
	candidate := m.pages.effects
	switch m.pages.effect {
	case 0, 1, 2, 3, 4:
		candidate.Gains[m.pages.effect] = 0
	case 5:
		candidate.Pan = 0
	case 6:
		candidate.Mono = false
	case 7:
		candidate.Swap = false
	case 8:
		candidate.PreampDB = 0
	case 9:
		candidate.Bypass = false
	}
	m.applyEffectEdit(candidate)
}
