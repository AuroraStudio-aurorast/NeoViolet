//! Preserve native caption geometry after sheet animations and AppKit layout.
#[path = "desktop_chrome_layout.rs"]
mod layout;
use block2::RcBlock;
use layout::Layout;
use objc2::rc::{Retained, Weak};
use objc2::runtime::{AnyObject, ProtocolObject};
use objc2_app_kit::{
    NSView, NSViewBoundsDidChangeNotification, NSViewFrameDidChangeNotification, NSWindow,
    NSWindowButton, NSWindowDidChangeBackingPropertiesNotification,
    NSWindowDidChangeScreenNotification, NSWindowDidEndSheetNotification,
    NSWindowDidEnterFullScreenNotification, NSWindowDidExitFullScreenNotification,
    NSWindowDidResizeNotification, NSWindowDidUpdateNotification, NSWindowStyleMask,
    NSWindowWillBeginSheetNotification, NSWindowWillCloseNotification,
    NSWindowWillEnterFullScreenNotification, NSWindowWillExitFullScreenNotification,
};
use objc2_foundation::{
    NSArray, NSNotification, NSNotificationCenter, NSObjectProtocol, NSPoint, NSRect, NSRunLoop,
    NSRunLoopCommonModes, NSString, ns_string,
};
use std::cell::{Cell, RefCell};
use std::collections::{HashMap, HashSet};
use std::ptr::NonNull;
use std::rc::Rc;

thread_local! {
    static TRACKERS:RefCell<HashMap<iced::window::Id,Tracker>>=RefCell::new(HashMap::new());
}

type TitleInsetCallback = Box<dyn Fn(f32)>;
type Geometry = Vec<(usize, NSRect, NSRect)>;

struct State {
    id: iced::window::Id,
    view: Weak<NSView>,
    window: Weak<NSWindow>,
    applying: Cell<bool>,
    queued: Cell<bool>,
    active: Cell<bool>,
    titlebar_height: f64,
    layout: Cell<Option<Layout>>,
    major: isize,
    fullscreen_transition: Cell<bool>,
    sheet_transition: Cell<bool>,
    repaint: Cell<bool>,
    title_inset: Cell<Option<f32>>,
    on_title_inset: RefCell<Option<TitleInsetCallback>>,
    geometry: RefCell<Geometry>,
}

impl State {
    fn check_update(self: &Rc<Self>) {
        if self.applying.get() || self.fullscreen_transition.get() || self.sheet_transition.get() {
            return;
        }
        if let (Some(view), Some(window)) = (self.view.load(), self.window.load())
            && !window.styleMask().contains(NSWindowStyleMask::FullScreen)
            && window.attachedSheet().is_none()
            && *self.geometry.borrow() != geometry(&view, &window)
        {
            self.schedule();
        }
    }

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
            if self.fullscreen_transition.get()
                || self.sheet_transition.get()
                || window.styleMask().contains(NSWindowStyleMask::FullScreen)
                || window.attachedSheet().is_some()
            {
                return;
            }
            TRACKERS.with(|trackers| {
                if let Some(tracker) = trackers.borrow_mut().get_mut(&self.id) {
                    tracker.rebind(&view, &window);
                }
            });
            let layout = self.layout.get().or_else(|| {
                let frames = button_frames(&view, &window)?;
                Layout::capture(self.major, frames)
            });
            if let Some(layout) = layout {
                self.layout.set(Some(layout));
                let inset = align(
                    &view,
                    &window,
                    self.titlebar_height,
                    layout,
                    self.repaint.get(),
                );
                if let Some(inset) = inset {
                    self.repaint.set(false);
                    if self.title_inset.replace(Some(inset)) != Some(inset)
                        && let Some(callback) = self.on_title_inset.borrow().as_ref()
                    {
                        callback(inset);
                    }
                }
            }
            // Cache even an incomplete layout: only a later geometry change can make it usable.
            self.geometry.replace(geometry(&view, &window));
        }
    }
}

