//! Floating lyrics toolbar and shared control styling.
use iced::widget::{button, column, container, mouse_area, row, space, text, tooltip};
use iced::{Color, Element, Length, Point};

use super::{Event, Overlay, Preferences};

impl Overlay {
    pub(super) fn toolbar_content(&self, preferences: &Preferences) -> Element<'_, Event> {
        if self.toolbar_visible && !preferences.locked {
            container(
                row![
                    hint(
                        mouse_area(container(icon(Icon::Move)).padding(6)).on_press(Event::Drag),
                        "Drag to move",
                        tooltip::Position::Top
                    ),
                    hint(
                        button(icon(if self.playback == Some(true) {
                            Icon::Pause
                        } else {
                            Icon::Play
                        }))
                        .padding(6)
                        .on_press_maybe(self.playback.map(|_| Event::PlayPause))
                        .style(|_, status| control_style(false, status)),
                        if self.playback == Some(true) {
                            "Pause"
                        } else {
                            "Play"
                        },
                        tooltip::Position::Top,
                    ),
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
        }
    }

    pub fn toolbar_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let toolbar = self.toolbar_content(preferences);
        let area = mouse_area(
            container(column![space().height(30), toolbar].spacing(8))
                .padding([8, 0])
                .center_x(Length::Fill)
                .height(Length::Fill),
        );
        #[cfg(not(target_os = "macos"))]
        let area = area
            .on_enter(Event::ToolbarHover(true))
            .on_exit(Event::ToolbarHover(false));
        area.into()
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

pub(super) fn control_style(selected: bool, status: button::Status) -> button::Style {
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

#[derive(Clone, Copy)]
enum Icon {
    Move,
    Play,
    Pause,
    Lock,
    Settings,
}

fn icon(kind: Icon) -> iced::widget::Canvas<Glyph, Event> {
    iced::widget::canvas(Glyph(kind)).width(16).height(16)
}

pub(super) fn hint<'a>(
    content: impl Into<Element<'a, Event>>,
    label: &'a str,
    position: tooltip::Position,
) -> Element<'a, Event> {
    tooltip(
        content,
        container(text(label).size(11).color(Color::from_rgb8(235, 232, 241))).padding([6, 9]),
        position,
    )
    .gap(7)
    .padding(0)
    .delay(std::time::Duration::from_millis(400))
    .style(|_| container::Style {
        background: Some(Color::from_rgba8(30, 29, 34, 0.98).into()),
        border: iced::Border {
            color: Color::WHITE.scale_alpha(0.14),
            width: 1.0,
            radius: 6.0.into(),
        },
        shadow: iced::Shadow {
            color: Color::BLACK.scale_alpha(0.2),
            offset: iced::Vector::new(0.0, 2.0),
            blur_radius: 6.0,
        },
        ..Default::default()
    })
    .into()
}

fn icon_button(kind: Icon, label: &'static str, event: Event) -> Element<'static, Event> {
    hint(
        button(icon(kind))
            .padding(6)
            .on_press(event)
            .style(|_, status| control_style(false, status)),
        label,
        tooltip::Position::Top,
    )
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
                Icon::Play => {
                    line((5.0, 3.0), (11.0, 8.0));
                    line((11.0, 8.0), (5.0, 13.0));
                    line((5.0, 13.0), (5.0, 3.0));
                }
                Icon::Pause => {
                    line((5.0, 3.0), (5.0, 13.0));
                    line((11.0, 3.0), (11.0, 13.0));
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

#[cfg(test)]
mod tests {
    use super::*;
    use iced::advanced::{
        mouse,
        renderer::{Headless, Style},
    };

    fn visible_bounds(kind: Icon) -> (u32, u32) {
        let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
            iced::Font::DEFAULT,
            14.0.into(),
        ));
        let view: Element<'static, Event> = container(icon(kind)).width(16).height(16).into();
        let mut ui = iced_runtime::UserInterface::build(
            view,
            iced::Size::new(16.0, 16.0),
            iced_runtime::user_interface::Cache::new(),
            &mut renderer,
        );
        ui.draw(
            &mut renderer,
            &iced::Theme::Dark,
            &Style {
                text_color: Color::WHITE,
            },
            mouse::Cursor::Unavailable,
        );
        let pixels = renderer.screenshot(iced::Size::new(16, 16), 1.0, Color::TRANSPARENT);
        let (pixels, _) = pixels.as_chunks::<4>();
        let mut min_x = 16;
        let mut max_x = 0;
        for (index, pixel) in pixels.iter().enumerate() {
            if pixel[3] > 0 {
                let x = (index % 16) as u32;
                min_x = min_x.min(x);
                max_x = max_x.max(x);
            }
        }
        (min_x, max_x)
    }

    #[test]
    fn play_and_pause_glyphs_share_a_centered_visual_width() {
        let play = visible_bounds(Icon::Play);
        let pause = visible_bounds(Icon::Pause);
        assert_eq!(
            play.1 - play.0,
            pause.1 - pause.0,
            "glyph bounds differ: play={play:?}, pause={pause:?}"
        );
        assert_eq!(
            play.0 + play.1,
            pause.0 + pause.1,
            "glyph centers differ: play={play:?}, pause={pause:?}"
        );
    }
}
