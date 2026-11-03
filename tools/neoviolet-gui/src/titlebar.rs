//! Main-window chrome and local menu navigation. Playback remains in the TUI.
use crate::actions::Action;
use iced::keyboard::{Key, key::Named};
use iced::widget::{
    button, canvas, column, container, mouse_area, opaque, pin, row, scrollable, stack, text,
    tooltip,
};
use iced::{Border, Color, Element, Length, Point, Shadow, Size, Theme, Vector, window};

pub const HEIGHT: f32 = 34.0;
const ROOT_WIDTH: f32 = 164.0;
const MENU_WIDTH: f32 = 282.0;
const ROW_HEIGHT: f32 = 30.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
enum Platform {
    MacOs,
    Windows,
    Linux,
}
impl Platform {
    fn current() -> Self {
        #[cfg(target_os = "macos")]
        {
            Self::MacOs
        }
        #[cfg(target_os = "windows")]
        {
            Self::Windows
        }
        #[cfg(not(any(target_os = "macos", target_os = "windows")))]
        {
            Self::Linux
        }
    }
}

struct MenuItem {
    label: &'static str,
    shortcut: Option<&'static str>,
    action: Action,
    checked: bool,
}

impl MenuItem {
    fn enabled(&self, lyrics: bool) -> bool {
        !matches!(self.action, Action::UnlockLyrics) || lyrics
    }
}

struct MenuGroup {
    name: &'static str,
    items: Vec<MenuItem>,
}

impl MenuGroup {
    fn all(platform: Platform, lyrics_enabled: bool, maximized: bool) -> Vec<Self> {
        let lyrics_shortcut = match platform {
            Platform::MacOs => "⌘⇧L",
            Platform::Windows | Platform::Linux => "Ctrl+Shift+L",
        };
        vec![
            Self {
                name: "File",
                items: vec![
                    MenuItem {
                        label: "Open File…",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘O"
                        } else {
                            "Ctrl+O"
                        }),
                        action: Action::Open,
                        checked: false,
                    },
                    MenuItem {
                        label: if platform == Platform::Windows {
                            "Exit"
                        } else {
                            "Quit"
                        },
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘Q"
                        } else {
                            "Ctrl+Q"
                        }),
                        action: Action::Quit,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Edit",
                items: vec![
                    MenuItem {
                        label: "Copy",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘C"
                        } else {
                            "Ctrl+C"
                        }),
                        action: Action::Copy,
                        checked: false,
                    },
                    MenuItem {
                        label: "Paste",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘V"
                        } else {
                            "Ctrl+V"
                        }),
                        action: Action::Paste,
                        checked: false,
                    },
                    MenuItem {
                        label: "Preferences…",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘,"
                        } else {
                            "Ctrl+,"
                        }),
                        action: Action::Preferences,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "View",
                items: vec![
                    MenuItem {
                        label: "Zoom In",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘+"
                        } else {
                            "Ctrl++"
                        }),
                        action: Action::ZoomIn,
                        checked: false,
                    },
                    MenuItem {
                        label: "Zoom Out",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘−"
                        } else {
                            "Ctrl+−"
                        }),
                        action: Action::ZoomOut,
                        checked: false,
                    },
                    MenuItem {
                        label: "Actual Size",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘0"
                        } else {
                            "Ctrl+0"
                        }),
                        action: Action::ZoomReset,
                        checked: false,
                    },
                    MenuItem {
                        label: "Desktop Lyrics",
                        shortcut: Some(lyrics_shortcut),
                        action: Action::DesktopLyrics,
                        checked: lyrics_enabled,
                    },
                    MenuItem {
                        label: "Desktop Lyrics Settings…",
                        shortcut: None,
                        action: Action::LyricsSettings,
                        checked: false,
                    },
                    MenuItem {
                        label: "Unlock Desktop Lyrics",
                        shortcut: None,
                        action: Action::UnlockLyrics,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Window",
                items: vec![
                    MenuItem {
                        label: "Minimize",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌘M"
                        } else {
                            ""
                        }),
                        action: Action::Minimize,
                        checked: false,
                    },
                    MenuItem {
                        label: if maximized {
                            "Restore"
                        } else if platform == Platform::MacOs {
                            "Zoom"
                        } else {
                            "Maximize"
                        },
                        shortcut: None,
                        action: Action::ZoomWindow,
                        checked: false,
                    },
                    MenuItem {
                        label: "Toggle Full Screen",
                        shortcut: Some(if platform == Platform::MacOs {
                            "⌃⌘F"
                        } else {
                            "F11"
                        }),
                        action: Action::Fullscreen,
                        checked: false,
                    },
                ],
            },
            Self {
                name: "Help",
                items: vec![
                    MenuItem {
                        label: "Project Repository",
                        shortcut: None,
                        action: Action::Repository,
                        checked: false,
                    },
                    MenuItem {
                        label: "About NeoViolet",
                        shortcut: None,
                        action: Action::About,
                        checked: false,
                    },
                ],
            },
        ]
    }
}

