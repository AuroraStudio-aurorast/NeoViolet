package anim

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// The fixtures are the decoder's own conformance vectors, copied into testdata
// so these tests exercise real files rather than files this package made up.
const (
	plainFixture  = "keyframe-and-delta.nvaa" // two 100ms frames, declares nothing
	strobeFixture = "strobe.nvaa"             // declares a failed flash check
	brokenFixture = "reject-truncated.nvaa"   // fails to decode
)

func TestLoadFor_NoSidecar(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(audioWith(t, ""), 40, 12, config.GateModeEither))

	if !errors.Is(msg.Err, ErrNoSidecar) {
		t.Fatalf("Err = %v, want ErrNoSidecar", msg.Err)
	}
	if cmd := state.Apply(msg); cmd != nil {
		t.Error("Apply returned a command for a track with no animation")
	}
	if state.Visible || state.Loading || state.Path() != "" {
		t.Errorf("visible=%v loading=%v path=%q, want an idle surface",
			state.Visible, state.Loading, state.Path())
	}
	if !errors.Is(state.Err, ErrNoSidecar) {
		t.Errorf("state.Err = %v, want ErrNoSidecar so the host can tell it from a failure", state.Err)
	}
}

func TestLoadFor_PlaysWhenNothingObjects(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(audioWith(t, plainFixture), 40, 12, config.GateModeEither))

	if msg.Err != nil {
		t.Fatalf("LoadFor error: %v", msg.Err)
	}
	// This vector declares no photosensitivity check and an analysis passes it.
	// Declaring nothing is not a reason to stop: only content that measures
	// badly is.
	if msg.Warning.Declared.Verdict != player.VerdictUnknown {
		t.Errorf("declared verdict = %v, want unknown", msg.Warning.Declared.Verdict)
	}
	if !msg.Warning.Analysed() {
		t.Error("the analysis did not run")
	}
	if msg.Warning.Fails(config.GateModeEither) {
		t.Fatalf("an animation that passes analysis was objected to: %+v", msg.Warning)
	}

	if cmd := state.Apply(msg); cmd == nil {
		t.Error("Apply should start the clock for an animation nothing objected to")
	}
	if !state.Visible || state.Gated() {
		t.Errorf("visible=%v gated=%v, want visible and ungated", state.Visible, state.Gated())
	}
	if state.View() == "" {
		t.Error("a visible animation should draw")
	}
}

func TestLoadFor_GatesAFlashingFile(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(audioWith(t, strobeFixture), 40, 12, config.GateModeEither))

	if msg.Err != nil {
		t.Fatalf("LoadFor error: %v", msg.Err)
	}
	// The file declares a failure and the analysis agrees. Both readings are
	// carried so the warning can show which one objected.
	if msg.Warning.Declared.Verdict != player.VerdictFail {
		t.Errorf("declared verdict = %v, want fail", msg.Warning.Declared.Verdict)
	}
	if msg.Warning.Assessment.Verdict != photosensitivity.VerdictFail {
		t.Errorf("measured verdict = %v, want fail", msg.Warning.Assessment.Verdict)
	}
	if !msg.Warning.Fails(config.GateModeEither) {
		t.Fatal("a file that fails both readings was not objected to")
	}

	if cmd := state.Apply(msg); cmd != nil {
		t.Error("a gated animation must not start before it is approved")
	}
	if !state.Gated() {
		t.Fatal("the gate is not up")
	}
	if state.View() != "" {
		t.Error("a gated animation must draw nothing: the warning goes in its box")
	}

	// Nothing can reach the screen until the gate is approved, and approving is
	// what starts the clock.
	if cmd := state.Approve(); cmd == nil {
		t.Error("Approve should start the clock")
	}
	if state.Gated() {
		t.Error("the gate is still up after approval")
	}
	if state.View() == "" {
		t.Error("the animation should draw after approval")
	}
	if cmd := state.Approve(); cmd != nil {
		t.Error("approving twice should do nothing")
	}
}

func TestLoadFor_BrokenFile(t *testing.T) {
	state := New()
	msg := run(t, state.LoadFor(audioWith(t, brokenFixture), 40, 12, config.GateModeEither))

	if msg.Err == nil {
		t.Fatal("a truncated file should report a decode error")
	}
	if errors.Is(msg.Err, ErrNoSidecar) {
		t.Error("a file that failed to decode is not the same as one that is missing")
	}

	// The host shows Err as a message and the animation stays closed.
	if cmd := state.Apply(msg); cmd != nil {
		t.Error("Apply returned a command for a broken file")
	}
	if state.Visible || state.Gated() {
		t.Errorf("visible=%v gated=%v, want a closed surface", state.Visible, state.Gated())
	}
	if state.Err == nil {
		t.Error("the decode error should survive Apply so the host can report it")
	}
}

