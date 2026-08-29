//go:build darwin

package mediactl

import (
	"time"

	"github.com/ebitengine/purego/objc"
)

// Command handler callbacks — called by ObjC runtime from NSApp event loop.

func handlePlay(_ objc.ID, _ objc.SEL, _ objc.ID) int32   { return sendCmd(CmdPlay) }
func handlePause(_ objc.ID, _ objc.SEL, _ objc.ID) int32  { return sendCmd(CmdPause) }
func handleStop(_ objc.ID, _ objc.SEL, _ objc.ID) int32   { return sendCmd(CmdStop) }
func handleToggle(_ objc.ID, _ objc.SEL, _ objc.ID) int32 { return sendCmd(CmdPlayPause) }
func handleNext(_ objc.ID, _ objc.SEL, _ objc.ID) int32   { return sendCmd(CmdNext) }
func handlePrev(_ objc.ID, _ objc.SEL, _ objc.ID) int32   { return sendCmd(CmdPrev) }

func handleChangePos(_ objc.ID, _ objc.SEL, event objc.ID) int32 {
	_darwinCtrlMu.Lock()
	c := _darwinCtrl
	_darwinCtrlMu.Unlock()
	if c == nil {
		return cmdHandlerCommandFailed
	}
	pos := objc.Send[float64](event, objc.RegisterName("positionTime"))
	us := int64(pos * float64(time.Second/time.Microsecond))
	select {
	case c.cmdChan <- Command{Type: CmdSetPosition, Value: us}:
	default:
	}
	return cmdHandlerSuccess
}

func handleSleep(_ objc.ID, _ objc.SEL, _ objc.ID) {
	_darwinCtrlMu.Lock()
	c := _darwinCtrl
	_darwinCtrlMu.Unlock()
	if c == nil {
		return
	}
	select {
	case c.cmdChan <- Command{Type: CmdPause}:
	default:
	}
}

func handleWake(_ objc.ID, _ objc.SEL, _ objc.ID) {}

// registerCommands wires MPRemoteCommandCenter to our handler.
// Caller MUST be inside an autorelease pool.

func (c *darwinCtrl) registerCommands() {
	skip := nsDouble(15.0)
	arr := objc.ID(classNSArray).Send(selArrayWithObject, skip)

	c.remoteCmd.Send(_cmdSels.skipBackward).Send(selSetPreferredIntervals, arr)
	c.remoteCmd.Send(_cmdSels.skipForward).Send(selSetPreferredIntervals, arr)

	pairs := []struct{ cmd, handler objc.SEL }{
		{_cmdSels.play, _handlerSels.play},
		{_cmdSels.pause, _handlerSels.pause},
		{_cmdSels.stop, _handlerSels.stop},
		{_cmdSels.toggle, _handlerSels.toggle},
		{_cmdSels.next, _handlerSels.next},
		{_cmdSels.prev, _handlerSels.prev},
		{_cmdSels.changePos, _handlerSels.changePos},
	}
	for _, p := range pairs {
		c.remoteCmd.Send(p.cmd).Send(selAddTargetAction, c.handler, p.handler)
	}
}

// registerNotifications observes sleep/power-off/wake notifications.
// Caller MUST be inside an autorelease pool.

func (c *darwinCtrl) registerNotifications() {
	nc := objc.ID(classNSWorkspace).Send(selSharedWorkspace).Send(selNotificationCenter)
	zero := objc.ID(0)

	for _, name := range []string{
		"NSWorkspaceWillSleepNotification",
		"NSWorkspaceWillPowerOffNotification",
	} {
		nc.Send(selAddObserverSelectorName, c.handler, _handlerSels.sleep, nsString(name), zero)
	}
	nc.Send(selAddObserverSelectorName, c.handler, _handlerSels.wake, nsString("NSWorkspaceDidWakeNotification"), zero)
}
