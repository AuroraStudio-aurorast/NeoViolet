//go:build darwin

package mediactl

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

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
		"nextTrackCommand":     _cmdSels.next,
		"previousTrackCommand": _cmdSels.prev,
	} {
		if !wired[sel] {
			t.Errorf("%s is missing from the wiring list, so its track button does nothing", name)
		}
	}
}
