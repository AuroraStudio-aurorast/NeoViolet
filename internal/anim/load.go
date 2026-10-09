package anim

import (
	"errors"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/WhatDamon/go-nvaa-codec"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// ErrSidecarTooLarge reports an animation file too large to read whole.
var ErrSidecarTooLarge = errors.New("animation file too large")

// maxSidecarSize is the largest sidecar read into memory. A full-length
// animation for a large terminal runs to tens of megabytes, which this leaves
// room for; a file beyond it is a mistake rather than an animation.
const maxSidecarSize = 64 << 20

// Load is one request for a track's animation.
//
// It is a struct rather than a list of arguments because the same request is
// made from three places -- the command, a track change, and the track named on
// the command line -- and because a load held behind the battery warning has to
// be kept, whole, until somebody answers.
type Load struct {
	Path     string
	Columns  int
	Lines    int
	GateMode string
}

// LoadedMsg carries one track's sidecar back to the host, which hands it to
// Apply.
type LoadedMsg struct {
	Path    string
	Warning Photosensitivity
	Err     error

	player     *player.Player
	viewW      int
	viewH      int
	columns    int
	lines      int
	generation uint64
}

// LoadFor reads the sidecar a Load names and prepares it for the area it asks
// for, drawing it at its own size when that is smaller than the area.
//
// It does not ask about the battery. The caller asks with LowBattery first, so
// that a warning about a flat battery costs nothing to find out about: by the
// time anything here runs, somebody has already decided the animation is worth
// the charge. See power.go.
//
// All the work happens in the returned command, and the file is read and decoded
// in one pass so the animation and its warning arrive together. Usually the
// analysis joins them, and it is cheap next to the decode -- microseconds for
// the vectors in testdata -- and it is what catches a file that declares nothing
// while flashing anyway. GateMode says whether to make that analysis at all, and
// which of the readings stop the animation.
func (s *State) LoadFor(load Load) tea.Cmd {
	if s == nil {
		return nil
	}
	s.held = false
	s.reading = power.Status{}
	s.pending = Load{}
	// A load starts a new player, whose first resize is worth hearing about even
	// at a size this surface has already complained about.
	s.failedW, s.failedH = 0, 0
	s.Loading = true
	s.Err = nil
	s.gateMode = load.GateMode
	s.generation++
	generation := s.generation

	return func() tea.Msg {
		path := FindSidecar(load.Path)
		if path == "" {
			return LoadedMsg{Err: ErrNoSidecar, generation: generation}
		}

		blob, err := readSidecar(path)
		if err != nil {
			return LoadedMsg{Path: path, Err: err, generation: generation}
		}

		animation, err := nvaa.Parse(blob)
		if err != nil {
			return LoadedMsg{Path: path, Err: err, generation: generation}
		}

		viewW, viewH := naturalSize(animation)
		boxW, boxH := fit(viewW, viewH, load.Columns, load.Lines)

		return LoadedMsg{
			Path:    path,
			Warning: assess(animation, load.GateMode),
			player: player.New(animation, player.Options{
				Columns: boxW,
				Lines:   boxH,
				// Looping the player's own timeline is what lets a position
				// taken modulo the length stay valid: a player that stopped at
				// the end would have nothing for the wrap to land on.
				Loop: true,
				// The gate is drawn by the host in this program's own words and
				// styles, and it is the host that starts the clock. The library
				// gate would claim the space bar, which is already play/pause.
				SkipWarning: true,
				// No key belongs to the animation. Every key in this program is
				// already spoken for, and the player reports unclaimed keys back
				// as unhandled rather than swallowing them.
				Keys: &player.KeyMap{},
			}),
			viewW:      viewW,
			viewH:      viewH,
			columns:    load.Columns,
			lines:      load.Lines,
			generation: generation,
		}
	}
}

// readSidecar reads a sidecar whole, refusing one larger than maxSidecarSize.
//
// The read is bounded rather than measured and then trusted: an animation is read
// into one buffer, so a file that is enormous -- or a device file that never ends
// -- has to be refused while it is being read.
func readSidecar(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- the animation beside the user's own track
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxSidecarSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSidecarSize {
		return nil, ErrSidecarTooLarge
	}
	return data, nil
}

// Apply installs a loaded sidecar and reports whether it was the one this
// surface was waiting for. A result older than the most recent LoadFor is
// dropped, so a second trigger or a track change during a load cannot install
// the animation that lost -- and the host is told, because a result that lost has
// nothing to say about the animation that won. The returned command starts the
// clock, except when the warning gate is up: the player is deliberately left
// unstarted until someone chooses to see it, and Init is what would otherwise
// begin immediately.
func (s *State) Apply(msg LoadedMsg) (tea.Cmd, bool) {
	if s == nil || msg.generation != s.generation {
		return nil, false
	}

	s.Loading = false
	s.path = msg.Path
	s.warning = Photosensitivity{}
	s.player = nil
	s.gated = false
	s.viewW, s.viewH = msg.viewW, msg.viewH
	s.columns, s.lines = msg.columns, msg.lines
	s.sizedW, s.sizedH = fit(s.viewW, s.viewH, s.columns, s.lines)
	s.Err = msg.Err

	if msg.Err != nil {
		s.Visible = false
		return nil, true
	}

	s.warning = msg.Warning
	s.player = msg.player
	s.gated = s.warning.Fails(s.gateMode)
	s.Visible = true

	if s.gated {
		return nil, true
	}
	return s.player.Init(), true
}