// A load that lost a race must not install. A second trigger, or a track change
// while the first read is in flight, both land here.
func TestLoadFor_SupersededResultIsDropped(t *testing.T) {
	audio := audioWith(t, plainFixture)
	state := New()
	stale := run(t, state.LoadFor(audio, 40, 12, config.GateModeEither))
	fresh := run(t, state.LoadFor(audio, 40, 12, config.GateModeEither))

	if cmd := state.Apply(stale); cmd != nil {
		t.Error("a superseded load took effect")
	}
	if state.Visible {
		t.Error("a superseded load made the surface visible")
	}

	if cmd := state.Apply(fresh); cmd == nil {
		t.Error("the current load should take effect")
	}
	if !state.Visible {
		t.Error("the current load did not make the surface visible")
	}
}

func TestPositionFor(t *testing.T) {
	const totalMS = 200

	tests := []struct {
		name    string
		elapsed time.Duration
		total   uint64
		want    uint64
	}{
		{"start", 0, totalMS, 0},
		{"inside", 50 * time.Millisecond, totalMS, 50},
		{"at the end", 200 * time.Millisecond, totalMS, 0},
		{"one loop in", 250 * time.Millisecond, totalMS, 50},
		{"many loops in", 2050 * time.Millisecond, totalMS, 50},
		{"longer animation left unfinished", 300 * time.Millisecond, 5000, 300},
		{"negative clock", -time.Second, totalMS, 0},
		{"no length to loop over", time.Second, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := positionFor(tt.elapsed, tt.total); got != tt.want {
				t.Errorf("positionFor(%v, %d) = %d, want %d", tt.elapsed, tt.total, got, tt.want)
			}
		})
	}
}

func TestSync_FollowsTheAudioClock(t *testing.T) {
	state := loaded(t, plainFixture, 40, 12)
	if got := state.player.Index(); got != 0 {
		t.Fatalf("the animation started on frame %d, want 0", got)
	}

	// 199ms is somewhere other than the first frame of a 200ms animation, and
	// far enough from the player's position to be worth correcting.
	const atMS = 199
	want := state.player.Animation().FrameAtTime(atMS)
	if want == 0 {
		t.Fatalf("the fixture must have a frame at %dms other than the first", atMS)
	}

	state.Sync(atMS*time.Millisecond, true, 40, 12)
	if got := state.player.Index(); got != want {
		t.Errorf("frame = %d, want %d: the animation did not follow the audio", got, want)
	}
}

func TestSync_ResizesOnlyForARealBox(t *testing.T) {
	state := loaded(t, plainFixture, 40, 12)
	pl := state.player

	// The fixture is 8x3. An area larger than that leaves the animation at its
	// own size, so that View has a margin to centre it in.
	state.Sync(0, true, 40, 12)
	if got := pl.Timeline().Columns(); got != 8 {
		t.Errorf("columns = %d after a 40-cell area, want the animation's own 8", got)
	}
	if got := pl.Timeline().Lines(); got != 3 {
		t.Errorf("lines = %d after a 12-row area, want the animation's own 3", got)
	}

	// An area smaller than the animation is the only thing that resizes it, and
	// then only as far as the area.
	state.Sync(0, true, 5, 2)
	if got := pl.Timeline().Columns(); got != 5 {
		t.Errorf("columns = %d, want 5", got)
	}
	if got := pl.Timeline().Lines(); got != 2 {
		t.Errorf("lines = %d, want 2", got)
	}

	// A box of nothing is a layout that has not been computed yet, not a
	// request to make the animation zero cells wide.
	state.Sync(0, true, 0, 0)
	if got := pl.Timeline().Columns(); got != 5 {
		t.Errorf("columns = %d after an empty box, want 5", got)
	}
	if got := pl.Timeline().Lines(); got != 2 {
		t.Errorf("lines = %d after an empty box, want 2", got)
	}
}

