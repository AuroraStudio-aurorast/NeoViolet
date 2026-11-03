use std::collections::VecDeque;
use std::path::PathBuf;

use iced::keyboard::{Key, Modifiers, key::Named};
use iced::{Font, Subscription, Task, Theme, window};
use iced_term::{BackendCommand, Terminal, TerminalView, actions, settings};
use tokio::sync::mpsc;

use crate::actions::Action;
use crate::{config, desktop, ipc, launch, lyrics_window};

#[cfg(test)]
#[path = "app_tests.rs"]
mod tests;

pub struct App {
    pub main_window: window::Id,
    pub lyrics: lyrics_window::Overlay,
    pub config: config::Config,
    pub terminal: Option<Terminal>,
    pub running: bool,
    pub dialog: Option<Dialog>,
    pub version: String,
    window_title: String,
    pub desktop: Option<desktop::Desktop>,
    pub ipc_connected: bool,
    session: u64,
    pid: Option<u32>,
    connection: Option<mpsc::Sender<ipc::Request>>,
    pending: VecDeque<PathBuf>,
    arguments: Vec<String>,
    family: &'static str,
    modifiers: Modifiers,
    exit_code: Option<i32>,
    closing: bool,
    save_settings: bool,
    environment: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone)]
pub enum Dialog {
    About,
    Quit,
    Error(String),
}

#[derive(Debug, Clone)]
pub enum Message {
    Terminal(iced_term::Event),
    Ipc(u64, ipc::Event),
    Desktop(desktop::Event),
    Native(window::Id, iced::Event),
    Lyrics(lyrics_window::Event),
    Ready,
    Action(Action),
    Picked(Option<PathBuf>),
    Pasted(Option<String>),
    Sent(Result<(), String>),
    Version(String),
    Dismiss,
    ConfirmQuit,
    Restart,
    QuitDeadline,
}

impl App {
    pub fn boot(
        config: config::Config,
        warning: Option<String>,
        desktop: Result<desktop::Desktop, String>,
    ) -> (Self, Task<Message>) {
        let save_settings = warning.is_none();
        let warning = warning.or_else(|| desktop.as_ref().err().cloned());
        let family = Box::leak(config.monospace_font.clone().into_boxed_str());
        let (main_window, open_main) = window::open(window::Settings {
            size: iced::Size::new(config.window_width as f32, config.window_height as f32),
            min_size: Some(iced::Size::new(480.0, 320.0)),
            transparent: config.opacity < 1.0,
            exit_on_close_request: false,
            ..Default::default()
        });
        let mut app = Self {
            main_window,
            lyrics: lyrics_window::Overlay::default(),
            config,
            terminal: None,
            running: false,
            dialog: warning.map(Dialog::Error),
            version: "Loading…".into(),
            window_title: "NeoViolet".into(),
            desktop: desktop.ok(),
            ipc_connected: false,
            session: 0,
            pid: None,
            connection: None,
            pending: VecDeque::new(),
            arguments: launch::arguments(std::env::args().skip(1)),
            family,
            modifiers: Modifiers::empty(),
            exit_code: None,
            closing: false,
            save_settings,
            environment: launch::environment(),
        };
        let start = app.start();
        let executable = launch::player(app.config.neoviolet_path.as_deref());
        let version = Task::perform(
            async move {
                tokio::task::spawn_blocking(move || launch::version(&executable))
                    .await
                    .unwrap_or_else(|_| "unavailable".into())
            },
            Message::Version,
        );
        (
            app,
            open_main
                .map(|_| Message::Ready)
                .chain(Task::batch([start, version])),
        )
    }

    fn start(&mut self) -> Task<Message> {
        self.terminal = None;
        self.connection = None;
        self.pid = None;
        self.ipc_connected = false;
        self.exit_code = None;
        self.closing = false;
        self.lyrics.clear();
        self.session += 1;
        let executable = launch::player(self.config.neoviolet_path.as_deref());
        let mut environment = self.environment.clone();
        environment.extend([
            ("TERM".into(), "xterm-256color".into()),
            ("COLORTERM".into(), "truecolor".into()),
            ("TERM_PROGRAM".into(), "neoviolet-gui".into()),
            ("NEOVIOLET_FONT".into(), self.config.monospace_font.clone()),
        ]);
        match Terminal::new(
            self.session,
            settings::Settings {
                backend: settings::BackendSettings {
                    program: executable.to_string_lossy().into_owned(),
                    args: self.arguments.clone(),
                    env: environment,
                    working_directory: std::env::current_dir().ok(),
                },
                font: self.font(),
                theme: self.terminal_theme(),
            },
        ) {
            Ok(mut terminal) => {
                self.pid = terminal.child_process_id();
                terminal.set_opacity(self.config.opacity);
                terminal.set_scroll_zoom(self.config.zoom_via_scroll);
                terminal.set_application_shortcuts(true);
                self.terminal = Some(terminal);
                self.running = true;
                let (lyrics, _) = self.lyrics.update(
                    lyrics_window::Event::Visible(self.config.desktop_lyrics.enabled),
                    &mut self.config.desktop_lyrics,
                );
                Task::batch([self.focus(), lyrics.map(Message::Lyrics)])
            }
            Err(error) => {
                self.running = false;
                self.dialog = Some(Dialog::Error(format!(
                    "Could not start {}: {error}\nCheck neoviolet_path in the GUI settings.",
                    executable.display()
                )));
                Task::none()
            }
        }
    }

