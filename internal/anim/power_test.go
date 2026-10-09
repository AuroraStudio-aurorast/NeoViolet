package anim

import (
	"errors"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// reading returns a reader that answers with one reading.
func reading(status power.Status) func() (power.Status, error) {
	return func() (power.Status, error) { return status, nil }
}

// onBattery is a machine running off a battery with a charge left.
func onBattery(percent int) power.Status {
	return power.Status{Percent: percent, OnBattery: true, Known: true}
}

func TestLowBattery_AsksOnlyWhenItShould(t *testing.T) {
	tests := []struct {
		name      string
		reader    func() (power.Status, error)
		warnBelow int
		answered  bool
		want      bool
	}{
		{"a battery under the line", reading(onBattery(18)), 20, false, true},
		{"a battery at the line", reading(onBattery(20)), 20, false, true},
		{"a battery over the line", reading(onBattery(21)), 20, false, false},
		{"a machine plugged in", reading(power.Status{Percent: 18, Known: true}), 20, false, false},
		{"a machine with no battery", reading(power.Status{}), 20, false, false},
		{"a warning that is turned off", reading(onBattery(18)), 0, false, false},
		{"a warning already answered", reading(onBattery(18)), 20, true, false},
		{"no reader at all", nil, 20, false, false},
		{
			"a reader that fails",
			func() (power.Status, error) { return power.Status{}, errors.New("unreadable") },
			20, false, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := New()
			state.ReadPower = tt.reader
			state.answered = tt.answered

			answer, low := state.LowBattery(tt.warnBelow)
			if low != tt.want {
				t.Fatalf("LowBattery(%d) = %v, want %v", tt.warnBelow, low, tt.want)
			}
			if low && !answer.OnBattery {
				t.Errorf("the answer carries %+v, want the low battery behind it", answer)
			}
		})
	}
}

// TestHold_ReadsNothingUntilApproved pins down the point of asking first. The
// fixture is a truncated file, so a surface that had read it would be carrying
// its parse error, and the only way that error can appear is on the other side
// of the answer.
func TestHold_ReadsNothingUntilApproved(t *testing.T) {
	state := New()
	state.Hold(onBattery(18), loadAt(audioWith(t, brokenFixture), 40, 12, config.GateModeEither))

	if !state.Held() {
		t.Fatal("the surface is not holding the load")
	}
	if state.Loading {
		t.Error("nothing is being loaded, so Loading must be false")
	}
	if state.Err != nil {
		t.Errorf("the sidecar was read before the warning was answered: %v", state.Err)
	}
	if got := state.View(); got != "" {
		t.Errorf("a held surface draws nothing, got %q", got)
	}
	if got := state.Reading().Percent; got != 18 {
		t.Errorf("Reading().Percent = %d, want the battery it was held for", got)
	}

	cmd := state.Approve()
	if cmd == nil {
		t.Fatal("Approve must load what it was holding")
	}
	if state.Held() {
		t.Error("the surface is still holding the load it approved")
	}
	if msg := run(t, cmd); msg.Err == nil {
		t.Error("the truncated fixture must report its own error once it is read")
	}
}

func TestHold_ClosesWhatWasShowing(t *testing.T) {
	state := loaded(t, plainFixture, 20, 9)
	if state.View() == "" {
		t.Fatal("the fixture should be drawing")
	}
	showing := state.player

	state.Hold(onBattery(18), loadAt(audioWith(t, plainFixture), 20, 9, config.GateModeEither))

	if !state.Held() {
		t.Fatal("the warning did not take the box")
	}
	if got := state.View(); got != "" {
		t.Errorf("the animation is still drawing under the warning: %q", got)
	}
	if showing == nil || !showing.Paused() {
		t.Error("the animation that was showing was left running its clock")
	}
}

func TestHold_ReplacesTheLoadItIsHolding(t *testing.T) {
	state := New()
	first := audioWith(t, plainFixture)
	second := audioWith(t, strobeFixture)

	state.Hold(onBattery(18), loadAt(first, 20, 9, config.GateModeEither))
	state.Hold(onBattery(18), loadAt(second, 20, 9, config.GateModeEither))

	msg := run(t, state.Approve())
	if want := FindSidecar(second); msg.Path != want {
		t.Errorf("Approve loaded %q, want the track it was last asked for (%q)", msg.Path, want)
	}
}

func TestClose_KeepsTheAnswer(t *testing.T) {
	state := New()
	state.ReadPower = reading(onBattery(18))
	state.Hold(onBattery(18), loadAt(audioWith(t, plainFixture), 20, 9, config.GateModeEither))
	state.Approve()

	// Escape, a tab change and a track change all end an animation by closing
	// the surface. None of them is a reason to ask again.
	state.Close()

	if state.Held() {
		t.Error("closing must clear the hold")
	}
	if _, low := state.LowBattery(20); low {
		t.Error("an answered warning is asked once, not twice")
	}
}
