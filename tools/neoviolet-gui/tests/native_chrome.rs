//! Run AppKit regression checks on the main thread without showing a window.
#[cfg(target_os = "macos")]
#[path = "../src/desktop_chrome_macos.rs"]
mod chrome;
#[cfg(target_os = "macos")]
#[path = "../src/desktop_lyrics_macos.rs"]
#[expect(
    dead_code,
    reason = "presentation is exercised by the native_touchbar focus harness"
)]
mod lyrics_pair;
#[cfg(target_os = "macos")]
#[path = "native_chrome/recovery.rs"]
mod recovery;
#[cfg(target_os = "macos")]
#[path = "../src/desktop_menu_macos.rs"]
mod startup_menu;

#[cfg(target_os = "macos")]
fn check_native_lyrics_pair(marker: objc2_foundation::MainThreadMarker) {
    use objc2::MainThreadOnly;
    use objc2_app_kit::{NSBackingStoreType, NSWindow, NSWindowStyleMask};
    use objc2_foundation::{NSPoint, NSRect, NSSize};
    let create = |rect| {
        // SAFETY: Both test windows stay hidden and are owned on this main thread.
        let window = unsafe {
            NSWindow::initWithContentRect_styleMask_backing_defer(
                NSWindow::alloc(marker),
                rect,
                NSWindowStyleMask::Borderless,
                NSBackingStoreType::Buffered,
                false,
            )
        };
        // SAFETY: The test retains ownership until after closing.
        unsafe {
            window.setReleasedWhenClosed(false);
        }
        window
    };
    let toolbar = create(NSRect::new(
        NSPoint::new(500.0, 600.0),
        NSSize::new(240.0, 88.0),
    ));
    let lyrics = create(NSRect::new(
        NSPoint::new(470.0, 558.0),
        NSSize::new(300.0, 50.0),
    ));
    let offset = NSPoint::new(-30.0, -42.0);
    lyrics_pair::attach(&lyrics, &toolbar, 80.0);
    for step in 1..=20 {
        let origin = NSPoint::new(500.0 + step as f64 * 7.0, 600.0 - step as f64 * 3.0);
        toolbar.setFrameOrigin(origin);
        assert_eq!(
            lyrics.frame().origin,
            NSPoint::new(origin.x + offset.x, origin.y + offset.y),
            "lyrics must follow immediately without an Iced event or run-loop drain"
        );
    }
    lyrics_pair::resize(&lyrics, NSSize::new(880.0, 120.0), 80.0);
    let parent = toolbar.frame();
    let frame = lyrics.frame();
    assert_eq!(
        frame.origin.x + frame.size.width / 2.0,
        parent.origin.x + parent.size.width / 2.0
    );
    assert_eq!(
        frame.origin.y + frame.size.height,
        parent.origin.y + parent.size.height - 80.0
    );
    toolbar.setFrameOrigin(NSPoint::new(-200.0, 100.0));
    assert_eq!(lyrics.frame().origin, NSPoint::new(-520.0, -12.0));
    lyrics_pair::detach(&toolbar);
    assert!(lyrics.parentWindow().is_none());
    let detached = lyrics.frame();
    toolbar.setFrameOrigin(NSPoint::new(300.0, 500.0));
    assert_eq!(lyrics.frame(), detached);
    lyrics_pair::attach(&lyrics, &toolbar, 80.0);
    lyrics_pair::attach(&lyrics, &toolbar, 80.0);
    assert_eq!(toolbar.childWindows().unwrap().count(), 1);
    lyrics_pair::detach(&lyrics);
    assert!(lyrics.parentWindow().is_none());
    toolbar.close();
    lyrics.close();
}

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
    check_native_lyrics_pair(marker);
    check_native_menu(marker);
    check_startup_player_menu();
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
    let native_x = buttons.each_ref().map(|button| button.frame().origin.x);
    chrome::install(id, &view, &window, 34.0);
    chrome::refresh(id, &view, &window, 34.0);
    settle();
    let expected = buttons.each_ref().map(|button| button.frame().origin);
    if objc2_foundation::NSProcessInfo::processInfo()
        .operatingSystemVersion()
        .majorVersion
        >= 26
    {
        for index in 1..3 {
            assert!(
                ((expected[index].x - expected[index - 1].x)
                    - (native_x[index] - native_x[index - 1]))
                    .abs()
                    < 0.1,
                "modern macOS must preserve native button spacing: native {native_x:?}, actual {expected:?}"
            );
        }
    }
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
    recovery::check(id, &window, &view, &buttons, expected);
    let original_frame = window.frame();
    for (step, (width, height, display)) in [
        (811.0, 607.0, false),
        (750.0, 550.0, false),
        (750.0, 600.0, true),
        (820.0, 600.0, true),
        (800.5, 600.5, false),
        (480.0, 320.0, true),
        (1000.0, 700.0, true),
        (800.0, 600.0, false),
    ]
    .into_iter()
    .enumerate()
    {
        let mut frame = original_frame;
        frame.size = NSSize::new(width, height);
        window.setFrame_display(frame, display);
        let actual = buttons.each_ref().map(|button| button.frame().origin);
        assert_eq!(
            actual, expected,
            "resize must restore buttons before the next display, without a run-loop correction (step {step})"
        );
        assert_eq!(
            buttons
                .each_ref()
                .map(|button| button.layer().unwrap().frame()),
            expected_layers,
            "native layers must be aligned in the same resize pass"
        );
        // Deliberately no run-loop drain between sizes: even a burst must never expose resets.
    }
    window.setFrame_display(original_frame, false);
    settle();
    // Reinstallation must remove the previous observer set, preserving original flags.
    chrome::install(id, &view, &window, 34.0);
    settle();
    assert_eq!(
        buttons.each_ref().map(|button| button.frame().origin),
        expected,
        "reinstallation must not recapture our adjusted layout as the native baseline"
    );
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

