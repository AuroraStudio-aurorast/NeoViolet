//! Titlebar controls, menu styling and vector glyphs.
use super::{MENU_BUTTON_WIDTH, MENU_HEADING, Message};
use iced::widget::{button, canvas, container, row, text, tooltip};
use iced::{Border, Color, Element, Length, Point, Shadow, Theme, Vector};

pub(super) fn chrome_background(color: Color) -> container::Style {
    container::Style {
        background: Some(color.into()),
        ..Default::default()
    }
}
pub(super) fn menu_trigger() -> Element<'static, Message> {
    button(
        row![icon(Glyph::Menu), text("Menu").size(12)]
            .spacing(6)
            .align_y(iced::Center),
    )
    .on_press(Message::ToggleMenu)
    .padding([7, 8])
    .width(MENU_BUTTON_WIDTH)
    .style(chrome_button)
    .into()
}
pub(super) fn menu_heading(label: &'static str) -> Element<'static, Message> {
    container(
        text(label)
            .size(11)
            .font(iced::Font {
                weight: iced::font::Weight::Semibold,
                ..crate::launch::interface_font()
            })
            .color(Color::from_rgb8(164, 164, 176)),
    )
    .height(MENU_HEADING)
    .padding([4, 9])
    .width(Length::Fill)
    .into()
}
pub(super) fn menu_panel<'a>(
    content: impl Into<Element<'a, Message>>,
    width: f32,
    max_height: f32,
) -> Element<'a, Message> {
    container(content)
        .width(width)
        .max_height(max_height)
        .padding(5)
        .style(|_| container::Style {
            background: Some(Color::from_rgb8(47, 47, 47).into()),
            text_color: Some(Color::from_rgb8(228, 228, 228)),
            border: Border {
                color: Color::from_rgb8(77, 77, 77),
                width: 1.0,
                radius: 6.0.into(),
            },
            shadow: Shadow {
                color: Color::BLACK.scale_alpha(0.3),
                offset: Vector::new(0.0, 6.0),
                blur_radius: 16.0,
            },
            ..Default::default()
        })
        .into()
}
pub(super) fn menu_button(theme: &Theme, status: button::Status, selected: bool) -> button::Style {
    let mut style = button::text(theme, status);
    style.text_color = Color::from_rgb8(222, 222, 222);
    if matches!(status, button::Status::Disabled) {
        style.text_color = Color::from_rgb8(110, 110, 110);
        return style;
    }
    if selected || matches!(status, button::Status::Hovered | button::Status::Pressed) {
        style.background = Some(Color::from_rgb8(82, 68, 101).into());
    }
    style.border.radius = 4.0.into();
    style
}
fn chrome_button(theme: &Theme, status: button::Status) -> button::Style {
    let mut style = button::text(theme, status);
    style.text_color = Color::from_rgb8(200, 200, 200);
    if matches!(status, button::Status::Hovered | button::Status::Pressed) {
        style.background = Some(Color::WHITE.scale_alpha(0.09).into());
    }
    style.border.radius = 4.0.into();
    style
}
pub(super) fn caption_style(
    theme: &Theme,
    status: button::Status,
    linux: bool,
    close: bool,
) -> button::Style {
    let mut style = chrome_button(theme, status);
    style.border.radius = if linux { 12.0.into() } else { 0.0.into() };
    if !linux && close && matches!(status, button::Status::Hovered | button::Status::Pressed) {
        style.background = Some(Color::from_rgb8(232, 17, 35).into());
        style.text_color = Color::WHITE;
    }
    style
}
pub(super) fn hint<'a>(
    content: impl Into<Element<'a, Message>>,
    label: &'static str,
) -> Element<'a, Message> {
    tooltip(
        content,
        container(text(label).size(11)).padding([6, 9]),
        tooltip::Position::Bottom,
    )
    .gap(6)
    .padding(0)
    .delay(std::time::Duration::from_millis(450))
    .style(|_| container::Style {
        background: Some(Color::from_rgb8(48, 48, 48).into()),
        text_color: Some(Color::from_rgb8(232, 232, 232)),
        border: Border {
            color: Color::from_rgb8(80, 80, 80),
            width: 1.0,
            radius: 5.0.into(),
        },
        ..Default::default()
    })
    .into()
}
#[derive(Clone, Copy)]
pub(super) enum Glyph {
    Menu,
    Minimize,
    Maximize,
    Restore,
    Close,
    Chevron,
}
struct Drawing(Glyph);
pub(super) fn icon(kind: Glyph) -> Element<'static, Message> {
    canvas(Drawing(kind)).width(16).height(16).into()
}
impl canvas::Program<Message> for Drawing {
    type State = ();
    fn draw(
        &self,
        _: &(),
        renderer: &iced::Renderer,
        _: &Theme,
        bounds: iced::Rectangle,
        _: iced::mouse::Cursor,
    ) -> Vec<canvas::Geometry> {
        let mut frame = canvas::Frame::new(renderer, bounds.size());
        let path = canvas::Path::new(|p| {
            let mut line = |a, b| {
                p.move_to(Point::from(a));
                p.line_to(Point::from(b));
            };
            match self.0 {
                Glyph::Menu => {
                    for y in [3.0, 8.0, 13.0] {
                        line((2.0, y), (14.0, y));
                    }
                }
                Glyph::Minimize => line((3.0, 10.0), (13.0, 10.0)),
                Glyph::Maximize => {
                    line((3.0, 3.0), (13.0, 3.0));
                    line((13.0, 3.0), (13.0, 13.0));
                    line((13.0, 13.0), (3.0, 13.0));
                    line((3.0, 13.0), (3.0, 3.0));
                }
                Glyph::Restore => {
                    line((5.0, 5.0), (13.0, 5.0));
                    line((13.0, 5.0), (13.0, 13.0));
                    line((13.0, 13.0), (5.0, 13.0));
                    line((5.0, 13.0), (5.0, 5.0));
                    line((3.0, 11.0), (3.0, 3.0));
                    line((3.0, 3.0), (11.0, 3.0));
                }
                Glyph::Close => {
                    line((4.0, 4.0), (12.0, 12.0));
                    line((4.0, 12.0), (12.0, 4.0));
                }
                Glyph::Chevron => {
                    line((6.0, 4.0), (10.0, 8.0));
                    line((10.0, 8.0), (6.0, 12.0));
                }
            }
        });
        frame.stroke(
            &path,
            canvas::Stroke::default()
                .with_width(1.1)
                .with_color(Color::from_rgb8(211, 211, 211)),
        );
        vec![frame.into_geometry()]
    }
}
