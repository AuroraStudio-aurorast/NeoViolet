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
	selAlloc              objc.SEL
	selInit               objc.SEL
	selRelease            objc.SEL
	selAutorelease        objc.SEL
	selNew                objc.SEL
	selInitWithUTF8String objc.SEL
	selNumberWithInt      objc.SEL
	selNumberWithDouble   objc.SEL
	selSetValueForKey     objc.SEL
	selArrayWithObject    objc.SEL

	selSharedApplication         objc.SEL
	selSetActivationPolicy       objc.SEL
	selActivateIgnoringOtherApps objc.SEL
	selSetDelegate               objc.SEL
	selRun                       objc.SEL
	selTerminate                 objc.SEL

	selSharedWorkspace         objc.SEL
	selNotificationCenter      objc.SEL
	selAddObserverSelectorName objc.SEL

	selDefaultCenter         objc.SEL
	selSetNowPlayingInfo     objc.SEL
	selSetPlaybackState      objc.SEL
	selSharedCommandCenter   objc.SEL
	selAddTargetAction       objc.SEL
	selSetPreferredIntervals objc.SEL

	selDataWithBytes objc.SEL
	selInitWithData  objc.SEL
	selInitWithImage objc.SEL

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
	selFinishLaunching       objc.SEL
	selAppDidFinishLaunching objc.SEL
	selAppShouldTerminate    objc.SEL
)

// ObjC class handles

var (
	classNSString               objc.Class
	classNSNumber               objc.Class
	classNSMutableDictionary    objc.Class
	classNSArray                objc.Class
	classNSData                 objc.Class
	classNSImage                objc.Class
	classNSApplication          objc.Class
	classNSWorkspace            objc.Class
	classMPNowPlayingInfoCenter objc.Class
	classMPRemoteCommandCenter  objc.Class
	classMPRemoteCommandHandler objc.Class
	classMPMediaItemArtwork     objc.Class
	classAppDelegate            objc.Class
)

// setup prepares the ObjC runtime this file is built on: framework loading,
// selector registration and the custom class definitions. Every public entry
// point calls it first.
//
// It is deliberately not an init(): loading a framework is an environment
// condition that can fail, and OS media integration is optional, so the failure
// is reported to the caller instead of aborting the process. When it fails the
// caller keeps running without media controls — see
// cmd/neoviolet/cmd/run_darwin.go.
var (
	setupOnce sync.Once
	setupErr  error
)

func setup() error {
	setupOnce.Do(func() {
		setupErr = registerObjCRuntime()
	})
	return setupErr
}

func registerObjCRuntime() error {
	// Load frameworks.
	for _, path := range []string{
		"/System/Library/Frameworks/Foundation.framework/Foundation",
		"/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer",
		"/System/Library/Frameworks/AppKit.framework/AppKit",
	} {
		if _, err := purego.Dlopen(path, purego.RTLD_GLOBAL); err != nil {
			return fmt.Errorf("mediactl: dlopen %s: %w", path, err)
		}
	}

	// Load libobjc and register autorelease pool functions. RegisterLibFunc
	// dereferences the handle it is given, so a failed dlopen has to stop here
	// rather than leave the pool functions pointing at a null library.
	var err error
	if _objcLib, err = purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL); err != nil {
		return fmt.Errorf("mediactl: dlopen libobjc: %w", err)
	}
	purego.RegisterLibFunc(&_objcAutoreleasePoolPush, _objcLib, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&_objcAutoreleasePoolPop, _objcLib, "objc_autoreleasePoolPop")

	// selectors
	selAlloc = objc.RegisterName("alloc")
	selInit = objc.RegisterName("init")
	selRelease = objc.RegisterName("release")
	selAutorelease = objc.RegisterName("autorelease")
	selNew = objc.RegisterName("new")
	selInitWithUTF8String = objc.RegisterName("initWithUTF8String:")
	selNumberWithInt = objc.RegisterName("numberWithInt:")
	selNumberWithDouble = objc.RegisterName("numberWithDouble:")
	selSetValueForKey = objc.RegisterName("setValue:forKey:")
	selArrayWithObject = objc.RegisterName("arrayWithObject:")

	selSharedApplication = objc.RegisterName("sharedApplication")
	selSetActivationPolicy = objc.RegisterName("setActivationPolicy:")
	selActivateIgnoringOtherApps = objc.RegisterName("activateIgnoringOtherApps:")
	selSetDelegate = objc.RegisterName("setDelegate:")
	selRun = objc.RegisterName("run")
	selTerminate = objc.RegisterName("terminate:")

	selSharedWorkspace = objc.RegisterName("sharedWorkspace")
	selNotificationCenter = objc.RegisterName("notificationCenter")
	selAddObserverSelectorName = objc.RegisterName("addObserver:selector:name:object:")

	selDefaultCenter = objc.RegisterName("defaultCenter")
	selSetNowPlayingInfo = objc.RegisterName("setNowPlayingInfo:")
	selSetPlaybackState = objc.RegisterName("setPlaybackState:")
	selSharedCommandCenter = objc.RegisterName("sharedCommandCenter")
	selAddTargetAction = objc.RegisterName("addTarget:action:")
	selSetPreferredIntervals = objc.RegisterName("setPreferredIntervals:")

	selDataWithBytes = objc.RegisterName("dataWithBytes:length:")
	selInitWithData = objc.RegisterName("initWithData:")
	selInitWithImage = objc.RegisterName("initWithImage:")

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

	selFinishLaunching = objc.RegisterName("finishLaunching")
	selAppDidFinishLaunching = objc.RegisterName("applicationDidFinishLaunching:")
	selAppShouldTerminate = objc.RegisterName("applicationShouldTerminateAfterLastWindowClosed:")

	// classes
	classNSString = objc.GetClass("NSString")
	classNSNumber = objc.GetClass("NSNumber")
	classNSMutableDictionary = objc.GetClass("NSMutableDictionary")
	classNSArray = objc.GetClass("NSArray")
	classNSData = objc.GetClass("NSData")
	classNSImage = objc.GetClass("NSImage")
	classNSApplication = objc.GetClass("NSApplication")
	classNSWorkspace = objc.GetClass("NSWorkspace")
	classMPNowPlayingInfoCenter = objc.GetClass("MPNowPlayingInfoCenter")
	classMPRemoteCommandCenter = objc.GetClass("MPRemoteCommandCenter")
	classMPMediaItemArtwork = objc.GetClass("MPMediaItemArtwork")

	// custom ObjC classes

	classMPRemoteCommandHandler, err = objc.RegisterClass(
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
		return fmt.Errorf("mediactl: register handler class: %w", err)
	}

	classAppDelegate, err = objc.RegisterClass(
		"NeoVioletAppDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("NSApplicationDelegate")},
		nil,
		[]objc.MethodDef{
			{Cmd: selAppDidFinishLaunching, Fn: appDidFinishLaunching},
			{Cmd: selAppShouldTerminate, Fn: appShouldTerminate},
		},
	)
	if err != nil {
		return fmt.Errorf("mediactl: register delegate class: %w", err)
	}

	return nil
}

