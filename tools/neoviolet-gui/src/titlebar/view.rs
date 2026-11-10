//! Title area, application menus and resize regions.
use super::widgets::{
    Glyph, caption_style, chrome_background, hint, icon, menu_button, menu_heading, menu_panel,
    menu_trigger,
};
use super::{
    CaptionControl, HEIGHT, MENU_HEADING, MENU_WIDTH, Message, Platform, ROOT_WIDTH, ROW_HEIGHT,
    State,
};
use crate::actions::Action;
use iced::widget::{
    button, column, container, mouse_area, opaque, pin, row, scrollable, stack, text,
};
use iced::{Color, Element, Length, window};

impl State {
    pub fn wrap<'a>(
        &'a self,
        content: Element<'a, crate::app::Message>,
        title: &'a str,
        lyrics: bool,
    ) -> Element<'a, crate::app::Message> {
        self.wrap_for(content, title, lyrics, Platform::current())
    }

    pub(super) fn wrap_for<'a>(
        &'a self,
        content: Element<'a, crate::app::Message>,
        title: &'a str,
        lyrics: bool,
        platform: Platform,
    ) -> Element<'a, crate::app::Message> {
        let mut bar = row![].align_y(iced::Center).height(HEIGHT);
        if platform == Platform::MacOs {
            bar = bar.push(iced::widget::Space::new().width(self.mac_title_inset.unwrap_or(78.0)));
        } else if !self.menu_bar {
            bar = bar.push(hint(menu_trigger(), "Application menu"));
        }
        let title = mouse_area(
            container(
                text(title)
                    .size(13)
                    .font(iced::Font {
                        weight: iced::font::Weight::Semibold,
                        ..crate::launch::interface_font()
                    })
                    .color(Color::from_rgb8(224, 224, 230))
                    .wrapping(text::Wrapping::None),
            )
            .width(Length::Fill)
            .height(HEIGHT)
            .padding([0, 10])
            .center_y(Length::Fill)
            .clip(true),
        )
        .on_press(Message::PressTitle)
        .on_release(Message::ReleaseTitle)
        .on_double_click(Message::DoubleClick);
        bar = bar.push(title);
        if platform != Platform::MacOs && !self.system_decorations {
            let mut controls = row![]
                .spacing(if platform == Platform::Linux { 8 } else { 0 })
                .align_y(iced::Center);
            for (glyph, label, action) in [
                (Glyph::Minimize, "Minimize", Action::Minimize),
                (
                    if self.maximized {
                        Glyph::Restore
                    } else {
                        Glyph::Maximize
                    },
                    if self.maximized {
                        "Restore"
                    } else {
                        "Maximize"
                    },
                    Action::ZoomWindow,
                ),
                (Glyph::Close, "Close NeoViolet", Action::Quit),
            ] {
                if matches!(action, Action::Minimize) && !self.minimize_button {
                    continue;
                }
                let linux = platform == Platform::Linux;
                let native_hover = platform == Platform::Windows
                    && self.caption_hover
                        == Some(match action {
                            Action::Minimize => CaptionControl::Minimize,
                            Action::ZoomWindow => CaptionControl::Maximize,
                            _ => CaptionControl::Close,
                        });
                let control = button(
                    container(icon(glyph))
                        .center_x(Length::Fill)
                        .center_y(Length::Fill),
                )
                .on_press(Message::WindowAction(action))
                .padding(0)
                .width(if linux { 24 } else { 42 })
                .height(if linux { 24 } else { HEIGHT as u32 })
                .style(move |theme, status| {
                    let status = if platform == Platform::Windows {
                        if native_hover {
                            button::Status::Hovered
                        } else {
                            button::Status::Active
                        }
                    } else {
                        status
                    };
                    caption_style(theme, status, linux, matches!(glyph, Glyph::Close))
                });
                controls = controls.push(hint(control, label));
            }
            bar = bar.push(
                container(controls).padding([0, if platform == Platform::Linux { 10 } else { 0 }]),
            );
        }
        let groups = self.groups(platform, lyrics);
        let mut chrome = column![];
        let background = self.chrome_color();
        let caption_height = if self.system_decorations {
            if self.menu_bar { 0.0 } else { HEIGHT }
        } else {
            HEIGHT
        };
        if caption_height > 0.0 {
            let bar: Element<'a, Message> = if self.system_decorations {
                container(menu_trigger())
                    .width(Length::Fill)
                    .height(HEIGHT)
                    .into()
            } else {
                bar.into()
            };
            chrome = chrome.push(container(bar).style(move |_| chrome_background(background)));
        }
        if self.menu_bar {
            let headings = row(groups.iter().enumerate().map(|(index, group)| {
                let selected = self.menu_open && self.selected_group == Some(index);
                let trigger = button(text(group.name).size(12))
                    .padding([5, 9])
                    .height(28)
                    .on_press(Message::SelectGroup(index))
                    .style(move |theme, status| menu_button(theme, status, selected));
                mouse_area(trigger)
                    .on_enter(Message::HoverGroup(index))
                    .into()
            }))
            .height(28);
            chrome = chrome.push(
                container(headings)
                    .width(Length::Fill)
                    .style(move |_| chrome_background(background)),
            );
        }
        let chrome_height = caption_height + if self.menu_bar { 28.0 } else { 0.0 };
        let base: Element<'a, crate::app::Message> = if self.fullscreen {
            content
        } else {
            column![
                Element::from(
                    mouse_area(chrome)
                        .on_enter(Message::Hover(true))
                        .on_exit(Message::Hover(false))
                )
                .map(crate::app::Message::Titlebar),
                content
            ]
            .height(Length::Fill)
            .into()
        };
        if !self.menu_open {
            return self.resize_edges(base, platform);
        }
        let menu_y = if self.fullscreen {
            8.0
        } else {
            chrome_height + 4.0
        };
        let max_height = (self.size.height - menu_y - 8.0).max(80.0);
        let root = column(groups.iter().enumerate().map(|(index, group)| {
            let selected = self.selected_group == Some(index);
            let item = button(row![
                text(group.name).size(12).font(iced::Font {
                    weight: iced::font::Weight::Medium,
                    ..crate::launch::interface_font()
                }),
                iced::widget::Space::new().width(Length::Fill),
                icon(Glyph::Chevron)
            ])
            .padding([6, 9])
            .height(ROW_HEIGHT)
            .width(Length::Fill)
            .on_press(Message::SelectGroup(index))
            .style(move |theme, status| menu_button(theme, status, selected));
            mouse_area(item).on_enter(Message::HoverGroup(index)).into()
        }))
        .spacing(1);
        let root: Element<'a, Message> = menu_panel(
            scrollable(column![menu_heading("NeoViolet"), root]).height(Length::Shrink),
            ROOT_WIDTH,
            max_height,
        );
        let dismiss_y = if self.fullscreen { 0.0 } else { chrome_height };
        let dismiss = mouse_area(
            container(iced::widget::Space::new())
                .width(Length::Fill)
                .height((self.size.height - dismiss_y).max(0.0)),
        )
        .on_press(Message::Dismiss)
        .on_right_press(Message::Dismiss);
        let mut layers = stack![
            base,
            pin(opaque(
                Element::from(dismiss).map(crate::app::Message::Titlebar)
            ))
            .y(dismiss_y)
        ]
        .width(Length::Fill)
        .height(Length::Fill);
        if !self.menu_bar {
            layers = layers.push(
                pin(opaque(root.map(crate::app::Message::Titlebar)))
                    .x(8)
                    .y(menu_y),
            );
        }
        if let Some(index) = self.selected_group {
            let group = &groups[index];
            let items = column(group.items.iter().enumerate().map(|(item_index, item)| {
                let selected = self.in_submenu && self.selected_item == item_index;
                let label = row![
                    text(if item.checked { "✓" } else { "" }).size(12).width(16),
                    text(item.label).size(12),
                    iced::widget::Space::new().width(Length::Fill),
                    text(item.shortcut.unwrap_or(""))
                        .size(11)
                        .color(Color::from_rgb8(175, 175, 185))
                ]
                .spacing(6)
                .align_y(iced::Center);
                let item = button(label)
                    .on_press_maybe(
                        self.item_enabled(item, lyrics)
                            .then_some(Message::Action(item.action)),
                    )
                    .padding([6, 8])
                    .height(ROW_HEIGHT)
                    .width(Length::Fill)
                    .style(move |theme, status| menu_button(theme, status, selected));
                let item: Element<'a, Message> = mouse_area(item)
                    .on_enter(Message::HoverItem(item_index))
                    .into();
                let separate = matches!(
                    (group.name, item_index),
                    ("File", 1) | ("Edit", 2) | ("View", 3) | ("Help", 1)
                );
                if separate {
                    column![
                        container(iced::widget::rule::horizontal(1)).padding([4, 4]),
                        item
                    ]
                    .into()
                } else {
                    item
                }
            }))
            .spacing(1);
            let panel = menu_panel(
                scrollable(column![menu_heading(group.name), items]).height(Length::Shrink),
                MENU_WIDTH,
                max_height,
            );
            let x = if self.menu_bar {
                [8.0, 50.0, 94.0, 140.0, 202.0][index]
            } else {
                8.0 + ROOT_WIDTH - 1.0
            };
            let x = x.min((self.size.width - MENU_WIDTH - 8.0).max(8.0));
            let y = if self.menu_bar {
                menu_y
            } else {
                (menu_y + MENU_HEADING + index as f32 * (ROW_HEIGHT + 1.0)).min(
                    (self.size.height - group.items.len() as f32 * 31.0 - MENU_HEADING - 32.0)
                        .max(menu_y),
                )
            };
            layers = layers.push(
                pin(opaque(panel.map(crate::app::Message::Titlebar)))
                    .x(x)
                    .y(y),
            );
        }
        layers.into()
    }

    fn resize_edges<'a>(
        &self,
        base: Element<'a, crate::app::Message>,
        platform: Platform,
    ) -> Element<'a, crate::app::Message> {
        if platform != Platform::Linux
            || self.system_decorations
            || self.fullscreen
            || self.maximized
        {
            return base;
        }
        let mut layers = stack![base].width(Length::Fill).height(Length::Fill);
        use window::Direction::*;
        let w = self.size.width;
        let h = self.size.height;
        for (x, y, width, height, direction) in [
            (6.0, 0.0, (w - 12.0).max(0.0), 4.0, North),
            (6.0, h - 4.0, (w - 12.0).max(0.0), 4.0, South),
            (0.0, 6.0, 4.0, (h - 12.0).max(0.0), West),
            (w - 4.0, 6.0, 4.0, (h - 12.0).max(0.0), East),
            (0.0, 0.0, 6.0, 6.0, NorthWest),
            (w - 6.0, 0.0, 6.0, 6.0, NorthEast),
            (0.0, h - 6.0, 6.0, 6.0, SouthWest),
            (w - 6.0, h - 6.0, 6.0, 6.0, SouthEast),
        ] {
            let interaction = match direction {
                North | South => iced::mouse::Interaction::ResizingVertically,
                West | East => iced::mouse::Interaction::ResizingHorizontally,
                NorthWest | SouthEast => iced::mouse::Interaction::ResizingDiagonallyDown,
                _ => iced::mouse::Interaction::ResizingDiagonallyUp,
            };
            let area = mouse_area(iced::widget::Space::new().width(width).height(height))
                .on_press(Message::Resize(direction))
                .interaction(interaction);
            layers = layers.push(
                pin(Element::from(area).map(crate::app::Message::Titlebar))
                    .x(x.max(0.0))
                    .y(y.max(0.0)),
            );
        }
        layers.into()
    }
}
