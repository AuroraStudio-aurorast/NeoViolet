//! Observe actual key/main-window notifications while showing overlays on Sidecar.
#[path = "../../src/desktop_lyrics_macos.rs"]
mod lyrics;

use block2::RcBlock;
use objc2::rc::Retained;
use objc2::{MainThreadOnly, define_class, msg_send};
use objc2_app_kit::{
    NSApplication, NSApplicationActivationPolicy, NSBackingStoreType, NSTouchBar, NSWindow,
    NSWindowDidBecomeKeyNotification, NSWindowDidBecomeMainNotification, NSWindowStyleMask,
};
use objc2_foundation::{
    MainThreadMarker, NSDate, NSNotification, NSNotificationCenter, NSObjectProtocol, NSPoint,
    NSRect, NSRunLoop, NSSize,
};
use std::cell::Cell;
use std::ptr::NonNull;
use std::rc::Rc;

define_class!(
    #[unsafe(super = NSWindow)]
    #[thread_kind = MainThreadOnly]
    struct OverlayWindow;
    unsafe impl NSObjectProtocol for OverlayWindow {}
    impl OverlayWindow {
        // Match winit's borderless windows: decorations do not prevent activation.
        #[unsafe(method(canBecomeKeyWindow))]
        fn can_become_key(&self) -> bool { true }
        #[unsafe(method(canBecomeMainWindow))]
        fn can_become_main(&self) -> bool { true }
    }
);

pub fn run(main: &NSWindow, bar: &NSTouchBar, marker: MainThreadMarker) {
    let app = NSApplication::sharedApplication(marker);
    app.setActivationPolicy(NSApplicationActivationPolicy::Regular);
    app.finishLaunching();
    main.makeKeyAndOrderFront(None);
    #[allow(deprecated)]
    app.activateIgnoringOtherApps(true);
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(2);
    while app.keyWindow().as_deref() != Some(main) && std::time::Instant::now() < deadline {
        let date = NSDate::dateWithTimeIntervalSinceNow(0.01);
        // SAFETY: AppKit's default run-loop mode is process-lifetime data.
        if let Some(event) = app.nextEventMatchingMask_untilDate_inMode_dequeue(
            objc2_app_kit::NSEventMask::Any,
            Some(&date),
            unsafe { objc2_foundation::NSDefaultRunLoopMode },
            true,
        ) {
            app.sendEvent(&event);
        }
        app.updateWindows();
    }
    assert_eq!(
        app.keyWindow().as_deref(),
        Some(main),
        "the main-window focus fixture must be active before testing overlays"
    );
    check(main, bar, marker);
    main.orderOut(None);
}

pub fn check(main: &NSWindow, bar: &NSTouchBar, marker: MainThreadMarker) {
    let app = NSApplication::sharedApplication(marker);
    let settle =
        || NSRunLoop::mainRunLoop().runUntilDate(&NSDate::dateWithTimeIntervalSinceNow(0.01));
    let create = |x| {
        // SAFETY: The test owns each window through close on AppKit's main thread.
        unsafe {
            let rect = NSRect::new(NSPoint::new(x, 300.0), NSSize::new(300.0, 50.0));
            let window: Retained<OverlayWindow> = msg_send![OverlayWindow::alloc(marker),
                initWithContentRect: rect, styleMask: NSWindowStyleMask::Borderless,
                backing: NSBackingStoreType::Buffered, defer: false];
            window.setReleasedWhenClosed(false);
            window
        }
    };
    let initially_visible = bar.isVisible();
    let center = NSNotificationCenter::defaultCenter();
    let focus_changes = Rc::new(Cell::new(0));
    for _ in 0..5 {
        let overlay = create(200.0);
        let toolbar = create(230.0);
        let mut observers = Vec::new();
        for window in [&overlay, &toolbar] {
            for name in unsafe {
                [
                    NSWindowDidBecomeKeyNotification,
                    NSWindowDidBecomeMainNotification,
                ]
            } {
                let changes = focus_changes.clone();
                let block =
                    RcBlock::new(move |_: NonNull<NSNotification>| changes.set(changes.get() + 1));
                // SAFETY: These main-thread observers are removed before closing the windows.
                observers.push(unsafe {
                    center.addObserverForName_object_queue_usingBlock(
                        Some(name),
                        Some(window),
                        None,
                        &block,
                    )
                });
            }
        }
        assert!(!overlay.isVisible() && !toolbar.isVisible());
        lyrics::show(&overlay);
        settle();
        assert_eq!(app.keyWindow().as_deref(), Some(main));
        assert_eq!(app.mainWindow().as_deref(), Some(main));
        assert!(!initially_visible || bar.isVisible());
        lyrics::show(&toolbar);
        lyrics::attach(&overlay, &toolbar, 80.0);
        settle();
        assert!(overlay.isVisible() && toolbar.isVisible());
        assert!(!overlay.ignoresMouseEvents() && !toolbar.ignoresMouseEvents());
        assert_eq!(app.keyWindow().as_deref(), Some(main));
        assert_eq!(app.mainWindow().as_deref(), Some(main));
        let old = overlay.frame().origin;
        let origin = toolbar.frame().origin;
        toolbar.setFrameOrigin(NSPoint::new(origin.x + 20.0, origin.y + 10.0));
        assert_eq!(
            overlay.frame().origin,
            NSPoint::new(old.x + 20.0, old.y + 10.0)
        );
        lyrics::detach(&toolbar);
        overlay.close();
        toolbar.close();
        settle();
        assert_eq!(app.keyWindow().as_deref(), Some(main));
        assert!(!initially_visible || bar.isVisible());
        for token in observers {
            // SAFETY: Token came from this notification center in the current loop.
            unsafe {
                center.removeObserver(token.as_ref());
            }
        }
    }
    assert_eq!(
        focus_changes.get(),
        0,
        "no transient activation is permitted, even between frames"
    );
    // A native chooser or another window owning input must not have its focus taken back.
    let chooser: Retained<OverlayWindow> = create(300.0);
    chooser.makeKeyAndOrderFront(None);
    settle();
    assert_eq!(app.keyWindow().as_deref(), Some(&**chooser));
    let overlay = create(200.0);
    lyrics::show(&overlay);
    assert_eq!(app.keyWindow().as_deref(), Some(&**chooser));
    overlay.close();
    chooser.close();
    println!(
        "Native focus: five overlay/toolbar open-close cycles, zero key/main changes; movement and chooser ownership preserved."
    );
}
