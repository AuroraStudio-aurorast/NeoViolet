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
