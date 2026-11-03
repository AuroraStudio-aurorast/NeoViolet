use std::path::PathBuf;

use crate::actions::Action;

#[derive(Debug, Clone)]
#[cfg_attr(not(target_os = "macos"), allow(dead_code))]
pub enum Event {
    Action(Action),
    Files(Vec<PathBuf>),
    Reopen,
    #[cfg(target_os = "macos")]
    LyricsHover(iced::window::Id, bool),
}

#[cfg(target_os = "macos")]
#[path = "desktop_macos.rs"]
mod macos;
#[cfg(target_os = "macos")]
pub use macos::Desktop;

#[cfg(target_os = "windows")]
#[path = "desktop_windows.rs"]
mod windows;

pub fn configure_main(id: iced::window::Id, system_decorations: bool) -> iced::Task<()> {
    let _ = system_decorations;
    #[cfg(target_os = "macos")]
    return macos::configure_main(id);
    #[cfg(target_os = "windows")]
    return if system_decorations {
        iced::Task::none()
    } else {
        windows::configure(id)
    };
    #[cfg(not(any(target_os = "macos", target_os = "windows")))]
    {
        let _ = id;
        iced::Task::none()
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

#[cfg(not(target_os = "macos"))]
#[derive(Clone)]
pub struct Desktop;

#[cfg(not(target_os = "macos"))]
impl Desktop {
    pub fn sync_lyrics(&self, _enabled: bool) {}
    pub fn configure_lyrics_window(&self, _id: iced::window::Id) -> iced::Task<()> {
        iced::Task::none()
    }
    pub fn install() -> Result<Self, String> {
        Ok(Self)
    }
    pub fn subscription(&self) -> iced::Subscription<Event> {
        iced::Subscription::none()
    }
}