#[derive(Debug, Clone)]
pub enum Message {
    ToggleMenu,
    SelectGroup(usize),
    HoverGroup(usize),
    HoverItem(usize),
    Dismiss,
    Action(Action),
    PressTitle,
    ReleaseTitle,
    DoubleClick,
    Resize(window::Direction),
}

#[derive(Debug, Default)]
pub struct State {
    menu_open: bool,
    selected_group: Option<usize>,
    selected_item: usize,
    in_submenu: bool,
    pub maximized: bool,
    pub fullscreen: bool,
    press: Option<Point>,
    pub size: Size,
    pub cursor: Point,
    pub menu_bar: bool,
    pub system_decorations: bool,
    pub minimize_button: bool,
}

impl State {
    pub fn new(size: Size) -> Self {
        Self {
            size,
            minimize_button: true,
            ..Self::default()
        }
    }
    pub fn update(&mut self, message: Message) -> Option<Action> {
        match message {
            Message::ToggleMenu => {
                self.menu_open = !self.menu_open;
                self.selected_group = if self.menu_open && self.menu_bar {
                    Some(0)
                } else {
                    None
                };
                self.selected_item = 0;
                self.in_submenu = false;
            }
            Message::SelectGroup(index) if index < 5 => {
                self.menu_open = true;
                self.selected_group = Some(index);
                self.selected_item = 0;
                self.in_submenu = false;
            }
            Message::HoverGroup(index)
                if self.menu_open && self.selected_group != Some(index) && index < 5 =>
            {
                self.selected_group = Some(index);
                self.selected_item = 0;
                self.in_submenu = false;
            }
            Message::HoverItem(index) => {
                self.selected_item = index;
                self.in_submenu = true;
            }
            Message::Dismiss => self.close(),
            Message::Action(action) => {
                self.close();
                return Some(action);
            }
            Message::PressTitle => {
                self.close();
                self.press = Some(self.cursor);
            }
            Message::ReleaseTitle | Message::DoubleClick => self.press = None,
            _ => {}
        }
        None
    }

    pub fn configure(&mut self, config: &crate::config::Config) {
        self.menu_bar = config.menu_bar && Platform::current() != Platform::MacOs;
        self.system_decorations =
            config.system_decorations && Platform::current() != Platform::MacOs;
        self.minimize_button = config.minimize_button || Platform::current() != Platform::Linux;
    }

    pub fn close(&mut self) {
        self.menu_open = false;
        self.selected_group = None;
        self.in_submenu = false;
        self.press = None;
    }

    pub fn take_drag(&mut self, position: Point) -> bool {
        if self
            .press
            .is_some_and(|start| start.distance(position) >= 3.0)
        {
            self.press = None;
            true
        } else {
            false
        }
    }

    pub fn visible(&self) -> bool {
        self.menu_open
    }

    fn groups(&self, platform: Platform, lyrics: bool) -> Vec<MenuGroup> {
        let mut groups = MenuGroup::all(platform, lyrics, self.maximized);
        if !self.minimize_button {
            groups[3]
                .items
                .retain(|item| !matches!(item.action, Action::Minimize));
        }
        groups
    }

