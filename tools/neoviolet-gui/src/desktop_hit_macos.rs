//! Track mouse movement even while the transparent part of the overlay ignores events.
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

use block2::RcBlock;
use objc2::rc::{Retained, Weak};
use objc2::runtime::AnyObject;
use objc2_app_kit::{NSEvent, NSEventMask, NSWindow};
use tokio::sync::mpsc;

use crate::desktop::Event;
use crate::lyrics_hit_test::Mask;

thread_local! {
    static TRACKERS: RefCell<HashMap<iced::window::Id, Tracker>> = RefCell::new(HashMap::new());
}

struct State {
    id: iced::window::Id,
    window: Weak<NSWindow>,
    sender: mpsc::UnboundedSender<Event>,
    mask: Option<Mask>,
    locked: bool,
    hovered: bool,
    mouse_down: bool,
}

impl State {
    fn apply(&mut self) {
        let Some(window) = self.window.load() else {
            return;
        };
        // Keep native drag/click sequences intact until the mouse is released.
        if NSEvent::pressedMouseButtons() != 0 {
            self.mouse_down = true;
            return;
        }
        if self.mouse_down {
            self.mouse_down = false;
            let _ = self.sender.send(Event::LyricsReleased(self.id));
        }
        let Some(view) = window.contentView() else {
            return;
        };
        let point = view.convertPoint_fromView(window.mouseLocationOutsideOfEventStream(), None);
        let bounds = view.bounds();
        let point = iced::Point::new(
            (point.x - bounds.origin.x) as f32,
            if view.isFlipped() {
                point.y - bounds.origin.y
            } else {
                bounds.size.height - (point.y - bounds.origin.y)
            } as f32,
        );
        let inside = point.x >= 0.0
            && point.y >= 0.0
            && point.x < bounds.size.width as f32
            && point.y < bounds.size.height as f32;
        let accepts = !self.locked && self.mask.as_ref().is_some_and(|mask| mask.contains(point));
        if window.ignoresMouseEvents() == accepts {
            window.setIgnoresMouseEvents(!accepts);
        }
        let hovered = inside && !self.locked;
        if hovered != self.hovered {
            self.hovered = hovered;
            let _ = self.sender.send(Event::LyricsHover(self.id, hovered));
        }
    }
}

struct Tracker {
    state: Rc<RefCell<State>>,
    local: Option<Retained<AnyObject>>,
    global: Option<Retained<AnyObject>>,
}

impl Drop for Tracker {
    fn drop(&mut self) {
        for monitor in [&self.local, &self.global].into_iter().flatten() {
            // SAFETY: Both objects came from NSEvent's monitor registration methods.
            unsafe {
                NSEvent::removeMonitor(monitor);
            }
        }
    }
}

pub fn install(id: iced::window::Id, window: &NSWindow, sender: mpsc::UnboundedSender<Event>) {
    let state = Rc::new(RefCell::new(State {
        id,
        window: Weak::new(window),
        sender,
        mask: None,
        locked: false,
        hovered: false,
        mouse_down: false,
    }));
    let mask = NSEventMask::MouseMoved
        | NSEventMask::LeftMouseDown
        | NSEventMask::LeftMouseDragged
        | NSEventMask::RightMouseDragged
        | NSEventMask::OtherMouseDragged
        | NSEventMask::LeftMouseUp
        | NSEventMask::RightMouseUp
        | NSEventMask::OtherMouseUp;
    let local_state = state.clone();
    let local = RcBlock::new(move |event: std::ptr::NonNull<NSEvent>| {
        local_state.borrow_mut().apply();
        event.as_ptr()
    });
    // SAFETY: The local monitor always returns the original, still-live event.
    let local = unsafe { NSEvent::addLocalMonitorForEventsMatchingMask_handler(mask, &local) };
    let global_state = state.clone();
    let global = RcBlock::new(move |_: std::ptr::NonNull<NSEvent>| {
        global_state.borrow_mut().apply();
    });
    let global = NSEvent::addGlobalMonitorForEventsMatchingMask_handler(mask, &global);
    state.borrow_mut().apply();
    TRACKERS.with_borrow_mut(|trackers| {
        trackers.insert(
            id,
            Tracker {
                state,
                local,
                global,
            },
        );
    });
}

pub fn set_mask(id: iced::window::Id, mask: Mask, locked: bool) {
    TRACKERS.with_borrow(|trackers| {
        if let Some(tracker) = trackers.get(&id) {
            let mut state = tracker.state.borrow_mut();
            state.mask = Some(mask);
            state.locked = locked;
            state.apply();
        }
    });
}

pub fn set_locked(id: iced::window::Id, locked: bool) {
    TRACKERS.with_borrow(|trackers| {
        if let Some(tracker) = trackers.get(&id) {
            let mut state = tracker.state.borrow_mut();
            state.locked = locked;
            state.apply();
        }
    });
}

pub fn remove(id: iced::window::Id) {
    TRACKERS.with_borrow_mut(|trackers| {
        if let Some(tracker) = trackers.remove(&id)
            && let Some(window) = tracker.state.borrow().window.load()
        {
            super::lyrics::detach(&window);
        }
    });
}

pub fn link(id: iced::window::Id, toolbar: iced::window::Id, offset: f64) -> bool {
    TRACKERS.with_borrow(|trackers| {
        let lyrics = trackers
            .get(&id)
            .and_then(|t| t.state.borrow().window.load());
        let toolbar = trackers
            .get(&toolbar)
            .and_then(|t| t.state.borrow().window.load());
        if let (Some(lyrics), Some(toolbar)) = (lyrics, toolbar) {
            super::lyrics::attach(&lyrics, &toolbar, offset);
            true
        } else {
            false
        }
    })
}

pub fn resize(id: iced::window::Id, size: iced::Size, offset: f64) {
    TRACKERS.with_borrow(|trackers| {
        if let Some(window) = trackers
            .get(&id)
            .and_then(|t| t.state.borrow().window.load())
        {
            super::lyrics::resize(
                &window,
                objc2_foundation::NSSize::new(f64::from(size.width), f64::from(size.height)),
                offset,
            );
        }
    });
}
