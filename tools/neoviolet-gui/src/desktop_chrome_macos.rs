//! Preserve native caption geometry after sheet animations and AppKit layout.
use block2::RcBlock;
use objc2::rc::{Retained, Weak};
use objc2::runtime::{AnyObject, ProtocolObject};
use objc2_app_kit::{
    NSView, NSViewBoundsDidChangeNotification, NSViewFrameDidChangeNotification, NSWindow,
    NSWindowButton, NSWindowDidEndSheetNotification, NSWindowDidExitFullScreenNotification,
    NSWindowStyleMask, NSWindowWillCloseNotification,
};
use objc2_foundation::{
    NSArray, NSNotification, NSNotificationCenter, NSObjectProtocol, NSPoint, NSRunLoop,
    NSRunLoopCommonModes, NSString,
};
use std::cell::{Cell, RefCell};
use std::collections::{HashMap, HashSet};
use std::ptr::NonNull;
use std::rc::Rc;

thread_local! {
    static TRACKERS:RefCell<HashMap<iced::window::Id,Tracker>>=RefCell::new(HashMap::new());
}

struct State {
    view: Weak<NSView>,
    window: Weak<NSWindow>,
    applying: Cell<bool>,
    queued: Cell<bool>,
    active: Cell<bool>,
    titlebar_height: f64,
}

impl State {
    fn schedule(self: &Rc<Self>) {
        if !self.active.get() || self.applying.get() || self.queued.replace(true) {
            return;
        }
        let state = Rc::downgrade(self);
        let callback = RcBlock::new(move || {
            if let Some(state) = state.upgrade() {
                state.queued.set(false);
                if state.active.get() {
                    state.apply();
                }
            }
        });
        // SAFETY: The block stays on the main run loop, where these AppKit objects and Rc
        // are confined. Coalescing lets AppKit finish moving all three buttons first.
        unsafe {
            NSRunLoop::mainRunLoop()
                .performInModes_block(&NSArray::from_slice(&[NSRunLoopCommonModes]), &callback);
        }
        // A queued correction must run even when closing the panel leaves the GUI idle.
        NSRunLoop::mainRunLoop().getCFRunLoop().wake_up();
    }

    fn apply(&self) {
        if self.applying.replace(true) {
            return;
        }
        struct Applying<'a>(&'a Cell<bool>);
        impl Drop for Applying<'_> {
            fn drop(&mut self) {
                self.0.set(false);
            }
        }
        let _guard = Applying(&self.applying);
        if let (Some(view), Some(window)) = (self.view.load(), self.window.load()) {
            align(&view, &window, self.titlebar_height);
        }
    }
}

struct ObservedView {
    view: Weak<NSView>,
    frame_notifications: bool,
    bounds_notifications: bool,
}

struct Tracker {
    tokens: Vec<Retained<ProtocolObject<dyn NSObjectProtocol>>>,
    views: Vec<ObservedView>,
    state: Rc<State>,
}

impl Drop for Tracker {
    fn drop(&mut self) {
        self.state.active.set(false);
        let center = NSNotificationCenter::defaultCenter();
        for token in &self.tokens {
            // SAFETY: These tokens came from this notification center's block registration.
            unsafe {
                center.removeObserver((**token).as_ref());
            }
        }
        for observed in &self.views {
            if let Some(view) = observed.view.load() {
                view.setPostsFrameChangedNotifications(observed.frame_notifications);
                view.setPostsBoundsChangedNotifications(observed.bounds_notifications);
            }
        }
    }
}

impl Tracker {
    fn observe(&mut self, name: &NSString, object: &AnyObject) {
        let state = self.state.clone();
        let callback = RcBlock::new(move |_: NonNull<NSNotification>| state.schedule());
        // SAFETY: All observed objects belong to the main-thread window hierarchy. A nil
        // queue delivers synchronously on that same thread; the Rc never crosses threads.
        let token = unsafe {
            NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
                Some(name),
                Some(object),
                None,
                &callback,
            )
        };
        self.tokens.push(token);
    }

    fn watch_hierarchy(&mut self, view: &NSView, seen: &mut HashSet<usize>) {
        let key = std::ptr::from_ref(view) as usize;
        if !seen.insert(key) {
            return;
        }
        self.views.push(ObservedView {
            view: Weak::new(view),
            frame_notifications: view.postsFrameChangedNotifications(),
            bounds_notifications: view.postsBoundsChangedNotifications(),
        });
        view.setPostsFrameChangedNotifications(true);
        view.setPostsBoundsChangedNotifications(true);
        // SAFETY: These AppKit notification names exist on every supported macOS release.
        unsafe {
            self.observe(NSViewFrameDidChangeNotification, view);
            self.observe(NSViewBoundsDidChangeNotification, view);
        }
        // SAFETY: Registration and hierarchy traversal run on the window's owning thread.
        if let Some(parent) = unsafe { view.superview() } {
            self.watch_hierarchy(&parent, seen);
        }
    }
}

