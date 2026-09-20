//go:build darwin

package mediactl

import (
	"math"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

// TestSkipCommand covers the conversion from a skip event's interval to the
// relative seek the UI layer consumes.
func TestSkipCommand(t *testing.T) {
	tests := []struct {
		name string
		secs float64
		sign int64
		want time.Duration
	}{
		{"forward moves by the interval the system chose", 10, 1, 10 * time.Second},
		{"backward is the same distance, negated", 10, -1, -10 * time.Second},
		{"another advertised interval is honoured", 30, 1, 30 * time.Second},
		{"a missing interval falls back to the advertised one", 0, 1, skipIntervalSeconds * time.Second},
		{"a negative interval falls back too", -1, -1, -skipIntervalSeconds * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := skipCommand(tt.secs, tt.sign)
			if cmd.Type != CmdSeek {
				t.Errorf("Type = %v, want CmdSeek", cmd.Type)
			}
			if got := time.Duration(cmd.Value) * time.Microsecond; got != tt.want {
				t.Errorf("seek distance = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSkipCommandRejectsNaN pins the one bad interval that a plain `secs <= 0`
// check would wave through, leaving the int64 conversion to invent a distance.
func TestSkipCommandRejectsNaN(t *testing.T) {
	if got, want := skipCommand(math.NaN(), 1), skipCommand(0, 1); got != want {
		t.Errorf("NaN interval produced %+v, want the missing-interval fallback %+v", got, want)
	}
}

// TestSkipCommandsAdvertiseTheirInterval asserts the number the system draws on
// the seek buttons equals the one the handler falls back to. The two are only
// useful together: an interval without a handler is a dead control, and a handler
// that ignores the interval moves a distance the button never promised.
func TestSkipCommandsAdvertiseTheirInterval(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	selPreferredIntervals := objc.RegisterName("preferredIntervals")
	selCount := objc.RegisterName("count")
	selObjectAtIndex := objc.RegisterName("objectAtIndex:")
	selDoubleValue := objc.RegisterName("doubleValue")

	// registerCommands is what sets preferredIntervals, so it has to run on a real
	// controller. Close detaches the handler from the shared singleton before
	// releasing it, so this test leaves the process-wide command center as it found
	// it instead of parking a dangling target on it for the rest of the binary.
	c := &darwinCtrl{remoteCmd: objc.ID(classMPRemoteCommandCenter).Send(selSharedCommandCenter)}
	autoPool(func() {
		c.handler = objc.ID(classMPRemoteCommandHandler).Send(selNew)
		c.registerCommands()
	})
	defer func() { _ = c.Close() }()

	for name, sel := range map[string]objc.SEL{
		"skipForwardCommand":  _cmdSels.skipForward,
		"skipBackwardCommand": _cmdSels.skipBackward,
	} {
		if sel == 0 {
			t.Fatalf("selector %s was not registered", name)
		}
		intervals := c.remoteCmd.Send(sel).Send(selPreferredIntervals)
		if n := intervals.Send(selCount); n != 1 {
			t.Errorf("%s advertises %d intervals, want 1", name, n)
			continue
		}
		got := objc.Send[float64](intervals.Send(selObjectAtIndex, 0), selDoubleValue)
		if got != skipIntervalSeconds {
			t.Errorf("%s advertises %v seconds, want %v", name, got, skipIntervalSeconds)
		}
	}
}

// TestRemoteCommandHandlersAreImplemented checks the wiring list against the
// handler class. addTarget:action: with a selector the class does not implement is
// a silent no-op, so this is what proves every command the app claims to service
// has somewhere for its event to land.
func TestRemoteCommandHandlersAreImplemented(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	handlerClass := objc.ID(classMPRemoteCommandHandler)
	selInstancesRespond := objc.RegisterName("instancesRespondToSelector:")

	wired := make(map[objc.SEL]bool)
	for _, p := range remoteCommandHandlers() {
		if p.cmd == 0 || p.handler == 0 {
			t.Errorf("unregistered selector in the wiring list (cmd=%v handler=%v)", p.cmd, p.handler)
			continue
		}
		wired[p.cmd] = true
		if handlerClass.Send(selInstancesRespond, p.handler) == 0 {
			t.Errorf("handler class does not implement the handler for command %v", p.cmd)
		}
	}

	for name, sel := range map[string]objc.SEL{
		"skipForwardCommand":  _cmdSels.skipForward,
		"skipBackwardCommand": _cmdSels.skipBackward,
	} {
		if !wired[sel] {
			t.Errorf("%s is missing from the wiring list, so its seek button does nothing", name)
		}
	}
}
