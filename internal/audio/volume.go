package audio

import (
	"math"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func (p *Player) applyLinearVolumeLocked() {
	if p.volume == nil {
		return
	}

	if p.linearVolume <= 0.0 {
		p.volume.Silent = true
		p.volume.Volume = 0
	} else {
		p.volume.Silent = false
		exponent := math.Log2(p.linearVolume)
		p.volume.Volume = exponent
	}
}

// SetVolume sets the linear playback volume in the range [0, 1].
func (p *Player) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	p.linearVolume = vol

	if p.isSynthActive() {
		p.synthCtrl.SetVolume(vol)
	}
	logger.Debug("Volume set", "volume", vol)
	p.applyLinearVolumeLocked()
}

// Volume returns the current linear playback volume in the range [0, 1].
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.linearVolume
}