    pub fn theme(&self) -> Theme {
        if self.config.theme == "light" {
            Theme::Light
        } else {
            Theme::custom(
                "NeoViolet",
                iced::theme::Palette {
                    background: iced::Color::from_rgb8(32, 32, 32),
                    ..Theme::Dark.palette()
                },
            )
        }
    }

    pub fn title(&self) -> String {
        self.window_title.clone()
    }

    pub fn title_for_window(&self, id: window::Id) -> String {
        if id == self.main_window {
            self.title()
        } else if Some(id) == self.lyrics.settings_window {
            "Desktop Lyrics Settings".into()
        } else {
            "NeoViolet Lyrics".into()
        }
    }

    fn font(&self) -> settings::FontSettings {
        settings::FontSettings {
            size: self.config.font_size as f32,
            font_type: Font::with_name(self.family),
            ..Default::default()
        }
    }

    fn terminal_theme(&self) -> settings::ThemeSettings {
        let mut palette = iced_term::ColorPalette::default();
        if self.config.theme == "light" {
            palette.background = "#eeeeee".into();
            palette.foreground = "#202020".into();
        } else {
            palette.background = "#202020".into();
        }
        settings::ThemeSettings::new(Box::new(palette))
    }

    pub fn subscription(&self) -> Subscription<Message> {
        let mut subscriptions = vec![iced::event::listen_with(|event, _, id| {
            matches!(
                event,
                iced::Event::Window(_)
                    | iced::Event::Keyboard(_)
                    | iced::Event::Mouse(iced::mouse::Event::WheelScrolled { .. })
            )
            .then_some(Message::Native(id, event))
        })];
        if self.running {
            if let Some(terminal) = &self.terminal {
                subscriptions.push(terminal.subscription().map(Message::Terminal));
            }
            if let Some(pid) = self.pid {
                subscriptions.push(
                    ipc::subscription(self.session, pid)
                        .with(self.session)
                        .map(|(session, event)| Message::Ipc(session, event)),
                );
            }
        }
        if let Some(desktop) = &self.desktop {
            subscriptions.push(desktop.subscription().map(Message::Desktop));
        }
        Subscription::batch(subscriptions)
    }