// NSApplication delegate — bootstraps the app inside [NSApp run]

var (
	_bootstrapMu   sync.Mutex
	_bootstrapFn   func()
	_bootstrapOnce sync.Once
)

func appDidFinishLaunching(_ objc.ID, _ objc.SEL, _ objc.ID) {
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
				nsApp := objc.ID(classNSApplication).Send(selSharedApplication)
				nsApp.Send(selTerminate, objc.ID(0))
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

func appShouldTerminate(_ objc.ID, _ objc.SEL, _ objc.ID) bool { return true }

// MacOSRun initialises NSApplication, registers a delegate, and blocks on
// [NSApp run] until the callback fn returns (which triggers terminate:).
// Must be called from the main thread.
//
// When the ObjC runtime cannot be prepared it returns that error without calling
// fn, so the caller can carry on without OS media controls instead of aborting.
func MacOSRun(fn func()) error {
	if err := setup(); err != nil {
		return err
	}

	nsApp := objc.ID(classNSApplication).Send(selSharedApplication)
	nsApp.Send(selSetActivationPolicy, 2) // Prohibited
	nsApp.Send(selActivateIgnoringOtherApps, true)

	delegate := objc.ID(classAppDelegate).Send(selAlloc).Send(selInit)
	defer delegate.Send(selRelease)
	nsApp.Send(selSetDelegate, delegate)

	_bootstrapMu.Lock()
	_bootstrapFn = fn
	_bootstrapMu.Unlock()

	// Explicitly finish launching so applicationDidFinishLaunching:
	// fires synchronously — essential for CLI (non-bundle) execution
	// where [NSApp run] alone may not reliably trigger the delegate
	// callback (e.g. after a Bubble Tea wizard has manipulated the
	// terminal).  sync.Once in appDidFinishLaunching prevents double
	// fire when [NSApp run] calls finishLaunching again internally.
	nsApp.Send(selFinishLaunching)
	nsApp.Send(selRun)
	return nil
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

func newController() (Controller, error) {
	if err := setup(); err != nil {
		return nil, err
	}
	return &darwinCtrl{}, nil
}

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
		c.handler = objc.ID(classMPRemoteCommandHandler).Send(selNew)
		c.nowPlaying = objc.ID(classMPNowPlayingInfoCenter).Send(selDefaultCenter)
		c.remoteCmd = objc.ID(classMPRemoteCommandCenter).Send(selSharedCommandCenter)

		c.registerCommands()
		c.nowPlaying.Send(selSetPlaybackState, playbackStateStopped)
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
		c.handler.Send(selRelease)
	}
	if c.coverArtwork != 0 {
		c.coverArtwork.Send(selRelease)
		c.coverArtwork = 0
		c.lastCoverImg = nil
	}
	return nil
}

var _ Controller = (*darwinCtrl)(nil)
