use std::hash::{Hash, Hasher};
use std::sync::Arc;

use iced::futures::SinkExt;
use muda::accelerator::{Accelerator, Code, Modifiers};
use muda::{Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu};
use objc2::rc::Retained;
use objc2::runtime::ProtocolObject;
use objc2::{DefinedClass, MainThreadOnly, define_class, msg_send};
use objc2_app_kit::{NSApplication, NSApplicationDelegate, NSApplicationTerminateReply};
use objc2_foundation::{MainThreadMarker, NSArray, NSObject, NSObjectProtocol, NSURL};
use tokio::sync::{Mutex, mpsc};

use super::{Action, Event};

struct FileEvents {
    sender: mpsc::UnboundedSender<Event>,
}

define_class!(
    #[unsafe(super = NSObject)]
    #[thread_kind = MainThreadOnly]
    #[ivars = FileEvents]
    struct FileDelegate;

    unsafe impl NSObjectProtocol for FileDelegate {}

    unsafe impl NSApplicationDelegate for FileDelegate {
        #[unsafe(method(applicationShouldTerminate:))]
        fn should_terminate(&self, _application: &NSApplication) -> NSApplicationTerminateReply {
            let _ = self.ivars().sender.send(Event::Action(Action::Quit));
            NSApplicationTerminateReply::TerminateCancel
        }

        #[unsafe(method(application:openURLs:))]
        fn open_urls(&self, _application: &NSApplication, urls: &NSArray<NSURL>) {
            let paths = urls
                .iter()
                .filter(|url| url.isFileURL())
                .filter_map(|url| url.path())
                .map(|path| std::path::PathBuf::from(path.to_string()))
                .collect();
            let _ = self.ivars().sender.send(Event::Files(paths));
        }

        #[unsafe(method(applicationShouldHandleReopen:hasVisibleWindows:))]
        fn reopen(&self, _application: &NSApplication, _visible: bool) -> bool {
            let _ = self.ivars().sender.send(Event::Reopen);
            true
        }
    }
);

#[derive(Clone)]
struct Events(Arc<Mutex<mpsc::UnboundedReceiver<Event>>>);

impl Hash for Events {
    fn hash<H: Hasher>(&self, state: &mut H) {
        Arc::as_ptr(&self.0).hash(state);
    }
}

#[derive(Clone)]
pub struct Desktop {
    _menu: Menu,
    _delegate: Retained<FileDelegate>,
    events: Events,
}

impl Desktop {
    pub fn install() -> Result<Self, String> {
        let marker = MainThreadMarker::new().ok_or("Desktop must start on the main thread")?;
        let (sender, receiver) = mpsc::unbounded_channel();
        let application = NSApplication::sharedApplication(marker);
        let delegate: Retained<FileDelegate> = unsafe {
            let object = FileDelegate::alloc(marker).set_ivars(FileEvents {
                sender: sender.clone(),
            });
            msg_send![super(object), init]
        };
        application.setDelegate(Some(ProtocolObject::from_ref(&*delegate)));
        let menu = Menu::new();
        for (label, entries) in [
            (
                "NeoViolet",
                vec![
                    ("about", "About NeoViolet", None),
                    ("preferences", "Preferences…", Some(Code::Comma)),
                    ("quit", "Quit NeoViolet", Some(Code::KeyQ)),
                ],
            ),
            ("File", vec![("open", "Open File…", Some(Code::KeyO))]),
            (
                "Edit",
                vec![
                    ("copy", "Copy", Some(Code::KeyC)),
                    ("paste", "Paste", Some(Code::KeyV)),
                ],
            ),
            (
                "View",
                vec![
                    ("zoom_in", "Zoom In", Some(Code::Equal)),
                    ("zoom_out", "Zoom Out", Some(Code::Minus)),
                    ("zoom_reset", "Actual Size", Some(Code::Digit0)),
                    ("theme", "Toggle Light / Dark", None),
                ],
            ),
            ("Help", vec![("repository", "Project Repository", None)]),
        ] {
            let submenu = Submenu::new(label, true);
            for (id, title, key) in entries {
                let accelerator = key.map(|key| Accelerator::new(Some(Modifiers::SUPER), key));
                submenu
                    .append(&MenuItem::with_id(id, title, true, accelerator))
                    .map_err(|error| error.to_string())?;
            }
            if label == "NeoViolet" {
                submenu
                    .append(&PredefinedMenuItem::separator())
                    .map_err(|error| error.to_string())?;
                submenu
                    .append(&PredefinedMenuItem::hide(None))
                    .map_err(|error| error.to_string())?;
                submenu
                    .append(&PredefinedMenuItem::hide_others(None))
                    .map_err(|error| error.to_string())?;
            }
            menu.append(&submenu).map_err(|error| error.to_string())?;
        }
        menu.init_for_nsapp();
        MenuEvent::set_event_handler(Some(move |event: MenuEvent| {
            let action = match event.id.as_ref() {
                "about" => Action::About,
                "preferences" => Action::Preferences,
                "quit" => Action::Quit,
                "open" => Action::Open,
                "copy" => Action::Copy,
                "paste" => Action::Paste,
                "zoom_in" => Action::ZoomIn,
                "zoom_out" => Action::ZoomOut,
                "zoom_reset" => Action::ZoomReset,
                "theme" => Action::Theme,
                "repository" => Action::Repository,
                _ => return,
            };
            let _ = sender.send(Event::Action(action));
        }));
        Ok(Self {
            _menu: menu,
            _delegate: delegate,
            events: Events(Arc::new(Mutex::new(receiver))),
        })
    }

    pub fn subscription(&self) -> iced::Subscription<Event> {
        iced::Subscription::run_with(self.events.clone(), |events| {
            let events = events.clone();
            iced::stream::channel(16, async move |mut output| {
                let mut receiver = events.0.lock().await;
                while let Some(event) = receiver.recv().await {
                    if output.send(event).await.is_err() {
                        return;
                    }
                }
            })
        })
    }
}
