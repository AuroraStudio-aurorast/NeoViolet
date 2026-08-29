//go:build darwin

// Package mediactl — macOS NowPlaying implementation.
//
// This file bridges Apple's MediaPlayer framework via purego/objc to expose
// macOS Control Center / lock screen / media key integration.
//
// The ObjC bridge pattern (dynamic class registration, MPRemoteCommand handler
// wiring, MPNowPlayingInfoCenter dictionary building) is derived from
// go-musicfox; see /docs/ACKNOWLEDGMENTS.md#github-com-go-musicfox-go-musicfox.

package mediactl

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// autorelease pool — ObjC objects created with "autorelease" need a pool
// on their thread. AppKit creates pools for the main event loop but our
// Bubble Tea goroutine has none. We create explicit pools via libobjc

var (
	_objcLib                 uintptr
	_objcAutoreleasePoolPush func() unsafe.Pointer
	_objcAutoreleasePoolPop  func(ptr unsafe.Pointer)
)

func autoPool(body func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := _objcAutoreleasePoolPush()
	defer _objcAutoreleasePoolPop(pool)
	body()
}

// ObjC constants (NSInteger → int32 for purego ABI compat on arm64)

const (
	cmdHandlerSuccess       int32 = 0
	cmdHandlerCommandFailed int32 = 200
)

const (
	playbackStateUnknown     int32 = iota // 0
	playbackStatePlaying                  // 1
	playbackStatePaused                   // 2
	playbackStateStopped                  // 3
	playbackStateInterrupted              // 4
)

// Global reference for ObjC handler callbacks

var (
	_darwinCtrlMu sync.Mutex
	_darwinCtrl   *darwinCtrl
)

// ObjC selector cache — all registered once at init()

var (
	sel_alloc              objc.SEL
	sel_init               objc.SEL
	sel_release            objc.SEL
	sel_autorelease        objc.SEL
	sel_new                objc.SEL
	sel_initWithUTF8String objc.SEL
	sel_numberWithInt      objc.SEL
	sel_numberWithDouble   objc.SEL
	sel_setValueForKey     objc.SEL
	sel_arrayWithObject    objc.SEL

	sel_sharedApplication         objc.SEL
	sel_setActivationPolicy       objc.SEL
	sel_activateIgnoringOtherApps objc.SEL
	sel_setDelegate               objc.SEL
	sel_run                       objc.SEL
	sel_terminate                 objc.SEL

	sel_sharedWorkspace         objc.SEL
	sel_notificationCenter      objc.SEL
	sel_addObserverSelectorName objc.SEL

	sel_defaultCenter         objc.SEL
	sel_setNowPlayingInfo     objc.SEL
	sel_setPlaybackState      objc.SEL
	sel_sharedCommandCenter   objc.SEL
	sel_addTargetAction       objc.SEL
	sel_setPreferredIntervals objc.SEL

	sel_dataWithBytes objc.SEL
	sel_initWithData  objc.SEL
	sel_initWithImage objc.SEL

	// MPRemoteCommand accessor selectors
	_cmdSels = struct {
		skipBackward, skipForward objc.SEL
		play, pause, stop, toggle objc.SEL
		next, prev, changePos     objc.SEL
	}{}

	// Handler selectors
	_handlerSels = struct {
		play, pause, stop, toggle objc.SEL
		next, prev, changePos     objc.SEL
		sleep, wake               objc.SEL
	}{}

	// App delegate selectors
	sel_finishLaunching       objc.SEL
	sel_appDidFinishLaunching objc.SEL
	sel_appShouldTerminate    objc.SEL
)

// ObjC class handles

var (
	class_NSString               objc.Class
	class_NSNumber               objc.Class
	class_NSMutableDictionary    objc.Class
	class_NSArray                objc.Class
	class_NSData                 objc.Class
	class_NSImage                objc.Class
	class_NSApplication          objc.Class
	class_NSWorkspace            objc.Class
	class_MPNowPlayingInfoCenter objc.Class
	class_MPRemoteCommandCenter  objc.Class
	class_MPRemoteCommandHandler objc.Class
	class_MPMediaItemArtwork     objc.Class
	class_AppDelegate            objc.Class
)

