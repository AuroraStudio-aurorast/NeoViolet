//go:build darwin

package mediactl

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

// TestSetup prepares the ObjC runtime the way production does on first use.
//
// It is the only thing that exercises registerObjCRuntime: setup() runs lazily
// behind sync.Once, so without this test none of the framework loading, selector
// registration or class definition would be measured on macOS.
//
// A failure means the machine cannot provide OS media integration at all, which
// is the condition newController and MacOSRun report as an error rather than
// panicking on. That makes this the assertion behind that error path.
func TestSetup(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	// setup must have actually resolved the classes the rest of this package
	// sends messages to. Returning nil without populating them would leave every
	// controller call dereferencing a null class.
	for name, class := range map[string]objc.Class{
		"NSString":               classNSString,
		"NSApplication":          classNSApplication,
		"MPNowPlayingInfoCenter": classMPNowPlayingInfoCenter,
		"MPRemoteCommandCenter":  classMPRemoteCommandCenter,
	} {
		if class == 0 {
			t.Errorf("class %s was not resolved", name)
		}
	}
	for name, sel := range map[string]objc.SEL{
		"sharedApplication": selSharedApplication,
		"run":               selRun,
	} {
		if sel == 0 {
			t.Errorf("selector %s was not registered", name)
		}
	}

	// The custom classes are the ones that cannot be registered twice, so a
	// second call is the regression guard for setup() losing its sync.Once: a
	// plain re-entry would fail here with "class already exists".
	if err := setup(); err != nil {
		t.Fatalf("second setup() error: %v", err)
	}
}

// TestCloseDetachesCommandTargets guards the dangling-target crash. The command
// center holds targets without retaining them, so a handler freed while the shared
// center still lists it leaves the next media key messaging freed memory.
//
// The count comes from the command's own handler table, which is private: the
// framework offers no way to ask what it holds. It is read the same way the
// framework reads it, and the test skips rather than fails if a later macOS moves
// the ivar, so a rename surfaces as a skip instead of a false alarm.
func TestCloseDetachesCommandTargets(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	center := objc.ID(classMPRemoteCommandCenter).Send(selSharedCommandCenter)
	play := center.Send(_cmdSels.play)
	handlers := play.Class().InstanceVariable("_handlers")
	if handlers == 0 {
		t.Skip("MPRemoteCommand has no _handlers ivar to read on this macOS")
	}

	selCount := objc.RegisterName("count")
	listed := func() uintptr { return uintptr(play.GetIvar(handlers).Send(selCount)) }

	before := listed()

	c := &darwinCtrl{remoteCmd: center}
	autoPool(func() {
		c.handler = objc.ID(classMPRemoteCommandHandler).Send(selNew)
		c.registerCommands()
	})
	if got := listed(); got != before+1 {
		t.Fatalf("registerCommands added %d handler(s), want 1", got-before)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if got := listed(); got != before {
		t.Errorf("the command still lists %d handler(s) after Close, want %d — the released handler is left dangling", got, before)
	}

	// Close is documented as idempotent, and the second call must not detach or
	// release anything a second time.
	if err := c.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
	if got := listed(); got != before {
		t.Errorf("second Close left %d handler(s), want %d", got, before)
	}
}
