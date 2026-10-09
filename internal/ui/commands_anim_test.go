package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// The fixtures are the decoder's conformance vectors, shared with internal/anim
// so both packages test against the same real files.
const (
	animPlainFixture = "keyframe-and-delta.nvaa" // two frames, declares nothing
	animGateFixture  = "strobe.nvaa"             // declares a failed flash check
)

// gateReadings is what the strobe fixture reports, written out so the panel can
// be tested without reading a file: both readings failed it, on the same numbers
// the declaration carries.
func gateReadings() anim.Photosensitivity {
	return anim.Photosensitivity{
		Declared: player.Warning{
			Verdict: player.VerdictFail, General: 5, Red: 0, Area: 1000, HasArea: true,
		},
		Assessment: photosensitivity.Assessment{
			Verdict:                   photosensitivity.VerdictFail,
			FramesAnalyzed:            12,
			Columns:                   8,
			Lines:                     4,
			GeneralFlashesPerSecond:   5,
			RedFlashesPerSecond:       0,
			MaxFlashAreaPermille:      1000,
			AreaThresholdPermille:     250,
			MaxLuminanceDeltaPermille: 1000,
			LuminanceDeltaPermille:    100,
		},
	}
}

// The gate is drawn inside the content box, which is sized by the layout
// contract. A line wider than the box or a block taller than it would push the
// border out and break the frame around it, so both limits are hard.
func TestWarnPanel_FitsItsBox(t *testing.T) {
	readings := gateReadings()

	cases := []struct {
		name string
		w, h int
	}{
		{"narrowest reachable box", 38, 4},
		{"smallest usable terminal", 62, 4},
		{"one row more than the compact tier needs", 62, 5},
		{"exactly the detail threshold", 64, warnDetailRows},
		{"typical", 64, 17},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := warnCompact(readings)
			if tc.h >= warnDetailRows {
				lines = warnDetail(readings)
			}
			got := layoutWarnLines(lines, tc.w, tc.h)

			rows := strings.Split(got, "\n")
			if len(rows) != tc.h {
				t.Fatalf("panel is %d rows, want exactly %d", len(rows), tc.h)
			}
			for i, row := range rows {
				if w := lipgloss.Width(row); w > tc.w {
					t.Errorf("row %d is %d cells wide, want at most %d: %q", i, w, tc.w, row)
				}
			}
			// The choice has to survive: it is the point of the gate, so it is
			// written before the provenance line that a cramped box drops.
			if !strings.Contains(got, "esc") {
				t.Errorf("the way out was lost:\n%s", got)
			}
			t.Logf("%dx%d:\n%s", tc.w, tc.h, got)
		})
	}
}

// Which reading objected is worth saying, because the two can disagree and the
// answer changes what the warning means.
func TestWarnReason_NamesTheReadingThatObjected(t *testing.T) {
	failed := photosensitivity.Assessment{Verdict: photosensitivity.VerdictFail, GeneralFlashesPerSecond: 5}
	passed := photosensitivity.Assessment{Verdict: photosensitivity.VerdictPass}
	missing := photosensitivity.Assessment{}

	cases := []struct {
		name string
		p    anim.Photosensitivity
		want string
	}{
		{
			"both readings failed",
			anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictFail}, Assessment: failed},
			"our analysis agrees",
		},
		{
			"only the file objected",
			anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictFail}, Assessment: missing},
			"could not run",
		},
		{
			"only our analysis objected, nothing declared",
			anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictUnknown}, Assessment: failed},
			"declares no check",
		},
		{
			"our analysis contradicted a declared pass",
			anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictPass}, Assessment: failed},
			"declares a pass",
		},
		{
			"nothing objected",
			anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictPass}, Assessment: passed},
			"declares a pass",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := warnReason(tc.p); !strings.Contains(got, tc.want) {
				t.Errorf("warnReason = %q, want it to mention %q", got, tc.want)
			}
		})
	}
}

