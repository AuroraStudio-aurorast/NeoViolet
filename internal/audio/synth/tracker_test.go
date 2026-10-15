package synth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gotracker/playback/output"
	"github.com/gotracker/playback/player/sampler"
)

// The guard exists because the tracker engine panics on input it cannot handle.
// These tests pin the mechanism itself: a panic must come back as an error that
// carries errTrackerPanic, and everything else must pass through untouched.
func TestGuardTrackerRecoversPanic(t *testing.T) {
	err := guardTracker("probe", func() error { panic("boom") })
	if err == nil {
		t.Fatal("guardTracker() = nil after a panic, want an error")
	}
	if !errors.Is(err, errTrackerPanic) {
		t.Errorf("guardTracker() error = %v, want it to wrap errTrackerPanic", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("guardTracker() error = %q, want it to carry the panic value", err)
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Errorf("guardTracker() error = %q, want it to name the operation", err)
	}
}

func TestGuardTrackerPassesResultsThrough(t *testing.T) {
	sentinel := errors.New("engine said no")
	err := guardTracker("probe", func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("guardTracker() error = %v, want the engine error unchanged", err)
	}
	if errors.Is(err, errTrackerPanic) {
		t.Error("guardTracker() reported a plain engine error as a panic")
	}

	if err := guardTracker("probe", func() error { return nil }); err != nil {
		t.Errorf("guardTracker() = %v, want nil", err)
	}
}

// panickingMachine is a machine whose rendering aborts the way the real engine
// does on modules it cannot handle.
type panickingMachine struct{}

func (panickingMachine) GetNumOrders() int  { return 1 }
func (panickingMachine) CanOrderLoop() bool { return false }
func (panickingMachine) GetName() string    { return "panicking machine" }
func (panickingMachine) Advance() error     { return nil }
func (panickingMachine) Render(*sampler.Sampler) error {
	return nil
}
func (panickingMachine) Tick(*sampler.Sampler) error {
	panic("Illegal bit width 125 for 8-bit sample\n")
}

// A panic mid-render must stop the player and return. The engine never filled
// receivedPremix on that path, so waiting for it would hang the audio thread for
// good — turning a caught crash into a silent freeze.
func TestRenderOneTickPanicStopsWithoutWaitingForPremix(t *testing.T) {
	p := &TrackerPlayer{
		machine:        panickingMachine{},
		receivedPremix: make(chan *output.PremixData, 1),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.renderOneTick()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("renderOneTick blocked after a panic, want it to return")
	}

	p.mu.Lock()
	finished, playing := p.finished, p.isPlaying
	p.mu.Unlock()

	if !finished {
		t.Error("renderOneTick did not mark the player finished after a panic")
	}
	if playing {
		t.Error("renderOneTick left the player marked as playing after a panic")
	}
}
