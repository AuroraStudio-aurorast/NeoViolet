#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]

use iced::widget::{container, text};
use iced::{Element, Length, Subscription, Task, Theme};
use iced_term::{Terminal, TerminalView, actions::Action, settings};

fn main() -> iced::Result {
    iced::application(App::boot, App::update, App::view)
        .title("NeoViolet")
        .theme(Theme::Dark)
        .window_size((1100.0, 720.0))
        .subscription(App::subscription)
        .run()
}

struct App {
    terminal: Result<Terminal, String>,
}

#[derive(Debug, Clone)]
enum Message {
    Terminal(iced_term::Event),
}

impl App {
    fn boot() -> (Self, Task<Message>) {
        #[cfg(unix)]
        let program = std::env::var("SHELL").unwrap_or_else(|_| "/bin/sh".into());
        #[cfg(windows)]
        let program = std::env::var("COMSPEC").unwrap_or_else(|_| "cmd.exe".into());
        let terminal = Terminal::new(
            1,
            settings::Settings {
                backend: settings::BackendSettings {
                    program,
                    env: std::collections::HashMap::from([
                        ("TERM".into(), "xterm-256color".into()),
                        ("COLORTERM".into(), "truecolor".into()),
                    ]),
                    ..Default::default()
                },
                ..Default::default()
            },
        )
        .map_err(|error| format!("Could not start terminal: {error}"));
        let focus = terminal
            .as_ref()
            .map(|terminal| TerminalView::focus(terminal.widget_id().clone()))
            .unwrap_or_else(|_| Task::none());
        (Self { terminal }, focus)
    }

    fn subscription(&self) -> Subscription<Message> {
        self.terminal
            .as_ref()
            .map(|terminal| terminal.subscription().map(Message::Terminal))
            .unwrap_or_else(|_| Subscription::none())
    }

    fn update(&mut self, message: Message) -> Task<Message> {
        match message {
            Message::Terminal(iced_term::Event::BackendCall(_, command)) => {
                if let Ok(terminal) = &mut self.terminal
                    && matches!(
                        terminal.handle(iced_term::Command::ProxyToBackend(command)),
                        Action::Shutdown
                    )
                {
                    return iced::exit();
                }
            }
        }
        Task::none()
    }

    fn view(&self) -> Element<'_, Message> {
        let content = match &self.terminal {
            Ok(terminal) => TerminalView::show(terminal).map(Message::Terminal),
            Err(error) => text(error).into(),
        };
        container(content)
            .width(Length::Fill)
            .height(Length::Fill)
            .into()
    }
}
