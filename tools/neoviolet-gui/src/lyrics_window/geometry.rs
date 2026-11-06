//! Lyrics sizing and text measurement.
use iced::widget::span;
use iced::{Font, Point, Size, Task, window};

use super::{Event, Overlay, Preferences, TOOLBAR_OFFSET, TOOLBAR_SIZE};

impl Overlay {
    pub fn display_size(&self, preferences: &Preferences) -> Size {
        let frame = self.display_frame();
        let active = frame.display_lines();
        let toolbar_height = if self.embedded_toolbar {
            TOOLBAR_OFFSET
        } else {
            0.0
        };
        let next = preferences.next_line.then(|| frame.next_line()).flatten();
        if active.is_empty() && next.is_none() {
            return Size::new(300.0, 50.0 + toolbar_height);
        }
        let measure = |width| {
            let mut bounds = Size::ZERO;
            let mut groups = 0;
            for line in &active {
                if groups > 0 {
                    bounds.height += 12.0;
                }
                groups += 1;
                let primary = measure_primary(line, preferences.font_size as f32, width);
                bounds.width = bounds.width.max(primary.width);
                bounds.height += primary.height;
                if preferences.additional_lines {
                    for additional in line
                        .additional_lines()
                        .iter()
                        .filter(|line| !line.trim().is_empty())
                    {
                        let size =
                            measure_caption(additional, preferences.font_size as f32 * 0.46, width);
                        bounds.width = bounds.width.max(size.width);
                        bounds.height += 5.0 + size.height;
                    }
                }
            }
            if active.is_empty() {
                let quiet = measure_caption("···", 17.0, width);
                bounds.width = bounds.width.max(quiet.width);
                bounds.height += quiet.height;
                groups += 1;
            }
            if let Some(next) = next {
                let size = measure_caption(
                    &format!("{}{}", next.prefix, next.first_text()),
                    13.0,
                    width,
                );
                bounds.width = bounds.width.max(size.width);
                bounds.height += if groups > 0 { 12.0 } else { 0.0 };
                bounds.height += size.height;
            }
            bounds
        };
        let width = (measure(f32::INFINITY).width.ceil() + 40.0).clamp(300.0, 880.0);
        let height = (measure(width - 40.0).height.ceil() + 16.0).clamp(50.0, 420.0);
        Size::new(width, height + toolbar_height)
    }

    pub fn fit(&mut self, preferences: &Preferences) -> Task<Event> {
        if !self.opened || self.locating {
            return Task::none();
        }
        let Some(id) = self.window else {
            return Task::none();
        };
        self.fit_revision = self.fit_revision.wrapping_add(1);
        let size = self.display_size(preferences);
        if size == self.size {
            return Task::none();
        }
        let revision = self.fit_revision;
        if let Some(position) = self.toolbar_position {
            return Task::done(Event::Sized(id, revision, position, self.size, size));
        }
        window::position(id).then(move |position| {
            window::size(id).map(move |previous| {
                Event::Sized(
                    id,
                    revision,
                    position.unwrap_or(Point::ORIGIN),
                    previous,
                    size,
                )
            })
        })
    }

    pub(super) fn lyrics_position(&self, toolbar: Point, size: Size) -> Point {
        Point::new(
            toolbar.x + (TOOLBAR_SIZE.width - size.width) / 2.0,
            toolbar.y + TOOLBAR_OFFSET,
        )
    }
}

pub(super) fn primary_font() -> Font {
    Font {
        weight: iced::font::Weight::Semibold,
        ..crate::launch::interface_font()
    }
}

fn measure_primary(line: &crate::lyrics::Line, size: f32, width: f32) -> Size {
    use iced::advanced::text::{Paragraph, Shaping};
    let mut spans: Vec<iced::advanced::text::Span<'_, (), Font>> = Vec::new();
    if !line.prefix.is_empty() {
        spans.push(span(line.prefix.as_str()).size(size * 0.5));
    }
    spans.extend(
        line.fragments(0.0, |_| 1.0)
            .into_iter()
            .map(|(text, _)| span(text)),
    );
    iced::advanced::graphics::text::paragraph::Paragraph::with_spans(iced::advanced::Text {
        content: spans.as_slice(),
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: Default::default(),
        font: primary_font(),
        align_x: iced::advanced::text::Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Default::default(),
    })
    .min_bounds()
}

fn measure_caption(content: &str, size: f32, width: f32) -> Size {
    measure_with_font(content, size, width, crate::launch::interface_font())
}

pub(super) fn measure_text(content: &str, size: f32, width: f32) -> Size {
    measure_with_font(content, size, width, primary_font())
}

fn measure_with_font(content: &str, size: f32, width: f32, font: Font) -> Size {
    use iced::advanced::text::{Alignment, LineHeight, Paragraph, Shaping};
    iced::advanced::graphics::text::paragraph::Paragraph::with_text(iced::advanced::Text {
        content,
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: LineHeight::default(),
        font,
        align_x: Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Default::default(),
    })
    .min_bounds()
}
