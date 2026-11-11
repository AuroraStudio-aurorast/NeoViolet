//! Compact dialog surfaces; lifecycle and playback actions stay in the application.
use iced::widget::{button, column, container, opaque, row, rule, scrollable, text, text_input};
use iced::{Border, Color, Element, Font, Length, Padding, Shadow, Theme, Vector};

use crate::app::{Dialog, Message};

const FOREGROUND: Color = Color::from_rgb8(239, 239, 242);
const SECONDARY: Color = Color::from_rgb8(181, 181, 189);
const MUTED: Color = Color::from_rgb8(155, 155, 165);

#[derive(Debug, Default)]
pub struct Navigation {
    selected: Option<usize>,
}

impl Navigation {
    pub fn reset(&mut self) {
        self.selected = None;
    }
    pub fn visible(&self) -> bool {
        self.selected.is_some()
    }
    pub fn key(
        &mut self,
        dialog: &Dialog,
        running: bool,
        key: &iced::keyboard::Key,
        shift: bool,
    ) -> Option<Message> {
        use iced::keyboard::{Key, key::Named};
        if matches!(dialog, Dialog::OpenOnlineAudio { .. }) {
            return match key {
                Key::Named(Named::Tab) => {
                    self.selected = match (self.selected, shift) {
                        (None, false) => Some(0),
                        (Some(0), false) => Some(1),
                        (None, true) => Some(1),
                        (Some(1), true) => Some(0),
                        _ => None,
                    };
                    None
                }
                Key::Named(Named::Enter | Named::Space) => match self.selected {
                    Some(0) => Some(Message::Dismiss),
                    Some(1) => Some(Message::SubmitOnlineAudio),
                    _ => None,
                },
                _ => None,
            };
        }
        let actions = match dialog {
            Dialog::Quit => vec![Message::Dismiss, Message::ConfirmQuit],
            Dialog::Error(_) if !running => vec![
                Message::ConfirmQuit,
                Message::ChoosePlayer,
                Message::Restart,
            ],
            _ => vec![Message::Dismiss],
        };
        let default = if matches!(dialog, Dialog::Error(_)) && !running {
            actions.len() - 1
        } else {
            0
        };
        let current = self.selected.unwrap_or(default).min(actions.len() - 1);
        match key {
            Key::Named(Named::Tab) => {
                self.selected =
                    Some((current + if shift { actions.len() - 1 } else { 1 }) % actions.len());
                None
            }
            Key::Named(Named::Enter | Named::Space) => Some(actions[current].clone()),
            _ => None,
        }
    }
}

