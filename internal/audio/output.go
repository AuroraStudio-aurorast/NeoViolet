package audio

import (
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/speaker"
)

var playToSpeaker = speaker.Play

// attachOutput submits a fresh control, replacing only this player's previous
// entry. A fresh entry also handles playback after the mixer drained the old one.
// Caller must hold p.mu.
func (p *Player) attachOutput(streamer beep.Streamer, paused bool) {
	p.detachOutput()
	p.processor = newEffectsProcessor(streamer, p.effectsSampleRateLocked(), p.effectSettings)
	p.volume = &effects.Volume{Streamer: p.processor, Base: 2}
	p.applyLinearVolumeLocked()
	p.completion = &outputCompletion{source: p.volume}
	p.output = &beep.Ctrl{Streamer: p.completion, Paused: paused}
	playToSpeaker(p.output)
}

// detachOutput waits for the speaker's current Stream call before removing our
// decoder from its chain. Resources can then be closed without an in-flight read.
// Returns whether playback was active at detachment, including completion that
// arrived during the last speaker callback. Caller must hold p.mu.
func (p *Player) detachOutput() bool {
	speaker.Lock()
	playing := p.isPlaying && !p.isPaused && (p.completion == nil || !p.completion.ended.Load())
	if p.output != nil {
		p.output.Streamer = nil
		p.output = nil
	}
	speaker.Unlock()
	p.completion = nil
	p.isPlaying = false
	return playing
}
