use std::cell::Cell;
use std::hash::{Hash, Hasher};
use std::sync::Arc;

use iced::futures::SinkExt;
use muda::accelerator::{Accelerator, Code, Modifiers};
use muda::{CheckMenuItem, ContextMenu, Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu};
use objc2::rc::Retained;
use objc2::runtime::ProtocolObject;
use objc2::{DefinedClass, MainThreadOnly, define_class, msg_send};
use objc2_app_kit::{NSApplication, NSApplicationDelegate, NSApplicationTerminateReply, NSWindow};
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
    pub(super) sender: mpsc::UnboundedSender<Event>,
    _menu: Menu,
    _delegate: Retained<FileDelegate>,
    events: Events,
    lyrics_item: CheckMenuItem,
    unlock_item: MenuItem,
    menus_ready: Cell<Option<bool>>,
}

impl Desktop {
    pub fn install() -> Result<Self, String> {
        let marker = MainThreadMarker::new().ok_or("Desktop must start on the main thread")?;
        // Native tabbing injects "Show Tab Bar" into the system View menu.
        NSWindow::setAllowsAutomaticWindowTabbing(false, marker);
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
        let app_menu = Submenu::new("NeoViolet", true);
        app_menu
            .append(&MenuItem::with_id("about", "About NeoViolet", true, None))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::separator())
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&MenuItem::with_id(
                "preferences",
                "Preferences…",
                true,
                Some(Accelerator::new(Some(Modifiers::SUPER), Code::Comma)),
            ))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::separator())
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::services(None))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::separator())
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::hide(None))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::hide_others(None))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::show_all(None))
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&PredefinedMenuItem::separator())
            .map_err(|error| error.to_string())?;
        app_menu
            .append(&MenuItem::with_id(
                "quit",
                "Quit NeoViolet",
                true,
                Some(Accelerator::new(Some(Modifiers::SUPER), Code::KeyQ)),
            ))
            .map_err(|error| error.to_string())?;
        menu.append(&app_menu).map_err(|error| error.to_string())?;
        let lyrics_item = CheckMenuItem::with_id(
            "desktop_lyrics",
            "Desktop Lyrics",
            true,
            false,
            Some(Accelerator::new(
                Some(Modifiers::SUPER | Modifiers::SHIFT),
                Code::KeyL,
            )),
        );
        let unlock_item = MenuItem::with_id("unlock_lyrics", "Unlock Desktop Lyrics", false, None);
        for (label, entries) in [
            (
                "File",
                vec![
                    ("open", "Open File…", Some(Code::KeyO)),
                    ("open_online_audio", "Open Online Audio…", None),
                ],
            ),
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
                    (
                        "desktop_lyrics",
                        "Show / Hide Desktop Lyrics",
                        Some(Code::KeyL),
                    ),
                    ("lyrics_settings", "Desktop Lyrics Settings…", None),
                    ("unlock_lyrics", "Unlock Desktop Lyrics", None),
                    ("reset_lyrics_position", "Reset Lyrics Position", None),
                ],
            ),
        ] {
            let submenu = Submenu::new(label, true);
            for (id, title, key) in entries {
                let accelerator = key.map(|key| {
                    Accelerator::new(
                        Some(if id == "desktop_lyrics" {
                            Modifiers::SUPER | Modifiers::SHIFT
                        } else {
                            Modifiers::SUPER
                        }),
                        key,
                    )
                });
                if id == "desktop_lyrics" {
                    submenu
                        .append(&PredefinedMenuItem::fullscreen(None))
                        .map_err(|error| error.to_string())?;
                    submenu
                        .append(&PredefinedMenuItem::separator())
                        .map_err(|error| error.to_string())?;
                    submenu
                        .append(&lyrics_item)
                        .map_err(|error| error.to_string())?;
                } else if id == "unlock_lyrics" {
                    submenu
                        .append(&unlock_item)
                        .map_err(|error| error.to_string())?;
                } else {
                    submenu
                        .append(&MenuItem::with_id(id, title, true, accelerator))
                        .map_err(|error| error.to_string())?;
                }
            }
            menu.append(&submenu).map_err(|error| error.to_string())?;
        }
        let window_menu = Submenu::new("Window", true);
        window_menu
            .append_items(&[
                &PredefinedMenuItem::minimize(None),
                &PredefinedMenuItem::maximize(None),
                &PredefinedMenuItem::separator(),
                &PredefinedMenuItem::bring_all_to_front(None),
            ])
            .map_err(|error| error.to_string())?;
        menu.append(&window_menu)
            .map_err(|error| error.to_string())?;
        let help_menu = Submenu::new("Help", true);
        help_menu
            .append(&MenuItem::with_id(
                "repository",
                "Project Repository",
                true,
                None,
            ))
            .map_err(|error| error.to_string())?;
        menu.append(&help_menu).map_err(|error| error.to_string())?;
        menu.init_for_nsapp();
        // Let AppKit maintain the window list and route standard window actions.
        window_menu.set_as_windows_menu_for_nsapp();
        let native_sender = sender.clone();
        MenuEvent::set_event_handler(Some(move |event: MenuEvent| {
            let action = match event.id.as_ref() {
                "about" => Action::About,
                "preferences" => Action::Preferences,
                "quit" => Action::Quit,
                "open" => Action::Open,
                "open_online_audio" => Action::OpenOnlineAudio,
                "copy" => Action::Copy,
                "paste" => Action::Paste,
                "zoom_in" => Action::ZoomIn,
                "zoom_out" => Action::ZoomOut,
                "zoom_reset" => Action::ZoomReset,
                "repository" => Action::Repository,
                "desktop_lyrics" => Action::DesktopLyrics,
                "lyrics_settings" => Action::LyricsSettings,
                "unlock_lyrics" => Action::UnlockLyrics,
                "reset_lyrics_position" => Action::ResetLyricsPosition,
                _ => return,
            };
            let _ = sender.send(Event::Action(action));
        }));
        let desktop = Self {
            sender: native_sender,
            _menu: menu,
            _delegate: delegate,
            events: Events(Arc::new(Mutex::new(receiver))),
            lyrics_item,
            unlock_item,
            menus_ready: Cell::new(None),
        };
        desktop.sync_menus(true, false, false);
        Ok(desktop)
    }

    pub fn sync_menus(&self, basic_available: bool, available: bool, enabled: bool) {
        if self.menus_ready.replace(Some(basic_available)) != Some(basic_available)
            || !basic_available
        {
            // SAFETY: muda owns this NSMenu for Desktop's lifetime. Updates run on
            // the Iced main thread, including all AppKit predefined commands.
            let menu = unsafe { &*self._menu.ns_menu().cast::<objc2_app_kit::NSMenu>() };
            super::menus::set_commands(menu, basic_available);
        }
        super::menus::set_player_commands(&self._menu, basic_available && available, enabled);
        self.lyrics_item.set_enabled(basic_available && available);
        if self.lyrics_item.is_checked() != enabled {
            self.lyrics_item.set_checked(enabled);
        }
        self.unlock_item
            .set_enabled(basic_available && available && enabled);
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

pub fn configure_main(
    id: iced::window::Id,
    sender: mpsc::UnboundedSender<Event>,
) -> iced::Task<()> {
    iced::window::run(id, move |handle| {
        use iced::window::raw_window_handle::RawWindowHandle;
        use objc2_app_kit::{
            NSAppearance, NSAppearanceCustomization, NSAppearanceNameDarkAqua, NSView,
        };
        let Some(marker) = MainThreadMarker::new() else {
            return;
        };
        let Ok(handle) = handle.window_handle() else {
            return;
        };
        let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
            return;
        };
        // SAFETY: The view is borrowed from the live Iced window on its owning thread.
        let view = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
        if let Some(window) = view.window() {
            // SAFETY: AppKit's exported appearance name is available on supported macOS versions.
            let appearance = NSAppearance::appearanceNamed(unsafe { NSAppearanceNameDarkAqua });
            window.setAppearance(appearance.as_deref());
            NSApplication::sharedApplication(marker).setAppearance(appearance.as_deref());
            super::chrome::install(id, view, &window, f64::from(crate::titlebar::HEIGHT));
            super::chrome::on_title_inset(id, move |inset| {
                let _ = sender.send(Event::TitleInset(id, inset));
            });
        }
    })
}

