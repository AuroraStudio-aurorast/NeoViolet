package audio

import (
	"os"
	"testing"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/soundfont"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

// lifecycleSynth keeps control calls real without requiring an audio device.
type lifecycleSynth struct {
	synth.Controller
	playing bool
	closed  bool
}

func (s *lifecycleSynth) ReusableStream() bool     { return true }
func (s *lifecycleSynth) Play() error              { s.playing = true; return nil }
func (s *lifecycleSynth) Pause()                   { s.playing = false }
func (s *lifecycleSynth) Stop()                    { s.playing = false }
func (s *lifecycleSynth) IsPlaying() bool          { return s.playing }
func (s *lifecycleSynth) Close() error             { s.closed = true; s.playing = false; return nil }
func (s *lifecycleSynth) Streamer() synth.Streamer { return nil }
func TestSynthSingleRegistration(t *testing.T) {
	s := &lifecycleSynth{}
	p := NewPlayer()
	p.synthCtrl = s
	p.synthActive = true
	calls := 0
	p.registerSynth = func(beep.Streamer) { calls++ }
	for range 3 {
		if err := p.Play(); err != nil {
			t.Fatal(err)
		}
		p.Pause()
		p.Resume()
		p.Stop()
	}
	if calls != 1 {
		t.Fatalf("registered %d times", calls)
	}
	s.playing = false
	p.isPlaying = true
	p.isPaused = false
	if p.IsPlaying() {
		t.Fatal("outer player ignored EOF")
	}
}
func TestSynthCloseCleansPlayerState(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "track")
	if e != nil {
		t.Fatal(e)
	}
	_ = f.Close()
	s := &lifecycleSynth{}
	p := NewPlayer()
	p.synthCtrl = s
	p.synthActive = true
	p.cachedSF = &soundfont.SoundFont{}
	p.cachedSFPath = "bank"
	p.path = "track"
	p.tempFiles = []string{f.Name()}
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
	if !s.closed || p.synthCtrl != nil || p.cachedSF != nil || p.path != "" || p.IsPlaying() {
		t.Fatal("close retained state")
	}
	if _, e = os.Stat(f.Name()); !os.IsNotExist(e) {
		t.Fatal("temporary file retained")
	}
}
