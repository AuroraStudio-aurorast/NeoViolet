//! Additional recovery checks run by the main-thread native chrome executable.
use objc2::MainThreadOnly;
use objc2::rc::Retained;
use objc2_app_kit::{NSButton, NSView, NSWindow};
use objc2_foundation::{NSDate, NSNotificationCenter, NSPoint, NSRunLoop};

pub(super) fn check(
    id: iced::window::Id,
    window: &NSWindow,
    view: &NSView,
    buttons: &[Retained<NSButton>; 3],
    expected: [NSPoint; 3],
) {
    let settle =
        || NSRunLoop::mainRunLoop().runUntilDate(&NSDate::dateWithTimeIntervalSinceNow(0.01));
    let center = NSNotificationCenter::defaultCenter();
    let insets = std::rc::Rc::new(std::cell::RefCell::new(Vec::new()));
    let received = insets.clone();
    super::chrome::on_title_inset(id, move |inset| received.borrow_mut().push(inset));
    settle();
    let sizes = buttons.each_ref().map(|button| button.frame().size);
    let modern = objc2_foundation::NSProcessInfo::processInfo()
        .operatingSystemVersion()
        .majorVersion
        >= 26;
    let expected_inset = if modern {
        (expected[2].x + sizes[2].width + 12.0).max(78.0) as f32
    } else {
        78.0
    };
    assert_eq!(
        *insets.borrow(),
        [expected_inset],
        "title reservation must follow actual native geometry"
    );
    // A native reset may arrive without the frame notifications we subscribed to.
    for button in buttons {
        button.setPostsFrameChangedNotifications(false);
    }
    buttons[1].setFrameOrigin(NSPoint::new(3.0, 2.0));
    unsafe {
        center.postNotificationName_object(
            objc2_app_kit::NSWindowDidUpdateNotification,
            Some(window),
        );
    }
    settle();
    assert_eq!(
        buttons[1].frame().origin,
        expected[1],
        "window updates must repair otherwise unobserved resets"
    );
    for button in buttons {
        button.setPostsFrameChangedNotifications(true);
    }

    // A no-op correction must not trigger another draw/update cycle.
    for button in buttons {
        NSView::setNeedsDisplay(button, false);
        unsafe { button.superview() }
            .unwrap()
            .setNeedsDisplay(false);
    }
    super::chrome::refresh(id, view, window, 34.0);
    settle();
    assert_eq!(
        insets.borrow().len(),
        1,
        "stable layout must not emit repeated title updates"
    );
    assert!(
        !buttons[0].needsDisplay(),
        "stable geometry must not request another redraw"
    );
    assert!(
        !unsafe { buttons[0].superview() }.unwrap().needsDisplay(),
        "stable geometry must not invalidate the native titlebar"
    );
    // Moving buttons under a replacement container must rebind the observer chain.
    // SAFETY: This hidden test window and all its views stay on the main thread.
    let parent = unsafe { buttons[0].superview() }.unwrap();
    let replacement = NSView::initWithFrame(
        NSView::alloc(objc2_foundation::MainThreadMarker::new().unwrap()),
        parent.bounds(),
    );
    let original_frame_notifications = replacement.postsFrameChangedNotifications();
    parent.addSubview(&replacement);
    for button in buttons {
        replacement.addSubview(button);
    }
    unsafe {
        center.postNotificationName_object(
            objc2_app_kit::NSWindowDidUpdateNotification,
            Some(window),
        );
    }
    settle();
    assert!(replacement.postsFrameChangedNotifications());
    let bounds = replacement.bounds();
    replacement.setBoundsOrigin(NSPoint::new(bounds.origin.x + 3.0, bounds.origin.y));
    settle();
    let point = view.convertPoint_fromView(buttons[0].frame().origin, Some(&replacement));
    assert!(
        (point.x - 14.0).abs() < 0.1,
        "new parent coordinate changes must trigger correction"
    );
    if modern {
        replacement.setFrameSize(objc2_foundation::NSSize::new(
            70.0,
            parent.bounds().size.height,
        ));
        settle();
        for index in 1..3 {
            let gap = buttons[index].frame().origin.x
                - buttons[index - 1].frame().origin.x
                - sizes[index - 1].width;
            let expected_gap = expected[index].x - expected[index - 1].x - sizes[index - 1].width;
            assert!(
                (gap - expected_gap).abs() < 0.1,
                "constraining a native container must translate the group without squeezing its gaps"
            );
        }
        replacement.setFrameSize(parent.bounds().size);
        settle();
    }
    for button in buttons {
        parent.addSubview(button);
    }
    unsafe {
        center.postNotificationName_object(
            objc2_app_kit::NSWindowDidUpdateNotification,
            Some(window),
        );
    }
    settle();
    assert_eq!(
        replacement.postsFrameChangedNotifications(),
        original_frame_notifications,
        "retired containers must restore their original notification flags"
    );
    replacement.removeFromSuperview();

    for (begin, end) in unsafe {
        [
            (
                objc2_app_kit::NSWindowWillEnterFullScreenNotification,
                objc2_app_kit::NSWindowDidEnterFullScreenNotification,
            ),
            (
                objc2_app_kit::NSWindowWillExitFullScreenNotification,
                objc2_app_kit::NSWindowDidExitFullScreenNotification,
            ),
            (
                objc2_app_kit::NSWindowWillEnterFullScreenNotification,
                objc2_foundation::ns_string!("NeoVioletFullscreenTransitionFailed"),
            ),
            (
                objc2_app_kit::NSWindowWillBeginSheetNotification,
                objc2_app_kit::NSWindowDidEndSheetNotification,
            ),
        ]
    } {
        unsafe {
            center.postNotificationName_object(begin, Some(window));
        }
        let transient = NSPoint::new(2.0, 2.0);
        buttons[0].setFrameOrigin(transient);
        // Resize notifications during system animations must respect the same suspension.
        unsafe {
            center.postNotificationName_object(
                objc2_app_kit::NSWindowDidResizeNotification,
                Some(window),
            );
        }
        settle();
        assert_eq!(
            buttons[0].frame().origin,
            transient,
            "system transitions must own button geometry"
        );
        unsafe {
            center.postNotificationName_object(end, Some(window));
        }
        settle();
        assert_eq!(
            buttons[0].frame().origin,
            expected[0],
            "ending or cancelling transitions must restore windowed geometry"
        );
    }
    if modern {
        super::chrome::install(id, view, window, 34.0);
        settle();
        buttons[0].setFrameSize(objc2_foundation::NSSize::new(
            sizes[0].width + 4.0,
            sizes[0].height,
        ));
        settle();
        assert_eq!(
            insets.borrow().last(),
            Some(&(expected_inset + 4.0)),
            "reinstallation must preserve title updates when native button sizes change"
        );
        buttons[0].setFrameSize(sizes[0]);
        settle();
    }
    assert_eq!(
        buttons.each_ref().map(|button| button.frame().size),
        sizes,
        "layout corrections must preserve native button sizes"
    );
}