// An animation is drawn at its own size and centred, not stretched to the box it
// has. The box it is handed is the content area's inner cells, so an animation
// left at the corner of them sits against the border while everything around it
// is a block of its own background.
func TestView_CentresTheAnimation(t *testing.T) {
	// The fixture is 8x3. An odd remainder has to land somewhere defined, so two
	// of these leave one and two leave none at all.
	const viewW, viewH = 8, 3
	cases := []struct {
		name string
		w, h int
	}{
		{"room on every side", 20, 9},
		{"an odd remainder", 21, 10},
		{"a margin above and below only", 8, 20},
		{"a margin either side only", 20, 3},
		{"exactly the animation", 8, 3},
		{"smaller than the animation", 4, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := loaded(t, plainFixture, c.w, c.h)

			boxW, boxH := fit(viewW, viewH, c.w, c.h)
			top := (c.h - boxH) / 2
			left := (c.w - boxW) / 2

			raw := strings.Split(state.player.View(), "\n")
			if len(raw) != boxH {
				t.Fatalf("the player drew %d rows, want %d", len(raw), boxH)
			}

			rows := strings.Split(state.View(), "\n")
			if len(rows) != c.h {
				t.Fatalf("the view is %d rows, want the area's %d", len(rows), c.h)
			}

			blank := strings.Repeat(" ", c.w)
			margin := strings.Repeat(" ", left)
			for i, row := range rows {
				if got := ansi.StringWidth(row); got != c.w {
					t.Errorf("row %d is %d cells wide, want the area's %d", i, got, c.w)
				}
				if i < top || i >= top+boxH {
					if row != blank {
						t.Errorf("row %d is %q, want a blank margin", i, row)
					}
					continue
				}
				if !strings.HasPrefix(row, margin+raw[i-top]) {
					t.Errorf("row %d is %q, want the animation's own row indented by %d columns", i, row, left)
				}
			}
		})
	}
}

// The size an animation is drawn at comes from the file rather than from the
// area, so a large terminal does not stretch it. The fixture's canvas is larger
// than the viewport its frames declare, which is what tells the two apart.
func TestLoadFor_DrawsAtTheAnimationsOwnSize(t *testing.T) {
	cases := []struct {
		fixture      string
		wantW, wantH int
	}{
		{plainFixture, 8, 3},
		{strobeFixture, 8, 4},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			state := loaded(t, c.fixture, 64, 17)

			if state.viewW != c.wantW || state.viewH != c.wantH {
				t.Errorf("the animation is %dx%d, want %dx%d", state.viewW, state.viewH, c.wantW, c.wantH)
			}
			if got := state.player.Timeline().Columns(); got != c.wantW {
				t.Errorf("the player was sized to %d columns, want the animation's own %d", got, c.wantW)
			}
			if got := state.player.Timeline().Lines(); got != c.wantH {
				t.Errorf("the player was sized to %d rows, want the animation's own %d", got, c.wantH)
			}
		})
	}
}

func TestFit(t *testing.T) {
	cases := []struct {
		name                       string
		viewW, viewH, areaW, areaH int
		wantW, wantH               int
	}{
		{"smaller than the area", 8, 3, 64, 17, 8, 3},
		{"larger than the area", 50, 18, 20, 4, 20, 4},
		{"wider than the area only", 50, 3, 20, 17, 20, 3},
		{"taller than the area only", 8, 30, 64, 9, 8, 9},
		{"exactly the area", 10, 6, 10, 6, 10, 6},
		{"empty area", 8, 3, 0, 0, 0, 0},
		{"negative area", 8, 3, -1, 5, 0, 0},
		{"empty animation", 0, 0, 64, 17, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, h := fit(c.viewW, c.viewH, c.areaW, c.areaH)
			if w != c.wantW || h != c.wantH {
				t.Errorf("fit(%d, %d, %d, %d) = %dx%d, want %dx%d",
					c.viewW, c.viewH, c.areaW, c.areaH, w, h, c.wantW, c.wantH)
			}
		})
	}
}

func TestSync_FollowsTheTransport(t *testing.T) {
	state := loaded(t, plainFixture, 40, 12)
	pl := state.player

	state.Sync(0, false, 40, 12)
	if !pl.Paused() {
		t.Error("a paused track did not pause the animation")
	}

	state.Sync(0, true, 40, 12)
	if pl.Paused() {
		t.Error("a playing track did not resume the animation")
	}

	state.Sync(0, true, 40, 12)
	if pl.Paused() {
		t.Error("asking an already-playing animation to resume paused it")
	}
}

