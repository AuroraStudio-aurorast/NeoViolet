use iced::Element;
use iced::widget::{button, column, container, row, slider, stack, text, tooltip};
use iced::{Color, Length};
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
        let mut content = column![
            container(self.player_view())
                .width(Length::Fill)
                .height(Length::Fill)
        ]
        .height(Length::Fill);
        if let Some(controls) = self.playback_controls() {
            content = content.push(controls);
        }
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
        } else if self.ipc_connected && self.handshake_supports_state() && self.playback.is_none() {
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

    fn playback_controls(&self) -> Option<Element<'_, Message>> {
        let state = self.playback.as_ref()?;
        let can_control = self.controls_available();
        let playback = button(if state.playing { "Pause" } else { "Play" })
            .on_press_maybe(can_control.then_some(Message::PlaybackToggle));
        let timeline: Element<'_, Message> = if state.has_track && state.duration > 0.0 {
            slider(
                0.0..=state.duration,
                self.seek_preview
                    .unwrap_or(state.position)
                    .min(state.duration),
                Message::SeekPreview,
            )
            .step(0.1)
            .on_release(Message::SeekCommit)
            .into()
        } else {
            text(if state.loading {
                "Opening file…"
            } else {
                "Open an audio file to play"
            })
            .size(12)
            .width(Length::Fill)
            .into()
        };
        let volume = slider(
            0.0..=1.0,
            self.volume_preview.unwrap_or(state.volume),
            Message::VolumePreview,
        )
        .step(0.01)
        .on_release(Message::VolumeCommit)
        .width(70);
        let details = format!(
            "{}\n{}\n{}\n{}\nSettings: {}\nDesktop lyrics: {}",
            state.title,
            state.artist,
            state.album,
            state.path,
            state.config_dir.display(),
            if state.desktop_lyrics { "On" } else { "Off" }
        );
        let clock = format!("{} / {}", clock(state.position), clock(state.duration));
        Some(
            container(
                row![
                    playback,
                    timeline,
                    tooltip(
                        text(clock).size(11),
                        text(details).size(12),
                        tooltip::Position::Top
                    ),
                    text("Vol").size(11),
                    volume
                ]
                .spacing(10)
                .align_y(iced::Center),
            )
            .padding([6, 10])
            .style(|_| container::Style {
                background: Some(Color::from_rgb8(37, 37, 40).into()),
                ..Default::default()
            })
            .into(),
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

fn clock(seconds: f64) -> String {
    let seconds = seconds.max(0.0) as u64;
    format!("{}:{:02}", seconds / 60, seconds % 60)
}