// A flash rate without the limit it broke is not something anyone can weigh.
func TestWarnTable_ShowsTheLimits(t *testing.T) {
	table := strings.Join(warnTable(gateReadings()), "\n")

	for _, want := range []string{"the file says", "our analysis", "limit", "5/s", "3/s", "100%", "25%"} {
		if !strings.Contains(table, want) {
			t.Errorf("table is missing %q:\n%s", want, table)
		}
	}
}

// A reading that was never made must not appear as a row of zeroes, which would
// read as a clean result rather than a missing one.
func TestWarnTable_OmitsReadingsThatWereNotMade(t *testing.T) {
	readings := anim.Photosensitivity{Declared: player.Warning{Verdict: player.VerdictFail, General: 5}}
	table := strings.Join(warnTable(readings), "\n")

	if !strings.Contains(table, "the file says") {
		t.Errorf("the declaration is missing:\n%s", table)
	}
	if strings.Contains(table, "our analysis") {
		t.Errorf("an analysis that never ran should have no row:\n%s", table)
	}
	if !strings.Contains(table, "limit") {
		t.Errorf("the limits are missing:\n%s", table)
	}
}

func TestAnimCommand_NoAudio(t *testing.T) {
	m := setupModel()
	_, _ = runAnim(m, invocation{})

	if m.Anim.Visible || m.Anim.Loading {
		t.Error("an animation was started without a track")
	}
	if !strings.Contains(m.Error.Message, "No audio loaded") {
		t.Errorf("error = %q, want it to name the missing track", m.Error.Message)
	}
}

// A track with no animation is answered quietly when it followed a track change,
// and named as a failure when somebody asked for one by name.
func TestAnimLoaded_NoSidecar(t *testing.T) {
	cases := []struct {
		name      string
		requested bool
		message   func(*Model) string
	}{
		{"asked for by name", true, func(m *Model) string { return m.Error.Message }},
		{"following a track change", false, func(m *Model) string { return m.Info.Message }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := setupModel()
			m.Anim = anim.New()
			m.animRequested = tc.requested

			cmd := m.Anim.LoadFor(anim.Load{Path: filepath.Join(t.TempDir(), "track.flac"), Columns: 40, Lines: 12, GateMode: config.GateModeEither})
			handleAnimLoaded(m, cmd().(anim.LoadedMsg))

			if got := tc.message(m); !strings.Contains(got, "No animation for this track") {
				t.Errorf("message = %q, want it to say there is no animation", got)
			}
			if m.animVisible() {
				t.Error("a track with no animation left one showing")
			}
		})
	}
}

func TestAnimLoaded_BrokenFile(t *testing.T) {
	m := setupModel()
	m.Anim = anim.New()

	cmd := m.Anim.LoadFor(anim.Load{Path: animFixturePath(t, "reject-truncated.nvaa"), Columns: 40, Lines: 12, GateMode: config.GateModeEither})
	handleAnimLoaded(m, cmd().(anim.LoadedMsg))

	if !strings.Contains(m.Error.Message, "Animation failed to load") {
		t.Errorf("error = %q, want it to report the decode failure", m.Error.Message)
	}
	if m.animVisible() {
		t.Error("a broken file left an animation showing")
	}
}

// The toggle is what ":anim" does, and a second call takes the animation away
// without touching the tab the box would otherwise be showing.
func TestAnimCommand_TogglesTheAnimation(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	if !m.animVisible() {
		t.Fatal("the fixture did not show an animation")
	}

	tab := m.UI.ActiveTab
	if _, _ = runAnim(m, invocation{}); m.animVisible() {
		t.Error("a second :anim did not close the animation")
	}
	if m.UI.ActiveTab != tab {
		t.Errorf("ActiveTab = %d, want %d: the animation never owned a tab", m.UI.ActiveTab, tab)
	}
}

// While the animation shows, the tab bar must not claim a tab is on screen: the
// animation borrows the content box rather than being a page of its own.
func TestTabs_NoTabClaimsTheAnimationBox(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)

	shown := renderTabs(m)
	m.Anim.Close()
	plain := renderTabs(m)

	if shown == plain {
		t.Error("the tab bar looks the same with and without the animation, so it claims a tab that is not showing")
	}
}

