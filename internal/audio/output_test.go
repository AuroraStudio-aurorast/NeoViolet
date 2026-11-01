package audio

import (
	"errors"
	"fmt"
	"image"
	"os"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
)

type outputSynth struct {
	playErr  error
	paused   bool
	closed   bool
	finished bool
}

func (s *outputSynth) Play() error              { s.paused = false; s.finished = false; return s.playErr }
func (s *outputSynth) Pause()                   { s.paused = true }
func (s *outputSynth) Stop()                    { s.paused = true }
func (s *outputSynth) Seek(time.Duration) error { return nil }
func (s *outputSynth) SetVolume(float64)        {}
func (s *outputSynth) Volume() float64          { return 1 }
func (s *outputSynth) Duration() time.Duration  { return time.Second }
func (s *outputSynth) Position() time.Duration  { return 0 }
func (s *outputSynth) Close() error             { s.closed = true; return nil }
func (s *outputSynth) Title() string            { return "synth" }
func (s *outputSynth) Artist() string           { return "" }
func (s *outputSynth) CoverImage() image.Image  { return nil }
func (s *outputSynth) Streamer() synth.Streamer { return s }
func (s *outputSynth) Format() beep.Format {
	return beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}
}
func (s *outputSynth) Err() error { return nil }
func (s *outputSynth) Stream(samples [][2]float64) (int, bool) {
	if s.closed {
		panic("streamed closed synth")
	}
	if s.finished {
		return 0, false
	}
	for i := range samples {
		samples[i] = [2]float64{0.25, 0.25}
		if s.paused {
			samples[i] = [2]float64{}
		}
	}
	return len(samples), true
}

func TestPlayerSynthPlayFailure(t *testing.T) {
	want := errors.New("synth start failed")
	p := NewPlayer()
	p.synthActive = true
	p.synthCtrl = &outputSynth{playErr: want, paused: true}
	t.Cleanup(func() { _ = p.Close() })
	if err := p.Play(); !errors.Is(err, want) {
		t.Fatalf("Play() = %v, want %v", err, want)
	}
	if p.IsPlaying() {
		t.Error("failed Play marked the player as playing")
	}
}

// Render through the same mixer beep's speaker uses, without opening a device.
func outputMixer(t *testing.T) *beep.Mixer {
	t.Helper()
	mixer := &beep.Mixer{}
	prev := playToSpeaker
	playToSpeaker = mixer.Add
	t.Cleanup(func() { playToSpeaker = prev })
	return mixer
}

type outputStream struct {
	fakeStream
	closed bool
}

func (s *outputStream) Stream(samples [][2]float64) (int, bool) {
	if s.closed {
		panic("streamed closed decoder")
	}
	for i := range samples {
		samples[i] = [2]float64{0.125, 0.125}
		s.pos++
	}
	return len(samples), true
}
func (s *outputStream) Close() error { s.closed = true; return nil }

func outputPlayer(t *testing.T) (*Player, *outputStream) {
	t.Helper()
	p := NewPlayer()
	s := &outputStream{fakeStream: fakeStream{total: 44100 * 60}}
	p.setupStreamer(s, beep.Format{SampleRate: 44100, NumChannels: 2, Precision: 2}, nopCloser{}, "track.wav")
	t.Cleanup(func() { _ = p.Close() })
	return p, s
}

func assertOutput(t *testing.T, m *beep.Mixer, want float64) {
	t.Helper()
	var samples [8][2]float64
	m.Stream(samples[:])
	for _, sample := range samples {
		if sample != [2]float64{want, want} {
			t.Fatalf("rendered %v, want stereo %v", sample, want)
		}
	}
}

