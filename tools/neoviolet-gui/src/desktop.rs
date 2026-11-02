use std::path::PathBuf;

use crate::actions::Action;

#[derive(Debug, Clone)]
#[cfg_attr(not(target_os = "macos"), allow(dead_code))]
pub enum Event {
    Action(Action),
    Files(Vec<PathBuf>),
    Reopen,
}

#[cfg(target_os = "macos")]
#[path = "desktop_macos.rs"]
mod macos;
#[cfg(target_os = "macos")]
pub use macos::Desktop;

#[cfg(not(target_os = "macos"))]
#[derive(Clone)]
pub struct Desktop;

#[cfg(not(target_os = "macos"))]
impl Desktop {
    pub fn install() -> Result<Self, String> {
        Ok(Self)
    }
    pub fn subscription(&self) -> iced::Subscription<Event> {
        iced::Subscription::none()
    }
}