// Escape closes the animation where it stands, including while the gate is up,
// which is how somebody declines to watch it.
func TestKey_EscapeClosesTheAnimation(t *testing.T) {
	for _, fixture := range []string{animPlainFixture, animGateFixture} {
		t.Run(fixture, func(t *testing.T) {
			m := animModel(t, fixture, 100, 30)

			handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})

			if m.animVisible() {
				t.Error("escape left the animation up")
			}
		})
	}
}

// Asking to go to another tab means leaving the animation, because the animation
// is borrowing the box that tab would be drawn in.
func TestKey_TabSwitchClosesTheAnimation(t *testing.T) {
	cases := []struct {
		name     string
		focus    Focus
		key      tea.KeyPressMsg
		movesTab bool
	}{
		{"next tab", FocusContent, tea.KeyPressMsg{Code: ']', Text: "]"}, true},
		{"previous tab", FocusContent, tea.KeyPressMsg{Code: '[', Text: "["}, true},
		{"arrow on the tab bar", FocusTabBar, tea.KeyPressMsg{Code: tea.KeyRight}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := animModel(t, animPlainFixture, 100, 30)
			m.UI.Focus = tc.focus
			before := m.UI.ActiveTab

			handleNormalModeKeyPress(m, tc.key)

			if m.animVisible() {
				t.Error("switching tabs left the animation up")
			}
			if tc.movesTab && m.UI.ActiveTab == before {
				t.Errorf("ActiveTab = %d, want it to have moved from %d", m.UI.ActiveTab, before)
			}
		})
	}
}

// Cycling focus goes nowhere: the tab on screen does not change, so the
// animation has no reason to close.
func TestKey_FocusCycleKeepsTheAnimation(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)

	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyTab})

	if !m.animVisible() {
		t.Error("cycling focus closed the animation")
	}
}

// The gate swallows nothing entirely: the transport keeps working while somebody
// decides, and enter is what lets the animation start.
func TestGate_EnterStartsAndEscapeDeclines(t *testing.T) {
	m := animModel(t, animGateFixture, 100, 30)
	if !m.Anim.Gated() {
		t.Fatal("the strobe fixture should raise the gate")
	}
	if m.Anim.View() != "" {
		t.Error("a gated animation drew something before it was approved")
	}

	_, cmd := handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Anim.Gated() {
		t.Error("enter left the gate up")
	}
	if cmd == nil {
		t.Error("enter did not start the clock")
	}
	if !m.animVisible() {
		t.Error("the animation is not showing after approval")
	}

	declined := animModel(t, animGateFixture, 100, 30)
	handleNormalModeKeyPress(declined, tea.KeyPressMsg{Code: tea.KeyEscape})
	if declined.animVisible() || declined.Anim.Gated() {
		t.Error("escape left the gate up")
	}
}

// The animation is a sidecar of the track, so it takes the content box and the
// tab description goes away while it is there.
func TestContent_AnimationBorrowsTheBox(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	plan := m.layoutPlan()

	frame, ok := animFrame(m, plan)
	if !ok {
		t.Fatal("the animation did not claim the content box")
	}
	if frame == "" {
		t.Error("the animation claimed the box and drew nothing")
	}
	if strings.Contains(frame, m.UI.Tabs[m.UI.ActiveTab]) {
		t.Error("the tab description is still in the box")
	}

	// The frame is what the box holds, and the description is not.
	box := renderContent(m, plan)
	if !strings.Contains(ansi.Strip(box), firstGlyphLine(frame)) {
		t.Error("the box does not hold the frame")
	}
	if strings.Contains(box, m.UI.Tabs[m.UI.ActiveTab]) {
		t.Error("the tab description is still in the box")
	}

	m.Anim.Close()
	if _, ok := animFrame(m, plan); ok {
		t.Error("a closed animation still draws a frame into the content box")
	}
	if _, ok := animBody(m, plan); ok {
		t.Error("a closed animation still claims the content box")
	}
}

