//! Main-window chrome and local menu navigation. Playback remains in the TUI.
mod appearance;
mod menus;
mod view;
mod widgets;

#[cfg(test)]
mod appearance_tests;
#[cfg(test)]
mod interaction_tests;
#[cfg(test)]
mod tests;

use crate::actions::Action;
use iced::keyboard::{Key, key::Named};
use iced::{Point, Size, window};
use menus::{MenuGroup, Platform};

pub const HEIGHT: f32 = 34.0;
pub const MENU_BUTTON_WIDTH: f32 = 68.0;
const ROOT_WIDTH: f32 = 164.0;
const MENU_WIDTH: f32 = 282.0;
const ROW_HEIGHT: f32 = 30.0;
const MENU_HEADING: f32 = 26.0;

#[derive(Debug, Clone)]
pub enum Message {
    Hover(bool),
    Animate,
    ToggleMenu,
    SelectGroup(usize),
    HoverGroup(usize),
    HoverItem(usize),
    Dismiss,
    Action(Action),
    WindowAction(Action),
    PressTitle,
    ReleaseTitle,
    DoubleClick,
    Resize(window::Direction),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaptionControl {
    Minimize,
    Maximize,
    Close,
}

#[derive(Debug, Default)]
pub struct State {
    appearance: appearance::Appearance,
    menu_open: bool,
    selected_group: Option<usize>,
    selected_item: usize,
    in_submenu: bool,
    pub commands_disabled: bool,
    pub player_unavailable: bool,
    pub maximized: bool,
    pub caption_hover: Option<CaptionControl>,
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
            Message::Hover(hovered) => self.appearance.hovered = hovered,
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
                if !self.commands_disabled
                    && (!action.requires_player() || !self.player_unavailable)
                {
                    self.close();
                    return Some(action);
                }
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
        self.appearance.opacity = config.opacity;
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

    fn item_enabled(&self, item: &menus::MenuItem, lyrics: bool) -> bool {
        !self.commands_disabled && item.enabled(lyrics, !self.player_unavailable)
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
                    let backwards = matches!(key, Key::Named(Named::ArrowUp | Named::End));
                    if let Some(index) = (0..count)
                        .map(|offset| {
                            if backwards {
                                (next + count - offset) % count
                            } else {
                                (next + offset) % count
                            }
                        })
                        .find(|&index| self.item_enabled(&items[index], lyrics))
                    {
                        self.selected_item = index;
                    }
                } else {
                    self.selected_group = Some(next);
                }
            }
            Key::Named(Named::Enter | Named::Space) => {
                if self.in_submenu {
                    let item = &groups[self.selected_group.unwrap_or(0)].items[self.selected_item];
                    if !self.item_enabled(item, lyrics) {
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
}
