//! Native lyrics and settings windows; playback remains in the TUI.
use iced::widget::{
    button, column, container, mouse_area, pin, rich_text, row, rule, scrollable, slider, space,
    span, stack, text, toggler, tooltip,
};
use iced::{Color, Element, Font, Length, Point, Size, Task, window};

use crate::ipc::Request;
use crate::lyrics::{Frame, Highlight, Position, Preferences};

#[derive(Debug, Clone)]
pub enum Event {
    Opened(window::Id),
    SettingsOpened(window::Id),
    Ready,
    Sized(window::Id, Point, Size, Size),
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
    size: Size,
    opened: bool,
}

impl Overlay {
    pub fn display_size(&self, preferences: &Preferences) -> Size {
        let active = self.frame.active_lines();
        let mut rows = Vec::new();
        for line in active {
            rows.push((
                format!("{}{}", line.prefix, line.first_text()),
                preferences.font_size as f32,
            ));
            if preferences.additional_lines {
                rows.extend(
                    line.additional_lines()
                        .iter()
                        .filter(|line| !line.trim().is_empty())
                        .map(|line| (line.clone(), preferences.font_size as f32 * 0.46)),
                );
            }
        }
        if preferences.next_line
            && let Some(next) = self.frame.next_line()
        {
            rows.push((format!("{}{}", next.prefix, next.first_text()), 13.0));
        }
        if rows.is_empty() {
            return Size::new(300.0, 100.0);
        }
        let width = (rows
            .iter()
            .map(|(line, size)| measure_text(line, *size, f32::INFINITY).width)
            .fold(0.0_f32, f32::max)
            .ceil()
            + 40.0)
            .clamp(300.0, 880.0);
        let height = rows
            .iter()
            .map(|(line, size)| measure_text(line, *size, width - 40.0).height)
            .sum::<f32>()
            + 58.0
            + (rows.len() - 1) as f32 * 8.0;
        Size::new(width, height.ceil().clamp(100.0, 500.0))
    }