#[cfg(target_os = "macos")]
fn check_native_menu(marker: objc2_foundation::MainThreadMarker) {
    use objc2_app_kit::{NSMenu, NSMenuItem};
    let root = NSMenu::new(marker);
    let submenu = NSMenu::new(marker);
    let heading = NSMenuItem::new(marker);
    heading.setEnabled(true);
    heading.setSubmenu(Some(&submenu));
    root.addItem(&heading);
    let nested = NSMenu::new(marker);
    let nested_heading = NSMenuItem::new(marker);
    nested_heading.setSubmenu(Some(&nested));
    submenu.addItem(&nested_heading);
    let command = NSMenuItem::new(marker);
    command.setEnabled(true);
    nested.addItem(&command);
    startup_menu::set_commands(&root, false);
    assert!(
        heading.isEnabled(),
        "menu headings must remain visible/openable during startup"
    );
    assert!(!nested_heading.isEnabled());
    assert!(!command.isEnabled(), "nested commands must be disabled");
    assert!(!submenu.autoenablesItems() && !nested.autoenablesItems());
    startup_menu::set_commands(&root, true);
    assert!(command.isEnabled() && nested_heading.isEnabled());
    assert!(submenu.autoenablesItems() && nested.autoenablesItems());
}

#[cfg(target_os = "macos")]
fn check_startup_player_menu() {
    use muda::{CheckMenuItem, ContextMenu, Menu, MenuItem, Submenu};
    let menu = Menu::new();
    let nested = Submenu::new("Commands", true);
    let open = MenuItem::with_id("open", "Open File…", true, None);
    let about = MenuItem::with_id("about", "About", true, None);
    let zoom = MenuItem::with_id("zoom_in", "Zoom In", true, None);
    let copy = MenuItem::with_id("copy", "Copy", true, None);
    let quit = MenuItem::with_id("quit", "Quit", true, None);
    let settings = MenuItem::with_id("lyrics_settings", "Lyrics Settings", true, None);
    let unlock = MenuItem::with_id("unlock_lyrics", "Unlock Lyrics", true, None);
    let reset = MenuItem::with_id("reset_lyrics_position", "Reset Lyrics Position", true, None);
    let lyrics = CheckMenuItem::with_id("desktop_lyrics", "Desktop Lyrics", true, true, None);
    nested
        .append_items(&[
            &open, &about, &zoom, &copy, &quit, &settings, &unlock, &lyrics, &reset,
        ])
        .unwrap();
    menu.append(&nested).unwrap();
    // SAFETY: Menu owns this native object; the harness is on AppKit's main thread.
    let native = unsafe { &*menu.ns_menu().cast::<objc2_app_kit::NSMenu>() };
    startup_menu::set_commands(native, true);
    startup_menu::set_player_commands(&menu, false, true);
    let native_submenu = native.itemAtIndex(0).unwrap().submenu().unwrap();
    native_submenu.update();
    for item in native_submenu.itemArray() {
        let title = item.title().to_string();
        if [
            "Open File…",
            "Lyrics Settings",
            "Unlock Lyrics",
            "Desktop Lyrics",
            "Reset Lyrics Position",
        ]
        .contains(&title.as_str())
        {
            assert!(
                !item.isEnabled(),
                "AppKit validation re-enabled {title} during setup"
            );
        }
    }
    assert!(
        !open.is_enabled()
            && !settings.is_enabled()
            && !unlock.is_enabled()
            && !lyrics.is_enabled()
            && !reset.is_enabled()
    );
    assert!(about.is_enabled() && zoom.is_enabled() && copy.is_enabled() && quit.is_enabled());
    startup_menu::set_player_commands(&menu, true, false);
    assert!(
        open.is_enabled() && settings.is_enabled() && lyrics.is_enabled() && reset.is_enabled()
    );
    assert!(
        !unlock.is_enabled(),
        "lyrics lock controls need an active overlay"
    );
    startup_menu::set_commands(native, false);
    startup_menu::set_player_commands(&menu, false, false);
    let native_submenu = native.itemAtIndex(0).unwrap().submenu().unwrap();
    assert!(
        native_submenu
            .itemArray()
            .iter()
            .all(|item| !item.isEnabled()),
        "native commands must disable while a setup sheet owns input"
    );
}