    pub fn key(&mut self, key: &Key, lyrics: bool) -> Option<Action> {
        let groups = self.groups(Platform::current(), lyrics);
        match key {
            Key::Named(Named::Escape | Named::F10) => self.close(),
            Key::Named(Named::ArrowLeft) if self.menu_bar => {
                self.selected_group =
                    Some((self.selected_group.unwrap_or(0) + groups.len() - 1) % groups.len());
                self.selected_item = 0;
            }
            Key::Named(Named::ArrowLeft) => {
                self.in_submenu = false;
                self.selected_group = None;
            }
            Key::Named(Named::ArrowRight) if self.menu_bar => {
                self.selected_group = Some((self.selected_group.unwrap_or(0) + 1) % groups.len());
                self.selected_item = 0;
            }
            Key::Named(Named::ArrowRight) => {
                self.selected_group.get_or_insert(0);
                self.in_submenu = true;
                self.selected_item = 0;
            }
            Key::Named(Named::ArrowDown) if self.menu_bar && !self.in_submenu => {
                self.in_submenu = true;
                self.selected_item = 0;
            }
            Key::Named(Named::ArrowDown | Named::ArrowUp | Named::Home | Named::End) => {
                let count = if self.in_submenu {
                    groups[self.selected_group.unwrap_or(0)].items.len()
                } else {
                    groups.len()
                };
                let index = if self.in_submenu {
                    self.selected_item
                } else {
                    self.selected_group.unwrap_or(count - 1)
                };
                let next = match key {
                    Key::Named(Named::Home) => 0,
                    Key::Named(Named::End) => count - 1,
                    Key::Named(Named::ArrowUp) => (index + count - 1) % count,
                    _ => (index + 1) % count,
                };
                if self.in_submenu {
                    let items = &groups[self.selected_group.unwrap_or(0)].items;
                    self.selected_item = if items[next].enabled(lyrics) {
                        next
                    } else if matches!(key, Key::Named(Named::ArrowUp | Named::End)) {
                        (next + count - 1) % count
                    } else {
                        (next + 1) % count
                    };
                } else {
                    self.selected_group = Some(next);
                }
            }
            Key::Named(Named::Enter | Named::Space) => {
                if self.in_submenu {
                    let item = &groups[self.selected_group.unwrap_or(0)].items[self.selected_item];
                    if !item.enabled(lyrics) {
                        return None;
                    }
                    let action = item.action;
                    self.close();
                    return Some(action);
                }
                self.selected_group.get_or_insert(0);
                self.in_submenu = true;
                self.selected_item = 0;
            }
            Key::Character(letter) if letter.len() == 1 => {
                if self.in_submenu {
                    if let Some(index) = groups[self.selected_group.unwrap_or(0)]
                        .items
                        .iter()
                        .position(|item| {
                            item.label
                                .to_lowercase()
                                .starts_with(&letter.to_lowercase())
                        })
                    {
                        self.selected_item = index;
                    }
                } else if let Some(index) = groups.iter().position(|group| {
                    group
                        .name
                        .to_lowercase()
                        .starts_with(&letter.to_lowercase())
                }) {
                    self.selected_group = Some(index);
                }
            }
            _ => {}
        }
        None
    }