    pub fn update(&mut self, message: Message) -> Task<Message> {
        match message {
            Message::Ready => {}
            Message::Lyrics(event) => {
                let opened = matches!(&event, lyrics_window::Event::Opened(id) if Some(*id) == self.lyrics.window);
                let closed_settings = matches!(&event, lyrics_window::Event::CloseSettings)
                    && self.lyrics.settings_window.is_some();
                let native_setup = if opened {
                    self.desktop.as_ref().map_or_else(Task::none, |desktop| {
                        desktop
                            .configure_lyrics_window(self.lyrics.window.unwrap())
                            .map(|_| Message::Ready)
                    })
                } else {
                    Task::none()
                };
                let (task, request) = self.lyrics.update(event, &mut self.config.desktop_lyrics);
                return Task::batch([
                    native_setup.chain(task.map(Message::Lyrics)),
                    self.send_all(request.into_iter().collect()),
                    if opened {
                        window::gain_focus(self.lyrics.settings_window.unwrap_or(self.main_window))
                    } else if closed_settings {
                        window::gain_focus(self.main_window)
                    } else {
                        Task::none()
                    },
                ]);
            }
            #[cfg(target_os = "macos")]
            Message::Desktop(desktop::Event::LyricsHover(id, hovered)) => {
                if self.lyrics.window == Some(id) {
                    return self.update(Message::Lyrics(lyrics_window::Event::Hover(hovered)));
                }
            }
            Message::Native(id, event) if id != self.main_window => {
                if self.lyrics.window == Some(id)
                    && matches!(
                        &event,
                        iced::Event::Window(window::Event::Resized(_))
                            | iced::Event::Mouse(iced::mouse::Event::WheelScrolled { .. })
                    )
                {
                    return self.lyrics.refresh_mask().map(Message::Lyrics);
                }
                if let iced::Event::Window(window::Event::Moved(point)) = &event {
                    self.lyrics
                        .moved(id, *point, &mut self.config.desktop_lyrics);
                }
                if matches!(event, iced::Event::Window(window::Event::CloseRequested)) {
                    if Some(id) == self.lyrics.window {
                        return self.update(Message::Lyrics(lyrics_window::Event::Visible(false)));
                    }
                    if Some(id) == self.lyrics.settings_window {
                        return self.update(Message::Lyrics(lyrics_window::Event::CloseSettings));
                    }
                }
                if let iced::Event::Keyboard(iced::keyboard::Event::KeyPressed {
                    key,
                    modifiers,
                    ..
                }) = event
                {
                    if key == Key::Named(Named::Escape) && Some(id) == self.lyrics.settings_window {
                        return self.update(Message::Lyrics(lyrics_window::Event::CloseSettings));
                    }
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
            }
            Message::Ipc(session, _) if session != self.session => return Task::none(),
            Message::Terminal(iced_term::Event::BackendCall(id, _)) if id != self.session => {
                return Task::none();
            }
            Message::Terminal(iced_term::Event::BackendCall(_, command)) => {
                if let BackendCommand::ProcessAlacrittyEvent(
                    iced_term::AlacrittyEvent::ChildExit(code),
                ) = &command
                {
                    self.exit_code = Some(*code);
                }
                if matches!(
                    &command,
                    BackendCommand::ProcessAlacrittyEvent(iced_term::AlacrittyEvent::ResetTitle)
                ) {
                    self.window_title = "NeoViolet".into();
                }
                if let Some(terminal) = &mut self.terminal {
                    match terminal.handle(iced_term::Command::ProxyToBackend(command)) {
                        actions::Action::Shutdown => {
                            self.running = false;
                            self.connection = None;
                            self.ipc_connected = false;
                            self.lyrics.clear();
                            if self.closing {
                                return self.finish_quit();
                            }
                            self.dialog = Some(Dialog::Error(match self.exit_code {
                                Some(0) => {
                                    "The player exited. You can restart it or close this window."
                                        .into()
                                }
                                Some(code) => format!(
                                    "The player exited with status {code}. Check the launch arguments and player settings."
                                ),
                                None => {
                                    "The player stopped. You can restart it or close this window."
                                        .into()
                                }
                            }));
                            return Task::batch([
                                self.reveal_main(),
                                self.lyrics.refresh_mask().map(Message::Lyrics),
                            ]);
                        }
                        actions::Action::ChangeTitle(title) => {
                            self.window_title = if title.trim().is_empty() {
                                "NeoViolet".into()
                            } else {
                                title
                            };
                        }
                        _ => {}
                    }
                }
            }
            Message::Ipc(_, ipc::Event::Connected(sender)) => {
                self.connection = Some(sender);
                self.ipc_connected = true;
                eprintln!(
                    "NeoViolet GUI: IPC authenticated (player PID {})",
                    self.pid.unwrap_or_default()
                );
                let mut requests = vec![ipc::Request::DesktopLyrics(
                    self.config.desktop_lyrics.enabled,
                )];
                if self.dialog.is_none() {
                    requests.extend(self.pending.drain(..).map(ipc::Request::Open));
                }
                return self.send_all(requests);
            }
            Message::Ipc(_, ipc::Event::Received(ipc::Incoming::Lyrics(frame))) => {
                if self.config.desktop_lyrics.enabled {
                    let changed = self.lyrics.frame.active_lines() != frame.active_lines()
                        || self.lyrics.frame.next_line() != frame.next_line()
                        || self.lyrics.frame.title != frame.title
                        || self.lyrics.frame.artist != frame.artist;
                    self.lyrics.frame = frame;
                    return Task::batch([
                        self.lyrics.fit(&self.config.desktop_lyrics),
                        if changed {
                            self.lyrics.refresh_mask()
                        } else {
                            Task::none()
                        },
                    ])
                    .map(Message::Lyrics);
                }
            }
            Message::Ipc(_, ipc::Event::Received(ipc::Incoming::DesktopLyrics { enable })) => {
                let (task, _) = self.lyrics.update(
                    lyrics_window::Event::Visible(enable),
                    &mut self.config.desktop_lyrics,
                );
                return task.map(Message::Lyrics);
            }
            Message::Ipc(_, ipc::Event::Received(ipc::Incoming::Quit { dialog })) => {
                if dialog {
                    self.dialog = Some(Dialog::Quit);
                    return self.reveal_main();
                } else {
                    self.closing = true;
                    return self.finish_quit();
                }
            }
            Message::Ipc(_, ipc::Event::Disconnected(error)) => {
                self.connection = None;
                self.ipc_connected = false;
                self.lyrics.clear();
                if !self.closing && self.running {
                    eprintln!("NeoViolet GUI: {error}");
                }
                return self.lyrics.refresh_mask().map(Message::Lyrics);
            }
            Message::Ipc(_, ipc::Event::Received(_)) => {}
            Message::Desktop(desktop::Event::Action(action)) | Message::Action(action) => {
                return self.action(action);
            }
            Message::Desktop(desktop::Event::Files(paths)) => {
                if let Some(path) = paths.into_iter().next() {
                    return self.open(path);
                }
            }
            Message::Desktop(desktop::Event::Reopen) => {
                return self.reveal_main();
            }
            Message::Native(_, iced::Event::Window(event)) => match event {
                window::Event::CloseRequested => return self.action(Action::Quit),
                window::Event::FileDropped(path) => return self.open(path),
                window::Event::Resized(size) => {
                    self.config.window_width = size.width.round() as u32;
                    self.config.window_height = size.height.round() as u32;
                }
                _ => {}
            },
            Message::Native(_, iced::Event::Keyboard(event)) => match event {
                iced::keyboard::Event::ModifiersChanged(modifiers) => self.modifiers = modifiers,
                iced::keyboard::Event::KeyPressed { key, modifiers, .. } => {
                    if key == Key::Named(Named::Escape) && self.dialog.is_some() {
                        return self.update(Message::Dismiss);
                    }
                    if let Some(action) = Action::shortcut(&key, modifiers) {
                        return self.action(action);
                    }
                }
                _ => {}
            },
            Message::Native(_, iced::Event::Mouse(iced::mouse::Event::WheelScrolled { delta })) => {
                if self.config.zoom_via_scroll && self.modifiers.command() {
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
            }
            Message::Native(_, _) => {}
            Message::Picked(Some(path)) => return self.open(path),
            Message::Picked(None) => return self.focus(),
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
                if self.config.monospace_font != config.monospace_font {
                    self.family = Box::leak(config.monospace_font.clone().into_boxed_str());
                }
                self.config = config;
                self.save_settings = warning.is_none();
                self.dialog = warning.map(Dialog::Error);
                return self.start();
            }
        }
        Task::none()
    }

    fn action(&mut self, action: Action) -> Task<Message> {
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
                return Task::perform(
                    async {
                        rfd::AsyncFileDialog::new()
                            .set_title("Open Audio File")
                            .pick_file()
                            .await
                            .map(|file| file.path().to_path_buf())
                    },
                    Message::Picked,
                );
            }
            Action::Preferences => {
                return Task::perform(
                    async {
                        open::that_detached(config::directory()).map_err(|error| error.to_string())
                    },
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
                if !self.running {
                    return self.finish_quit();
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
            Action::Theme => {
                self.config.theme = if self.config.theme == "light" {
                    "dark"
                } else {
                    "light"
                }
                .into();
                let theme = self.terminal_theme();
                if let Some(terminal) = &mut self.terminal {
                    terminal.handle(iced_term::Command::ChangeTheme(theme.color_pallete));
                }
            }
        }
        Task::none()
    }

    fn open(&mut self, path: PathBuf) -> Task<Message> {
        if !self.running {
            self.pending.push_back(path);
            self.dialog = None;
            return self.start();
        }
        if self.dialog.is_some() {
            self.pending.push_back(path);
            return Task::none();
        }
        if self.connection.is_some() {
            return self.send_all(vec![ipc::Request::Open(path)]);
        }
        self.pending.push_back(path);
        Task::none()
    }

    fn send_all(&self, requests: Vec<ipc::Request>) -> Task<Message> {
        let Some(sender) = self.connection.clone() else {
            return Task::none();
        };
        Task::perform(
            async move {
                for request in requests {
                    sender
                        .send(request)
                        .await
                        .map_err(|_| "Player IPC is unavailable".to_string())?;
                }
                Ok(())
            },
            Message::Sent,
        )
    }

    fn finish_quit(&mut self) -> Task<Message> {
        if self.save_settings
            && let Err(error) = self.config.save()
        {
            eprintln!("NeoViolet GUI: settings were not saved: {error}");
        }
        self.terminal = None;
        iced::exit()
    }

    fn focus(&self) -> Task<Message> {
        if self.dialog.is_none()
            && let Some(terminal) = &self.terminal
        {
            return TerminalView::focus(terminal.widget_id().clone());
        }
        Task::none()
    }

    fn reveal_main(&self) -> Task<Message> {
        window::minimize(self.main_window, false).chain(window::gain_focus(self.main_window))
    }
}
