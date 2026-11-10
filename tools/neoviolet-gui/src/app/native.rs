//! Main and auxiliary window input; modal and menu event isolation.
use super::{App, Message};
use crate::actions::Action;
use crate::{lyrics_window, titlebar};
use iced::keyboard::{Key, key::Named};
use iced::{Task, window};

impl App {
    pub(super) fn native_event(&mut self, id: window::Id, event: iced::Event) -> Task<Message> {
        if id != self.main_window {
            return self.secondary_event(id, event);
        }
        match event {
            iced::Event::Window(event) => match event {
                window::Event::CloseRequested => return self.window_action(Action::Quit),
                window::Event::FileDropped(path) => return self.open(path),
                window::Event::Unfocused => {
                    self.titlebar.close();
                    self.titlebar.update(titlebar::Message::Hover(false));
                    self.titlebar.caption_hover = None;
                }
                window::Event::Focused => {
                    return Task::batch([self.refresh_chrome(), self.focus()]);
                }
                window::Event::Resized(size) => {
                    self.titlebar.size = size;
                    self.config.window_width = size.width.round() as u32;
                    self.config.window_height = size.height.round() as u32;
                    return self.refresh_chrome();
                }
                _ => {}
            },
            iced::Event::Keyboard(event) => match event {
                iced::keyboard::Event::ModifiersChanged(modifiers) => self.modifiers = modifiers,
                iced::keyboard::Event::KeyPressed {
                    key,
                    modifiers,
                    repeat,
                    ..
                } => {
                    if let Some(dialog) = &self.dialog {
                        if key == Key::Named(Named::Escape) {
                            return self.update(Message::Dismiss);
                        }
                        if !modifiers.command() && !modifiers.alt() && !modifiers.control() {
                            if repeat && matches!(key, Key::Named(Named::Enter | Named::Space)) {
                                return Task::none();
                            }
                            if let Some(message) = self.dialog_navigation.key(
                                dialog,
                                self.running,
                                &key,
                                modifiers.shift(),
                            ) {
                                return self.update(message);
                            }
                        }
                        return Task::none();
                    }
                    if let Some(action) = Action::shortcut(&key, modifiers) {
                        return self.action(action);
                    }
                    if self.titlebar.visible() {
                        if let Some(action) =
                            self.titlebar.key(&key, self.config.desktop_lyrics.enabled)
                        {
                            return self.action(action);
                        }
                        return Task::none();
                    }
                    if key == Key::Named(Named::F10)
                        && !cfg!(target_os = "macos")
                        && modifiers.is_empty()
                        && self.dialog.is_none()
                    {
                        return self.update(Message::Titlebar(titlebar::Message::ToggleMenu));
                    }
                    if key == Key::Named(Named::F11)
                        && !cfg!(target_os = "macos")
                        && modifiers.is_empty()
                    {
                        return self.action(Action::Fullscreen);
                    }
                    if key == Key::Named(Named::Escape) && self.dialog.is_some() {
                        return self.update(Message::Dismiss);
                    }
                    if key == Key::Named(Named::Escape) && self.titlebar.fullscreen {
                        return self.action(Action::Fullscreen);
                    }
                }
                _ => {}
            },
            iced::Event::Mouse(iced::mouse::Event::ButtonPressed(_)) => {
                self.dialog_navigation.reset()
            }
            iced::Event::Mouse(iced::mouse::Event::CursorMoved { position }) => {
                self.titlebar.cursor = position;
                if self.titlebar.take_drag(position) {
                    return window::drag(self.main_window);
                }
            }
            iced::Event::Mouse(iced::mouse::Event::CursorLeft) => {
                self.titlebar.update(titlebar::Message::Hover(false));
            }
            iced::Event::Mouse(iced::mouse::Event::ButtonReleased(iced::mouse::Button::Left)) => {
                self.titlebar.update(titlebar::Message::ReleaseTitle);
                return self.focus();
            }
            iced::Event::Mouse(iced::mouse::Event::WheelScrolled { delta })
                if self.config.zoom_via_scroll && self.modifiers.command() =>
            {
                let y = match delta {
                    iced::mouse::ScrollDelta::Lines { y, .. }
                    | iced::mouse::ScrollDelta::Pixels { y, .. } => y,
                };
                if y != 0.0 {
                    return self.action(if y > 0.0 {
                        Action::ZoomIn
                    } else {
                        Action::ZoomOut
                    });
                }
            }
            _ => {}
        }
        Task::none()
    }

    fn secondary_event(&mut self, id: window::Id, event: iced::Event) -> Task<Message> {
        // Linux window managers can consume the release after taking the drag grab.
        #[cfg(target_os = "linux")]
        if self.lyrics.contains(id)
            && matches!(
                &event,
                iced::Event::Mouse(iced::mouse::Event::CursorEntered)
            )
        {
            return self.update(Message::Lyrics(lyrics_window::Event::DragEnded));
        }
        if self.lyrics.contains(id)
            && matches!(
                &event,
                iced::Event::Mouse(iced::mouse::Event::ButtonReleased(
                    iced::mouse::Button::Left,
                ))
            )
        {
            return self.update(Message::Lyrics(lyrics_window::Event::DragEnded));
        }
        if self.lyrics.contains(id)
            && matches!(
                &event,
                iced::Event::Window(window::Event::Resized(_))
                    | iced::Event::Mouse(
                        iced::mouse::Event::WheelScrolled { .. }
                            | iced::mouse::Event::CursorMoved { .. }
                            | iced::mouse::Event::ButtonReleased(_)
                            | iced::mouse::Event::ButtonPressed(_)
                    )
            )
        {
            return self.lyrics.refresh_mask().map(Message::Lyrics);
        }
        if let iced::Event::Window(window::Event::Moved(point)) = &event {
            return self
                .lyrics
                .moved(id, *point, &mut self.config.desktop_lyrics)
                .map(Message::Lyrics);
        }
        if matches!(event, iced::Event::Window(window::Event::CloseRequested))
            && self.lyrics.contains(id)
        {
            return self.update(Message::Lyrics(lyrics_window::Event::Visible(false)));
        }
        if let iced::Event::Keyboard(iced::keyboard::Event::KeyPressed { key, modifiers, .. }) =
            event
        {
            if matches!(Action::shortcut(&key, modifiers), Some(Action::Quit)) {
                return self.action(Action::Quit);
            }
            if matches!(
                Action::shortcut(&key, modifiers),
                Some(Action::DesktopLyrics)
            ) {
                return self.action(Action::DesktopLyrics);
            }
        }
        Task::none()
    }
}
