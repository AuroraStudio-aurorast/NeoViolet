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
