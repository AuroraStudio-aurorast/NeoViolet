//! Window-owned AppKit Touch Bar; all objects and callbacks stay on the main thread.
use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::ptr::NonNull;

use block2::RcBlock;
use objc2::rc::{Retained, Weak};
use objc2::runtime::ProtocolObject;
use objc2::{DefinedClass, MainThreadOnly, define_class, msg_send, sel};
use objc2_app_kit::{
    NSAccessibility, NSButton, NSButtonType, NSCustomTouchBarItem, NSGroupTouchBarItem, NSImage,
    NSTouchBar, NSTouchBarDelegate, NSTouchBarItem, NSTouchBarItemIdentifier,
    NSTouchBarItemIdentifierFlexibleSpace, NSWindow, NSWindowWillCloseNotification,
};
use objc2_foundation::{
    MainThreadMarker, NSArray, NSNotification, NSNotificationCenter, NSObject, NSObjectProtocol,
    NSString,
};

use crate::touchbar::{Command, Context, Event, State};

thread_local! {
    static BARS: RefCell<HashMap<iced::window::Id, Bar>> = RefCell::new(HashMap::new());
}

const ITEMS: [&str; 3] = ["open", "transport", "lyrics"];
const BUTTONS: [&str; 5] = ["open", "backward", "playback", "forward", "lyrics"];

#[derive(Default)]
struct Controls {
    items: HashMap<String, Retained<NSTouchBarItem>>,
    buttons: HashMap<isize, Retained<NSButton>>,
}

struct ControllerState {
    state: Cell<State>,
    controls: RefCell<Controls>,
    emit: Box<dyn Fn(Event)>,
}

define_class!(
    #[unsafe(super = NSObject)]
    #[thread_kind = MainThreadOnly]
    #[ivars = ControllerState]
    struct Controller;

    unsafe impl NSObjectProtocol for Controller {}

    unsafe impl NSTouchBarDelegate for Controller {
        #[unsafe(method_id(touchBar:makeItemForIdentifier:))]
        fn make_item(&self, _bar: &NSTouchBar, identifier: &NSTouchBarItemIdentifier)
            -> Option<Retained<NSTouchBarItem>>
        {
            self.item(identifier)
        }
    }

    impl Controller {
        #[unsafe(method(activate:))]
        fn activate(&self, sender: &NSButton) {
            let command = match sender.tag() {
                0 => Command::Open,
                1 => Command::SeekBy(-15.0),
                2 => Command::PlayPause,
                3 => Command::SeekBy(15.0),
                4 => Command::Lyrics,
                _ => return,
            };
            // A toggle control must display the confirmed preference until Go saves it.
            if sender.tag() == 4 {
                sender.setState(isize::from(self.ivars().state.get().lyrics));
            }
            self.emit(command);
        }
    }
);

impl Controller {
    fn emit(&self, command: Command) {
        let state = self.ivars().state.get();
        if state.allows(command) {
            (self.ivars().emit)(Event {
                context: state.context,
                command,
            });
        }
    }

    fn item(&self, identifier: &NSString) -> Option<Retained<NSTouchBarItem>> {
        let name = identifier.to_string();
        if let Some(item) = self.ivars().controls.borrow().items.get(&name) {
            return Some(item.clone());
        }
        let key = name.strip_prefix("neoviolet.touchbar.")?;
        let item = match key {
            "transport" => self.transport(identifier),
            _ => {
                let tag = BUTTONS.iter().position(|item| *item == key)? as isize;
                let (label, symbol) = match tag {
                    0 => ("Open", "folder"),
                    1 => ("Back 15 Seconds", "gobackward.15"),
                    2 => ("Play", "play.fill"),
                    3 => ("Forward 15 Seconds", "goforward.15"),
                    4 => ("Desktop Lyrics", "text.bubble"),
                    _ => return None,
                };
                // SAFETY: The target implements activate: with an NSButton argument.
                let button = unsafe {
                    NSButton::buttonWithTitle_target_action(
                        &NSString::from_str(label),
                        Some(self),
                        Some(sel!(activate:)),
                        self.mtm(),
                    )
                };
                button.setTag(tag);
                if tag == 4 {
                    button.setButtonType(NSButtonType::PushOnPushOff);
                }
                appearance(&button, label, symbol);
                let item = NSCustomTouchBarItem::initWithIdentifier(
                    NSCustomTouchBarItem::alloc(self.mtm()),
                    identifier,
                );
                item.setCustomizationLabel(Some(&NSString::from_str(label)));
                item.setView(&button);
                self.ivars()
                    .controls
                    .borrow_mut()
                    .buttons
                    .insert(tag, button);
                item.into_super()
            }
        };
        self.ivars()
            .controls
            .borrow_mut()
            .items
            .insert(name, item.clone());
        self.apply();
        Some(item)
    }

    fn transport(&self, identifier: &NSString) -> Retained<NSTouchBarItem> {
        let items: Vec<_> = ["backward", "playback", "forward"]
            .iter()
            .map(|key| {
                self.item(&NSString::from_str(&format!("neoviolet.touchbar.{key}")))
                    .expect("transport buttons have fixed identifiers")
            })
            .collect();
        let group = NSGroupTouchBarItem::groupItemWithIdentifier_items(
            identifier,
            &NSArray::from_retained_slice(&items),
            self.mtm(),
        );
        group.setPrefersEqualWidths(true);
        group.setCustomizationLabel(Some(&NSString::from_str("Playback Controls")));
        group.into_super()
    }