pub fn overlay<'a>(
    dialog: &'a Dialog,
    version: &'a str,
    libopenmpt_version: Option<&'a str>,
    running: bool,
    navigation: &Navigation,
) -> Element<'a, Message> {
    let action = |label, message, tone, index| {
        action(
            label,
            message,
            tone,
            navigation.visible() && navigation.selected == Some(index),
        )
    };
    let (heading, mark, details, actions, width): (
        &str,
        Mark,
        Element<'a, Message>,
        iced::widget::Row<'a, Message>,
        u32,
    ) = match dialog {
        Dialog::About => {
            let mut details = column![
                text("Player version").size(11).color(MUTED),
                text(version)
                    .size(12)
                    .color(SECONDARY)
                    .wrapping(text::Wrapping::WordOrGlyph),
            ]
            .spacing(4);
            if let Some(version) = libopenmpt_version {
                details = details.push(text("libopenmpt").size(11).color(MUTED)).push(
                    text(version)
                        .size(12)
                        .color(SECONDARY)
                        .wrapping(text::Wrapping::WordOrGlyph),
                );
            }
            (
                "NeoViolet",
                Mark::Music,
                details.into(),
                row![action("Close", Message::Dismiss, Tone::Primary, 0)],
                408,
            )
        }
        Dialog::Quit => (
            "Close NeoViolet?",
            Mark::Leave,
            paragraph("Playback will stop when the player closes."),
            row![
                action("Cancel", Message::Dismiss, Tone::Neutral, 0),
                action("Close player", Message::ConfirmQuit, Tone::Danger, 1),
            ],
            408,
        ),
        Dialog::Error(error) => (
            if running {
                "Unable to complete action"
            } else {
                "Player unavailable"
            },
            Mark::Attention,
            paragraph(error),
            if running {
                row![action("Continue", Message::Dismiss, Tone::Primary, 0)]
            } else {
                row![
                    action("Close", Message::ConfirmQuit, Tone::Neutral, 0),
                    action("Choose player…", Message::ChoosePlayer, Tone::Neutral, 1),
                    action("Restart", Message::Restart, Tone::Primary, 2),
                ]
            },
            432,
        ),
        Dialog::OpenOnlineAudio { url, error } => (
            "Open Online Audio",
            Mark::Music,
            column![
                paragraph("Enter a direct HTTP or HTTPS audio file URL. The file is downloaded before playback; live streams are not supported."),
                text_input("https://example.com/audio.mp3", url)
                    .id("online-audio-url")
                    .on_input(Message::OnlineAudioURLChanged)
                    .on_submit(Message::SubmitOnlineAudio)
                    .padding([9, 10])
                    .size(13),
            ]
            .push(
                text(error.as_deref().unwrap_or(""))
                    .size(12)
                    .color(Color::from_rgb8(240, 155, 155)),
            )
            .spacing(10)
            .into(),
            row![
                action("Cancel", Message::Dismiss, Tone::Neutral, 0),
                action("Open", Message::SubmitOnlineAudio, Tone::Primary, 1),
            ],
            480,
        ),
    };
    let heading = text(heading)
        .size(18)
        .font(Font {
            weight: iced::font::Weight::Semibold,
            ..crate::launch::interface_font()
        })
        .color(FOREGROUND);
    let title: Element<'a, Message> = if matches!(dialog, Dialog::About) {
        column![
            heading,
            text(format!("GUI {}", env!("CARGO_PKG_VERSION")))
                .size(11)
                .color(MUTED)
        ]
        .spacing(3)
        .into()
    } else {
        heading.into()
    };
    let header = row![Badge(mark), title].spacing(14).align_y(iced::Center);
    let body = column![
        header,
        container(
            scrollable(container(details).padding(Padding::ZERO.right(10)))
                .direction(scrollable::Direction::Vertical(
                    scrollable::Scrollbar::new()
                        .width(4)
                        .scroller_width(4)
                        .margin(2)
                ))
                .height(Length::Shrink)
        )
        .width(Length::Fill)
        .max_height(if matches!(dialog, Dialog::OpenOnlineAudio { .. }) {
            150
        } else {
            104
        }),
    ]
    .spacing(16)
    .padding(24);
    let left: Element<'a, Message> = if matches!(dialog, Dialog::About) {
        text("MIT License").size(11).color(MUTED).into()
    } else {
        iced::widget::Space::new().into()
    };
    let footer = container(
        row![
            left,
            iced::widget::Space::new().width(Length::Fill),
            actions.spacing(8)
        ]
        .align_y(iced::Center),
    )
    .padding(Padding {
        top: 14.0,
        right: 22.0,
        bottom: 16.0,
        left: 24.0,
    })
    .width(Length::Fill)
    .style(|_| container::Style {
        background: Some(Color::from_rgb8(39, 39, 41).into()),
        border: Border {
            radius: iced::border::Radius {
                bottom_left: 13.0,
                bottom_right: 13.0,
                ..Default::default()
            },
            ..Default::default()
        },
        ..Default::default()
    });
    let divider = rule::horizontal(1).style(|_| rule::Style {
        color: Color::WHITE.scale_alpha(0.065),
        radius: 0.0.into(),
        fill_mode: rule::FillMode::Full,
        snap: true,
    });
    let panel = container(column![body, divider, footer])
        .padding(1)
        .width(Length::Fill)
        .max_width(width)
        .max_height(if matches!(dialog, Dialog::OpenOnlineAudio { .. }) {
            330
        } else {
            280
        })
        .style(|_| container::Style {
            background: Some(Color::from_rgb8(43, 43, 46).into()),
            text_color: Some(FOREGROUND),
            border: Border {
                color: Color::WHITE.scale_alpha(0.11),
                width: 1.0,
                radius: 14.0.into(),
            },
            shadow: Shadow {
                color: Color::BLACK.scale_alpha(0.38),
                offset: Vector::new(0.0, 14.0),
                blur_radius: 40.0,
            },
            ..Default::default()
        });
    opaque(
        container(panel)
            .padding(20)
            .center_x(Length::Fill)
            .center_y(Length::Fill)
            .style(|_| container::Style {
                background: Some(Color::BLACK.scale_alpha(0.54).into()),
                ..Default::default()
            }),
    )
}

fn paragraph(content: &str) -> Element<'_, Message> {
    text(content)
        .size(13)
        .line_height(text::LineHeight::Relative(1.5))
        .color(SECONDARY)
        .shaping(text::Shaping::Advanced)
        .wrapping(text::Wrapping::WordOrGlyph)
        .into()
}

#[derive(Clone, Copy)]
enum Tone {
    Neutral,
    Primary,
    Danger,
}

