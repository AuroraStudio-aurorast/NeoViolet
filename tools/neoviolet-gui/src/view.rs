use iced::Element;
use iced::Length;
use iced::widget::{button, column, container, row, stack, text};
use iced_term::TerminalView;

use crate::app::{App, Message};

impl App {
    pub fn view(&self, id: iced::window::Id) -> Element<'_, Message> {
        if Some(id) == self.lyrics.toolbar_window {
            return self
                .lyrics
                .toolbar_view(&self.config.desktop_lyrics)
                .map(Message::Lyrics);
        }
        if Some(id) == self.lyrics.window {
            return self
                .lyrics
                .view(&self.config.desktop_lyrics)
                .map(Message::Lyrics);
        }
        if id == self.main_window {
            self.main_view()
        } else {
            text("").into()
        }
    }

    fn main_view(&self) -> Element<'_, Message> {
        let mut content = column![
            container(self.player_view())
                .width(Length::Fill)
                .height(Length::Fill)
        ]
        .height(Length::Fill);
        if self.running && !self.ipc_connected {
            let status = self.connection_error.as_deref().map_or(
                "Waiting for player. Complete setup in the terminal if prompted.",
                |error| {
                    if error.starts_with("Incompatible") {
                        error
                    } else {
                        "Player connection interrupted. Reconnecting…"
                    }
                },
            );
            content = content.push(container(text(status).size(12)).padding([6, 10]));
        } else if self.actions_available()
            && self.ipc_connected
            && self.handshake_supports_state()
            && self.playback.is_none()
        {
            content = content.push(
                row![
                    text("Waiting for playback status…").size(12),
                    button("Refresh").on_press(Message::RefreshState)
                ]
                .spacing(10)
                .padding([6, 10]),
            );
        }
        if let Some(notice) = &self.notice {
            content = content.push(
                row![
                    text(notice).size(12).width(Length::Fill),
                    button("Dismiss").on_press(Message::DismissNotice)
                ]
                .spacing(10)
                .padding([6, 10]),
            );
        }
        let content = self.with_titlebar(content.into());
        if let Some(dialog) = &self.dialog {
            return stack![
                content,
                crate::dialog::overlay(
                    dialog,
                    &self.version,
                    self.libopenmpt_version(),
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
                TerminalView::show_with_input(terminal, self.terminal_input_available())
                    .map(Message::Terminal)
            })
            .unwrap_or_else(|| text("Starting player…").into())
    }
}