// Close must pause, because the player renews its own ticks: one left playing
// would either keep waking up for an invisible surface or, once nothing
// forwarded those ticks, refuse to start again.
func TestClose_StopsTheClock(t *testing.T) {
	state := loaded(t, plainFixture, 40, 12)
	pl := state.player

	state.Close()

	if !pl.Paused() {
		t.Error("Close left the player running")
	}
	if state.Visible || state.Loading || state.Gated() {
		t.Errorf("visible=%v loading=%v gated=%v, want a closed surface",
			state.Visible, state.Loading, state.Gated())
	}
	if state.Path() != "" || state.Err != nil || state.View() != "" {
		t.Errorf("path=%q err=%v view=%q, want nothing left behind", state.Path(), state.Err, state.View())
	}
	if _, handled := state.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); handled {
		t.Error("a closed animation claimed a key")
	}
}

// Every key in this program already does something, so the animation must claim
// none of them: the player reports what it does not recognise as unhandled, and
// that is what leaves the host's bindings intact.
func TestUpdate_ClaimsNoKeys(t *testing.T) {
	state := loaded(t, plainFixture, 40, 12)

	keys := []tea.KeyPressMsg{
		{Code: tea.KeySpace},
		{Code: tea.KeyUp},
		{Code: tea.KeyDown},
		{Code: tea.KeyLeft},
		{Code: tea.KeyRight},
		{Code: tea.KeyEnter},
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
		{Code: 'r', Text: "r"},
		{Code: 'n', Text: "n"},
		{Code: '5', Text: "5"},
	}

	for _, key := range keys {
		cmd, handled := state.Update(key)
		if handled || cmd != nil {
			t.Errorf("the animation claimed %q", key.String())
		}
	}
}