func init() {
	// Load frameworks.
	for _, path := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
	} {
		if _, err := purego.Dlopen(path, purego.RTLD_GLOBAL); err != nil {
			panic(fmt.Sprintf("mediactl: dlopen %s: %v", path, err))
		}
	}

	// Load libobjc and register autorelease pool functions.
	_objcLib, _ = purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL)
	purego.RegisterLibFunc(&_objcAutoreleasePoolPush, _objcLib, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&_objcAutoreleasePoolPop, _objcLib, "objc_autoreleasePoolPop")

	// selectors
	sel_alloc = objc.RegisterName("alloc")
	sel_init = objc.RegisterName("init")
	sel_release = objc.RegisterName("release")
	sel_autorelease = objc.RegisterName("autorelease")
	sel_new = objc.RegisterName("new")
	sel_initWithUTF8String = objc.RegisterName("initWithUTF8String:")
	sel_numberWithInt = objc.RegisterName("numberWithInt:")
	sel_numberWithDouble = objc.RegisterName("numberWithDouble:")
	sel_setValueForKey = objc.RegisterName("setValue:forKey:")
	sel_arrayWithObject = objc.RegisterName("arrayWithObject:")

	sel_sharedApplication = objc.RegisterName("sharedApplication")
	sel_setActivationPolicy = objc.RegisterName("setActivationPolicy:")
	sel_activateIgnoringOtherApps = objc.RegisterName("activateIgnoringOtherApps:")
	sel_setDelegate = objc.RegisterName("setDelegate:")
	sel_run = objc.RegisterName("run")
	sel_terminate = objc.RegisterName("terminate:")

	sel_sharedWorkspace = objc.RegisterName("sharedWorkspace")
	sel_notificationCenter = objc.RegisterName("notificationCenter")
	sel_addObserverSelectorName = objc.RegisterName("addObserver:selector:name:object:")

	sel_defaultCenter = objc.RegisterName("defaultCenter")
	sel_setNowPlayingInfo = objc.RegisterName("setNowPlayingInfo:")
	sel_setPlaybackState = objc.RegisterName("setPlaybackState:")
	sel_sharedCommandCenter = objc.RegisterName("sharedCommandCenter")
	sel_addTargetAction = objc.RegisterName("addTarget:action:")
	sel_setPreferredIntervals = objc.RegisterName("setPreferredIntervals:")

	sel_dataWithBytes = objc.RegisterName("dataWithBytes:length:")
	sel_initWithData = objc.RegisterName("initWithData:")
	sel_initWithImage = objc.RegisterName("initWithImage:")

	_cmdSels.skipBackward = objc.RegisterName("skipBackwardCommand")
	_cmdSels.skipForward = objc.RegisterName("skipForwardCommand")
	_cmdSels.play = objc.RegisterName("playCommand")
	_cmdSels.pause = objc.RegisterName("pauseCommand")
	_cmdSels.stop = objc.RegisterName("stopCommand")
	_cmdSels.toggle = objc.RegisterName("togglePlayPauseCommand")
	_cmdSels.next = objc.RegisterName("nextTrackCommand")
	_cmdSels.prev = objc.RegisterName("previousTrackCommand")
	_cmdSels.changePos = objc.RegisterName("changePlaybackPositionCommand")

	_handlerSels.play = objc.RegisterName("handlePlayCommand:")
	_handlerSels.pause = objc.RegisterName("handlePauseCommand:")
	_handlerSels.stop = objc.RegisterName("handleStopCommand:")
	_handlerSels.toggle = objc.RegisterName("handleTogglePlayPauseCommand:")
	_handlerSels.next = objc.RegisterName("handleNextTrackCommand:")
	_handlerSels.prev = objc.RegisterName("handlePreviousTrackCommand:")
	_handlerSels.changePos = objc.RegisterName("handleChangePlaybackPositionCommand:")
	_handlerSels.sleep = objc.RegisterName("handleWillSleepOrPowerOff:")
	_handlerSels.wake = objc.RegisterName("handleDidWake:")

	sel_finishLaunching = objc.RegisterName("finishLaunching")
	sel_appDidFinishLaunching = objc.RegisterName("applicationDidFinishLaunching:")
	sel_appShouldTerminate = objc.RegisterName("applicationShouldTerminateAfterLastWindowClosed:")

	// classes
	class_NSString = objc.GetClass("NSString")
	class_NSNumber = objc.GetClass("NSNumber")
	class_NSMutableDictionary = objc.GetClass("NSMutableDictionary")
	class_NSArray = objc.GetClass("NSArray")
	class_NSData = objc.GetClass("NSData")
	class_NSImage = objc.GetClass("NSImage")
	class_NSApplication = objc.GetClass("NSApplication")
	class_NSWorkspace = objc.GetClass("NSWorkspace")
	class_MPNowPlayingInfoCenter = objc.GetClass("MPNowPlayingInfoCenter")
	class_MPRemoteCommandCenter = objc.GetClass("MPRemoteCommandCenter")
	class_MPMediaItemArtwork = objc.GetClass("MPMediaItemArtwork")

	// custom ObjC classes
	var err error

	class_MPRemoteCommandHandler, err = objc.RegisterClass(
		"NeoVioletCommandHandler", objc.GetClass("NSObject"),
		nil, nil,
		[]objc.MethodDef{
			{Cmd: _handlerSels.play, Fn: handlePlay},
			{Cmd: _handlerSels.pause, Fn: handlePause},
			{Cmd: _handlerSels.stop, Fn: handleStop},
			{Cmd: _handlerSels.toggle, Fn: handleToggle},
			{Cmd: _handlerSels.next, Fn: handleNext},
			{Cmd: _handlerSels.prev, Fn: handlePrev},
			{Cmd: _handlerSels.changePos, Fn: handleChangePos},
			{Cmd: _handlerSels.sleep, Fn: handleSleep},
			{Cmd: _handlerSels.wake, Fn: handleWake},
		},
	)
	if err != nil {
		panic(fmt.Sprintf("mediactl: register handler class: %v", err))
	}

	class_AppDelegate, err = objc.RegisterClass(
		"NeoVioletAppDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("NSApplicationDelegate")},
		nil,
		[]objc.MethodDef{
			{Cmd: sel_appDidFinishLaunching, Fn: appDidFinishLaunching},
			{Cmd: sel_appShouldTerminate, Fn: appShouldTerminate},
		},
	)
	if err != nil {
		panic(fmt.Sprintf("mediactl: register delegate class: %v", err))
	}
}

