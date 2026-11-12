//! Real AppKit controls and target/action callbacks, exercised on the main thread.
#[cfg(target_os = "macos")]
#[path = "native_touchbar/focus.rs"]
mod focus;
#[cfg(target_os = "macos")]
#[path = "../src/desktop_touchbar_macos.rs"]
mod native;
#[cfg(target_os = "macos")]
#[path = "native_touchbar/sidecar.rs"]
mod sidecar;
#[cfg(target_os = "macos")]
#[path = "../src/touchbar.rs"]
mod touchbar;

#[cfg(target_os = "macos")]
fn main() {
    objc2::rc::autoreleasepool(|_| check());
}

#[cfg(not(target_os = "macos"))]
fn main() {}

#[cfg(target_os = "macos")]
fn check() {
    use objc2::MainThreadOnly;
    use objc2_app_kit::{
        NSApplication, NSBackingStoreType, NSButton, NSCustomTouchBarItem, NSGroupTouchBarItem,
        NSTouchBarItemIdentifierFlexibleSpace, NSWindow, NSWindowStyleMask,
    };
    use objc2_foundation::{MainThreadMarker, NSPoint, NSRect, NSSize, NSString};
    use touchbar::{Command, Context, State};

    let marker = MainThreadMarker::new().unwrap();
    let _app = NSApplication::sharedApplication(marker);
    // SAFETY: Both construction and close run on AppKit's main thread; this test retains the window.
    let window = unsafe {
        let window = NSWindow::initWithContentRect_styleMask_backing_defer(
            NSWindow::alloc(marker),
            NSRect::new(NSPoint::ZERO, NSSize::new(600.0, 400.0)),
            NSWindowStyleMask::Titled,
            NSBackingStoreType::Buffered,
            false,
        );
        window.setReleasedWhenClosed(false);
        window
    };
    let id = iced::window::Id::unique();
    let (sender, mut receiver) = tokio::sync::mpsc::unbounded_channel();
    native::install(id, &window, move |event| {
        let _ = sender.send(event);
    });
    let bar = window
        .touchBar()
        .expect("the main window must own the Touch Bar");
    let mut state = State {
        context: Context {
            window: id,
            session: 2,
            epoch: 1,
            track_id: Some(3),
        },
        available: true,
        can_play: true,
        can_seek: true,
        playing: true,
        lyrics: false,
    };
    native::sync(id, state);
    let defaults: Vec<_> = bar
        .defaultItemIdentifiers()
        .iter()
        .map(|id| id.to_string())
        .collect();
    assert_eq!(
        defaults,
        vec![
            "neoviolet.touchbar.open".to_string(),
            "neoviolet.touchbar.transport".to_string(),
            // SAFETY: This public AppKit identifier is valid on all supported versions.
            unsafe { NSTouchBarItemIdentifierFlexibleSpace }.to_string(),
            "neoviolet.touchbar.lyrics".to_string(),
        ],
        "only the complete transport group may be positioned in the top-level bar"
    );
    assert!(
        bar.itemForIdentifier(&NSString::from_str("neoviolet.touchbar.volume"))
            .is_none()
    );
    let group_item = bar
        .itemForIdentifier(&NSString::from_str("neoviolet.touchbar.transport"))
        .unwrap();
    let group = group_item.downcast_ref::<NSGroupTouchBarItem>().unwrap();
    let transport = group.groupTouchBar();
    let ids: Vec<_> = transport
        .defaultItemIdentifiers()
        .iter()
        .map(|id| id.to_string())
        .collect();
    assert_eq!(
        ids,
        [
            "neoviolet.touchbar.backward",
            "neoviolet.touchbar.playback",
            "neoviolet.touchbar.forward"
        ]
    );
    assert_eq!(
        bar.principalItemIdentifier().unwrap().to_string(),
        "neoviolet.touchbar.transport"
    );
    let item = transport
        .itemForIdentifier(&NSString::from_str("neoviolet.touchbar.playback"))
        .unwrap();
    let custom = item.downcast_ref::<NSCustomTouchBarItem>().unwrap();
    let view = custom.view();
    let play = view.downcast_ref::<NSButton>().unwrap();
    assert!(play.isEnabled());
    assert_eq!(play.toolTip().unwrap().to_string(), "Pause");
    // SAFETY: The registered action and target are supplied by our native controller.
    unsafe {
        play.performClick(None);
    }
    let event = receiver.try_recv().unwrap();
    assert_eq!(event.context, state.context);
    assert!(matches!(event.command, Command::PlayPause));
    state.playing = false;
    native::sync(id, state);
    assert_eq!(play.toolTip().unwrap().to_string(), "Play");
    assert_eq!(
        window.touchBar().unwrap(),
        bar,
        "snapshots must not rebuild the bar"
    );

    for (identifier, expected) in [
        ("open", Command::Open),
        ("backward", Command::SeekBy(-15.0)),
        ("forward", Command::SeekBy(15.0)),
        ("lyrics", Command::Lyrics),
    ] {
        let source = if matches!(identifier, "backward" | "forward") {
            &transport
        } else {
            &bar
        };
        let item = source
            .itemForIdentifier(&NSString::from_str(&format!(
                "neoviolet.touchbar.{identifier}"
            )))
            .unwrap();
        let view = item.downcast_ref::<NSCustomTouchBarItem>().unwrap().view();
        let button = view.downcast_ref::<NSButton>().unwrap();
        unsafe {
            button.performClick(None);
        }
        let event = receiver.try_recv().unwrap();
        assert_eq!(event.context, state.context);
        assert_eq!(event.command, expected);
    }

    sidecar::check(&window, &bar, marker);
    if std::env::var_os("NEOVIOLET_TEST_LYRICS_FOCUS").is_some() {
        focus::run(&window, &bar, marker);
    }
    state.can_play = false;
    state.can_seek = false;
    state.available = false;
    native::sync(id, state);
    assert!(!play.isEnabled());
    unsafe {
        play.performClick(None);
    }
    assert!(receiver.try_recv().is_err());

    native::install(id, &window, |_| {});
    assert!(
        bar.delegate().is_none(),
        "reinstall releases the previous delegate"
    );
    assert!(
        play.target().is_none(),
        "old controls must not retain stale actions"
    );
    let replacement = window.touchBar().unwrap();
    window.close();
    assert!(
        replacement.delegate().is_none(),
        "closing the window removes the controller"
    );
    assert!(window.touchBar().is_none());
    println!(
        "Native Touch Bar: window attachment, real actions, grouped transport, snapshots and cleanup passed."
    );
}