func TestPhotosensitivity_ObjectsToTheReadingItsModeActsOn(t *testing.T) {
	pass := photosensitivity.Assessment{Verdict: photosensitivity.VerdictPass}
	fail := photosensitivity.Assessment{Verdict: photosensitivity.VerdictFail}
	unknown := photosensitivity.Assessment{}
	declaredPass := player.Warning{Verdict: player.VerdictPass}
	declaredFail := player.Warning{Verdict: player.VerdictFail}
	declaredUnknown := player.Warning{Verdict: player.VerdictUnknown}

	tests := []struct {
		name string
		p    Photosensitivity
		mode string
		want bool
	}{
		// The union is the default: either reading may object.
		{"union: both pass", Photosensitivity{declaredPass, pass}, config.GateModeEither, false},
		{"union: nothing said, nothing found", Photosensitivity{declaredUnknown, unknown}, config.GateModeEither, false},
		{"union: nothing said, analysis objects", Photosensitivity{declaredUnknown, fail}, config.GateModeEither, true},
		{"union: declared pass, analysis objects", Photosensitivity{declaredPass, fail}, config.GateModeEither, true},
		{"union: declared fail", Photosensitivity{declaredFail, pass}, config.GateModeEither, true},
		{"union: declared fail, analysis broken", Photosensitivity{declaredFail, unknown}, config.GateModeEither, true},

		// Declared reads only the file's own claim, which is all a load in that
		// mode ever makes.
		{"declared: declared fail", Photosensitivity{declaredFail, pass}, config.GateModeDeclared, true},
		{"declared: declared fail, analysis broken", Photosensitivity{declaredFail, unknown}, config.GateModeDeclared, true},
		{"declared: analysis objects alone", Photosensitivity{declaredUnknown, fail}, config.GateModeDeclared, false},
		{"declared: declared pass, analysis objects", Photosensitivity{declaredPass, fail}, config.GateModeDeclared, false},

		// Off counts neither reading.
		{"off: declared fail", Photosensitivity{declaredFail, fail}, config.GateModeOff, false},
		{"off: analysis objects", Photosensitivity{declaredUnknown, fail}, config.GateModeOff, false},

		// A mode that is not one of the three documented words behaves like the
		// union rather than like off, so a value that ever slips past Normalize
		// cannot remove the check.
		{"an unknown mode behaves like the union", Photosensitivity{declaredUnknown, fail}, "sometimes", true},
		{"an empty mode behaves like the union", Photosensitivity{declaredFail, pass}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Fails(tt.mode); got != tt.want {
				t.Errorf("Fails(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

// The modes that do not act on our analysis do not make it. It is a walk over
// every frame, so this is the difference between loading an animation and
// loading one and measuring all of it -- and Analysed() is what says the walk
// did not happen, since the fixture's analysis would otherwise return a verdict.
func TestLoadFor_GateModeSkipsTheAnalysis(t *testing.T) {
	for _, tt := range []struct {
		mode         string
		wantAnalysed bool
		wantGated    bool
	}{
		// strobe.nvaa fails its own check, so declared still stops it while off
		// plays it.
		{config.GateModeEither, true, true},
		{config.GateModeDeclared, false, true},
		{config.GateModeOff, false, false},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			state := New()
			msg := run(t, state.LoadFor(audioWith(t, strobeFixture), 40, 12, tt.mode))
			if msg.Err != nil {
				t.Fatalf("LoadFor error: %v", msg.Err)
			}
			if got := msg.Warning.Analysed(); got != tt.wantAnalysed {
				t.Errorf("Analysed() = %v, want %v in mode %q", got, tt.wantAnalysed, tt.mode)
			}

			state.Apply(msg)
			if got := state.Gated(); got != tt.wantGated {
				t.Errorf("Gated() = %v, want %v in mode %q", got, tt.wantGated, tt.mode)
			}
		})
	}
}

// A renderer is entitled to erase to the end of a terminal line, and it erases
// with whatever colour its pen happens to hold. The player therefore has to leave
// the pen reset at the end of every row it draws: otherwise such an erase would
// repaint the cells beside the animation -- the content box's own padding, and
// the lyric panel past it -- in the animation's background.
//
// The library's README names this as the one caveat a host has to know about, so
// it is measured here rather than assumed. A release that stopped resetting would
// be a visible regression in this program and invisible in the library's own
// tests.
func TestView_LeavesThePenReset(t *testing.T) {
	const columns, lines = 20, 4
	state := loaded(t, plainFixture, columns, lines)

	rows := strings.Split(state.View(), "\n")
	if len(rows) != lines {
		t.Fatalf("the animation drew %d rows, want %d", len(rows), lines)
	}
	for i, row := range rows {
		// A row that sets no style at all is the safest case rather than a
		// missing one: the margin rows above and below the animation have no
		// colour of their own to leave behind.
		if last := lastSGR(row); last != "" && last != "\x1b[0m" && last != "\x1b[m" {
			t.Errorf("row %d leaves the pen at %q, so an erase would repaint the cells beside the animation", i, last)
		}
	}
}

// sgrPattern matches a select-graphic-rendition sequence.
var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// lastSGR returns the final style sequence in a rendered row, or "" when the row
// sets no style at all.
func lastSGR(row string) string {
	all := sgrPattern.FindAllString(row, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

func TestPhotosensitivity_AnalysedTellsFailuresApart(t *testing.T) {
	if (Photosensitivity{}).Analysed() {
		t.Error("an analysis that never ran should not read as one that passed")
	}
	ran := Photosensitivity{Assessment: photosensitivity.Assessment{Verdict: photosensitivity.VerdictPass}}
	if !ran.Analysed() {
		t.Error("a completed analysis should read as analysed")
	}
}

// audioWith builds a temp directory holding an audio file and, when sidecar is
// not empty, the named testdata animation beside it under the same base name.
func audioWith(t *testing.T, sidecar string) string {
	t.Helper()

	dir := t.TempDir()
	audio := filepath.Join(dir, "track.flac")
	touch(t, audio)
	if sidecar == "" {
		return audio
	}

	// #nosec G304 -- the path is this package's own testdata and the name is one
	// of the fixtures named above.
	data, err := os.ReadFile(filepath.Join("testdata", sidecar))
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- the directory is a test temporary directory, not user input.
	if err := os.WriteFile(filepath.Join(dir, "track"+SidecarExt), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return audio
}

// run executes a load command and returns the message it produced.
func run(t *testing.T, cmd tea.Cmd) LoadedMsg {
	t.Helper()

	if cmd == nil {
		t.Fatal("LoadFor returned no command")
	}
	msg, ok := cmd().(LoadedMsg)
	if !ok {
		t.Fatalf("the load produced %T, want LoadedMsg", msg)
	}
	return msg
}

// loaded returns a surface with the named fixture already applied.
func loaded(t *testing.T, sidecar string, columns, lines int) *State {
	t.Helper()

	state := New()
	msg := run(t, state.LoadFor(audioWith(t, sidecar), columns, lines, config.GateModeEither))
	if msg.Err != nil {
		t.Fatalf("LoadFor(%s) error: %v", sidecar, msg.Err)
	}
	state.Apply(msg)
	if state.player == nil {
		t.Fatal("the fixture did not load a player")
	}
	return state
}
