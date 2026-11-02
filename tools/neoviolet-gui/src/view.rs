use iced::widget::{button, column, container, row, text};
use iced::{Element, Length};
use iced_term::TerminalView;

use crate::actions::Action;
use crate::app::{App, Dialog, Message};

impl App {
    pub fn view(&self) -> Element<'_, Message> {
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
                        button("Cancel").on_press(Message::Dismiss),
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
            return container(
                column![text(heading).size(24), text(body), controls.spacing(12)].spacing(20),
            )
            .padding(32)
            .center_x(Length::Fill)
            .center_y(Length::Fill)
            .style(container::rounded_box)
            .into();
        }
        let terminal = self
            .terminal
            .as_ref()
            .map(|terminal| TerminalView::show(terminal).map(Message::Terminal))
            .unwrap_or_else(|| text("Starting player…").into());
        // macOS uses the system menu bar; other desktops get ordinary window content controls.
        if cfg!(target_os = "macos") && self.desktop.is_some() {
            return terminal;
        }
        column![
            container(
                row![
                    button("Open").on_press(Message::Action(Action::Open)),
                    button("Settings").on_press(Message::Action(Action::Preferences)),
                    button("Copy").on_press(Message::Action(Action::Copy)),
                    button("Paste").on_press(Message::Action(Action::Paste)),
                    button("−").on_press(Message::Action(Action::ZoomOut)),
                    button("+").on_press(Message::Action(Action::ZoomIn)),
                    button("Theme").on_press(Message::Action(Action::Theme)),
                    button("About").on_press(Message::Action(Action::About)),
                ]
                .spacing(6)
            )
            .padding(8),
            terminal,
        ]
        .height(Length::Fill)
        .into()
    }
}