pub fn install(id: iced::window::Id, view: &NSView, window: &NSWindow, titlebar_height: f64) {
    TRACKERS.with(|trackers| {
        trackers.borrow_mut().remove(&id);
    });
    let state = Rc::new(State {
        view: Weak::new(view),
        window: Weak::new(window),
        applying: Cell::new(false),
        queued: Cell::new(false),
        active: Cell::new(true),
        titlebar_height,
    });
    let mut tracker = Tracker {
        tokens: Vec::new(),
        views: Vec::new(),
        state: state.clone(),
    };
    let mut seen = HashSet::new();
    tracker.watch_hierarchy(view, &mut seen);
    for kind in [
        NSWindowButton::CloseButton,
        NSWindowButton::MiniaturizeButton,
        NSWindowButton::ZoomButton,
    ] {
        if let Some(button) = window.standardWindowButton(kind) {
            tracker.watch_hierarchy(&button, &mut seen);
        }
    }
    // SAFETY: Public AppKit window notifications; filtered to this main-thread window.
    unsafe {
        tracker.observe(NSWindowDidEndSheetNotification, window);
        tracker.observe(NSWindowDidExitFullScreenNotification, window);
    }
    let close = RcBlock::new(move |_: NonNull<NSNotification>| {
        TRACKERS.with(|trackers| {
            trackers.borrow_mut().remove(&id);
        });
    });
    // SAFETY: The callback is synchronous for this main-thread-only window. Closing removes
    // all observer tokens and restores notification settings without retaining the window.
    tracker.tokens.push(unsafe {
        NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
            Some(NSWindowWillCloseNotification),
            Some(window),
            None,
            &close,
        )
    });
    TRACKERS.with(|trackers| {
        trackers.borrow_mut().insert(id, tracker);
    });
    state.schedule();
}

pub fn refresh(id: iced::window::Id, view: &NSView, window: &NSWindow, titlebar_height: f64) {
    let state = TRACKERS.with(|trackers| {
        trackers
            .borrow()
            .get(&id)
            .map(|tracker| tracker.state.clone())
    });
    if let Some(state) = state {
        state.schedule();
    } else {
        install(id, view, window, titlebar_height);
    }
}

fn align(view: &NSView, window: &NSWindow, titlebar_height: f64) {
    // AppKit owns the auto-hiding fullscreen controls and their animation.
    if window.styleMask().contains(NSWindowStyleMask::FullScreen)
        || window.attachedSheet().is_some()
    {
        return;
    }
    let content = view.bounds();
    let center_y = if view.isFlipped() {
        content.origin.y + titlebar_height / 2.0
    } else {
        content.origin.y + content.size.height - titlebar_height / 2.0
    };
    for (index, kind) in [
        NSWindowButton::CloseButton,
        NSWindowButton::MiniaturizeButton,
        NSWindowButton::ZoomButton,
    ]
    .into_iter()
    .enumerate()
    {
        let Some(button) = window.standardWindowButton(kind) else {
            continue;
        };
        // SAFETY: The native button hierarchy is accessed only on the window's owning thread.
        let Some(parent) = (unsafe { button.superview() }) else {
            continue;
        };
        let size = button.frame().size;
        let center = parent.convertPoint_fromView(
            NSPoint::new(
                content.origin.x + 14.0 + index as f64 * 20.0 + size.width / 2.0,
                center_y,
            ),
            Some(view),
        );
        let bounds = parent.bounds();
        // Preserve each native hit target, including systems with a shorter titlebar container.
        let origin = NSPoint::new(
            (center.x - size.width / 2.0).clamp(
                bounds.origin.x,
                (bounds.origin.x + bounds.size.width - size.width).max(bounds.origin.x),
            ),
            (center.y - size.height / 2.0).clamp(
                bounds.origin.y,
                (bounds.origin.y + bounds.size.height - size.height).max(bounds.origin.y),
            ),
        );
        if button.frame().origin != origin {
            button.setFrameOrigin(origin);
        }
        // Native buttons can keep stale pixels after a sheet's compositing pass even
        // when their logical frame is already correct. Invalidate the entire group.
        NSView::setNeedsDisplay(&button, true);
        parent.setNeedsDisplay(true);
    }
}
