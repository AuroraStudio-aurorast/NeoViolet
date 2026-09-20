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

// skipIntervalSeconds is both the interval advertised on the skip buttons and the
// distance used when a skip event arrives without one. The app cannot name a
// symbol: the system picks the glyph and derives the badge it draws from this
// number, so the two must not be changed independently.
const skipIntervalSeconds = 10.0

func handleSkipForward(_ objc.ID, _ objc.SEL, event objc.ID) int32  { return sendSkip(event, 1) }
func handleSkipBackward(_ objc.ID, _ objc.SEL, event objc.ID) int32 { return sendSkip(event, -1) }

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

// sendSkip forwards the interval the system put on the pressed skip button as a
// relative seek. Taking it from the event rather than hardcoding the distance is
// what stops the number drawn on the button from drifting away from the distance
// actually moved.
func sendSkip(event objc.ID, sign int64) int32 {
	_darwinCtrlMu.Lock()
	c := _darwinCtrl
	_darwinCtrlMu.Unlock()
	if c == nil {
		return cmdHandlerCommandFailed
	}
	select {
	case c.cmdChan <- skipCommand(objc.Send[float64](event, selInterval), sign):
	default:
	}
	return cmdHandlerSuccess
}

// skipCommand turns a skip event's interval into the relative seek it stands for.
// A zero or missing interval falls back to the advertised one; `!(secs > 0)`
// rather than `secs <= 0` so a NaN from the runtime takes that path too.
func skipCommand(secs float64, sign int64) Command {
	if !(secs > 0) {
		secs = skipIntervalSeconds
	}
	return Command{
		Type:  CmdSeek,
		Value: int64(secs*float64(time.Second/time.Microsecond)) * sign,
	}
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
	skip := nsDouble(skipIntervalSeconds)
	arr := objc.ID(classNSArray).Send(selArrayWithObject, skip)

	c.remoteCmd.Send(_cmdSels.skipBackward).Send(selSetPreferredIntervals, arr)
	c.remoteCmd.Send(_cmdSels.skipForward).Send(selSetPreferredIntervals, arr)

	for _, p := range remoteCommandHandlers() {
		c.remoteCmd.Send(p.cmd).Send(selAddTargetAction, c.handler, p.handler)
	}
}

// cmdHandler pairs a remote command with the handler method that services it.
type cmdHandler struct {
	cmd, handler objc.SEL
}

// remoteCommandHandlers is every MPRemoteCommandCenter command this app services.
//
// nextTrack/previousTrack stay here even though there is no track list to advance:
// they are what the F7/F9 media keys deliver, and both seek today. The seek
// buttons proper are the skip pair, drawn with the interval badge.
//
// A command left off this list keeps its default enabled state, so the system may
// still draw a control for it that cannot do anything.
func remoteCommandHandlers() []cmdHandler {
	return []cmdHandler{
		{_cmdSels.play, _handlerSels.play},
		{_cmdSels.pause, _handlerSels.pause},
		{_cmdSels.stop, _handlerSels.stop},
		{_cmdSels.toggle, _handlerSels.toggle},
		{_cmdSels.next, _handlerSels.next},
		{_cmdSels.prev, _handlerSels.prev},
		{_cmdSels.changePos, _handlerSels.changePos},
		{_cmdSels.skipBackward, _handlerSels.skipBackward},
		{_cmdSels.skipForward, _handlerSels.skipForward},
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
