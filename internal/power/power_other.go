//go:build !darwin && !linux && !windows

package power

// read reports that this platform's power source cannot be consulted.
func read() (Status, error) { return Status{}, ErrUnsupported }
