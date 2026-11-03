package audio

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

// lifecycleSynth keeps control calls real without requiring an audio device.
type lifecycleSynth struct {
	synth.Controller
	playing bool
}

func (s *lifecycleSynth) Play() error              { s.playing = true; return nil }
func (s *lifecycleSynth) Pause()                   { s.playing = false }
func (s *lifecycleSynth) Stop()                    { s.playing = false }
func (s *lifecycleSynth) IsPlaying() bool          { return s.playing }
func (s *lifecycleSynth) Seek(time.Duration) error { return nil }
func (s *lifecycleSynth) Close() error             { s.playing = false; return nil }
func (s *lifecycleSynth) Streamer() synth.Streamer { return nil }

// A synth can end on its own (MIDI EOF); the outer flags would otherwise stay
// playing until another control call.
func TestSynthIsPlayingTracksBackend(t *testing.T) {
	s := &lifecycleSynth{}
	p := NewPlayer()
	p.synthCtrl = s
	p.synthActive = true
	p.isPlaying = true
	p.isPaused = false
	if p.IsPlaying() {
		t.Fatal("outer flags overrode a finished synth")
	}
	s.playing = true
	if !p.IsPlaying() {
		t.Fatal("playing synth reported as stopped")
	}
}
