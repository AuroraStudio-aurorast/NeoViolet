// Package mediactl integrates NeoViolet with OS-level media controls:
//   - Linux: MPRIS2 over D-Bus
//   - macOS: MPNowPlayingInfoCenter (planned)
//   - Windows: SMTC (planned)
//
// On unsupported platforms it's a no-op.
package mediactl

import "time"

// Command represents a playback command from the OS media control layer.
type Command int

const (
	CmdPlayPause Command = iota
	CmdPlay
	CmdPause
	CmdStop
	CmdNext
	CmdPrev
	CmdSeek
)

// PlayState carries the current playback info pushed to the OS.
type PlayState struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Position time.Duration
	Playing  bool
}

// Controller is the interface each platform backend must implement.
type Controller interface {
	// Start registers with the OS and returns a channel that receives
	// external playback commands (media keys, lock screen controls, etc.).
	// The channel is closed when Close is called.
	Start() (<-chan Command, error)

	// Update pushes the current playback state to the OS.
	// Callers should call this on every tick so that lock screens,
	// control centers and task switchers see fresh position data.
	Update(state PlayState)

	// Close unregisters from the OS and cleans up.
	Close() error
}

// New creates a platform-appropriate Controller.
// On unsupported platforms it returns a no-op controller (nil error).
func New() (Controller, error) {
	return newController()
}
