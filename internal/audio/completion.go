package audio

import (
	"sync/atomic"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

// outputCompletion is written only by the speaker callback. Observing EOF does
// no UI work, no allocation and does not acquire Player.mu in the audio thread.
type outputCompletion struct {
	source beep.Streamer
	ended  atomic.Bool
	err    error
}

func (s *outputCompletion) Stream(samples [][2]float64) (int, bool) {
	n, ok := s.source.Stream(samples)
	// Mixer removes a final short block immediately, without another pull.
	if n < len(samples) || !ok {
		s.err = s.source.Err()
		s.ended.Store(true)
	}
	return n, ok
}
func (s *outputCompletion) Err() error { return s.source.Err() }

// PlaybackEnd distinguishes natural drain/error from a paused or detached
// output, including streams whose duration is unknown before playback.
func (p *Player) PlaybackEnd() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	speaker.Lock()
	defer speaker.Unlock()
	if p.completion == nil || !p.completion.ended.Load() {
		return false, nil
	}
	return true, p.completion.err
}
