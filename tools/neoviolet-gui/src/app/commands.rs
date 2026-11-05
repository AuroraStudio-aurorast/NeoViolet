//! Application commands, file selection, shutdown and focus effects.
use super::{App, Dialog, Message};
use crate::actions::Action;
use crate::{config, ipc, lyrics_window};
use iced::{Task, window};
use iced_term::TerminalView;
use std::path::PathBuf;

impl App {
    pub(super) fn action(&mut self, action: Action) -> Task<Message> {
        if !self.basic_actions_available()
            || (action.requires_player() && !self.actions_available())
        {
            return Task::none();
        }
        self.dispatch_action(action)
    }

    pub(super) fn window_action(&mut self, action: Action) -> Task<Message> {
        if !matches!(action, Action::Quit | Action::Minimize | Action::ZoomWindow) {
            return Task::none();
        }
        self.dispatch_action(action)
    }

    fn dispatch_action(&mut self, action: Action) -> Task<Message> {
        self.titlebar.close();
        match action {
            Action::DesktopLyrics => {
                return self.update(Message::Lyrics(lyrics_window::Event::Visible(
                    !self.config.desktop_lyrics.enabled,
                )));
            }
            Action::LyricsSettings => {
                return self.update(Message::Lyrics(lyrics_window::Event::Settings));
            }
            Action::UnlockLyrics => {
                return self.update(Message::Lyrics(lyrics_window::Event::Locked(false)));
            }
            Action::Open => {
                return self.reveal_main().chain(
                    window::run(self.main_window, |parent| {
                        rfd::AsyncFileDialog::new()
                            .set_title("Open Audio File")
                            .set_parent(&parent)
                            .pick_file()
                    })
                    .then(|picker| {
                        Task::perform(
                            async move { picker.await.map(|file| file.path().to_path_buf()) },
                            Message::Picked,
                        )
                    }),
                );
            }
            Action::OpenOnlineAudio => {
                self.dialog = Some(Dialog::OpenOnlineAudio(String::new()));
                return self.reveal_main();
            }
            Action::Preferences => {
                let directory = self
                    .playback
                    .as_ref()
                    .map(|state| state.config_dir.clone())
                    .filter(|path| !path.as_os_str().is_empty())
                    .unwrap_or_else(config::directory);
                return Task::perform(
                    async move { open::that_detached(directory).map_err(|error| error.to_string()) },
                    Message::Sent,
                );
            }
            Action::Repository => {
                return Task::perform(
                    async {
                        open::that_detached("https://github.com/AuroraStudio-aurorast/NeoViolet")
                            .map_err(|error| error.to_string())
                    },
                    Message::Sent,
                );
            }
            Action::About => {
                self.dialog = Some(Dialog::About);
                return self.reveal_main();
            }
            Action::Quit => {
                if self.closing {
                    return Task::none();
                }
                if !self.running {
                    return self.finish_quit();
                }
                if self.ipc_connected && self.handshake.supports("request_quit") {
                    return self.send_all(vec![ipc::Request::CheckQuit]);
                }
                self.dialog = Some(Dialog::Quit);
                return self.reveal_main();
            }
            Action::Copy => {
                if self.dialog.is_none()
                    && let Some(terminal) = &self.terminal
                {
                    return iced::clipboard::write(terminal.selection());
                }
            }
            Action::Paste => return iced::clipboard::read().map(Message::Pasted),
            Action::ZoomIn | Action::ZoomOut | Action::ZoomReset => {
                self.config.font_size = match action {
                    Action::ZoomIn => (self.config.font_size + 2).min(48),
                    Action::ZoomOut => self.config.font_size.saturating_sub(2).max(8),
                    _ => 14,
                };
                let font = self.font();
                if let Some(terminal) = &mut self.terminal {
                    terminal.handle(iced_term::Command::ChangeFont(font));
                }
                return self.focus();
            }
            Action::Fullscreen => {
                return window::mode(self.main_window).map(|mode| {
                    Message::SetMode(if mode == window::Mode::Fullscreen {
                        window::Mode::Windowed
                    } else {
                        window::Mode::Fullscreen
                    })
                });
            }
            Action::Minimize => return window::minimize(self.main_window, true),
            Action::ZoomWindow => {
                return window::toggle_maximize(self.main_window).chain(self.refresh_chrome());
            }
        }
        Task::none()
    }

    pub(super) fn open(&mut self, path: PathBuf) -> Task<Message> {
        if self.closing || !self.actions_available() {
            return Task::none();
        }
        if !self.running {
            self.pending.push_back(path);
            self.dialog = None;
            return self.start();
        }
        if self.dialog.is_some() {
            self.pending.push_back(path);
            return Task::none();
        }
        self.pending.push_back(path);
        if self.connection.is_some() {
            let requests = self.pending.drain(..).map(ipc::Request::Open).collect();
            return self.send_all(requests);
        }
        Task::none()
    }

    pub(super) fn finish_quit(&mut self) -> Task<Message> {
        if self.save_settings
            && let Err(error) = self.config.save()
        {
            eprintln!("NeoViolet GUI: settings were not saved: {error}");
        }
        self.terminal = None;
        iced::exit()
    }

    pub(super) fn focus(&self) -> Task<Message> {
        if self.dialog.is_none()
            && !self.titlebar.visible()
            && let Some(terminal) = &self.terminal
        {
            return TerminalView::focus(terminal.widget_id().clone());
        }
        Task::none()
    }

    pub(super) fn refresh_chrome(&self) -> Task<Message> {
        let id = self.main_window;
        window::is_maximized(id).then(move |maximized| {
            window::mode(id).map(move |mode| Message::ChromeState(maximized, mode))
        })
    }

    pub(super) fn reveal_main(&self) -> Task<Message> {
        window::minimize(self.main_window, false).chain(window::gain_focus(self.main_window))
    }
}