struct ObservedView {
    view: Weak<NSView>,
    frame_notifications: bool,
    bounds_notifications: bool,
    tokens: Vec<Retained<ProtocolObject<dyn NSObjectProtocol>>>,
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
        self.clear_views();
    }
}

impl Tracker {
    fn observe(
        &mut self,
        name: &NSString,
        object: &AnyObject,
        event: impl Fn(&Rc<State>) + 'static,
    ) {
        self.tokens.push(observer(&self.state, name, object, event));
    }

    fn clear_views(&mut self) {
        let center = NSNotificationCenter::defaultCenter();
        for observed in self.views.drain(..) {
            for token in observed.tokens {
                // SAFETY: Each token was registered on this notification center.
                unsafe {
                    center.removeObserver(token.as_ref());
                }
            }
            if let Some(view) = observed.view.load() {
                view.setPostsFrameChangedNotifications(observed.frame_notifications);
                view.setPostsBoundsChangedNotifications(observed.bounds_notifications);
            }
        }
    }

    fn rebind(&mut self, view: &NSView, window: &NSWindow) {
        let mut current = HashSet::new();
        collect_hierarchy(view, &mut current);
        for kind in BUTTONS {
            if let Some(button) = window.standardWindowButton(kind) {
                collect_hierarchy(&button, &mut current);
            }
        }
        let previous: HashSet<_> = self
            .views
            .iter()
            .filter_map(|observed| observed.view.load())
            .map(|view| std::ptr::from_ref(&*view) as usize)
            .collect();
        if previous == current {
            return;
        }
        self.clear_views();
        let mut seen = HashSet::new();
        self.watch_hierarchy(view, &mut seen);
        for kind in BUTTONS {
            if let Some(button) = window.standardWindowButton(kind) {
                self.watch_hierarchy(&button, &mut seen);
            }
        }
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
            // SAFETY: Public notifications, confined to the window's owning main thread.
            tokens: unsafe {
                [
                    NSViewFrameDidChangeNotification,
                    NSViewBoundsDidChangeNotification,
                ]
            }
            .into_iter()
            .map(|name| observer(&self.state, name, view, |state| state.schedule()))
            .collect(),
        });
        view.setPostsFrameChangedNotifications(true);
        view.setPostsBoundsChangedNotifications(true);
        // SAFETY: Registration and hierarchy traversal run on the window's owning thread.
        if let Some(parent) = unsafe { view.superview() } {
            self.watch_hierarchy(&parent, seen);
        }
    }
}