    fn apply(&self) {
        let state = self.ivars().state.get();
        let controls = self.ivars().controls.borrow();
        for (tag, button) in &controls.buttons {
            button.setEnabled(match tag {
                2 => state.can_play,
                1 | 3 => state.can_seek,
                _ => state.available,
            });
            if *tag == 2 {
                let (label, symbol) = if state.playing {
                    ("Pause", "pause.fill")
                } else {
                    ("Play", "play.fill")
                };
                appearance(button, label, symbol);
            } else if *tag == 4 {
                button.setState(isize::from(state.lyrics));
            }
        }
    }

    fn sync(&self, state: State) {
        self.ivars().state.set(state);
        self.apply();
    }
}

fn appearance(button: &NSButton, label: &str, symbol: &str) {
    let label = NSString::from_str(label);
    let image = NSImage::imageWithSystemSymbolName_accessibilityDescription(
        &NSString::from_str(symbol),
        Some(&label),
    );
    let title = if image.is_some() {
        NSString::new()
    } else {
        label.clone()
    };
    button.setTitle(&title);
    button.setImage(image.as_deref());
    button.setToolTip(Some(&label));
    button.setAccessibilityLabel(Some(&label));
}

struct Bar {
    window: Weak<NSWindow>,
    bar: Retained<NSTouchBar>,
    controller: Retained<Controller>,
    observer: Retained<ProtocolObject<dyn NSObjectProtocol>>,
}

impl Drop for Bar {
    fn drop(&mut self) {
        // SAFETY: This token belongs to the main-thread window-close registration below.
        unsafe {
            NSNotificationCenter::defaultCenter().removeObserver(self.observer.as_ref());
        }
        self.bar.setDelegate(None);
        let controls = self.controller.ivars().controls.borrow();
        for button in controls.buttons.values() {
            // SAFETY: Clearing our own target/action pairs before releasing the controller.
            unsafe {
                button.setTarget(None);
                button.setAction(None);
            }
        }
        if let Some(window) = self.window.load()
            && window.touchBar().as_deref() == Some(&*self.bar)
        {
            window.setTouchBar(None);
        }
    }
}

pub fn install(id: iced::window::Id, window: &NSWindow, emit: impl Fn(Event) + 'static) {
    let previous = BARS.with(|bars| bars.borrow_mut().remove(&id));
    drop(previous);
    let marker = MainThreadMarker::new().expect("Touch Bar installation needs the main thread");
    let initial = State {
        context: Context {
            window: id,
            session: 0,
            epoch: 0,
            track_id: None,
        },
        available: false,
        can_play: false,
        can_seek: false,
        playing: false,
        lyrics: false,
    };
    // SAFETY: NSObject's initializer and the declared ivars are used on AppKit's main thread.
    let controller: Retained<Controller> = unsafe {
        let object = Controller::alloc(marker).set_ivars(ControllerState {
            state: Cell::new(initial),
            controls: RefCell::new(Controls::default()),
            emit: Box::new(emit),
        });
        msg_send![super(object), init]
    };
    let bar = NSTouchBar::new(marker);
    bar.setDelegate(Some(ProtocolObject::from_ref(&*controller)));
    let mut identifiers: Vec<_> = ITEMS
        .iter()
        .map(|key| NSString::from_str(&format!("neoviolet.touchbar.{key}")))
        .collect();
    // Let AppKit absorb trailing room so Lyrics sits at the available bar's right edge.
    // SAFETY: The standard AppKit identifier is available on all supported macOS versions.
    identifiers.insert(
        2,
        unsafe { NSTouchBarItemIdentifierFlexibleSpace }.to_owned(),
    );
    bar.setDefaultItemIdentifiers(&NSArray::from_retained_slice(&identifiers));
    // Center the whole group; a standalone principal playback button splits the seek pair.
    bar.setPrincipalItemIdentifier(Some(&NSString::from_str("neoviolet.touchbar.transport")));
    // The winit content view returns nil from touchBar; AppKit can continue to its window.
    window.setTouchBar(Some(&bar));
    let close = RcBlock::new(move |_: NonNull<NSNotification>| {
        let bar = BARS.with(|bars| bars.borrow_mut().remove(&id));
        drop(bar);
    });
    // SAFETY: The filtered window and the synchronous notification both belong to this thread.
    let observer = unsafe {
        NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
            Some(NSWindowWillCloseNotification),
            Some(window),
            None,
            &close,
        )
    };
    BARS.with(|bars| {
        bars.borrow_mut().insert(
            id,
            Bar {
                window: Weak::new(window),
                bar,
                controller,
                observer,
            },
        );
    });
}

pub fn sync(id: iced::window::Id, state: State) {
    let controller = BARS.with(|bars| bars.borrow().get(&id).map(|bar| bar.controller.clone()));
    if let Some(controller) = controller {
        controller.sync(state);
    }
}
