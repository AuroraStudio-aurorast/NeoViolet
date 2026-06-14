// Package mediactl integrates NeoViolet with OS-level media controls:
//   - Linux: MPRIS2 over D-Bus
//   - macOS: MPNowPlayingInfoCenter + MPRemoteCommandCenter
//   - Windows: SMTC (planned)
//
// On unsupported platforms it's a no-op.
package mediactl

import (
	"image"
	"time"
)

// CommandType identifies the kind of media control command.
type CommandType int

const (
	CmdPlayPause  CommandType = iota
	CmdPlay
	CmdPause
	CmdStop
	CmdNext
	CmdPrev
	CmdSeek        // Value: offset in microseconds (relative)
	CmdSetPosition // Value: target position in microseconds (absolute)
)

// Command carries a media-control command and optional payload.
type Command struct {
	Type  CommandType
	Value int64 // microseconds for Seek/SetPosition; zero for other commands
}

// PlayState carries the current playback info pushed to the OS.
type PlayState struct {
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	Position time.Duration
	Playing  bool
	Cover    image.Image // nil if no cover art available
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