fn action(
    label: &'static str,
    message: Message,
    tone: Tone,
    focused: bool,
) -> Element<'static, Message> {
    button(
        container(text(label).size(12).font(Font {
            weight: iced::font::Weight::Medium,
            ..crate::launch::interface_font()
        }))
        .center_x(Length::Fill)
        .center_y(Length::Fill),
    )
    .padding([0, 14])
    .width(if label == "Close player" || label == "Choose player…" {
        112
    } else {
        84
    })
    .height(34)
    .on_press(message)
    .style(move |_, status| {
        let (normal, hover, pressed, foreground) = match tone {
            Tone::Neutral => (
                Color::from_rgb8(53, 53, 57),
                Color::from_rgb8(64, 64, 69),
                Color::from_rgb8(46, 46, 50),
                FOREGROUND,
            ),
            Tone::Primary => (
                Color::from_rgb8(177, 151, 218),
                Color::from_rgb8(194, 171, 232),
                Color::from_rgb8(156, 129, 198),
                Color::from_rgb8(31, 24, 41),
            ),
            Tone::Danger => (
                Color::from_rgb8(159, 79, 94),
                Color::from_rgb8(177, 91, 107),
                Color::from_rgb8(138, 65, 79),
                Color::WHITE,
            ),
        };
        button::Style {
            background: Some(
                match status {
                    button::Status::Hovered => hover,
                    button::Status::Pressed => pressed,
                    _ => normal,
                }
                .into(),
            ),
            text_color: foreground,
            border: Border {
                color: if focused {
                    Color::from_rgb8(226, 208, 250)
                } else {
                    Color::WHITE.scale_alpha(if matches!(tone, Tone::Neutral) {
                        0.08
                    } else {
                        0.06
                    })
                },
                width: if focused { 2.0 } else { 1.0 },
                radius: 7.0.into(),
            },
            ..Default::default()
        }
    })
    .into()
}

#[derive(Clone, Copy)]
enum Mark {
    Music,
    Leave,
    Attention,
}
struct Badge(Mark);

impl From<Badge> for Element<'_, Message> {
    fn from(badge: Badge) -> Self {
        Self::new(badge)
    }
}

impl iced::advanced::Widget<Message, Theme, iced::Renderer> for Badge {
    fn size(&self) -> iced::Size<Length> {
        iced::Size::new(Length::Fixed(36.0), Length::Fixed(36.0))
    }
    fn layout(
        &mut self,
        _: &mut iced::advanced::widget::Tree,
        _: &iced::Renderer,
        limits: &iced::advanced::layout::Limits,
    ) -> iced::advanced::layout::Node {
        iced::advanced::layout::Node::new(limits.resolve(36.0, 36.0, iced::Size::new(36.0, 36.0)))
    }
    fn draw(
        &self,
        _: &iced::advanced::widget::Tree,
        renderer: &mut iced::Renderer,
        _: &Theme,
        _: &iced::advanced::renderer::Style,
        layout: iced::advanced::Layout<'_>,
        _: iced::mouse::Cursor,
        _: &iced::Rectangle,
    ) {
        use iced::advanced::Renderer as _;
        let origin = layout.position();
        let (background, foreground) = match self.0 {
            Mark::Music => (
                Color::from_rgb8(62, 51, 79),
                Color::from_rgb8(195, 166, 237),
            ),
            Mark::Leave => (
                Color::from_rgb8(55, 51, 61),
                Color::from_rgb8(196, 186, 209),
            ),
            Mark::Attention => (
                Color::from_rgb8(63, 56, 43),
                Color::from_rgb8(220, 186, 128),
            ),
        };
        let mut shape = |x, y, width, height, radius, color| {
            renderer.fill_quad(
                iced::advanced::renderer::Quad {
                    bounds: iced::Rectangle {
                        x: origin.x + x,
                        y: origin.y + y,
                        width,
                        height,
                    },
                    border: Border {
                        radius: iced::border::Radius::from(radius),
                        ..Default::default()
                    },
                    ..Default::default()
                },
                color,
            )
        };
        shape(0.0, 0.0, 36.0, 36.0, 10.0, background);
        match self.0 {
            Mark::Music => {
                for (x, y, w, h, r) in [
                    (13.5, 11.0, 1.5, 13.0, 0.7),
                    (23.5, 10.0, 1.5, 12.0, 0.7),
                    (13.5, 10.0, 11.5, 1.5, 0.7),
                    (13.5, 14.0, 11.5, 1.5, 0.7),
                    (9.0, 21.0, 6.0, 5.0, 2.5),
                    (19.0, 19.0, 6.0, 5.0, 2.5),
                ] {
                    shape(x, y, w, h, r, foreground);
                }
            }
            Mark::Leave => {
                for (x, y, w, h) in [
                    (10.0, 10.0, 1.5, 16.0),
                    (10.0, 10.0, 7.0, 1.5),
                    (10.0, 24.5, 7.0, 1.5),
                    (16.0, 17.25, 10.0, 1.5),
                ] {
                    shape(x, y, w, h, 0.7, foreground);
                }
                for offset in 0..5 {
                    let offset = offset as f32 * 0.8;
                    shape(21.5 + offset, 13.5 + offset, 1.6, 1.6, 0.7, foreground);
                    shape(21.5 + offset, 21.0 - offset, 1.6, 1.6, 0.7, foreground);
                }
            }
            Mark::Attention => {
                shape(17.25, 10.0, 1.5, 11.0, 0.75, foreground);
                shape(17.0, 24.0, 2.0, 2.0, 1.0, foreground);
            }
        }
    }
}

#[cfg(test)]
#[path = "dialog_tests.rs"]
mod tests;
