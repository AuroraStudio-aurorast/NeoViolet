use std::path::PathBuf;

use crate::actions::Action;

#[derive(Debug, Clone)]
#[cfg_attr(not(target_os = "macos"), allow(dead_code))]
pub enum Event {
    Action(Action),
    Files(Vec<PathBuf>),
    Reopen,
    #[cfg(any(target_os = "windows", test))]
    CaptionHover(iced::window::Id, Option<crate::titlebar::CaptionControl>),
    #[cfg(any(target_os = "windows", test))]
    CaptionAction(iced::window::Id, Action),
    #[cfg(any(target_os = "macos", test))]
    TitleInset(iced::window::Id, f32),
    #[cfg(target_os = "macos")]
    LyricsHover(iced::window::Id, bool),
    #[cfg(target_os = "macos")]
    LyricsReleased(iced::window::Id),
}

#[cfg(target_os = "macos")]
#[path = "desktop_macos.rs"]
mod macos;
#[cfg(target_os = "macos")]
pub use macos::Desktop;

#[cfg(target_os = "macos")]
#[path = "desktop_chrome_macos.rs"]
mod chrome;

#[cfg(target_os = "macos")]
#[path = "desktop_menu_macos.rs"]
mod menus;

#[cfg(target_os = "macos")]
#[path = "desktop_lyrics_macos.rs"]
mod lyrics;

#[cfg(target_os = "linux")]
#[path = "desktop_linux.rs"]
mod linux;
#[cfg(target_os = "windows")]
#[path = "desktop_windows.rs"]
mod windows;
#[cfg(target_os = "windows")]
pub use windows::Desktop;

impl Desktop {
    pub fn configure_main(&self, id: iced::window::Id, system_decorations: bool) -> iced::Task<()> {
        let _ = system_decorations;
        #[cfg(target_os = "macos")]
        return macos::configure_main(id, self.sender.clone());
        #[cfg(target_os = "windows")]
        return if system_decorations {
            iced::Task::none()
        } else {
            windows::configure(id, self.sender.clone())
        };
        #[cfg(not(any(target_os = "macos", target_os = "windows")))]
        {
            let _ = id;
            iced::Task::none()
        }
    }
}

pub fn chrome_input(id: iced::window::Id, menu: bool, fullscreen: bool) -> iced::Task<()> {
    #[cfg(target_os = "windows")]
    return windows::input_state(id, menu, fullscreen);
    #[cfg(not(target_os = "windows"))]
    {
        let _ = (id, menu, fullscreen);
        iced::Task::none()
    }
}

pub fn align_main_controls(id: iced::window::Id) -> iced::Task<()> {
    #[cfg(target_os = "macos")]
    return macos::align_main_controls(id);
    #[cfg(not(target_os = "macos"))]
    {
        let _ = id;
        iced::Task::none()
    }
}

pub fn title_double_click<T: Send + 'static>(id: iced::window::Id) -> iced::Task<T> {
    #[cfg(target_os = "macos")]
    return macos::title_double_click(id).discard();
    #[cfg(not(target_os = "macos"))]
    iced::window::toggle_maximize(id)
}

#[cfg(target_os = "macos")]
#[path = "desktop_hit_macos.rs"]
pub(crate) mod hit_test;

#[cfg(target_os = "macos")]
pub fn set_lyrics_locked(id: iced::window::Id, locked: bool) -> iced::Task<()> {
    iced::window::run(id, move |_| hit_test::set_locked(id, locked))
}

#[cfg(target_os = "macos")]
pub fn link_lyrics(
    id: iced::window::Id,
    toolbar: iced::window::Id,
    offset: f32,
) -> iced::Task<bool> {
    iced::window::run(toolbar, move |_| {
        hit_test::link(id, toolbar, f64::from(offset))
    })
}

#[cfg(target_os = "macos")]
pub fn resize_lyrics(id: iced::window::Id, size: iced::Size, offset: f32) -> iced::Task<()> {
    iced::window::run(id, move |_| hit_test::resize(id, size, f64::from(offset)))
}

#[cfg(target_os = "linux")]
pub fn link_lyrics(lyrics: iced::window::Id, toolbar: iced::window::Id) -> iced::Task<()> {
    linux::link_lyrics(lyrics, toolbar)
}

#[cfg(target_os = "macos")]
pub fn set_lyrics_mask(
    id: iced::window::Id,
    mask: crate::lyrics_hit_test::Mask,
    locked: bool,
) -> iced::Task<()> {
    iced::window::run(id, move |_| hit_test::set_mask(id, mask, locked))
}

#[cfg(target_os = "macos")]
pub fn close_lyrics(id: iced::window::Id) -> iced::Task<crate::lyrics_window::Event> {
    iced::window::run(id, move |_| hit_test::remove(id))
        .discard()
        .chain(iced::window::close(id))
}

#[cfg(not(target_os = "macos"))]
pub fn close_lyrics(id: iced::window::Id) -> iced::Task<crate::lyrics_window::Event> {
    iced::window::close(id)
}

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
#[derive(Clone)]
pub struct Desktop;

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
impl Desktop {
    pub fn sync_menus(&self, _basic: bool, _available: bool, _enabled: bool) {}
    pub fn configure_lyrics_window(&self, id: iced::window::Id) -> iced::Task<()> {
        #[cfg(target_os = "linux")]
        return linux::configure_lyrics(id);
        #[cfg(not(target_os = "linux"))]
        {
            let _ = id;
            iced::Task::none()
        }
    }
    pub fn install() -> Result<Self, String> {
        Ok(Self)
    }
    pub fn subscription(&self) -> iced::Subscription<Event> {
        iced::Subscription::none()
    }
}