// The gate goes in the animation's box, not over the interface: the lyrics panel
// and the transport control stay visible and usable while it is up.
func TestContent_GateTakesTheSameBox(t *testing.T) {
	m := animModel(t, animGateFixture, 100, 30)
	plan := m.layoutPlan()

	body, ok := animBody(m, plan)
	if !ok {
		t.Fatal("the gate did not claim the content box")
	}
	if !strings.Contains(body, "PHOTOSENSITIVITY") {
		t.Errorf("the gate is not what was drawn:\n%s", body)
	}
	if rows := strings.Split(body, "\n"); len(rows) != plan.ContentInnerH {
		t.Errorf("the gate filled %d rows, want %d", len(rows), plan.ContentInnerH)
	}
}

// The content box keeps its size and stays inside its region whichever body it
// is given. This is the layout contract seen from the renderer: an animation or a
// warning that overflowed would push the border out and break the frame around
// it, and the tab beside it would no longer line up.
func TestContent_BoxKeepsItsSize(t *testing.T) {
	for _, fixture := range []string{animPlainFixture, animGateFixture} {
		t.Run(fixture, func(t *testing.T) {
			for _, size := range []struct{ w, h int }{{68, 17}, {100, 30}, {140, 40}} {
				m := animModel(t, fixture, size.w, size.h)
				plan := m.layoutPlan()
				got := renderContent(m, plan)

				rows := strings.Split(got, "\n")
				if len(rows) != plan.ContentHeight {
					t.Fatalf("%dx%d: the box is %d rows, want %d", size.w, size.h, len(rows), plan.ContentHeight)
				}
				for i, row := range rows {
					if w := lipgloss.Width(row); w != plan.ContentWidth {
						t.Fatalf("%dx%d: row %d is %d cells wide, want %d: %q",
							size.w, size.h, i, w, plan.ContentWidth, row)
					}
				}
			}
		})
	}
}

// animModel returns a model showing the named fixture in a w x h terminal.
func animModel(t *testing.T, fixture string, w, h int) *Model {
	t.Helper()

	m := setupModel()
	m.UI.Width, m.UI.Height = w, h
	m.Anim = anim.New()

	plan := m.layoutPlan()
	cmd := m.Anim.LoadFor(anim.Load{Path: animFixturePath(t, fixture), Columns: plan.ContentInnerW, Lines: plan.ContentInnerH, GateMode: config.GateModeEither})
	if cmd == nil {
		t.Fatal("LoadFor returned no command")
	}
	msg, ok := cmd().(anim.LoadedMsg)
	if !ok {
		t.Fatal("the load did not produce a LoadedMsg")
	}
	m.Anim.Apply(msg)
	return m
}

// animFixturePath copies a decoder vector next to a synthetic track path. The
// audio file itself is never opened: finding a sidecar is a stat beside the
// track, so a UI test needs no audio backend.
func animFixturePath(t *testing.T, fixture string) string {
	t.Helper()

	dir := t.TempDir()
	// #nosec G304 -- the path is internal/anim's own testdata and the name is one
	// of the fixtures named above.
	data, err := os.ReadFile(filepath.Join("..", "anim", "testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- the directory is a test temporary directory, not user input.
	if err := os.WriteFile(filepath.Join(dir, "track.nvaa"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "track.flac")
}

// A track named on the command line never goes through handleLoadTrack, so the
// animation has a second entry point at Init. Without it, "play animations on
// their own" would miss the one track that is certain to be there.
func TestAnimAuto_StartsTheTrackNamedOnTheCommandLine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		auto      bool
		wantStart bool
	}{
		{"auto on", true, true},
		{"auto off", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := setupModel()
			m.Anim = anim.New()
			m.Config.Anim.Auto = tc.auto
			// This is the first test to call Init, which divides by the tick rate,
			// and setupModel's config leaves it zero.
			m.Config.TickRate = 30
			m.pendingPath = animFixturePath(t, animPlainFixture)

			m.Init()

			// LoadFor marks the surface loading before it returns its command, so
			// this says whether a load was started without running the batch, and
			// without opening the audio file the other command in it names.
			if got := m.Anim.Loading; got != tc.wantStart {
				t.Errorf("animation loading = %v, want %v", got, tc.wantStart)
			}
		})
	}
}

