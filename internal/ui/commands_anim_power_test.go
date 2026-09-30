package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

// batteryModel is a model with a track loaded and a battery at percent, ready
// for something to be asked for.
func batteryModel(t *testing.T, fixture string, percent int) *Model {
	t.Helper()

	m := setupModel()
	m.UI.Width, m.UI.Height = 100, 30
	m.Anim = anim.New()
	m.Anim.ReadPower = func() (power.Status, error) {
		return power.Status{Percent: percent, OnBattery: true, Known: true}, nil
	}
	m.Config.Anim.Power.WarnBelow = 20
	m.Audio.Player = &mockPlayer{path: animFixturePath(t, fixture)}
	return m
}

// The battery is read before the sidecar is, which is the point of asking
// first. The fixture is a truncated file, so a surface that had read it would
// already be carrying its parse error.
func TestAnimCommand_HoldsBeforeReading(t *testing.T) {
	m := batteryModel(t, "reject-truncated.nvaa", 18)

	_, cmd := runAnim(m, invocation{})

	if cmd != nil {
		t.Fatal("a held load must not schedule a read")
	}
	if !m.Anim.Held() {
		t.Fatal("a low battery did not hold the animation back")
	}
	if m.Anim.Loading {
		t.Error("nothing is being loaded, so Loading must be false")
	}
	if m.Error.Message != "" {
		t.Errorf("error = %q, want the sidecar left unread", m.Error.Message)
	}
	if got := m.Anim.Reading().Percent; got != 18 {
		t.Errorf("the warning shows %d%%, want the charge behind it", got)
	}

	// Answering the warning is what reads the file, and a truncated file says so
	// the moment it is read.
	_, cmd = handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must load what the warning was holding")
	}
	msg, ok := cmd().(anim.LoadedMsg)
	if !ok {
		t.Fatal("the approved load did not produce a LoadedMsg")
	}
	if msg.Err == nil {
		t.Error("the truncated fixture must fail once it is read")
	}
	if m.Anim.Held() {
		t.Error("the warning is still up after being answered")
	}
}

// A charge over the line, and a warning line of zero, are both reasons not to
// ask at all.
func TestAnimCommand_PlaysWithoutAsking(t *testing.T) {
	for _, tc := range []struct {
		name      string
		percent   int
		warnBelow int
	}{
		{"a battery over the line", 21, 20},
		{"the warning turned off", 18, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := batteryModel(t, animPlainFixture, tc.percent)
			m.Config.Anim.Power.WarnBelow = tc.warnBelow

			_, cmd := runAnim(m, invocation{})

			if cmd == nil {
				t.Fatal("the animation should have been loaded")
			}
			if m.Anim.Held() {
				t.Error("the animation was held back for no reason")
			}
			if !m.Anim.Loading {
				t.Error("the load did not start")
			}
		})
	}
}

// Escape declines the battery warning: it closes, nothing is read, and the
// answer is not remembered.
func TestKey_EscapeDeclinesTheBatteryWarning(t *testing.T) {
	m := batteryModel(t, "reject-truncated.nvaa", 18)
	runAnim(m, invocation{})

	handleNormalModeKeyPress(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.Anim.Held() || m.animVisible() {
		t.Error("escape left the battery warning up")
	}
	if m.Anim.Answered() {
		t.Error("declining must not count as an answer for the rest of the run")
	}
}

// An answer, on the other hand, lasts the run: the track change that follows it
// must not ask again.
func TestBatteryAnswer_LastsTheRun(t *testing.T) {
	m := batteryModel(t, animPlainFixture, 18)
	runAnim(m, invocation{})
	if !m.Anim.Held() {
		t.Fatal("the low battery did not hold the animation back")
	}

	cmd := m.Anim.Approve()
	if cmd == nil {
		t.Fatal("approving must load what the warning was holding")
	}
	m.Anim.Apply(cmd().(anim.LoadedMsg))

	if _, low := m.Anim.LowBattery(m.warnBelow()); low {
		t.Error("the warning asks again after being answered")
	}

	handleLoadTrack(m, LoadTrackMsg{Path: animFixturePath(t, animPlainFixture)})
	if m.Anim.Held() {
		t.Error("the answered warning held the next track back")
	}
	if !m.Anim.Loading {
		t.Error("the next track's animation did not start")
	}
}

// An animation nobody asked for does not open a warning. It stands down, says
// why, and is overruled by asking for it.
func TestAnimAuto_StandsDownOnALowBattery(t *testing.T) {
	m := batteryModel(t, animPlainFixture, 18)
	m.Config.Anim.Auto = true

	_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: animFixturePath(t, animPlainFixture)})

	if m.Anim.Held() || m.Anim.Loading {
		t.Error("an animation nobody asked for was held back or started")
	}
	if cmd == nil {
		t.Error("the track itself must still load")
	}
	if !strings.Contains(m.Info.Message, "battery at 18%") {
		t.Errorf("info = %q, want it to say why the animation was skipped", m.Info.Message)
	}

	// Asking for it is what opens the warning. A track change leaves no player
	// behind until the new audio arrives, so the model gets one back first.
	m.Audio.Player = &mockPlayer{path: animFixturePath(t, animPlainFixture)}
	runAnim(m, invocation{})
	if !m.Anim.Held() {
		t.Error("asking for the animation did not reach the battery gate")
	}
}

