package audio

import (
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

var playToSpeaker = speaker.Play

// attachOutput submits a fresh control, replacing only this player's previous
// entry. A fresh entry also handles playback after the mixer drained the old one.
// Caller must hold p.mu.
func (p *Player) attachOutput(streamer beep.Streamer) {
	p.detachOutput()
	p.output = &beep.Ctrl{Streamer: streamer}
	playToSpeaker(p.output)
}

// detachOutput waits for the speaker's current Stream call before removing our
// decoder from its chain. Resources can then be closed without an in-flight read.
// Caller must hold p.mu.
func (p *Player) detachOutput() {
	speaker.Lock()
	if p.output != nil {
		p.output.Streamer = nil
		p.output = nil
	}
	speaker.Unlock()
	p.isPlaying = false
}
