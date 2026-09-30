// Package anim plays NVAA animations beside the track they belong to.
//
// An animation is a sidecar, found the way a lyric file is: same directory, same
// base name, .nvaa instead of .lrc. Nothing is read until the animation is asked
// for, so a library of tracks costs one stat per trigger rather than a parse per
// track.
//
// The animation follows the audio clock rather than a clock of its own. Its
// position is the audio position taken modulo the animation's length, so a
// ten-second loop over a four-minute track repeats in time with the audio, and a
// paused, seeked or replaced track needs no handling of its own -- the position
// simply follows. The player is still left to run its own clock between
// corrections, which is what keeps the frame rate free of the host's tick rate.
//
// An animation is drawn at its own size rather than stretched to the box it has,
// and centred in it, so a small animation sits in the middle of the content area
// instead of in the corner of a large block of its own background.
package anim

import (
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WhatDamon/go-nvaa-codec"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// ErrNoSidecar reports a track with no animation beside it. Most tracks will
// not have one, so callers treat this as a normal answer rather than a failure.
var ErrNoSidecar = errors.New("no animation for this track")

// minDriftMS is the floor on how far the animation may drift before it is
// corrected. It keeps a file of very short frames from seeking on every tick.
const minDriftMS = 50

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

// State is the animation surface for the current track.
//
// It holds no layout of its own. The box it draws into is handed to Sync along
// with the audio position, so the animation follows a terminal resize, the
// lyric panel being toggled, or a one-line lyric row appearing without any of
// those needing to know an animation exists.
type State struct {
	Visible bool
	Loading bool
	Err     error

	path     string
	player   *player.Player
	warning  Photosensitivity
	gated    bool
	gateMode string

	// viewW and viewH are the animation's own size, read from the file.
	// columns and lines are the area it has to draw in, as most recently
	// reported to Sync. sizedW and sizedH are the box the player was given:
	// the animation's size, capped by the area, centred in it by View.
	viewW      int
	viewH      int
	columns    int
	lines      int
	sizedW     int
	sizedH     int
	generation uint64
}

// New returns an idle animation surface: nothing loaded, nothing shown.
func New() *State { return &State{} }

// Path reports the sidecar currently loaded, or "" when none is.
func (s *State) Path() string { return s.path }

// Warning reports the photosensitivity readings behind the gate.
func (s *State) Warning() Photosensitivity { return s.warning }

// Gated reports whether the warning is waiting for the viewer to choose. While
// it is true the animation is loaded but not started, and View draws nothing:
// the host draws the warning in the same box instead.
func (s *State) Gated() bool { return s.gated }

// LoadFor reads the sidecar for audioPath and prepares it for a columns x lines
// area, drawing it at its own size when that is smaller than the area.
//
// All the work happens in the returned command, and the file is read and decoded
// in one pass so the animation and its warning arrive together. Usually the
// analysis joins them, and it is cheap next to the decode -- microseconds for
// the vectors in testdata -- and it is what catches a file that declares nothing
// while flashing anyway. gateMode says whether to make that analysis at all, and
// which of the readings stop the animation.
func (s *State) LoadFor(audioPath string, columns, lines int, gateMode string) tea.Cmd {
	s.Loading = true
	s.Err = nil
	s.gateMode = gateMode
	s.generation++
	generation := s.generation

	return func() tea.Msg {
		path := FindSidecar(audioPath)
		if path == "" {
			return LoadedMsg{Err: ErrNoSidecar, generation: generation}
		}

		animation, err := nvaa.ReadFile(path)
		if err != nil {
			return LoadedMsg{Path: path, Err: err, generation: generation}
		}

		viewW, viewH := naturalSize(animation)
		boxW, boxH := fit(viewW, viewH, columns, lines)

		return LoadedMsg{
			Path:    path,
			Warning: assess(animation, gateMode),
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
			columns:    columns,
			lines:      lines,
			generation: generation,
		}
	}
}

// Apply installs a loaded sidecar. A result older than the most recent LoadFor
// is dropped, so a second trigger or a track change during a load cannot install
// the animation that lost. The returned command starts the clock, except when
// the warning gate is up: the player is deliberately left unstarted until
// someone chooses to see it, and Init is what would otherwise begin
// immediately.
func (s *State) Apply(msg LoadedMsg) tea.Cmd {
	if msg.generation != s.generation {
		return nil
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
		return nil
	}

	s.warning = msg.Warning
	s.player = msg.player
	s.gated = s.warning.Fails(s.gateMode)
	s.Visible = true

	if s.gated {
		return nil
	}
	return s.player.Init()
}

// Sync aligns the animation with the audio, and sizes it to the box it is in.
//
// The size is passed in on every call rather than pushed from a resize hook,
// because the box depends on the terminal, the panel mode and whether the lyric
// row is showing, and one idempotent call covers all of them. The same call
// covers the clock: comparing positions here is what makes a pause, a seek and
// a track change need no case each.
func (s *State) Sync(elapsed time.Duration, playing bool, columns, lines int) tea.Cmd {
	if s == nil || !s.Visible || s.player == nil || s.gated {
		return nil
	}

	var cmds []tea.Cmd

	// The area is recorded on every call, including the ones that change
	// nothing about the player's box, because it is what View centres in.
	s.columns, s.lines = columns, lines

	boxW, boxH := fit(s.viewW, s.viewH, columns, lines)
	if boxW > 0 && boxH > 0 && (boxW != s.sizedW || boxH != s.sizedH) {
		if err := s.player.SetSize(boxW, boxH); err != nil {
			logger.Warn("animation resize failed", "error", err)
		} else {
			s.sizedW, s.sizedH = boxW, boxH
		}
	}

	// Resume reports nothing to do for a player that is already running, so the
	// playing case is safe to ask for every tick.
	if playing {
		cmds = append(cmds, s.player.Resume())
	} else if !s.player.Paused() {
		s.player.Pause()
	}

	if total := s.player.TotalMS(); total > 0 {
		target := positionFor(elapsed, total)
		if absDiff(s.player.ElapsedMS(), target) > s.tolerance(target) {
			cmds = append(cmds, s.player.SeekToTime(target))
		}
	}

	return tea.Batch(cmds...)
}

// Update forwards a message to the player, reporting whether the player claimed
// it.
//
// The player wakes itself with a tick whose type is unexported, so a host
// cannot address it by name and cannot tell it apart from anything else it does
// not recognise. Everything the host does not claim itself therefore arrives
// here, and the player answers for what was its own.
func (s *State) Update(msg tea.Msg) (tea.Cmd, bool) {
	if s == nil || s.player == nil || s.gated {
		return nil, false
	}
	return s.player.Update(msg)
}

// View renders the animation centred in the area the last Sync reported.
//
// The result is exactly that area: the animation's own box padded out with
// blanks, and nothing outside the area is written, so the border and padding
// around it remain the content area's.
func (s *State) View() string {
	if s == nil || s.player == nil || s.gated || !s.Visible {
		return ""
	}
	if s.columns <= 0 || s.lines <= 0 || s.sizedW <= 0 || s.sizedH <= 0 {
		return ""
	}
	return centre(s.player.View(), s.sizedW, s.sizedH, s.columns, s.lines)
}

// Fits reports whether the animation is already sized for a columns x lines
// area. Until it is, a frame from it would not fit the box it is drawn in, so
// the host shows nothing for that one frame rather than a frame that overflows
// the border around it.
//
// It is the area that is compared, not the player's box, because the box
// follows from the area: an animation smaller than the area has a box of its own
// size at any area large enough to hold it.
func (s *State) Fits(columns, lines int) bool {
	return s != nil && s.columns == columns && s.lines == lines
}

// Approve dismisses the warning gate and starts the animation. It is the only
// way out of the gate: a file that flashes cannot reach the screen before
// somebody has chosen to see it.
func (s *State) Approve() tea.Cmd {
	if s == nil || s.player == nil || !s.gated {
		return nil
	}
	s.gated = false
	return s.player.Init()
}

// Close hides the animation and stops its clock.
//
// Pausing is not optional. The player renews its own ticks, so a player left in
// its playing phase would keep waking up for a surface nobody is drawing -- and
// once nothing forwarded those ticks, Resume would refuse to start it again,
// reporting that a player which believes it is already running has nothing to
// do.
func (s *State) Close() {
	if s == nil {
		return
	}
	if s.player != nil {
		s.player.Pause()
	}
	s.Visible = false
	s.Loading = false
	s.gated = false
	s.gateMode = ""
	s.Err = nil
	s.path = ""
	s.player = nil
	s.warning = Photosensitivity{}
	s.viewW, s.viewH = 0, 0
	s.columns, s.lines = 0, 0
	s.sizedW, s.sizedH = 0, 0
	s.generation++
}

// naturalSize is the animation's own size: the largest viewport any of its
// frames asks for.
//
// A frame names its own viewport and the renderer draws at most that many cells,
// so the largest one is the size a box must offer for every frame to arrive at
// full size. Headers carry it, so the file's payloads are not decoded to find
// out.
func naturalSize(a *nvaa.Animation) (int, int) {
	var w, h uint32
	for header := range a.Headers() {
		w = max(w, header.ViewportW)
		h = max(h, header.ViewportH)
	}
	// #nosec G115 -- a viewport is a canvas dimension, which is checked while
	// the container is parsed and so fits an int.
	return int(w), int(h)
}

// fit is the box the player is given: the animation's own size, but never
// larger than the area it has to fit in.
//
// Handing the player the whole area instead would leave the renderer to fill the
// difference, and it fills from the left: a small animation would sit in the
// corner of a large block of its own background. Sizing the box to the animation
// leaves that difference as margin, to be centred once. An area no larger than
// the animation is passed through, and the renderer crops it to the camera, which
// is where a box that has to lose pixels should lose them.
func fit(viewW, viewH, areaW, areaH int) (int, int) {
	if areaW <= 0 || areaH <= 0 || viewW <= 0 || viewH <= 0 {
		return 0, 0
	}
	return min(viewW, areaW), min(viewH, areaH)
}

// centre places a block in the middle of an area, padding with blanks so the
// result is exactly the area.
//
// An animation is drawn at its own size rather than stretched to the box it has,
// which is what leaves something to centre. An odd remainder goes to the right
// and the bottom, which is where the rest of the program puts a cell it cannot
// divide evenly.
func centre(block string, blockW, blockH, areaW, areaH int) string {
	if blockW > areaW || blockH > areaH {
		return block
	}
	// A block that is already the whole area has nothing to centre, and it is
	// the case with the most cells to copy, so it is the one worth not copying.
	if blockW == areaW && blockH == areaH {
		return block
	}

	top := (areaH - blockH) / 2
	left := (areaW - blockW) / 2

	blank := strings.Repeat(" ", areaW)
	head := strings.Repeat(" ", left)
	tail := strings.Repeat(" ", areaW-left-blockW)

	rows := strings.Split(block, "\n")
	if len(rows) > blockH {
		rows = rows[:blockH]
	}

	// Written into one buffer, sized for what is about to go into it, rather than
	// a new string per row followed by a join of the lot: the rows are the whole
	// area, so most of those allocations were producing a row of spaces. A
	// margin row is one byte per column; an animation's row also carries the
	// escapes it was rendered with, which is the difference from its width.
	size := areaH*(areaW+1) - 1
	for _, row := range rows {
		size += len(row) - blockW
	}

	var out strings.Builder
	out.Grow(max(0, size))
	for i := range areaH {
		if i > 0 {
			out.WriteByte('\n')
		}
		row := i - top
		if row < 0 || row >= len(rows) {
			out.WriteString(blank)
			continue
		}
		out.WriteString(head)
		out.WriteString(rows[row])
		out.WriteString(tail)
	}
	return out.String()
}

// positionFor maps an audio position onto the animation's timeline.
//
// The animation repeats for as long as the track plays rather than being
// stretched to fit it or stopped at its end. A ten-second animation over a
// four-minute track therefore loops in time with the audio, and one longer than
// its track is simply left unfinished. Either way the audio clock stays the only
// clock, so nothing has to decide what to do when the two lengths disagree.
func positionFor(elapsed time.Duration, totalMS uint64) uint64 {
	if totalMS == 0 {
		return 0
	}
	ms := elapsed.Milliseconds()
	if ms <= 0 {
		return 0
	}
	return uint64(ms) % totalMS
}

// tolerance is the drift worth correcting: one whole frame.
//
// Comparing frame indices exactly would be tighter, but the audio position
// arrives on the host's tick and so trails the player's own clock by up to one
// tick. At a frame boundary that lag reads as a mismatch, and the correction it
// starts is one the next tick would undo. A frame of slack costs at most a frame
// of error, which is invisible, and settles instead of oscillating.
func (s *State) tolerance(targetMS uint64) uint64 {
	animation := s.player.Animation()
	if index := animation.FrameAtTime(targetMS); index >= 0 {
		if header, ok := animation.HeaderAt(index); ok && header.DurationMS > minDriftMS {
			return header.DurationMS
		}
	}
	return minDriftMS
}

func absDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}