    pub fn fit(&mut self, preferences: &Preferences) -> Task<Event> {
        if !self.opened {
            return Task::none();
        }
        let Some(id) = self.window else {
            return Task::none();
        };
        let size = self.display_size(preferences);
        if size == self.size {
            return Task::none();
        }
        self.size = size;
        window::position(id).then(move |position| {
            window::size(id).map(move |previous| {
                Event::Sized(id, position.unwrap_or(Point::ORIGIN), previous, size)
            })
        })
    }
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
        self.size = self.display_size(preferences);
        self.opened = false;
        let (id, open) = window::open(window::Settings {
            size: self.size,
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
                self.opened = true;
                return (
                    Task::batch([
                        crate::desktop::configure_lyrics_window(id).map(|_| Event::Ready),
                        self.passthrough(preferences.locked),
                        self.fit(preferences),
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
                    self.opened = false;
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
                        size: Size::new(300.0, 416.0),
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
            Event::Sized(id, position, previous, size) => {
                if self.window != Some(id) || self.size != size {
                    return (Task::none(), None);
                }
                let position =
                    Point::new(position.x + (previous.width - size.width) / 2.0, position.y);
                Task::batch([window::move_to(id, position), window::resize(id, size)])
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
        (Task::batch([task, self.fit(preferences)]), None)
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
            let shadow = fragments
                .iter()
                .cloned()
                .map(|fragment| fragment.color(Color::BLACK.scale_alpha(0.6 * alpha)))
                .collect::<Vec<_>>();
            let make_text = |fragments| {
                rich_text::<(), _, _, _>(fragments)
                    .size(preferences.font_size)
                    .font(Font {
                        weight: iced::font::Weight::Medium,
                        ..Font::DEFAULT
                    })
                    .align_x(iced::Center)
                    .width(Length::Fill)
            };
            let primary = stack![make_text(fragments)]
                .push_under(pin(make_text(shadow)).height(Length::Shrink).y(0.75));
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
                    let additional_shadow = text(additional.as_str())
                        .size(preferences.font_size as f32 * 0.46)
                        .shaping(iced::advanced::text::Shaping::Advanced)
                        .color(Color::BLACK.scale_alpha(0.55 * alpha))
                        .align_x(iced::Center)
                        .width(Length::Fill);
                    group = group.push(
                        stack![
                            text(additional.as_str())
                                .size(preferences.font_size as f32 * 0.46)
                                .shaping(iced::advanced::text::Shaping::Advanced)
                                .color(Color::WHITE.scale_alpha(0.7 * alpha))
                                .align_x(iced::Center)
                                .width(Length::Fill)
                        ]
                        .push_under(pin(additional_shadow).height(Length::Shrink).y(0.75)),
                    );
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
            container(
                row![
                    tooltip(
                        mouse_area(container(icon(Icon::Move)).padding(6)).on_press(Event::Drag),
                        text("Move lyrics").size(11),
                        tooltip::Position::Top
                    ),
                    icon_button(Icon::Playback, "Play / Pause", Event::PlayPause),
                    icon_button(Icon::Lock, "Lock position", Event::Locked(true)),
                    icon_button(Icon::Settings, "Lyrics settings", Event::Settings),
                ]
                .spacing(2)
                .align_y(iced::Center),
            )
            .padding(3)
            .style(toolbar_style)
            .into()
        } else {
            space().height(34).into()
        };
        let content = column![
            container(toolbar).height(34).center_x(Length::Fill),
            scrollable(lines).height(Length::Shrink),
        ]
        .spacing(8)
        .align_x(iced::Center);
        mouse_area(
            container(content)
                .padding([8, 20])
                .center_x(Length::Fill)
                .center_y(Length::Fill),
        )
        .on_press(Event::Drag)
        .on_enter(Event::Hover(true))
        .on_exit(Event::Hover(false))
        .interaction(iced::mouse::Interaction::Grab)
        .into()
    }

    pub fn settings_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let label = |name, value| {
            row![
                text(name).size(12).color(Color::from_rgb8(199, 196, 207)),
                space().width(Length::Fill),
                text(value).size(11).color(Color::from_rgb8(139, 136, 148)),
            ]
            .align_y(iced::Center)
        };
        let toggle = |title, checked, event: fn(bool) -> Event| {
            row![
                text(title).size(12).color(Color::from_rgb8(199, 196, 207)),
                space().width(Length::Fill),
                toggler(checked)
                    .size(16)
                    .on_toggle(event)
                    .style(|theme, status| {
                        let mut style = toggler::default(theme, status);
                        if matches!(
                            status,
                            toggler::Status::Active { is_toggled: true }
                                | toggler::Status::Hovered { is_toggled: true }
                        ) {
                            style.background = Color::from_rgb8(170, 146, 205).into();
                        }
                        style
                    }),
            ]
            .align_y(iced::Center)
        };
        let color_button = |name, color, selected| {
            button(text(name).size(12))
                .padding([5, 18])
                .on_press(Event::Highlight(color))
                .style(move |_, status| control_style(selected, status))
        };
        let content = column![
            label("Text size", format!("{} px", preferences.font_size)),
            slider(22.0..=48.0, preferences.font_size as f32, Event::FontSize)
                .step(1.0_f32)
                .height(14)
                .style(slider_style),
            space().height(2),
            label("Opacity", format!("{:.0}%", preferences.opacity * 100.0)),
            slider(0.4..=1.0, preferences.opacity, Event::Opacity)
                .step(0.01_f32)
                .height(14)
                .style(slider_style),
            space().height(2),
            text("Highlight")
                .size(12)
                .color(Color::from_rgb8(199, 196, 207)),
            row![
                color_button(
                    "Violet",
                    Highlight::Violet,
                    matches!(preferences.highlight, Highlight::Violet)
                ),
                color_button(
                    "White",
                    Highlight::White,
                    matches!(preferences.highlight, Highlight::White)
                ),
            ]
            .spacing(4),
            rule::horizontal(1),
            toggle(
                "Additional lines",
                preferences.additional_lines,
                Event::AdditionalLines
            ),
            toggle("Next line", preferences.next_line, Event::NextLine),
            toggle("Lock position", preferences.locked, Event::Locked),
            toggle("Show lyrics", preferences.enabled, Event::Visible),
            row![
                button(text("Reset position").size(11))
                    .on_press(Event::ResetPosition)
                    .padding([5, 0])
                    .style(button::text),
                space().width(Length::Fill),
                button(text("Close").size(12))
                    .on_press(Event::CloseSettings)
                    .padding([5, 14])
                    .style(|_, status| control_style(false, status)),
            ]
            .align_y(iced::Center),
        ]
        .spacing(12);
        container(scrollable(content))
            .padding(20)
            .width(Length::Fill)
            .height(Length::Fill)
            .style(|_| container::Style {
                background: Some(Color::from_rgb8(32, 32, 35).into()),
                text_color: Some(Color::from_rgb8(232, 230, 237)),
                ..Default::default()
            })
            .into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn overlay_bounds_follow_lyrics_and_optional_sublines_without_growing_on_progress() {
        let mut overlay = Overlay { frame: serde_json::from_value(serde_json::json!({
            "elapsed": 1, "lines": [{"time":0,"text":"Short line", "parts":["Short line","A second line"]}]
        })).unwrap(), ..Default::default() };
        let mut preferences = Preferences {
            additional_lines: false,
            ..Default::default()
        };
        let short = overlay.display_size(&preferences);
        preferences.additional_lines = true;
        let bilingual = overlay.display_size(&preferences);
        assert!(bilingual.height > short.height);
        overlay.frame.lines[0].parts[0] =
            "A considerably longer line with more words to read on the desktop".into();
        let long = overlay.display_size(&preferences);
        assert!(long.width > short.width);
        overlay.frame.elapsed = 2.0;
        assert_eq!(overlay.display_size(&preferences), long);
    }

    #[test]
    fn frames_received_during_opening_are_sized_after_the_window_is_ready() {
        let id = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            size: Size::new(300.0, 100.0),
            frame: serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"text":"A considerably longer line than the initial waiting message"}
            ]}))
            .unwrap(),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let initial = overlay.size;
        let _ = overlay.fit(&preferences);
        assert_eq!(overlay.size, initial);
        let expected = overlay.display_size(&preferences);
        assert!(expected.width > initial.width);
        let _ = overlay.update(Event::Opened(id), &mut preferences);
        assert!(overlay.opened);
        assert_eq!(overlay.size, expected);
    }
}

