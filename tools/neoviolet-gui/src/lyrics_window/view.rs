//! Lyrics content and outline rendering.
use iced::widget::{
    column, container, mouse_area, rich_text, scrollable, space, span, text, tooltip,
};
use iced::{Color, Element, Length};

use crate::lyrics_text::{emphasized, outlined};

use super::controls::hint;
use super::geometry::{measure_text, primary_font};
use super::{Event, Highlight, Overlay, Preferences};

impl Overlay {
    pub fn view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let alpha = preferences.opacity;
        let sung = match preferences.highlight {
            Highlight::Violet => Color::from_rgb8(224, 189, 255),
            Highlight::White => Color::WHITE,
        }
        .scale_alpha(alpha);
        let frame = self.display_frame();
        let active = frame.display_lines();
        let mut lines = column![]
            .spacing(12)
            .align_x(iced::Center)
            .width(Length::Fill);
        if active.is_empty() {
            let label = if frame.next_line().is_some() || !frame.lines.is_empty() {
                "···"
            } else if frame.title.is_empty() && frame.artist.is_empty() {
                "Waiting for music"
            } else {
                "No lyrics"
            };
            lines = lines.push(caption(label.to_owned(), 17.0, 0.85 * alpha, alpha));
        }
        for line in active {
            let mut fragments = Vec::new();
            let mut emphasis = Vec::new();
            if !line.prefix.is_empty() {
                fragments.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(sung),
                );
                emphasis.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(Color::TRANSPARENT),
                );
            }
            for (fragment, intensity) in line.fragments(frame.elapsed, |text| {
                measure_text(text, preferences.font_size as f32, f32::INFINITY).width
            }) {
                let grey = Color::from_rgb8(181, 177, 188);
                fragments.push(span(fragment).color(Color {
                    r: grey.r + (sung.r - grey.r) * intensity,
                    g: grey.g + (sung.g - grey.g) * intensity,
                    b: grey.b + (sung.b - grey.b) * intensity,
                    a: alpha,
                }));
                emphasis.push(span(fragment).color(Color {
                    r: sung.r * intensity,
                    g: sung.g * intensity,
                    b: sung.b * intensity,
                    a: 0.22 * alpha,
                }));
            }
            let shadow = fragments
                .iter()
                .cloned()
                .map(|fragment| {
                    fragment.color(Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * alpha))
                })
                .collect::<Vec<_>>();
            let make_text = |fragments| {
                rich_text::<(), _, _, _>(fragments)
                    .size(preferences.font_size)
                    .font(primary_font())
                    .align_x(iced::Center)
                    .width(Length::Fill)
            };
            let primary = emphasized(make_text(fragments), make_text(shadow), make_text(emphasis));
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
                            .font(crate::launch::interface_font())
                            .size(preferences.font_size as f32 * 0.46)
                            .shaping(iced::advanced::text::Shaping::Advanced)
                            .color(color)
                            .align_x(iced::Center)
                            .width(Length::Fill)
                    };
                    group = group.push(outlined(
                        additional_text(Color::WHITE.scale_alpha(0.95 * alpha)),
                        additional_text(Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * alpha)),
                        0.7,
                    ));
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
                hint(group, agent.as_str(), tooltip::Position::Bottom)
            };
            lines = lines.push(group);
        }
        if preferences.next_line
            && let Some(next) = frame.next_line()
        {
            lines = lines.push(caption(
                format!("{}{}", next.prefix, next.first_text()),
                13.0,
                0.8 * alpha,
                alpha,
            ));
        }
        let content: Element<'_, Event> = if self.embedded_toolbar {
            column![
                space().height(30),
                container(self.toolbar_content(preferences))
                    .height(34)
                    .center_x(Length::Fill),
                scrollable(lines).height(Length::Shrink),
            ]
            .spacing(8)
            .into()
        } else {
            scrollable(lines).height(Length::Shrink).into()
        };
        let area = mouse_area(
            container(content)
                .padding([8, 20])
                .center_x(Length::Fill)
                .height(Length::Fill),
        );
        #[cfg(not(target_os = "macos"))]
        let area = area
            .on_enter(Event::Hover(true))
            .on_exit(Event::Hover(false));
        area.into()
    }
}

fn caption<'a>(label: String, size: f32, opacity: f32, outline_opacity: f32) -> Element<'a, Event> {
    let make_text = |label, color| {
        text(label)
            .font(crate::launch::interface_font())
            .size(size)
            .shaping(iced::advanced::text::Shaping::Advanced)
            .color(color)
            .align_x(iced::Center)
            .width(Length::Fill)
    };
    outlined(
        make_text(label.clone(), Color::WHITE.scale_alpha(opacity)),
        make_text(
            label,
            Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * outline_opacity),
        ),
        0.7,
    )
}
