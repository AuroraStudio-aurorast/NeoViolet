use iced::Element;
use iced::widget::{stack, text};
use iced_term::TerminalView;

use crate::app::{App, Message};

impl App {
    pub fn view(&self, id: iced::window::Id) -> Element<'_, Message> {
        if Some(id) == self.lyrics.window {
            return self
                .lyrics
                .view(&self.config.desktop_lyrics)
                .map(Message::Lyrics);
        }
        if Some(id) == self.lyrics.settings_window {
            return self
                .lyrics
                .settings_view(&self.config.desktop_lyrics)
                .map(Message::Lyrics);
        }
        if id == self.main_window {
            self.main_view()
        } else {
            text("").into()
        }
    }

    fn main_view(&self) -> Element<'_, Message> {
        let content = self.with_titlebar(self.player_view());
        if let Some(dialog) = &self.dialog {
            return stack![
                content,
                crate::dialog::overlay(
                    dialog,
                    &self.version,
                    self.running,
                    &self.dialog_navigation
                )
            ]
            .into();
        }
        content
    }

    fn with_titlebar<'a>(&'a self, content: Element<'a, Message>) -> Element<'a, Message> {
        self.titlebar.wrap(
            content,
            &self.window_title,
            self.config.desktop_lyrics.enabled,
        )
    }

    fn player_view(&self) -> Element<'_, Message> {
        self.terminal
            .as_ref()
            .map(|terminal| {
                TerminalView::show_with_input(
                    terminal,
                    self.dialog.is_none() && !self.titlebar.visible(),
                )
                .map(Message::Terminal)
            })
            .unwrap_or_else(|| text("Starting player…").into())
    }
}