fn toolbar_style(_: &iced::Theme) -> container::Style {
    container::Style {
        background: Some(Color::from_rgba8(34, 34, 37, 0.94).into()),
        border: iced::Border {
            color: Color::WHITE.scale_alpha(0.09),
            width: 1.0,
            radius: 8.0.into(),
        },
        ..Default::default()
    }
}

fn control_style(selected: bool, status: button::Status) -> button::Style {
    let hovered = matches!(status, button::Status::Hovered | button::Status::Pressed);
    button::Style {
        background: Some(
            if selected {
                Color::from_rgb8(56, 52, 63)
            } else if hovered {
                Color::from_rgb8(48, 46, 52)
            } else {
                Color::TRANSPARENT
            }
            .into(),
        ),
        text_color: if selected {
            Color::from_rgb8(221, 203, 248)
        } else {
            Color::from_rgb8(180, 176, 190)
        },
        border: iced::Border {
            radius: 5.0.into(),
            ..Default::default()
        },
        ..Default::default()
    }
}

fn slider_style(theme: &iced::Theme, status: slider::Status) -> slider::Style {
    let mut style = slider::default(theme, status).with_circular_handle(5.0);
    style.rail.width = 3.0;
    style.rail.backgrounds = (
        Color::from_rgb8(176, 153, 213).into(),
        Color::from_rgb8(68, 62, 76).into(),
    );
    style.handle.background = Color::from_rgb8(217, 200, 247).into();
    style
}

#[derive(Clone, Copy)]
enum Icon {
    Move,
    Playback,
    Lock,
    Settings,
}

fn icon(kind: Icon) -> iced::widget::Canvas<Glyph, Event> {
    iced::widget::canvas(Glyph(kind)).width(16).height(16)
}

fn icon_button(kind: Icon, label: &'static str, event: Event) -> Element<'static, Event> {
    tooltip(
        button(icon(kind))
            .padding(6)
            .on_press(event)
            .style(|_, status| control_style(false, status)),
        text(label).size(11),
        tooltip::Position::Top,
    )
    .into()
}

struct Glyph(Icon);

impl iced::widget::canvas::Program<Event> for Glyph {
    type State = ();
    fn draw(
        &self,
        _: &(),
        renderer: &iced::Renderer,
        _: &iced::Theme,
        bounds: iced::Rectangle,
        _: iced::mouse::Cursor,
    ) -> Vec<iced::widget::canvas::Geometry> {
        use iced::widget::canvas::{Frame, Path, Stroke};
        let mut frame = Frame::new(renderer, bounds.size());
        let path = Path::new(|p| {
            let mut line = |a, b| {
                p.move_to(Point::from(a));
                p.line_to(Point::from(b));
            };
            match self.0 {
                Icon::Move => {
                    for y in [4.0, 8.0, 12.0] {
                        line((5.0, y), (5.5, y));
                        line((10.5, y), (11.0, y));
                    }
                }
                Icon::Playback => {
                    line((2.0, 3.0), (8.0, 8.0));
                    line((8.0, 8.0), (2.0, 13.0));
                    line((2.0, 13.0), (2.0, 3.0));
                    line((11.0, 3.0), (11.0, 13.0));
                    line((14.0, 3.0), (14.0, 13.0));
                }
                Icon::Lock => {
                    line((3.0, 7.0), (13.0, 7.0));
                    line((13.0, 7.0), (13.0, 14.0));
                    line((13.0, 14.0), (3.0, 14.0));
                    line((3.0, 14.0), (3.0, 7.0));
                    line((5.0, 7.0), (5.0, 4.0));
                    line((5.0, 4.0), (8.0, 2.0));
                    line((8.0, 2.0), (11.0, 4.0));
                    line((11.0, 4.0), (11.0, 7.0));
                    line((8.0, 10.0), (8.0, 12.0));
                }
                Icon::Settings => {
                    line((2.0, 5.0), (14.0, 5.0));
                    line((2.0, 11.0), (14.0, 11.0));
                    line((6.0, 3.0), (6.0, 7.0));
                    line((11.0, 9.0), (11.0, 13.0));
                }
            }
        });
        frame.stroke(
            &path,
            Stroke::default()
                .with_width(1.3)
                .with_color(Color::from_rgb8(205, 201, 216)),
        );
        vec![frame.into_geometry()]
    }
}

fn measure_text(content: &str, size: f32, width: f32) -> Size {
    use iced::advanced::text::{Alignment, LineHeight, Paragraph, Shaping, Wrapping};
    iced::advanced::graphics::text::paragraph::Paragraph::with_text(iced::advanced::Text {
        content,
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: LineHeight::Relative(1.3),
        font: Font {
            weight: iced::font::Weight::Medium,
            ..Font::DEFAULT
        },
        align_x: Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Wrapping::WordOrGlyph,
    })
    .min_bounds()
}
