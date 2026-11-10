package synth

import (
	"errors"
	"testing"
	"time"
)

func TestMidiGainAndClipping(t *testing.T) {
	p, e := newMIDIPlayer(func() (midiRenderer, error) { return &frameRenderer{pos: 25000}, nil }, 1000000, 44100)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = p.Close() }()
	if e = p.Play(); e != nil {
		t.Fatal(e)
	}
	out := make([][2]float64, 1)
	p.Stream(out)
	if out[0][0] != 0.25 {
		t.Fatalf("normal amplitude compressed %v", out)
	}
	p.mu.Lock()
	p.renderer = &frameRenderer{pos: 200000}
	p.mu.Unlock()
	p.Stream(out)
	if out[0][0] != 1 {
		t.Fatal("output not limited")
	}
	p.SetVolume(0.25)
	p.Stream(out)
	if out[0][0] < 0.5 || out[0][0] > 0.501 {
		t.Fatal("user gain not applied before clipping")
	}
}
func TestMidiSeekBoundsAndPauseIntent(t *testing.T) {
	p := testMIDIPlayer(t)
	for _, pos := range []time.Duration{-1, 100 * time.Hour, 500 * time.Millisecond} {
		p.Pause()
		if e := p.Seek(pos); e != nil {
			t.Fatal(e)
		}
		p.mu.Lock()
		done := p.seekReady
		p.mu.Unlock()
		<-done
		if p.Position() < 0 || p.Position() > p.Duration() {
			t.Fatal("seek not clamped")
		}
		if p.IsPlaying() {
			t.Fatal("paused seek resumed")
		}
		before := p.Position()
		p.Stream(make([][2]float64, 32))
		if p.Position() != before {
			t.Fatal("paused seek advanced")
		}
	}
}
func TestMidiSeekFactoryError(t *testing.T) {
	p := testMIDIPlayer(t)
	p.mu.Lock()
	p.factory = func() (midiRenderer, error) { return nil, errors.New("render construction failed") }
	p.mu.Unlock()
	if e := p.Seek(time.Second); e != nil {
		t.Fatal(e)
	}
	p.mu.Lock()
	done := p.seekReady
	p.mu.Unlock()
	<-done
	if p.Err() == nil || p.IsPlaying() {
		t.Fatal("seek failure hidden")
	}
}
