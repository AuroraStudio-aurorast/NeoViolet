// Package power reads the machine's power source: how much charge is left, and
// whether the machine is running off a battery.
//
// The reading stands between an animation and a flat battery, so it is taken on
// the way into something expensive rather than cached: one synchronous call,
// tens of microseconds, on a platform that has a battery at all.
//
// Nothing here is a safety gate. A machine with no battery, a platform this
// build cannot read, and a call that fails all read the same way -- Known false
// -- and the caller carries on.
package power

import "errors"

// ErrUnsupported reports a platform this build cannot read.
var ErrUnsupported = errors.New("power: unsupported platform")

// Status is one reading of the machine's power source.
type Status struct {
	// Percent is the charge left, 0 to 100.
	Percent int

	// OnBattery reports that the machine is running off its battery rather than
	// off external power. A machine plugged in and holding its charge is on
	// external power, and so is one that is charging.
	OnBattery bool

	// Known reports that a battery was found. A desktop, and a platform this
	// build cannot read, report false.
	Known bool
}

// Low reports whether a reading is a reason to hold back something expensive.
// warnBelow is the charge at or below which a battery counts as low, and zero
// turns the question off.
func (s Status) Low(warnBelow int) bool {
	return s.Known && s.OnBattery && warnBelow > 0 && s.Percent <= warnBelow
}

// Read asks the system for the power source.
//
// A machine with no battery is not an error: it reports Known false, because
// there is nothing to hold back for.
func Read() (Status, error) { return read() }