// The other entry point: a track that replaces the current one.
func TestAnimAuto_FollowsATrackChange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		auto      bool
		wantStart bool
	}{
		{"auto on", true, true},
		{"auto off", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := setupModel()
			m.Anim = anim.New()
			m.Config.Anim.Auto = tc.auto

			handleLoadTrack(m, LoadTrackMsg{Path: animFixturePath(t, animPlainFixture)})

			if got := m.Anim.Loading; got != tc.wantStart {
				t.Errorf("animation loading = %v, want %v", got, tc.wantStart)
			}
		})
	}
}

// The gate mode reaches the animation from the config, and the command and the
// track-change path pass it through unchanged.
func TestAnimGateMode_ReachesTheAnimation(t *testing.T) {
	for _, tc := range []struct {
		mode      string
		wantGated bool
	}{
		// strobe.nvaa fails its own check, so two of the three modes stop it.
		{config.GateModeEither, true},
		{config.GateModeDeclared, true},
		{config.GateModeOff, false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			m := setupModel()
			m.UI.Width, m.UI.Height = 100, 30
			m.Anim = anim.New()
			m.Config.Anim.Photosensitivity.Mode = tc.mode

			// The config is what the two load sites hand to LoadFor.
			if got := m.gateMode(); got != tc.mode {
				t.Fatalf("gateMode() = %q, want %q", got, tc.mode)
			}

			plan := m.layoutPlan()
			cmd := m.Anim.LoadFor(anim.Load{Path: animFixturePath(t, animGateFixture), Columns: plan.ContentInnerW, Lines: plan.ContentInnerH, GateMode: m.gateMode()})
			m.Anim.Apply(cmd().(anim.LoadedMsg))

			if got := m.Anim.Gated(); got != tc.wantGated {
				t.Errorf("gated = %v, want %v in mode %q", got, tc.wantGated, tc.mode)
			}
		})
	}
}

// firstGlyphLine is the first line of a frame that has something in it. An
// animation smaller than the area it sits in is centred, so the lines before it
// are blank.
func firstGlyphLine(frame string) string {
	for _, line := range strings.Split(ansi.Strip(frame), "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// A load that lost to a newer one says nothing about the animation that won: not
// the news the newer one brought, and not the state of a request that is still
// waiting. Both arrive as messages, and the older one can arrive last.
func TestAnimLoaded_AnOlderResultSaysNothingAboutTheCurrentOne(t *testing.T) {
	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 30
	m.Anim = anim.New()
	m.Anim.ReadPower = func() (power.Status, error) { return power.Status{}, nil }

	plan := m.layoutPlan()
	load := func(path string) anim.Load {
		return anim.Load{Path: path, Columns: plan.ContentInnerW, Lines: plan.ContentInnerH, GateMode: config.GateModeEither}
	}

	// One load is under way when a second starts and finishes first, with a
	// broken file to report.
	older := m.Anim.LoadFor(load(animFixturePath(t, animPlainFixture)))
	newer := m.Anim.LoadFor(load(animFixturePath(t, "reject-truncated.nvaa")))
	if _, cmd := handleAnimLoaded(m, newer().(anim.LoadedMsg)); cmd != nil {
		t.Error("a broken animation asked for a command")
	}
	if m.Error.Message == "" {
		t.Fatal("the broken animation was not reported")
	}

	// Now the older one arrives. It is not about what is on the surface, and the
	// request that is waiting is not its to answer, so it clears neither.
	m.Error = &MessageState{}
	m.animRequested = true
	if _, _ = handleAnimLoaded(m, older().(anim.LoadedMsg)); m.Error.Message != "" {
		t.Errorf("an older result reported %q", m.Error.Message)
	}
	if !m.animRequested {
		t.Error("an older result answered for a request that is still waiting")
	}
}
