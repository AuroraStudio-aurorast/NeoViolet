//! Application bootstrap, player launch and configuration reload.
use super::{App, Dialog, Message};
use crate::{config, desktop, dialog, launch, lyrics_window, titlebar};
use iced::keyboard::Modifiers;
use iced::{Font, Task, Theme, window};
use iced_term::{Terminal, settings};
use std::collections::VecDeque;

impl App {
    pub fn boot(
        config: config::Config,
        warning: Option<String>,
        desktop: Result<desktop::Desktop, String>,
    ) -> (Self, Task<Message>) {
        let save_settings = warning.is_none();
        let warning = warning.or_else(|| desktop.as_ref().err().cloned());
        let family = Box::leak(config.monospace_font.clone().into_boxed_str());
        #[cfg(any(target_os = "macos", target_os = "windows"))]
        let window_decorations = true;
        #[cfg(not(any(target_os = "macos", target_os = "windows")))]
        let window_decorations = config.system_decorations;
        let (main_window, open_main) = window::open(window::Settings {
            size: iced::Size::new(config.window_width as f32, config.window_height as f32),
            min_size: Some(iced::Size::new(480.0, 320.0)),
            transparent: config.opacity < 1.0,
            decorations: window_decorations,
            #[cfg(target_os = "macos")]
            platform_specific: window::settings::PlatformSpecific {
                title_hidden: true,
                titlebar_transparent: true,
                fullsize_content_view: true,
            },
            exit_on_close_request: false,
            ..Default::default()
        });
        let mut app = Self {
            main_window,
            lyrics: lyrics_window::Overlay::default(),
            titlebar: titlebar::State::new(iced::Size::new(
                config.window_width as f32,
                config.window_height as f32,
            )),
            config,
            terminal: None,
            running: false,
            dialog: warning.map(Dialog::Error),
            dialog_navigation: dialog::Navigation::default(),
            version: "Loading…".into(),
            window_title: "NeoViolet".into(),
            desktop: desktop.ok(),
            ipc_connected: false,
            playback: None,
            notice: None,
            connection_error: None,
            seek_preview: None,
            volume_preview: None,
            handshake: Default::default(),
            connection_epoch: 0,
            request_sequence: 0,
            requests: Default::default(),
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
        app.titlebar.configure(&app.config);
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
                .map(|_| Message::MainOpened)
                .chain(Task::batch([start, version])),
        )
    }

    pub(super) fn start(&mut self) -> Task<Message> {
        self.terminal = None;
        self.connection = None;
        self.pid = None;
        self.ipc_connected = false;
        self.playback = None;
        self.notice = None;
        self.connection_error = None;
        self.handshake = Default::default();
        self.requests.clear();
        self.seek_preview = None;
        self.volume_preview = None;
        self.lyrics.playback = None;
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
        Theme::custom(
            "NeoViolet",
            iced::theme::Palette {
                background: iced::Color::from_rgb8(32, 32, 32),
                ..Theme::Dark.palette()
            },
        )
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

    pub(super) fn font(&self) -> settings::FontSettings {
        settings::FontSettings {
            size: self.config.font_size as f32,
            font_type: Font::with_name(self.family),
            ..Default::default()
        }
    }

    pub(super) fn terminal_theme(&self) -> settings::ThemeSettings {
        let palette = iced_term::ColorPalette {
            background: "#202020".into(),
            ..Default::default()
        };
        settings::ThemeSettings::new(Box::new(palette))
    }

    pub(super) fn restart_with_config(
        &mut self,
        config: config::Config,
        warning: Option<String>,
    ) -> Task<Message> {
        if self.config.monospace_font != config.monospace_font {
            self.family = Box::leak(config.monospace_font.clone().into_boxed_str());
        }
        self.config = config;
        self.save_settings = warning.is_none();
        self.dialog = warning.map(Dialog::Error);
        self.arguments = launch::arguments(std::iter::empty());
        self.pending.clear();
        self.window_title = "NeoViolet".into();
        self.start()
    }
}