    pub fn wrap<'a>(
        &'a self,
        content: Element<'a, crate::app::Message>,
        title: &'a str,
        lyrics: bool,
    ) -> Element<'a, crate::app::Message> {
        self.wrap_for(content, title, lyrics, Platform::current())
    }

    fn wrap_for<'a>(
        &'a self,
        content: Element<'a, crate::app::Message>,
        title: &'a str,
        lyrics: bool,
        platform: Platform,
    ) -> Element<'a, crate::app::Message> {
        let mut bar = row![].align_y(iced::Center).height(HEIGHT);
        if platform == Platform::MacOs {
            bar = bar.push(iced::widget::Space::new().width(78));
        } else if !self.menu_bar {
            bar = bar.push(hint(
                button(icon(Glyph::Menu))
                    .on_press(Message::ToggleMenu)
                    .padding([7, 9])
                    .style(chrome_button),
                "Application menu",
            ));
        }
        let title = mouse_area(
            container(text(title).size(12).wrapping(text::Wrapping::None))
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
                let control = button(
                    container(icon(glyph))
                        .center_x(Length::Fill)
                        .center_y(Length::Fill),
                )
                .on_press(Message::Action(action))
                .padding(0)
                .width(if linux { 24 } else { 42 })
                .height(if linux { 24 } else { HEIGHT as u32 })
                .style(move |theme, status| {
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
        let caption_height = if self.system_decorations {
            if self.menu_bar { 0.0 } else { HEIGHT }
        } else {
            HEIGHT
        };
        if caption_height > 0.0 {
            let bar: Element<'a, Message> = if self.system_decorations {
                container(
                    button(icon(Glyph::Menu))
                        .padding([7, 9])
                        .on_press(Message::ToggleMenu)
                        .style(chrome_button),
                )
                .width(Length::Fill)
                .height(HEIGHT)
                .into()
            } else {
                bar.into()
            };
            chrome = chrome.push(container(bar).style(chrome_background));
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
                    .style(chrome_background),
            );
        }
        let chrome_height = caption_height + if self.menu_bar { 28.0 } else { 0.0 };
        let base: Element<'a, crate::app::Message> = if self.fullscreen {
            content
        } else {
            column![
                Element::from(chrome).map(crate::app::Message::Titlebar),
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
                text(group.name).size(12),
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
            scrollable(root).height(Length::Shrink),
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
                        .size(10)
                        .color(Color::from_rgb8(172, 172, 172))
                ]
                .spacing(6)
                .align_y(iced::Center);
                let item = button(label)
                    .on_press_maybe(item.enabled(lyrics).then_some(Message::Action(item.action)))
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
                scrollable(items).height(Length::Shrink),
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
                (menu_y + index as f32 * (ROW_HEIGHT + 1.0))
                    .min((self.size.height - group.items.len() as f32 * 31.0 - 14.0).max(menu_y))
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

fn chrome_background(_: &Theme) -> container::Style {
    container::Style {
        background: Some(Color::from_rgb8(43, 43, 43).into()),
        ..Default::default()
    }
}
fn menu_panel<'a>(
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
fn menu_button(theme: &Theme, status: button::Status, selected: bool) -> button::Style {
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
fn caption_style(theme: &Theme, status: button::Status, linux: bool, close: bool) -> button::Style {
    let mut style = chrome_button(theme, status);
    style.border.radius = if linux { 12.0.into() } else { 0.0.into() };
    if !linux && close && matches!(status, button::Status::Hovered | button::Status::Pressed) {
        style.background = Some(Color::from_rgb8(232, 17, 35).into());
        style.text_color = Color::WHITE;
    }
    style
}
fn hint<'a>(content: impl Into<Element<'a, Message>>, label: &'static str) -> Element<'a, Message> {
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
enum Glyph {
    Menu,
    Minimize,
    Maximize,
    Restore,
    Close,
    Chevron,
}
struct Drawing(Glyph);
fn icon(kind: Glyph) -> Element<'static, Message> {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn windows_and_linux_menus_are_grouped_and_platform_shortcuts_are_distinct() {
        let windows = MenuGroup::all(Platform::Windows, true, false);
        let linux = MenuGroup::all(Platform::Linux, true, false);
        assert_eq!(
            windows.iter().map(|group| group.name).collect::<Vec<_>>(),
            ["File", "Edit", "View", "Window", "Help"]
        );
        assert_eq!(windows[0].items[0].shortcut, Some("Ctrl+O"));
        assert_eq!(linux[0].items[0].shortcut, Some("Ctrl+O"));
        assert_eq!(windows[2].items[3].shortcut, Some("Ctrl+Shift+L"));
    }

    #[test]
    fn menu_state_tracks_lyrics_and_maximize_actions_without_theme_switching() {
        let normal = MenuGroup::all(Platform::Linux, false, false);
        let maximized = MenuGroup::all(Platform::Linux, true, true);
        let lyrics = &normal[2].items;
        assert!(
            !lyrics
                .iter()
                .any(|item| item.label == "Desktop Lyrics" && item.checked)
        );
        assert!(
            maximized[2]
                .items
                .iter()
                .any(|item| item.label == "Desktop Lyrics" && item.checked)
        );
        assert!(
            !lyrics
                .iter()
                .any(|item| item.label.contains("Appearance") || item.label.contains("Theme"))
        );
        assert_eq!(normal[3].items[1].label, "Maximize");
        assert_eq!(maximized[3].items[1].label, "Restore");
    }

    #[test]
    fn menu_dismissal_returns_from_submenu_before_closing() {
        let mut state = State::default();
        assert!(state.update(Message::ToggleMenu).is_none());
        assert!(state.visible());
        state.update(Message::SelectGroup(2));
        state.key(&Key::Named(Named::ArrowLeft), false);
        assert!(state.visible());
        state.update(Message::Dismiss);
        assert!(!state.visible());
    }
}

#[cfg(test)]
mod interaction_tests {
    use super::*;

    #[test]
    fn keyboard_traversal_dispatches_existing_actions_and_skips_unavailable_unlock() {
        let mut state = State::new(Size::new(480.0, 320.0));
        state.update(Message::ToggleMenu);
        state.key(&Key::Character("v".into()), false);
        assert_eq!(state.selected_group, Some(2));
        state.key(&Key::Named(Named::ArrowRight), false);
        state.key(&Key::Named(Named::End), false);
        assert_eq!(state.selected_item, 4);
        let action = state.key(&Key::Named(Named::Enter), false);
        assert!(matches!(action, Some(Action::LyricsSettings)));
        assert!(!state.visible());
    }

    #[test]
    fn press_only_starts_a_drag_after_movement_and_does_not_change_maximize_state() {
        let mut state = State::new(Size::new(800.0, 600.0));
        state.cursor = Point::new(200.0, 15.0);
        state.update(Message::PressTitle);
        assert!(!state.take_drag(Point::new(201.0, 15.0)));
        assert!(state.take_drag(Point::new(205.0, 15.0)));
        assert!(!state.take_drag(Point::new(210.0, 15.0)));
        assert!(!state.maximized);
    }

    #[test]
    fn headless_menu_layout_routes_clicks_to_submenus_without_reaching_terminal() {
        use iced::advanced::mouse;
        let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
            iced::Font::DEFAULT,
            14.0.into(),
        ));
        for platform in [Platform::Windows, Platform::Linux] {
            for size in [Size::new(480.0, 320.0), Size::new(1000.0, 700.0)] {
                let mut state = State::new(size);
                state.update(Message::ToggleMenu);
                state.update(Message::SelectGroup(2));
                let content = button("Terminal sentinel")
                    .on_press(crate::app::Message::ConfirmQuit)
                    .width(Length::Fill)
                    .height(Length::Fill);
                let view = state.wrap_for(
                    content.into(),
                    "NeoViolet | A very long title that must stay within the available space",
                    true,
                    platform,
                );
                let mut ui = iced_runtime::UserInterface::build(
                    view,
                    size,
                    iced_runtime::user_interface::Cache::new(),
                    &mut renderer,
                );
                let mut messages = Vec::new();
                let mut clipboard = iced::advanced::clipboard::Null;
                let position = Point::new(195.0, 110.0);
                let events = [
                    iced::Event::Mouse(mouse::Event::CursorMoved { position }),
                    iced::Event::Mouse(mouse::Event::ButtonPressed(mouse::Button::Left)),
                    iced::Event::Mouse(mouse::Event::ButtonReleased(mouse::Button::Left)),
                ];
                let _ = ui.update(
                    &events,
                    mouse::Cursor::Available(position),
                    &mut renderer,
                    &mut clipboard,
                    &mut messages,
                );
                assert!(
                    !messages
                        .iter()
                        .any(|message| matches!(message, crate::app::Message::ConfirmQuit))
                );
                assert!(messages.iter().any(|message| matches!(
                    message,
                    crate::app::Message::Titlebar(Message::Action(_))
                )));
            }
        }
    }
}
