//! Message routing and application lifecycle decisions.
use super::{App, Dialog, Message};
use crate::{config, desktop, ipc};
use iced::{Task, window};

impl App {
    pub(super) fn update_inner(&mut self, message: Message) -> Task<Message> {
        match message {
            Message::Ready => {}
            Message::MainOpened => {
                return Task::batch([
                    self.desktop.as_ref().map_or_else(Task::none, |desktop| {
                        desktop
                            .configure_main(self.main_window, self.config.system_decorations)
                            .map(|_| Message::MainConfigured)
                    }),
                    self.refresh_chrome(),
                ]);
            }
            Message::MainConfigured => {
                return desktop::chrome_input(
                    self.main_window,
                    self.titlebar.visible() || self.dialog.is_some(),
                    self.titlebar.fullscreen,
                )
                .map(|_| Message::Ready);
            }
            Message::SetMode(mode) => {
                return window::set_mode(self.main_window, mode).chain(self.refresh_chrome());
            }
            Message::ChromeState(maximized, mode) => {
                self.titlebar.maximized = maximized;
                self.titlebar.fullscreen = mode == window::Mode::Fullscreen;
                return desktop::align_main_controls(self.main_window).map(|_| Message::Ready);
            }
            Message::Lyrics(event) => return self.lyrics_event(event),
            Message::Titlebar(event) => return self.titlebar_event(event),
            Message::Ipc(session, event) => return self.ipc_event(session, event),
            Message::Terminal(event) => return self.terminal_event(event),
            Message::Desktop(event) => return self.desktop_event(event),
            Message::Native(id, event) => return self.native_event(id, event),
            Message::Picked(path) => {
                let open = path.map_or_else(Task::none, |path| self.open(path));
                return Task::batch([
                    open,
                    window::gain_focus(self.main_window),
                    self.focus(),
                    desktop::align_main_controls(self.main_window).map(|_| Message::Ready),
                ]);
            }
            Message::Pasted(Some(text)) => {
                if self.dialog.is_none()
                    && self.running
                    && let Some(terminal) = &mut self.terminal
                {
                    terminal.paste(text);
                }
            }
            Message::Pasted(None) => {}
            Message::Version(version) => self.version = version,
            Message::Sent(Err(error)) => {
                self.dialog = Some(Dialog::Error(error));
                return self.reveal_main();
            }
            Message::Sent(Ok(())) => {}
            Message::Dismiss => {
                if self.running {
                    self.dialog = None;
                    let requests = if self.connection.is_some() {
                        self.pending.drain(..).map(ipc::Request::Open).collect()
                    } else {
                        Vec::new()
                    };
                    return Task::batch([self.focus(), self.send_all(requests)]);
                } else if matches!(self.dialog, Some(Dialog::About)) {
                    self.dialog = Some(Dialog::Error(
                        "The player is not running. Restart it or close this window.".into(),
                    ));
                }
            }
            Message::ConfirmQuit => {
                self.closing = true;
                if self.connection.is_some() {
                    return Task::batch([
                        self.send_all(vec![ipc::Request::Quit]),
                        Task::perform(
                            async { tokio::time::sleep(std::time::Duration::from_secs(2)).await },
                            |_| Message::QuitDeadline,
                        ),
                    ]);
                }
                return self.finish_quit();
            }
            Message::QuitDeadline => return self.finish_quit(),
            Message::Restart => {
                let (config, warning) = config::Config::load();
                return self.restart_with_config(config, warning);
            }
        }
        Task::none()
    }
}
