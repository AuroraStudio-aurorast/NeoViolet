//! Optional presentation check on a connected Sidecar display. Ordinary CI stays hidden.
use objc2::rc::Retained;
use objc2::{MainThreadOnly, define_class, msg_send};
use objc2_app_kit::{
    NSApplication, NSApplicationActivationPolicy, NSBitmapImageFileType, NSCustomTouchBarItem,
    NSEventMask, NSGroupTouchBarItem, NSScreen, NSTouchBar, NSView, NSWindow,
};
use objc2_foundation::{
    MainThreadMarker, NSDate, NSDefaultRunLoopMode, NSDictionary, NSObjectProtocol, NSPoint,
    NSString,
};

define_class!(
    #[unsafe(super = NSView)]
    #[thread_kind = MainThreadOnly]
    struct TerminalView;
    unsafe impl NSObjectProtocol for TerminalView {}
    impl TerminalView {
        #[unsafe(method(acceptsFirstResponder))]
        fn accepts_first_responder(&self) -> bool { true }
        // Match the winit content view: nil lets AppKit continue to the owning window.
        #[unsafe(method_id(touchBar))]
        fn touch_bar(&self) -> Option<Retained<NSTouchBar>> { None }
    }
);

pub fn check(window: &NSWindow, bar: &NSTouchBar, marker: MainThreadMarker) {
    let Ok(name) = std::env::var("NEOVIOLET_TEST_SIDECAR_SCREEN") else {
        return;
    };
    let screen = NSScreen::screens(marker)
        .iter()
        .find(|screen| screen.localizedName().to_string() == name)
        .expect("the requested Sidecar screen must be connected");
    let app = NSApplication::sharedApplication(marker);
    app.setActivationPolicy(NSApplicationActivationPolicy::Regular);
    app.finishLaunching();
    // SAFETY: NSView initialization on AppKit's main thread; the window retains its view.
    let view: Retained<TerminalView> = unsafe {
        msg_send![TerminalView::alloc(marker), initWithFrame: window.contentView().unwrap().frame()]
    };
    window.setContentView(Some(&view));
    window.setTitle(&NSString::from_str("NeoViolet Touch Bar verification"));
    let frame = screen.visibleFrame();
    window.setFrameOrigin(NSPoint::new(frame.origin.x + 30.0, frame.origin.y + 30.0));
    window.makeKeyAndOrderFront(None);
    assert!(window.makeFirstResponder(Some(&view)));
    assert!(view.touchBar().is_none());
    #[allow(deprecated)]
    app.activateIgnoringOtherApps(true);
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(5);
    while !bar.isVisible() && std::time::Instant::now() < deadline {
        // SAFETY: AppKit's exported run-loop mode is valid for the duration of the process.
        let mode = unsafe { NSDefaultRunLoopMode };
        let date = NSDate::dateWithTimeIntervalSinceNow(0.05);
        if let Some(event) = app.nextEventMatchingMask_untilDate_inMode_dequeue(
            NSEventMask::Any,
            Some(&date),
            mode,
            true,
        ) {
            app.sendEvent(&event);
        }
        app.updateWindows();
    }
    assert!(
        bar.isVisible(),
        "the system must present the window Touch Bar on Sidecar"
    );
    let group = bar
        .itemForIdentifier(&NSString::from_str("neoviolet.touchbar.transport"))
        .unwrap();
    let transport = group
        .downcast_ref::<NSGroupTouchBarItem>()
        .unwrap()
        .groupTouchBar();
    let buttons: Vec<_> = ["backward", "playback", "forward"]
        .iter()
        .map(|key| {
            let item = transport
                .itemForIdentifier(&NSString::from_str(&format!("neoviolet.touchbar.{key}")))
                .unwrap();
            item.downcast_ref::<NSCustomTouchBarItem>().unwrap().view()
        })
        .collect();
    // The group item itself has no view. Its child buttons are hosted in a shared
    // native container after AppKit finishes presenting the remote Touch Bar.
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(2);
    while buttons.iter().any(|button| {
        button.window().is_none()
            || button.bounds().size.width <= 0.0
            || button.bounds().size.height <= 0.0
    }) && std::time::Instant::now() < deadline
    {
        let date = NSDate::dateWithTimeIntervalSinceNow(0.05);
        // SAFETY: The exported mode is valid for the process lifetime.
        if let Some(event) = app.nextEventMatchingMask_untilDate_inMode_dequeue(
            NSEventMask::Any,
            Some(&date),
            unsafe { NSDefaultRunLoopMode },
            true,
        ) {
            app.sendEvent(&event);
        }
        app.updateWindows();
    }
    // SAFETY: These are live main-thread AppKit views owned by the group items.
    let mut group_view =
        unsafe { buttons[0].superview() }.expect("visible controls must have a native container");
    while !buttons
        .iter()
        .all(|button| button.isDescendantOf(&group_view))
    {
        // SAFETY: Walk the live main-thread view hierarchy to the common group container.
        group_view =
            unsafe { group_view.superview() }.expect("transport buttons must share a container");
    }
    group_view.layoutSubtreeIfNeeded();
    let rects: Vec<_> = buttons
        .iter()
        .map(|button| button.convertRect_toView(button.bounds(), Some(&group_view)))
        .collect();
    for rect in &rects {
        assert!(rect.size.width > 0.0 && rect.size.height > 0.0);
    }
    for pair in rects.windows(2) {
        let gap = pair[1].origin.x - (pair[0].origin.x + pair[0].size.width);
        assert!(
            (0.0..=24.0).contains(&gap),
            "transport controls must be adjacent: {rects:?}"
        );
        assert!((pair[0].origin.y - pair[1].origin.y).abs() < 1.0);
    }
    let lyrics = bar
        .itemForIdentifier(&NSString::from_str("neoviolet.touchbar.lyrics"))
        .unwrap();
    let lyrics_view = lyrics
        .downcast_ref::<NSCustomTouchBarItem>()
        .unwrap()
        .view();
    let lyrics_frame = lyrics_view.convertRect_toView(
        lyrics_view.alignmentRectForFrame(lyrics_view.bounds()),
        Some(&group_view),
    );
    let right_gap =
        group_view.bounds().size.width - lyrics_frame.origin.x - lyrics_frame.size.width;
    let visible = lyrics_view.visibleRect();
    let bounds = lyrics_view.bounds();
    assert!(
        (visible.origin.x - bounds.origin.x).abs() < 0.1
            && (visible.origin.y - bounds.origin.y).abs() < 0.1
            && (visible.size.width - bounds.size.width).abs() < 0.1
            && (visible.size.height - bounds.size.height).abs() < 0.1,
        "the entire lyrics button must be visible: bounds={bounds:?}, visible={visible:?}"
    );
    assert!(
        (0.0..=8.0).contains(&right_gap),
        "lyrics must remain right-aligned inside the bar, gap={right_gap}"
    );
    let playback_center = rects[1].origin.x + rects[1].size.width / 2.0;
    assert!(
        (playback_center - group_view.bounds().size.width / 2.0).abs() < 1.0,
        "transport must stay centered"
    );
    if let Ok(path) = std::env::var("NEOVIOLET_TEST_TOUCHBAR_PREVIEW") {
        let bounds = group_view.bounds();
        let image = group_view
            .bitmapImageRepForCachingDisplayInRect(bounds)
            .unwrap();
        group_view.cacheDisplayInRect_toBitmapImageRep(bounds, &image);
        // SAFETY: PNG requires no representation-specific properties; this image owns its pixels.
        let png = unsafe {
            image.representationUsingType_properties(
                NSBitmapImageFileType::PNG,
                &NSDictionary::new(),
            )
        }
        .unwrap();
        assert!(png.writeToFile_atomically(&NSString::from_str(&path), true));
    }
    println!(
        "Sidecar presentation: centered transport, right-aligned lyrics (gap={right_gap}): {rects:?}"
    );
    super::focus::check(window, bar, marker);
    window.orderOut(None);
}
