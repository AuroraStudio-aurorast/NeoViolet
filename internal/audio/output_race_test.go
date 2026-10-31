//go:build race

package audio

import (
	"sync"
	"testing"
)

// This checks the happens-before relationship with the race detector rather
// than depending on the scheduler to expose a stale sample-rate value.
func TestPlayerOutputSyntheticSpeakerRateSynchronization(t *testing.T) {
	if speakerSampleRate != 0 {
		t.Skip("speaker already initialized by an audio device test")
	}
	initSpeakerOnce = sync.Once{}
	t.Cleanup(func() {
		initSpeakerOnce = sync.Once{}
		speakerSampleRate = 0
	})
	started := make(chan struct{})
	release := make(chan struct{})
	initialized := make(chan struct{})
	go func() {
		defer close(initialized)
		initSpeakerOnce.Do(func() {
			close(started)
			<-release
			speakerSampleRate = 48000
		})
	}()
	<-started
	p := NewPlayer()
	p.SetTrackerBackend("gotracker")
	t.Cleanup(func() { _ = p.Close() })
	done := make(chan error, 1)
	go func() {
		done <- p.Open("synth/testdata/minimal.mod")
	}()
	// No channel carries the initialization result to the opening goroutine.
	// Its rate read must synchronize through Once.Do, regardless of which
	// goroutine runs first after this release. Reading before Once.Do races.
	close(release)
	<-initialized
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := p.Format().SampleRate; got != 48000 {
		t.Errorf("synth rate = %v, want initialized speaker rate 48000", got)
	}
}
