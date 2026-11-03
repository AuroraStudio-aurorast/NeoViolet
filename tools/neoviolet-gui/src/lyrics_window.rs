//! Native lyrics and settings windows; playback remains in the TUI.
use iced::widget::{
    button, checkbox, column, container, mouse_area, rich_text, row, scrollable, slider, space,
    span, stack, text, tooltip,
};
use iced::{Color, Element, Font, Length, Point, Size, Task, window};

use crate::ipc::Request;
use crate::lyrics::{Frame, Highlight, Position, Preferences};

#[derive(Debug, Clone)]
pub enum Event {
    Opened(window::Id),
    SettingsOpened(window::Id),
    Ready,
    Visible(bool),
    Settings,
    CloseSettings,
    ResetPosition,
    Hover(bool),
    Drag,
    PlayPause,
    Locked(bool),
    FontSize(f32),
    Opacity(f32),
    Highlight(Highlight),
    AdditionalLines(bool),
    NextLine(bool),
}

#[derive(Default)]
pub struct Overlay {
    pub frame: Frame,
    pub window: Option<window::Id>,
    pub settings_window: Option<window::Id>,
    hovered: bool,
}

impl Overlay {
    pub fn show(&mut self, preferences: &Preferences) -> Task<Event> {
        if !preferences.enabled {
            return Task::none();
        }
        if self.window.is_some() {
            return self.passthrough(preferences.locked);
        }
        let position = preferences.position.map_or(
            window::Position::SpecificWith(|window, monitor| {
                Point::new(
                    ((monitor.width - window.width) / 2.0).max(0.0),
                    (monitor.height * 0.65 - window.height / 2.0).max(0.0),
                )
            }),
            |point| window::Position::Specific(Point::new(point.x, point.y)),
        );
        let (id, open) = window::open(window::Settings {
            size: Size::new(880.0, 240.0),
            position,
            decorations: false,
            transparent: true,
            resizable: false,
            minimizable: false,
            closeable: false,
            level: window::Level::AlwaysOnTop,
            exit_on_close_request: false,
            ..Default::default()
        });
        self.window = Some(id);
        open.map(Event::Opened)
    }

    pub fn clear(&mut self) {
        self.frame = Frame::default();
    }

    pub fn moved(&self, id: window::Id, point: Point, preferences: &mut Preferences) {
        if self.window == Some(id) && point.x.is_finite() && point.y.is_finite() {
            preferences.position = Some(Position {
                x: point.x,
                y: point.y,
            });
        }
    }

    pub fn update(
        &mut self,
        event: Event,
        preferences: &mut Preferences,
    ) -> (Task<Event>, Option<Request>) {
        let task = match event {
            Event::Opened(id) if self.window == Some(id) => {
                return (
                    Task::batch([
                        crate::desktop::configure_lyrics_window(id).map(|_| Event::Ready),
                        self.passthrough(preferences.locked),
                    ]),
                    None,
                );
            }
            Event::SettingsOpened(id) if self.settings_window == Some(id) => Task::none(),
            Event::Opened(id) | Event::SettingsOpened(id) => window::close(id),
            Event::Visible(enable) => {
                preferences.enabled = enable;
                self.hovered = false;
                let task = if enable {
                    self.show(preferences)
                } else {
                    self.clear();
                    self.window.take().map_or_else(Task::none, window::close)
                };
                return (task, Some(Request::DesktopLyrics(enable)));
            }
            Event::Settings => {
                if let Some(id) = self.settings_window {
                    window::gain_focus(id)
                } else {
                    let (id, open) = window::open(window::Settings {
                        size: Size::new(320.0, 420.0),
                        position: window::Position::Centered,
                        resizable: false,
                        minimizable: false,
                        exit_on_close_request: false,
                        level: window::Level::AlwaysOnTop,
                        ..Default::default()
                    });
                    self.settings_window = Some(id);
                    open.map(Event::SettingsOpened)
                }
            }
            Event::CloseSettings => self
                .settings_window
                .take()
                .map_or_else(Task::none, window::close),
            Event::ResetPosition => {
                preferences.position = None;
                let close = self.window.take().map_or_else(Task::none, window::close);
                Task::batch([close, self.show(preferences)])
            }
            Event::Hover(hovered) => {
                self.hovered = hovered;
                Task::none()
            }
            Event::Drag if !preferences.locked => self.window.map_or_else(Task::none, window::drag),
            Event::Locked(locked) => {
                preferences.locked = locked;
                self.hovered = false;
                self.passthrough(locked)
            }
            Event::PlayPause => return (Task::none(), Some(Request::PlayPause)),
            Event::FontSize(size) => {
                preferences.font_size = size.round() as u32;
                Task::none()
            }
            Event::Opacity(opacity) => {
                preferences.opacity = opacity;
                Task::none()
            }
            Event::Highlight(highlight) => {
                preferences.highlight = highlight;
                Task::none()
            }
            Event::AdditionalLines(enable) => {
                preferences.additional_lines = enable;
                Task::none()
            }
            Event::NextLine(enable) => {
                preferences.next_line = enable;
                Task::none()
            }
            Event::Ready | Event::Drag => Task::none(),
        };
        preferences.normalize();
        (task, None)
    }

