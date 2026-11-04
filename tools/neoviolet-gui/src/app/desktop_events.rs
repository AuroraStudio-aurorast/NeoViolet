//! Platform menu actions and desktop overlay window coordination.
use super::{App, Message};
use crate::{desktop, lyrics_window, titlebar};
use iced::{Task, window};

impl App {
    pub(super) fn desktop_event(&mut self, event: desktop::Event) -> Task<Message> {
        match event {
            #[cfg(any(target_os = "windows", test))]
            desktop::Event::CaptionAction(id, action) => {
                if id == self.main_window
                    && self.dialog.is_none()
                    && !self.titlebar.visible()
                    && !self.titlebar.fullscreen
                {
                    return self.action(action);
                }
            }
            #[cfg(any(target_os = "windows", test))]
            desktop::Event::CaptionHover(id, control) => {
                if id == self.main_window {
                    self.titlebar.caption_hover = control;
                }
            }
            #[cfg(target_os = "macos")]
            desktop::Event::LyricsHover(id, hovered) => {
                if self.lyrics.window == Some(id) {
                    return self.update(Message::Lyrics(lyrics_window::Event::Hover(hovered)));
                }
                if self.lyrics.toolbar_window == Some(id) {
                    return self
                        .update(Message::Lyrics(lyrics_window::Event::ToolbarHover(hovered)));
                }
            }
            #[cfg(target_os = "macos")]
            desktop::Event::LyricsReleased(id) => {
                if self.lyrics.contains(id) {
                    return self.update(Message::Lyrics(lyrics_window::Event::DragEnded));
                }
            }
            desktop::Event::Action(action) => {
                return self.action(action);
            }
            desktop::Event::Files(paths) => {
                if let Some(path) = paths.into_iter().next() {
                    return self.open(path);
                }
            }
            desktop::Event::Reopen => {
                return self.reveal_main();
            }
        }
        Task::none()
    }

    pub(super) fn lyrics_event(&mut self, event: lyrics_window::Event) -> Task<Message> {
        let opened = match &event {
            lyrics_window::Event::Opened(id) if self.lyrics.contains(*id) => Some(*id),
            _ => None,
        };
        let closed_settings = matches!(&event, lyrics_window::Event::CloseSettings)
            && self.lyrics.settings_window.is_some();
        let native_setup = if let Some(id) = opened {
            self.desktop.as_ref().map_or_else(Task::none, |desktop| {
                desktop.configure_lyrics_window(id).map(|_| Message::Ready)
            })
        } else {
            Task::none()
        };
        let (task, request) = self.lyrics.update(event, &mut self.config.desktop_lyrics);
        Task::batch([
            native_setup.chain(task.map(Message::Lyrics)),
            self.send_all(request.into_iter().collect()),
            if opened.is_some() {
                window::gain_focus(self.lyrics.settings_window.unwrap_or(self.main_window))
            } else if closed_settings {
                window::gain_focus(self.main_window)
            } else {
                Task::none()
            },
        ])
    }

    pub(super) fn titlebar_event(&mut self, event: titlebar::Message) -> Task<Message> {
        match event {
            titlebar::Message::DoubleClick => {
                self.titlebar.update(titlebar::Message::DoubleClick);
                desktop::title_double_click(self.main_window).chain(self.refresh_chrome())
            }
            titlebar::Message::Resize(direction) => {
                window::drag_resize(self.main_window, direction)
            }
            event => {
                if let Some(action) = self.titlebar.update(event) {
                    return self.action(action);
                }
                Task::none()
            }
        }
    }
}
