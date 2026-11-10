package synth

import "github.com/AuroraStudio-aurorast/neoviolet/internal/audio/midi"

// CheckpointStats reports indexed progress and the retained cache budget. The
// live/working renderers and immutable SoundFont lie outside that budget.
func (p *MidiPlayer) CheckpointStats() midi.CheckpointStats {
	p.mu.Lock()
	cache := p.checkpoints
	p.mu.Unlock()
	if cache == nil {
		return midi.CheckpointStats{}
	}
	return cache.Stats()
}

// wakeWorkerLocked keeps indexing and seeks on one rendering owner.
func (p *MidiPlayer) wakeWorkerLocked() {
	if p.seekWake == nil {
		p.seekWake = make(chan struct{}, 1)
		p.workerStop = make(chan struct{})
		p.workerDone = make(chan struct{})
		go p.seekWorker()
	}
	select {
	case p.seekWake <- struct{}{}:
	default:
	}
}
