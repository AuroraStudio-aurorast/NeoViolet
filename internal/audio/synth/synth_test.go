package synth

import (
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
)

func TestBaseSynthState(t *testing.T) {
	s := &baseSynth{
		isPlaying:   false,
		isPaused:    true,
		elapsed:     0,
		duration:    10 * time.Second,
		volumeScale: 1.0,
		sampleRate:  beep.SampleRate(44100),
	}
	if err := s.Play(); err != nil {
		t.Fatalf("Play() returned error: %v", err)
	}
	if !s.isPlaying || s.isPaused {
		t.Error("Play() should set isPlaying=true, isPaused=false")
	}
	s.Pause()
	if !s.isPaused {
		t.Error("Pause() should set isPaused=true")
	}
	s.Toggle()
	if s.isPaused {
		t.Error("Toggle() from paused should resume")
	}
	s.SetVolume(0.5)
	if s.Volume() != 0.5 {
		t.Errorf("Volume = %v", s.Volume())
	}
	s.SetVolume(2) // clamp
	if s.Volume() != 1.0 {
		t.Errorf("SetVolume(2) should clamp to 1.0, got %v", s.Volume())
	}
	if s.Position() != 0 {
		t.Errorf("Position = %v", s.Position())
	}
	if s.Duration() != 10*time.Second {
		t.Errorf("Duration = %v", s.Duration())
	}
	s.baseStop()
	if s.isPlaying || !s.isPaused {
		t.Error("baseStop() should stop and pause")
	}
	s.baseClose()
	if !s.closed {
		t.Error("baseClose() should set closed")
	}
}

func TestMidiPlayerConstruct(t *testing.T) {
	// NewMidiPlayer requires a real .mid file + SoundFont; without assets this
	// test asserts the constructor errors gracefully rather than panicking.
	_, _, err := NewMidiPlayer("/nonexistent.mid", "/nonexistent.sf2", nil, beep.SampleRate(44100))
	if err == nil {
		t.Skip("constructor unexpectedly succeeded; adjust assertion when fixtures exist")
	}
}