// NSApplication delegate — bootstraps the app inside [NSApp run]

var (
	_bootstrapMu   sync.Mutex
	_bootstrapFn   func()
	_bootstrapOnce sync.Once
)

func appDidFinishLaunching(id objc.ID, cmd objc.SEL, notification objc.ID) {
	_bootstrapOnce.Do(func() {
		_bootstrapMu.Lock()
		fn := _bootstrapFn
		_bootstrapMu.Unlock()
		if fn == nil {
			return
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("mediactl: panic in bootstrap: %v\n", r)
				}
				nsApp := objc.ID(class_NSApplication).Send(sel_sharedApplication)
				nsApp.Send(sel_terminate, objc.ID(0))
			}()
			// IMPORTANT: Do NOT wrap fn() in autoPool.  autoPool calls
			// runtime.LockOSThread(), which would pin the entire Bubble Tea
			// app (audio, seeking, UI, ticks) to a single OS thread and
			// severely degrade seek/playback performance.
			// Individual ObjC operations (Start, Update) use their own
			// short-lived autoPool wrappers — that's sufficient.
			fn()
		}()
	})
}

func appShouldTerminate(id objc.ID, cmd objc.SEL, notification objc.ID) bool { return true }

// MacOSRun initialises NSApplication, registers a delegate, and blocks on
// [NSApp run] until the callback fn returns (which triggers terminate:).
// Must be called from the main thread.

func MacOSRun(fn func()) {
	nsApp := objc.ID(class_NSApplication).Send(sel_sharedApplication)
	nsApp.Send(sel_setActivationPolicy, 2) // Prohibited
	nsApp.Send(sel_activateIgnoringOtherApps, true)

	delegate := objc.ID(class_AppDelegate).Send(sel_alloc).Send(sel_init)
	defer delegate.Send(sel_release)
	nsApp.Send(sel_setDelegate, delegate)

	_bootstrapMu.Lock()
	_bootstrapFn = fn
	_bootstrapMu.Unlock()

	// Explicitly finish launching so applicationDidFinishLaunching:
	// fires synchronously — essential for CLI (non-bundle) execution
	// where [NSApp run] alone may not reliably trigger the delegate
	// callback (e.g. after a Bubble Tea wizard has manipulated the
	// terminal).  sync.Once in appDidFinishLaunching prevents double
	// fire when [NSApp run] calls finishLaunching again internally.
	nsApp.Send(sel_finishLaunching)
	nsApp.Send(sel_run)
}

// darwinCtrl

type darwinCtrl struct {
	mu         sync.Mutex
	cmdChan    chan Command
	handler    objc.ID
	nowPlaying objc.ID
	remoteCmd  objc.ID
	closed     bool

	lastCoverImg image.Image // cached for identity comparison
	coverArtwork objc.ID     // cached MPMediaItemArtwork
}

func newController() (Controller, error) { return &darwinCtrl{}, nil }

func (c *darwinCtrl) Start() (<-chan Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmdChan != nil {
		return c.cmdChan, nil
	}

	ch := make(chan Command, 8)
	c.cmdChan = ch

	// All ObjC object creation MUST be wrapped in an autorelease pool
	// because the Bubble Tea goroutine has none. Without this, NSNumber
	// and NSString objects created by registerCommands / registerNotifications
	// accumulate indefinitely.
	autoPool(func() {
		c.handler = objc.ID(class_MPRemoteCommandHandler).Send(sel_new)
		c.nowPlaying = objc.ID(class_MPNowPlayingInfoCenter).Send(sel_defaultCenter)
		c.remoteCmd = objc.ID(class_MPRemoteCommandCenter).Send(sel_sharedCommandCenter)

		c.registerCommands()
		c.nowPlaying.Send(sel_setPlaybackState, playbackStateStopped)
		c.registerNotifications()
	})

	_darwinCtrlMu.Lock()
	_darwinCtrl = c
	_darwinCtrlMu.Unlock()

	return ch, nil
}

func (c *darwinCtrl) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_darwinCtrlMu.Lock()
	_darwinCtrl = nil
	_darwinCtrlMu.Unlock()

	if c.cmdChan != nil {
		close(c.cmdChan)
	}
	if c.handler != 0 {
		c.handler.Send(sel_release)
	}
	if c.coverArtwork != 0 {
		c.coverArtwork.Send(sel_release)
		c.coverArtwork = 0
		c.lastCoverImg = nil
	}
	return nil
}

var _ Controller = (*darwinCtrl)(nil)