pub fn install(id: iced::window::Id, view: &NSView, window: &NSWindow, titlebar_height: f64) {
    let previous = TRACKERS.with(|trackers| trackers.borrow_mut().remove(&id));
    let (previous_layout, previous_callback, fullscreen_transition, sheet_transition) = previous
        .filter(|tracker| {
            tracker
                .state
                .window
                .load()
                .is_some_and(|previous| std::ptr::eq(&*previous, window))
        })
        .map(|tracker| {
            let callback = tracker.state.on_title_inset.borrow_mut().take();
            (
                tracker.state.layout.get(),
                callback,
                tracker.state.fullscreen_transition.get(),
                tracker.state.sheet_transition.get(),
            )
        })
        .unwrap_or((None, None, false, false));
    let state = Rc::new(State {
        id,
        view: Weak::new(view),
        window: Weak::new(window),
        applying: Cell::new(false),
        queued: Cell::new(false),
        active: Cell::new(true),
        titlebar_height,
        layout: Cell::new(previous_layout),
        major: objc2_foundation::NSProcessInfo::processInfo()
            .operatingSystemVersion()
            .majorVersion,
        fullscreen_transition: Cell::new(fullscreen_transition),
        sheet_transition: Cell::new(sheet_transition),
        repaint: Cell::new(false),
        title_inset: Cell::new(None),
        on_title_inset: RefCell::new(previous_callback),
        geometry: RefCell::new(Vec::new()),
    });
    let mut tracker = Tracker {
        tokens: Vec::new(),
        views: Vec::new(),
        state: state.clone(),
    };
    tracker.rebind(view, window);
    // SAFETY: Public AppKit notifications filtered to this main-thread window.
    unsafe {
        // Resize has finished laying out all native controls. Correct them synchronously
        // before AppKit displays this size; a run-loop correction exposes the reset each frame.
        tracker.observe(NSWindowDidResizeNotification, window, |state| state.apply());
        tracker.observe(NSWindowDidUpdateNotification, window, |state| {
            state.check_update()
        });
        for name in [
            NSWindowDidChangeScreenNotification,
            NSWindowDidChangeBackingPropertiesNotification,
        ] {
            tracker.observe(name, window, |state| state.schedule());
        }
        tracker.observe(NSWindowWillBeginSheetNotification, window, |state| {
            state.sheet_transition.set(true)
        });
        tracker.observe(NSWindowDidEndSheetNotification, window, |state| {
            state.sheet_transition.set(false);
            state.repaint.set(true);
            state.schedule();
        });
        for name in [
            NSWindowWillEnterFullScreenNotification,
            NSWindowWillExitFullScreenNotification,
        ] {
            tracker.observe(name, window, |state| state.fullscreen_transition.set(true));
        }
        for name in [
            NSWindowDidEnterFullScreenNotification,
            NSWindowDidExitFullScreenNotification,
            ns_string!("NeoVioletFullscreenTransitionFailed"),
        ] {
            tracker.observe(name, window, |state| {
                state.fullscreen_transition.set(false);
                state.schedule();
            });
        }
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

fn button_frames(view: &NSView, window: &NSWindow) -> Option<[(f64, f64); 3]> {
    let mut frames = [(0.0, 0.0); 3];
    for (index, kind) in [
        NSWindowButton::CloseButton,
        NSWindowButton::MiniaturizeButton,
        NSWindowButton::ZoomButton,
    ]
    .into_iter()
    .enumerate()
    {
        let button = window.standardWindowButton(kind)?;
        // SAFETY: Native geometry is borrowed on the owning main thread.
        let parent = unsafe { button.superview() }?;
        let frame = button.frame();
        let point = view.convertPoint_fromView(frame.origin, Some(&parent));
        frames[index] = (point.x, frame.size.width);
    }
    Some(frames)
}

fn align(
    view: &NSView,
    window: &NSWindow,
    titlebar_height: f64,
    layout: Layout,
    repaint: bool,
) -> Option<f32> {
    // AppKit owns the auto-hiding fullscreen controls and their animation.
    if window.styleMask().contains(NSWindowStyleMask::FullScreen)
        || window.attachedSheet().is_some()
    {
        return None;
    }
    let frames = button_frames(view, window)?;
    let origins = layout.origins(frames.map(|(_, width)| width));
    let content = view.bounds();
    let center_y = if view.isFlipped() {
        content.origin.y + titlebar_height / 2.0
    } else {
        content.origin.y + content.size.height - titlebar_height / 2.0
    };
    let mut targets = Vec::new();
    let mut low = NSPoint::new(f64::NEG_INFINITY, f64::NEG_INFINITY);
    let mut high = NSPoint::new(f64::INFINITY, f64::INFINITY);
    for (index, kind) in BUTTONS.into_iter().enumerate() {
        let Some(button) = window.standardWindowButton(kind) else {
            continue;
        };
        // SAFETY: The native button hierarchy is accessed only on the window's owning thread.
        let Some(parent) = (unsafe { button.superview() }) else {
            continue;
        };
        let size = button.frame().size;
        let desired = NSPoint::new(
            content.origin.x + origins[index],
            center_y - size.height / 2.0,
        );
        let bounds_in_content = view.convertRect_fromView(parent.bounds(), Some(&parent));
        low.x = low.x.max(bounds_in_content.origin.x - desired.x);
        high.x = high.x.min(
            bounds_in_content.origin.x + bounds_in_content.size.width - size.width - desired.x,
        );
        low.y = low.y.max(bounds_in_content.origin.y - desired.y);
        high.y = high.y.min(
            bounds_in_content.origin.y + bounds_in_content.size.height - size.height - desired.y,
        );
        targets.push((button, parent, desired));
    }
    let shift = if layout.modern() {
        // Translate the entire group when its container is narrower/shorter; never squeeze gaps.
        // If AppKit supplies an intermediate container too small for the group, await its next layout.
        if low.x > high.x || low.y > high.y {
            return None;
        }
        NSPoint::new(0.0_f64.clamp(low.x, high.x), 0.0_f64.clamp(low.y, high.y))
    } else {
        NSPoint::new(0.0, 0.0)
    };
    for (button, parent, desired) in targets {
        let size = button.frame().size;
        let center = parent.convertPoint_fromView(
            NSPoint::new(
                desired.x + shift.x + size.width / 2.0,
                desired.y + shift.y + size.height / 2.0,
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
        let moved = (button.frame().origin.x - origin.x).abs() > 0.01
            || (button.frame().origin.y - origin.y).abs() > 0.01;
        if moved {
            button.setFrameOrigin(origin);
        }
        // A sheet may leave stale pixels even when its final geometry is unchanged.
        if moved || repaint {
            NSView::setNeedsDisplay(&button, true);
            parent.setNeedsDisplay(true);
        }
    }
    Some(layout.title_inset(frames.map(|(_, width)| width)) + shift.x as f32)
}

const BUTTONS: [NSWindowButton; 3] = [
    NSWindowButton::CloseButton,
    NSWindowButton::MiniaturizeButton,
    NSWindowButton::ZoomButton,
];

fn collect_hierarchy(view: &NSView, seen: &mut HashSet<usize>) {
    if !seen.insert(std::ptr::from_ref(view) as usize) {
        return;
    }
    // SAFETY: Called only on the native window's owning main thread.
    if let Some(parent) = unsafe { view.superview() } {
        collect_hierarchy(&parent, seen);
    }
}

fn observer(
    state: &Rc<State>,
    name: &NSString,
    object: &AnyObject,
    event: impl Fn(&Rc<State>) + 'static,
) -> Retained<ProtocolObject<dyn NSObjectProtocol>> {
    let state = state.clone();
    let callback = RcBlock::new(move |_: NonNull<NSNotification>| event(&state));
    // SAFETY: Objects and callback belong to the main thread; nil queue delivers synchronously.
    unsafe {
        NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
            Some(name),
            Some(object),
            None,
            &callback,
        )
    }
}

pub fn on_title_inset(id: iced::window::Id, callback: impl Fn(f32) + 'static) {
    TRACKERS.with(|trackers| {
        if let Some(tracker) = trackers.borrow().get(&id) {
            tracker
                .state
                .on_title_inset
                .replace(Some(Box::new(callback)));
            tracker.state.title_inset.set(None);
            tracker.state.schedule();
        }
    });
}

fn geometry(view: &NSView, window: &NSWindow) -> Geometry {
    fn collect(view: &NSView, seen: &mut HashSet<usize>, geometry: &mut Geometry) {
        let key = std::ptr::from_ref(view) as usize;
        if !seen.insert(key) {
            return;
        }
        geometry.push((key, view.frame(), view.bounds()));
        // SAFETY: The snapshot is collected only on the window's owning main thread.
        if let Some(parent) = unsafe { view.superview() } {
            collect(&parent, seen, geometry);
        }
    }
    let mut seen = HashSet::new();
    let mut snapshot = Vec::new();
    collect(view, &mut seen, &mut snapshot);
    for kind in BUTTONS {
        if let Some(button) = window.standardWindowButton(kind) {
            collect(&button, &mut seen, &mut snapshot);
        }
    }
    snapshot
}
