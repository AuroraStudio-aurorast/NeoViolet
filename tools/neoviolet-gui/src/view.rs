use iced::widget::{button, column, container, opaque, row, scrollable, stack, text};
use iced::{Border, Color, Element, Length, Shadow, Vector};
use iced_term::TerminalView;

use crate::app::{App, Dialog, Message};

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
        let content = self.player_view();
        if let Some(dialog) = &self.dialog {
            let (heading, body, controls) = match dialog {
                Dialog::About => (
                    "NeoViolet",
                    format!(
                        "GUI {} · MIT License\n{}\nIced + iced_term",
                        env!("CARGO_PKG_VERSION"),
                        self.version
                    ),
                    row![button("Close").on_press(Message::Dismiss)],
                ),
                Dialog::Quit => (
                    "Close NeoViolet?",
                    "Playback will stop when the player closes.".into(),
                    row![
                        button("Cancel")
                            .on_press(Message::Dismiss)
                            .style(button::secondary),
                        button("Close player").on_press(Message::ConfirmQuit)
                    ],
                ),
                Dialog::Error(error) => (
                    "NeoViolet",
                    error.clone(),
                    if self.running {
                        row![button("Continue").on_press(Message::Dismiss)]
                    } else {
                        row![
                            button("Restart").on_press(Message::Restart),
                            button("Close").on_press(Message::ConfirmQuit)
                        ]
                    },
                ),
            };
            let panel = container(
                column![
                    text(heading).size(20),
                    container(scrollable(text(body).size(14)).height(Length::Shrink))
                        .width(Length::Fill)
                        .max_height(120),
                    container(controls.spacing(8))
                        .width(Length::Fill)
                        .align_x(iced::Right),
                ]
                .spacing(20),
            )
            .padding(24)
            .width(Length::Fill)
            .max_width(420)
            .max_height(280)
            .style(|theme: &iced::Theme| {
                let palette = theme.extended_palette();
                container::Style {
                    background: Some(palette.background.weak.color.into()),
                    text_color: Some(palette.background.base.text),
                    border: Border {
                        color: palette.background.strong.color,
                        width: 1.0,
                        radius: 12.0.into(),
                    },
                    shadow: Shadow {
                        color: Color::BLACK.scale_alpha(0.35),
                        offset: Vector::new(0.0, 8.0),
                        blur_radius: 24.0,
                    },
                    ..Default::default()
                }
            });
            let modal = container(panel)
                .padding(24)
                .center_x(Length::Fill)
                .center_y(Length::Fill)
                .style(|_| container::Style {
                    background: Some(Color::BLACK.scale_alpha(0.45).into()),
                    ..Default::default()
                });
            let content = self.with_titlebar(content);
            return stack![content, opaque(modal)].into();
        }
        self.with_titlebar(content)
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
