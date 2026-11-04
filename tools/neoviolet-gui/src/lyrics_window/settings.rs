//! Desktop lyrics preference controls.
use iced::widget::{
    button, column, container, row, rule, scrollable, slider, space, text, toggler,
};
use iced::{Color, Element, Font, Length};

use super::controls::{control_style, slider_style};
use super::{Event, Highlight, Overlay, Preferences};

impl Overlay {
    pub fn settings_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let label = |name, value| {
            row![
                text(name).size(12).color(Color::from_rgb8(206, 204, 214)),
                space().width(Length::Fill),
                text(value).size(12).color(Color::from_rgb8(228, 220, 242)),
            ]
            .align_y(iced::Center)
        };
        let heading = |name| {
            text(name)
                .size(12)
                .font(Font {
                    weight: iced::font::Weight::Semibold,
                    ..Font::DEFAULT
                })
                .color(Color::from_rgb8(236, 232, 242))
        };
        let toggle = |title, checked, event: fn(bool) -> Event| {
            row![
                text(title).size(12).color(Color::from_rgb8(206, 204, 214)),
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
            button(
                row![
                    text(if selected { "✓" } else { "" }).size(11).width(12),
                    text(name).size(12)
                ]
                .spacing(5),
            )
            .padding([5, 12])
            .on_press(Event::Highlight(color))
            .style(move |_, status| control_style(selected, status))
        };
        let appearance = column![
            heading("Appearance"),
            label("Text size", format!("{} px", preferences.font_size)),
            slider(22.0..=48.0, preferences.font_size as f32, Event::FontSize)
                .step(1.0_f32)
                .height(14)
                .style(slider_style),
            label("Opacity", format!("{:.0}%", preferences.opacity * 100.0)),
            slider(0.4..=1.0, preferences.opacity, Event::Opacity)
                .step(0.01_f32)
                .height(14)
                .style(slider_style),
            text("Highlight color")
                .size(12)
                .color(Color::from_rgb8(206, 204, 214)),
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
                )
            ]
            .spacing(6),
        ]
        .spacing(8);
        let lines = column![
            heading("Lines"),
            toggle(
                "Additional lines",
                preferences.additional_lines,
                Event::AdditionalLines
            ),
            toggle("Next line", preferences.next_line, Event::NextLine),
        ]
        .spacing(10);
        let window = column![
            heading("Window"),
            toggle("Lock position", preferences.locked, Event::Locked)
        ]
        .spacing(10);
        let content = column![
            toggle("Show lyrics", preferences.enabled, Event::Visible),
            rule::horizontal(1).style(rule::weak),
            appearance,
            rule::horizontal(1).style(rule::weak),
            lines,
            rule::horizontal(1).style(rule::weak),
            window,
        ]
        .spacing(12);
        let header = column![
            text("Desktop Lyrics").size(17).font(Font {
                weight: iced::font::Weight::Semibold,
                ..Font::DEFAULT
            }),
            text("Display settings")
                .size(11)
                .color(Color::from_rgb8(160, 157, 170))
        ]
        .spacing(3);
        let footer = row![
            button(text("Reset position").size(11))
                .on_press(Event::ResetPosition)
                .padding([7, 0])
                .style(button::text),
            space().width(Length::Fill),
            button(text("Close").size(12))
                .on_press(Event::CloseSettings)
                .padding([7, 18])
                .style(|_, status| control_style(true, status)),
        ]
        .align_y(iced::Center);
        container(
            column![
                header,
                scrollable(container(content).padding(iced::Padding::ZERO.right(8)))
                    .height(Length::Fill),
                rule::horizontal(1).style(rule::weak),
                footer,
            ]
            .spacing(14),
        )
        .padding(20)
        .width(Length::Fill)
        .height(Length::Fill)
        .style(|_| container::Style {
            background: Some(Color::from_rgb8(32, 32, 35).into()),
            ..Default::default()
        })
        .into()
    }
}
