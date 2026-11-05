//! GUI state, subscriptions and shared update invariants.
mod bootstrap;
mod commands;
mod desktop_events;
mod integration;
mod native;
mod player;
mod startup;
#[cfg(test)]
mod tests;
mod update;

#[cfg(test)]
mod online_audio_tests;

use crate::{config, desktop, dialog, ipc, lyrics_window, titlebar};
use iced::keyboard::Modifiers;
use iced::{Subscription, Task, window};
use iced_term::Terminal;
use std::collections::VecDeque;
use std::path::PathBuf;
use tokio::sync::mpsc;

pub(crate) const TERMINAL_BACKGROUND: iced::Color = iced::Color::BLACK;

pub(super) fn online_audio_url_is_valid(value: &str) -> bool {
    let value = value.trim();
    let Some((scheme, authority)) = value.split_once("://") else {
        return false;
    };
    if !(scheme.eq_ignore_ascii_case("http") || scheme.eq_ignore_ascii_case("https")) {
        return false;
    }
    let authority = authority.split(['/', '?', '#']).next().unwrap_or_default();
    !authority.is_empty() && !authority.chars().any(char::is_whitespace)
}

pub struct App {
    pub main_window: window::Id,
    pub lyrics: lyrics_window::Overlay,
    pub titlebar: titlebar::State,
    pub config: config::Config,
    pub terminal: Option<Terminal>,
    pub running: bool,
    pub dialog: Option<Dialog>,
    pub dialog_navigation: dialog::Navigation,
    pub version: String,
    pub window_title: String,
    pub desktop: Option<desktop::Desktop>,
    pub ipc_connected: bool,
    pub playback: Option<ipc::PlaybackState>,
    pub notice: Option<String>,
    pub connection_error: Option<String>,
    startup: startup::Stage,
    lyrics_restored: bool,
    picker: Option<startup::Picker>,
    handshake: ipc::Handshake,
    connection_epoch: u64,
    request_sequence: u64,
    requests: std::collections::HashMap<String, bool>,
    session: u64,
    pid: Option<u32>,
    connection: Option<mpsc::Sender<ipc::Command>>,
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
    OpenOnlineAudio(String),
}

#[derive(Debug, Clone)]
pub enum Message {
    Terminal(iced_term::Event),
    Ipc(u64, ipc::Event),
    Desktop(desktop::Event),
    Native(window::Id, iced::Event),
    Lyrics(lyrics_window::Event),
    Titlebar(titlebar::Message),
    Ready,
    MainOpened,
    MainConfigured,
    ChromeState(bool, window::Mode),
    SetMode(window::Mode),
    Picked(Option<PathBuf>),
    SetupPicked {
        session: u64,
        epoch: u64,
        request_id: String,
        result: Result<Option<PathBuf>, String>,
    },
    Pasted(Option<String>),
    OnlineAudioURLChanged(String),
    SubmitOnlineAudio,
    Sent(Result<(), String>),
    LifecycleSent {
        session: u64,
        epoch: u64,
        result: Result<(), String>,
    },
    Dismiss,
    ConfirmQuit,
    Restart,
    QuitDeadline,
    DismissNotice,
    RefreshState,
    FlushPending {
        session: u64,
        epoch: u64,
    },
    RequestTimeout {
        session: u64,
        epoch: u64,
        id: String,
        accepted: bool,
    },
}

impl App {
    pub fn subscription(&self) -> Subscription<Message> {
        let mut subscriptions = vec![iced::event::listen_with(|event, _, id| {
            matches!(
                event,
                iced::Event::Window(_)
                    | iced::Event::Keyboard(_)
                    | iced::Event::Mouse(
                        iced::mouse::Event::WheelScrolled { .. }
                            | iced::mouse::Event::CursorEntered
                            | iced::mouse::Event::CursorLeft
                            | iced::mouse::Event::CursorMoved { .. }
                            | iced::mouse::Event::ButtonReleased(_)
                            | iced::mouse::Event::ButtonPressed(_)
                    )
            )
            .then_some(Message::Native(id, event))
        })];
        if self.titlebar.chrome_animating() {
            subscriptions
                .push(window::frames().map(|_| Message::Titlebar(titlebar::Message::Animate)));
        }
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
        let menu = self.titlebar.visible();
        let modal = self.dialog.as_ref().map(std::mem::discriminant);
        let running = self.running;
        let fullscreen = self.titlebar.fullscreen;
        let mut task = self.update_inner(message);
        self.titlebar.commands_disabled = !self.basic_actions_available();
        self.titlebar.player_unavailable = !self.actions_available();
        if modal != self.dialog.as_ref().map(std::mem::discriminant) || running != self.running {
            self.dialog_navigation.reset();
        }
        if self.dialog.is_some() {
            self.titlebar.close();
        }
        self.titlebar.animate_chrome(iced::time::Instant::now());
        if menu && !self.titlebar.visible() && self.dialog.is_none() {
            task = task.chain(self.focus());
        }
        if let Some(desktop) = &self.desktop {
            desktop.sync_menus(
                self.basic_actions_available(),
                self.actions_available(),
                self.config.desktop_lyrics.enabled,
            );
        }
        if (menu || modal.is_some()) != (self.titlebar.visible() || self.dialog.is_some())
            || fullscreen != self.titlebar.fullscreen
        {
            return desktop::chrome_input(
                self.main_window,
                self.titlebar.visible() || self.dialog.is_some(),
                self.titlebar.fullscreen,
            )
            .discard()
            .chain(task);
        }
        task
    }
}