func TestPlayerOutputOwnership(t *testing.T) {
	m := outputMixer(t)
	old, stream := outputPlayer(t)
	if err := old.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.125)
	candidate := NewPlayer()
	path := t.TempDir() + "/broken.wav"
	if err := os.WriteFile(path, []byte("RIFFbroken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := candidate.Open(path); err == nil {
		t.Fatal("Open corrupted audio succeeded")
	}
	_ = candidate.Close()
	assertOutput(t, m, 0.125)
	if stream.closed || !old.IsPlaying() {
		t.Fatal("candidate failure disturbed old player")
	}

	replacement, _ := outputPlayer(t)
	old.Pause()
	if err := replacement.Play(); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	assertOutput(t, m, 0.125)
	if !stream.closed {
		t.Error("old decoder was not closed")
	}
}

func TestPlayerOutputPauseResumeSeekStop(t *testing.T) {
	m := outputMixer(t)
	p, s := outputPlayer(t)
	other, _ := outputPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if err := other.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25)
	p.Pause()
	pos := s.pos
	assertOutput(t, m, 0.125)
	if s.pos != pos {
		t.Fatal("pause advanced decoder")
	}
	p.Resume()
	assertOutput(t, m, 0.25)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25) // Play again must not attach a duplicate output.
	if err := p.Seek(time.Second); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25)
	if s.seeks[0] != 44100 {
		t.Fatalf("seek = %v", s.seeks)
	}
	p.Pause()
	if err := p.Seek(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.125)
	p.Resume()
	assertOutput(t, m, 0.25)
	p.Stop()
	assertOutput(t, m, 0.125)
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25)
}

func TestPlayerOutputSynthHandoff(t *testing.T) {
	for _, synthFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("synthFirst=%v", synthFirst), func(t *testing.T) {
			m := outputMixer(t)
			regular, _ := outputPlayer(t)
			s := &outputSynth{paused: true}
			synthetic := NewPlayer()
			synthetic.synthCtrl, synthetic.synthActive = s, true
			t.Cleanup(func() { _ = synthetic.Close() })
			old, next := regular, synthetic
			want := 0.25
			if synthFirst {
				old, next = synthetic, regular
				want = 0.125
			}
			if err := old.Play(); err != nil {
				t.Fatal(err)
			}
			old.Pause()
			if err := next.Play(); err != nil {
				t.Fatal(err)
			}
			_ = old.Close()
			assertOutput(t, m, want)
		})
	}
}

func TestPlayerOutputPlayAfterDrain(t *testing.T) {
	m := outputMixer(t)
	p := NewPlayer()
	s := &outputSynth{paused: true}
	p.synthCtrl, p.synthActive = s, true
	t.Cleanup(func() { _ = p.Close() })
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25)
	s.finished = true
	assertOutput(t, m, 0)
	if m.Len() != 0 {
		t.Fatal("finished track did not drain from mixer")
	}
	// Synth Play resets the backend's finished state; its speaker entry must
	// also be reattached after the mixer drained it.
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.25)
	if m.Len() != 1 {
		t.Fatal("Play did not reattach output after it drained")
	}
}

func TestPlayerOutputCloseWhilePlaying(t *testing.T) {
	m := outputMixer(t)
	old, stream := outputPlayer(t)
	replacement, _ := outputPlayer(t)
	if err := old.Play(); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Play(); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0.125)
	if !stream.closed {
		t.Error("Close did not close decoder")
	}
}

func TestPlayerOutputSynthCloseCleansResources(t *testing.T) {
	m := outputMixer(t)
	p := NewPlayer()
	s := &outputSynth{paused: true}
	p.synthCtrl, p.synthActive = s, true
	path := t.TempDir() + "/stdin.mid"
	if err := os.WriteFile(path, []byte("MThd"), 0600); err != nil {
		t.Fatal(err)
	}
	p.tempFiles = []string{path}
	p.cachedSFPath = "cached.sf2"
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, m, 0)
	if !s.closed || p.synthCtrl != nil || p.synthActive || p.IsPlaying() {
		t.Error("Close retained synth or playing state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("temporary synth input remains: %v", err)
	}
	if p.cachedSFPath != "" || p.tempFiles != nil {
		t.Error("Close retained SoundFont cache or temporary paths")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
