//! Message routing and application lifecycle decisions.
use super::{App, Dialog, Message, online_audio_url_is_valid};
use crate::{config, desktop, ipc};
use iced::{Task, window};

impl App {
    pub(super) fn update_inner(&mut self, message: Message) -> Task<Message> {
        match message {
            Message::Ready => {}
            Message::FlushPending { session, epoch } => {
                if self.actions_available()
                    && !self.closing
                    && session == self.session
                    && epoch == self.connection_epoch
                    && self.dialog.is_none()
                    && self.connection.is_some()
                    && !self.pending.is_empty()
                {
                    let requests = self.pending.drain(..).map(ipc::Request::Open).collect();
                    return self.send_all(requests);
                }
            }
            Message::DismissNotice => self.notice = None,
            Message::RefreshState => {
                if self.actions_available() {
                    return self.send_all(vec![ipc::Request::GetState]);
                }
            }
            Message::RequestTimeout {
                session,
                epoch,
                id,
                accepted,
            } => {
                if session == self.session
                    && epoch == self.connection_epoch
                    && self.requests.get(&id) == Some(&accepted)
                {
                    self.requests.remove(&id);
                    self.notice = Some("The player did not confirm the action in time. Check playback before trying again.".into());
                }
            }
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
                if !self.basic_actions_available() {
                    return Task::none();
                }
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
            Message::SetupPicked {
                session,
                epoch,
                request_id,
                result,
            } => {
                return self.setup_picked(session, epoch, request_id, result);
            }
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
                if self.basic_actions_available()
                    && self.dialog.is_none()
                    && self.running
                    && let Some(terminal) = &mut self.terminal
                {
                    terminal.paste(text);
                }
            }
            Message::Pasted(None) => {}
            Message::OnlineAudioURLChanged(value) => {
                if let Some(Dialog::OpenOnlineAudio(url)) = &mut self.dialog {
                    *url = value;
                }
            }
            Message::SubmitOnlineAudio => {
                let Some(Dialog::OpenOnlineAudio(url)) = self.dialog.take() else {
                    return Task::none();
                };
                let url = url.trim().to_owned();
                if !online_audio_url_is_valid(&url) {
                    self.dialog = Some(Dialog::OpenOnlineAudio(url));
                    self.notice = Some("Enter a valid HTTP or HTTPS audio URL.".into());
                    return self.reveal_main();
                }
                self.notice = None;
                return self.open(std::path::PathBuf::from(url));
            }
            Message::Sent(Err(error)) => {
                self.dialog = Some(Dialog::Error(error));
                return self.reveal_main();
            }
            Message::Sent(Ok(())) => {}
            Message::LifecycleSent {
                session,
                epoch,
                result,
            } => {
                if session == self.session
                    && epoch == self.connection_epoch
                    && let Err(error) = result
                {
                    if self.closing {
                        return self.finish_quit();
                    }
                    self.notice = Some(error);
                }
            }
            Message::Dismiss => {
                if self.running {
                    self.dialog = None;
                    let requests = if self.actions_available() && self.connection.is_some() {
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
                if self.closing {
                    return Task::none();
                }
                self.closing = true;
                self.dialog = None;
                self.pending.clear();
                self.requests.clear();
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