// The battery warning borrows the content box the same way the animation does,
// so nothing in the tab bar claims the area while it is up.
func TestTabs_NoTabClaimsTheBatteryWarning(t *testing.T) {
	m := batteryModel(t, animPlainFixture, 18)
	runAnim(m, invocation{})

	withWarning := renderTabs(m)
	m.Anim.Close()
	plain := renderTabs(m)

	if withWarning == plain {
		t.Error("the tab bar looks the same with and without the battery warning, so it claims a tab that is not showing")
	}
}

// The track named on the command line goes through Init rather than through a
// track change, so it stands down in its own right.
func TestAnimAuto_StandsDownAtStartup(t *testing.T) {
	m := batteryModel(t, animPlainFixture, 18)
	m.Config.Anim.Auto = true
	// Init divides by the tick rate, which setupModel's config leaves zero.
	m.Config.TickRate = 30
	m.pendingPath = animFixturePath(t, animPlainFixture)

	m.Init()

	if m.Anim.Held() || m.Anim.Loading {
		t.Error("an animation nobody asked for was held back or started")
	}
	if !strings.Contains(m.Info.Message, "battery at 18%") {
		t.Errorf("info = %q, want it to say why the animation was skipped", m.Info.Message)
	}
}

// The battery warning has to fit the box the animation would have used: exactly
// the height of the content area, and no row wider than it, at every size the
// program runs at.
func TestPowerWarnPanel_FitsItsBox(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{
		{"the smallest usable terminal", 68, 17},
		{"a box too small for the explanation", 100, 24},
		{"the common terminal", 100, 30},
		{"a wide terminal", 140, 40},
	} {
		t.Run(size.name, func(t *testing.T) {
			m := batteryModel(t, animPlainFixture, 18)
			m.UI.Width, m.UI.Height = size.w, size.h
			runAnim(m, invocation{})
			if !m.Anim.Held() {
				t.Fatal("the fixture did not raise the battery warning")
			}

			plan := m.layoutPlan()
			box := renderContent(m, plan)
			rows := strings.Split(box, "\n")
			if len(rows) != plan.ContentHeight {
				t.Errorf("the box is %d rows, want %d", len(rows), plan.ContentHeight)
			}
			for i, row := range rows {
				if w := lipgloss.Width(row); w != plan.ContentWidth {
					t.Errorf("row %d is %d cells wide, want %d", i, w, plan.ContentWidth)
				}
			}

			plain := ansi.Strip(box)
			for _, want := range []string{"LOW BATTERY WARNING", "Battery at 18%, below 20%", "[enter]", "[esc]"} {
				if !strings.Contains(plain, want) {
					t.Errorf("the box does not say %q:\n%s", want, plain)
				}
			}
			if strings.Contains(plain, "anim.") {
				t.Errorf("the box names a setting instead of talking to whoever is listening:\n%s", plain)
			}
			t.Logf("%dx%d:\n%s", size.w, size.h, box)
		})
	}
}

// The box talks to whoever is playing music, not to whoever edits the config:
// it says what the charge is, what an animation costs, and how to answer.
func TestPowerWarnDetail_TalksToWhoeverIsListening(t *testing.T) {
	var text strings.Builder
	for _, line := range powerWarnDetail(power.Status{Percent: 18, OnBattery: true, Known: true}, 20) {
		text.WriteString(line.Text)
		text.WriteString("\n")
	}

	for _, want := range []string{
		"LOW BATTERY WARNING",
		"Battery at 18%, below 20%",
		"may drain the battery faster",
		"play anyway", "cancel",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the warning does not say %q:\n%s", want, text.String())
		}
	}
	for _, unwanted := range []string{"anim.", "warn_below", "config", "loaded"} {
		if strings.Contains(text.String(), unwanted) {
			t.Errorf("the warning says %q, which belongs to whoever edits the config:\n%s", unwanted, text.String())
		}
	}
}