    fn passthrough(&self, locked: bool) -> Task<Event> {
        self.window.map_or_else(Task::none, |id| {
            if locked {
                window::enable_mouse_passthrough(id)
            } else {
                window::disable_mouse_passthrough(id)
            }
        })
    }

    pub fn view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let alpha = preferences.opacity;
        let sung = match preferences.highlight {
            Highlight::Violet => Color::from_rgb8(220, 205, 255),
            Highlight::White => Color::from_rgb8(245, 243, 250),
        }
        .scale_alpha(alpha);
        let active = self.frame.active_lines();
        let mut lines = column![]
            .spacing(12)
            .align_x(iced::Center)
            .width(Length::Fill);
        if active.is_empty() {
            let label = if self.frame.next_line().is_some() || !self.frame.lines.is_empty() {
                "···"
            } else if self.frame.title.is_empty() && self.frame.artist.is_empty() {
                "Waiting for music"
            } else {
                "No lyrics"
            };
            lines = lines.push(
                text(label)
                    .size(17)
                    .color(Color::WHITE.scale_alpha(0.45 * alpha)),
            );
        }
        for line in active {
            let mut fragments = Vec::new();
            if !line.prefix.is_empty() {
                fragments.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(sung),
                );
            }
            for (fragment, played) in line.fragments(self.frame.elapsed) {
                fragments.push(span(fragment).color(if played {
                    sung
                } else {
                    Color::WHITE.scale_alpha(0.48 * alpha)
                }));
            }
            let shadow_color = Color::BLACK.scale_alpha(0.65 * alpha);
            let shadow_fragments = fragments
                .iter()
                .cloned()
                .map(|fragment| fragment.color(shadow_color))
                .collect::<Vec<_>>();
            let primary_text = |fragments| {
                rich_text::<(), _, _, _>(fragments)
                    .size(preferences.font_size)
                    .font(Font {
                        weight: iced::font::Weight::Medium,
                        ..Font::DEFAULT
                    })
                    .align_x(iced::Center)
                    .width(Length::Fill)
            };
            // Keep a small text shadow for contrast over light desktop backgrounds.
            let shadow_offset = iced::Padding {
                top: 2.0,
                right: 0.0,
                bottom: 0.0,
                left: 0.0,
            };
            let primary = stack![
                container(primary_text(shadow_fragments)).padding(shadow_offset),
                primary_text(fragments),
            ];
            let mut group = column![primary]
                .spacing(5)
                .align_x(iced::Center)
                .width(Length::Fill);
            if preferences.additional_lines {
                for additional in line
                    .additional_lines()
                    .iter()
                    .filter(|line| !line.trim().is_empty())
                {
                    let additional_text = |color| {
                        text(additional.as_str())
                            .size(preferences.font_size as f32 * 0.46)
                            .shaping(iced::advanced::text::Shaping::Advanced)
                            .color(color)
                            .align_x(iced::Center)
                            .width(Length::Fill)
                    };
                    group = group.push(stack![
                        container(additional_text(shadow_color)).padding(shadow_offset),
                        additional_text(Color::WHITE.scale_alpha(0.7 * alpha)),
                    ]);
                }
            }
            let agent = if line.agent_name.is_empty() {
                &line.agent
            } else {
                &line.agent_name
            };
            let group: Element<'_, Event> = if agent.is_empty() {
                group.into()
            } else {
                tooltip(
                    group,
                    text(agent.as_str()).size(11),
                    tooltip::Position::Bottom,
                )
                .into()
            };
            lines = lines.push(group);
        }
        if preferences.next_line
            && let Some(next) = self.frame.next_line()
        {
            lines = lines.push(
                text(format!("{}{}", next.prefix, next.first_text()))
                    .size(13)
                    .shaping(iced::advanced::text::Shaping::Advanced)
                    .color(Color::WHITE.scale_alpha(0.4 * alpha))
                    .align_x(iced::Center)
                    .width(Length::Fill),
            );
        }
        let toolbar: Element<'_, Event> = if self.hovered && !preferences.locked {
            row![
                tooltip(
                    container(text("Move").size(11)).padding([4, 8]),
                    "Drag to move lyrics",
                    tooltip::Position::Top
                ),
                button(text("Play / Pause").size(11)).on_press(Event::PlayPause),
                button(text("Lock").size(11)).on_press(Event::Locked(true)),
                button(text("Settings").size(11)).on_press(Event::Settings),
            ]
            .spacing(4)
            .into()
        } else {
            space().height(28).into()
        };
        mouse_area(
            container(
                column![
                    container(toolbar).center_x(Length::Fill),
                    container(scrollable(lines).height(Length::Shrink)).center_y(Length::Fill),
                ]
                .spacing(8),
            )
            .padding(12)
            .width(Length::Fill)
            .height(Length::Fill),
        )
        .on_press(Event::Drag)
        .on_enter(Event::Hover(true))
        .on_exit(Event::Hover(false))
        .into()
    }

    pub fn settings_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let content = column![
            text("Lyrics settings").size(19),
            text(format!("Text size · {} px", preferences.font_size)).size(13),
            slider(22.0..=48.0, preferences.font_size as f32, Event::FontSize).step(1.0_f32),
            text(format!("Opacity · {:.0}%", preferences.opacity * 100.0)).size(13),
            slider(0.4..=1.0, preferences.opacity, Event::Opacity).step(0.01_f32),
            text("Highlight").size(13),
            row![
                button("Violet").on_press(Event::Highlight(Highlight::Violet)),
                button("White").on_press(Event::Highlight(Highlight::White)),
            ]
            .spacing(8),
            checkbox(preferences.additional_lines)
                .label("Additional lines")
                .on_toggle(Event::AdditionalLines),
            checkbox(preferences.next_line)
                .label("Next line")
                .on_toggle(Event::NextLine),
            checkbox(preferences.locked)
                .label("Lock position")
                .on_toggle(Event::Locked),
            checkbox(preferences.enabled)
                .label("Show lyrics")
                .on_toggle(Event::Visible),
            button("Reset position").on_press(Event::ResetPosition),
            text("Locked lyrics pass clicks through. Unlock them here or from the View menu.")
                .size(11),
            button("Close").on_press(Event::CloseSettings),
        ]
        .spacing(14);
        container(scrollable(content))
            .padding(20)
            .width(Length::Fill)
            .height(Length::Fill)
            .style(container::dark)
            .into()
    }
}