pub fn title_double_click(id: iced::window::Id) -> iced::Task<()> {
    iced::window::run(id, move |handle| {
        use iced::window::raw_window_handle::RawWindowHandle;
        use objc2_app_kit::NSView;
        use objc2_foundation::{NSUserDefaults, ns_string};
        let Some(_) = MainThreadMarker::new() else {
            return;
        };
        let Ok(handle) = handle.window_handle() else {
            return;
        };
        let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
            return;
        };
        // SAFETY: Iced keeps the native view alive for this main-thread callback.
        let view = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
        if let Some(window) = view.window() {
            let action = NSUserDefaults::standardUserDefaults()
                .stringForKey(ns_string!("AppleActionOnDoubleClick"))
                .map(|value| value.to_string())
                .unwrap_or_else(|| "Maximize".into());
            match action.as_str() {
                "Minimize" => window.miniaturize(None),
                "None" => {}
                _ => window.zoom(None),
            }
        }
    })
}

pub fn align_main_controls(id: iced::window::Id) -> iced::Task<()> {
    iced::window::run(id, move |handle| {
        use iced::window::raw_window_handle::RawWindowHandle;
        use objc2_app_kit::NSView;
        let Some(_) = MainThreadMarker::new() else {
            return;
        };
        let Ok(handle) = handle.window_handle() else {
            return;
        };
        let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
            return;
        };
        // SAFETY: Iced lends the live main-window view on its owning thread.
        let view = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
        if let Some(window) = view.window() {
            super::chrome::refresh(id, view, &window, f64::from(crate::titlebar::HEIGHT));
        }
    })
}

impl Desktop {
    pub fn configure_lyrics_window(&self, id: iced::window::Id) -> iced::Task<()> {
        let sender = self.sender.clone();
        iced::window::run(id, move |window| {
            use iced::window::raw_window_handle::RawWindowHandle;
            use objc2_app_kit::{NSView, NSWindowCollectionBehavior};

            let Some(_marker) = MainThreadMarker::new() else {
                return;
            };
            let Ok(handle) = window.window_handle() else {
                return;
            };
            let RawWindowHandle::AppKit(handle) = handle.as_raw() else {
                return;
            };
            // SAFETY: Iced invokes this callback on the window thread while the handle is alive.
            let view = unsafe { handle.ns_view.cast::<NSView>().as_ref() };
            if let Some(window) = view.window() {
                super::hit_test::install(id, &window, sender);
                window.setExcludedFromWindowsMenu(true);
                window.setHasShadow(false);
                window.setHidesOnDeactivate(false);
                window.setCollectionBehavior(
                    NSWindowCollectionBehavior::CanJoinAllSpaces
                        | NSWindowCollectionBehavior::FullScreenAuxiliary
                        | NSWindowCollectionBehavior::IgnoresCycle,
                );
            }
        })
    }
}
