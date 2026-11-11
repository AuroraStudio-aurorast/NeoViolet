//go:build darwin

package mediactl

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestRemoteCommandsUseTrackNavigation(t *testing.T) {
	if err := setup(); err != nil {
		t.Fatal(err)
	}
	setEnabled := objc.RegisterName("setEnabled:")
	isEnabled := objc.RegisterName("isEnabled")
	c := &darwinCtrl{remoteCmd: objc.ID(classMPRemoteCommandCenter).Send(selSharedCommandCenter)}
	autoPool(func() { c.handler = objc.ID(classMPRemoteCommandHandler).Send(selNew) })
	t.Cleanup(func() { _ = c.Close() })
	for _, tc := range []struct {
		name string
		sel  objc.SEL
		want bool
	}{
		{name: "next track", sel: _cmdSels.next, want: true},
		{name: "previous track", sel: _cmdSels.prev, want: true},
		{name: "skip forward", sel: _cmdSels.skipForward, want: false},
		{name: "skip backward", sel: _cmdSels.skipBackward, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := c.remoteCmd.Send(tc.sel)
			before := command.Send(isEnabled) != 0
			t.Cleanup(func() { command.Send(setEnabled, before) })
			// Registration must repair the state even after NowPlaying changes it.
			command.Send(setEnabled, !tc.want)
			autoPool(c.registerCommands)
			if got := command.Send(isEnabled) != 0; got != tc.want {
				t.Fatalf("command enabled=%v, want %v", got, tc.want)
			}
		})
	}
}
