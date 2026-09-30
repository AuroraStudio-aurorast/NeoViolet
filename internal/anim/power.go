package anim

import (
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// LowBattery reports whether the machine is running off a battery low enough to
// hold an animation back, and the reading behind that answer.
//
// It is asked before a sidecar is read, so that a warning about a flat battery
// costs nothing to find out about, and it is only asked for an animation
// somebody asked for: a load that merely follows the track stands down instead,
// because a warning nobody is waiting to read is noise.
//
// warnBelow is the charge at or below which a battery counts as low, and zero
// turns the question off. Every way of not knowing answers no: no reader at all,
// a machine with no battery, a platform this build cannot read, a call that
// fails, and a warning that has already been answered once. Nothing here is a
// safety gate, so not knowing plays the animation.
func (s *State) LowBattery(warnBelow int) (power.Status, bool) {
	if s == nil || s.ReadPower == nil || s.answered || warnBelow <= 0 {
		return power.Status{}, false
	}
	reading, err := s.ReadPower()
	if err != nil || !reading.Low(warnBelow) {
		return power.Status{}, false
	}
	return reading, true
}

// Hold parks a load behind the battery warning. Nothing is read until Approve
// runs the load this is holding, which is the whole point of asking first: a
// warning about a flat battery is worth nothing once the file has been read,
// decoded and analysed.
//
// Any animation already on the surface is closed first, because the warning
// takes the box it was drawn in.
func (s *State) Hold(reading power.Status, load Load) {
	if s == nil {
		return
	}
	s.Close()
	s.held = true
	s.reading = reading
	s.pending = load
	s.Visible = true
}

// Held reports whether a load is waiting for an answer about the battery.
// Nothing has been read while it is true, so View draws nothing and the host
// draws the warning in the same box instead.
func (s *State) Held() bool { return s != nil && s.held }

// Reading reports the power source reading behind the battery warning.
func (s *State) Reading() power.Status { return s.reading }

// Answered reports whether the battery warning has already been answered in
// this run. Only an answer is remembered: declining closes the warning, and
// asking for the animation again asks again.
func (s *State) Answered() bool { return s != nil && s.answered }
