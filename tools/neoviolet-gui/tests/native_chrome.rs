//! Run AppKit regression checks on the main thread without showing a window.
#[cfg(target_os = "macos")]
#[path = "../src/desktop_chrome_macos.rs"]
mod chrome;

#[cfg(not(target_os = "macos"))]
fn main() {
    println!("Native caption layout regression applies to macOS only.");
}

#[cfg(target_os = "macos")]
fn main() {
    objc2::rc::autoreleasepool(|_| check_native_layout());
}

#[cfg(target_os = "macos")]
fn check_native_layout() {
    use objc2::MainThreadOnly;
    use objc2_app_kit::{
        NSApplication, NSBackingStoreType, NSWindow, NSWindowButton, NSWindowStyleMask,
    };
    use objc2_foundation::{MainThreadMarker, NSDate, NSPoint, NSRect, NSRunLoop, NSSize};
    let settle =
        || NSRunLoop::mainRunLoop().runUntilDate(&NSDate::dateWithTimeIntervalSinceNow(0.01));
    let marker = MainThreadMarker::new().expect("native regression must run on the main thread");
    let _application = NSApplication::sharedApplication(marker);
    // SAFETY: The window and all its native views are confined to this main thread.
    let window = unsafe {
        NSWindow::initWithContentRect_styleMask_backing_defer(
            NSWindow::alloc(marker),
            NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(800.0, 600.0)),
            NSWindowStyleMask::Titled
                | NSWindowStyleMask::Closable
                | NSWindowStyleMask::Miniaturizable
                | NSWindowStyleMask::Resizable
                | NSWindowStyleMask::FullSizeContentView,
            NSBackingStoreType::Buffered,
            false,
        )
    };
    // SAFETY: Retained owns the window; AppKit must not release that ownership on close.
    unsafe {
        window.setReleasedWhenClosed(false);
    }
    window.setTitlebarAppearsTransparent(true);
    let view = window.contentView().unwrap();
    let id = iced::window::Id::unique();
    let buttons = [
        NSWindowButton::CloseButton,
        NSWindowButton::MiniaturizeButton,
        NSWindowButton::ZoomButton,
    ]
    .map(|kind| window.standardWindowButton(kind).unwrap());
    for button in &buttons {
        button.setWantsLayer(true);
        button.setPostsFrameChangedNotifications(false);
        button.setPostsBoundsChangedNotifications(false);
    }
    chrome::install(id, &view, &window, 34.0);
    chrome::refresh(id, &view, &window, 34.0);
    settle();
    let expected = buttons.each_ref().map(|button| button.frame().origin);
    let expected_layers = buttons
        .each_ref()
        .map(|button| button.layer().unwrap().frame());
    for round in 0..3 {
        for (index, button) in buttons.iter().enumerate() {
            // AppKit lays out the group as a batch. Correction must run only after
            // the complete native pass, without an Iced focus/resize/input event.
            let native = NSPoint::new(2.0 + index as f64 * 20.0, 2.0);
            button.setFrameOrigin(native);
            assert_eq!(
                button.frame().origin,
                native,
                "do not modify AppKit's group during its layout pass"
            );
        }
        settle();
        for (index, button) in buttons.iter().enumerate() {
            let actual = button.frame().origin;
            assert_eq!(
                button.layer().unwrap().frame(),
                expected_layers[index],
                "native layer and view geometry must settle together"
            );
            assert!(
                (actual.x - expected[index].x).abs() < 0.1
                    && (actual.y - expected[index].y).abs() < 0.1,
                "late layout {round}, button {index} persisted at {actual:?}, expected {:?}",
                expected[index]
            );
        }
    }
    // A titlebar container can change coordinates independently of its button frames.
    // SAFETY: The native hierarchy is borrowed on its owning thread.
    let parent = unsafe { buttons[0].superview() }.unwrap();
    let bounds = parent.bounds();
    parent.setBoundsOrigin(NSPoint::new(bounds.origin.x + 3.0, bounds.origin.y));
    settle();
    let origin = view.convertPoint_fromView(buttons[0].frame().origin, Some(&parent));
    assert!(
        (origin.x - 14.0).abs() < 0.1,
        "container coordinate changes must preserve the leading inset"
    );
    parent.setBoundsOrigin(bounds.origin);
    settle();
    // Reinstallation must remove the previous observer set, preserving original flags.
    chrome::install(id, &view, &window, 34.0);
    window.close();
    settle();
    for (index, button) in buttons.iter().enumerate() {
        assert!(!button.postsFrameChangedNotifications());
        assert!(!button.postsBoundsChangedNotifications());
        let reset = NSPoint::new(2.0 + index as f64 * 20.0, 2.0);
        button.setFrameOrigin(reset);
        assert_eq!(
            button.frame().origin,
            reset,
            "closing must remove the geometry observers"
        );
    }
    println!(
        "Native chrome: late frame resets, container bounds, reinstallation and cleanup passed (hidden window)."
    );
}
