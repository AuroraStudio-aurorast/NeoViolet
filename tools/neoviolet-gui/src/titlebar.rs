use gpui::prelude::*;
use gpui::*;
use yororen_ui::headless::dropdown_menu::{
    DropdownItem, DropdownMenuGroup, DropdownMenuItem, DropdownMenuState, dropdown_menu,
};
use yororen_ui::headless::icon::IconSource;
use yororen_ui::headless::icon_button::icon_button;

use crate::menus::{self, MenuEntry};
use crate::neo_violet_app::NeoVioletApp;
use crate::platform::{PlatformKind, current_platform_kind};
use crate::state::AppState;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TitlebarButton {
    Minimize,
    MaximizeRestore,
    Close,
    Menu,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TitlebarAction {
    Minimize,
    Zoom,
    RequestClose,
    ToggleMenu,
}

pub fn button_action(button: TitlebarButton) -> TitlebarAction {
    match button {
        TitlebarButton::Minimize => TitlebarAction::Minimize,
        TitlebarButton::MaximizeRestore => TitlebarAction::Zoom,
        TitlebarButton::Close => TitlebarAction::RequestClose,
        TitlebarButton::Menu => TitlebarAction::ToggleMenu,
    }
}

#[allow(dead_code)]
pub fn edge_for_point(
    x: f32,
    y: f32,
    width: f32,
    height: f32,
    thickness: f32,
) -> Option<ResizeEdge> {
    let left = x <= thickness;
    let right = x >= width - thickness;
    let top = y <= thickness;
    let bottom = y >= height - thickness;
    match (left, right, top, bottom) {
        (true, _, true, _) => Some(ResizeEdge::TopLeft),
        (_, true, true, _) => Some(ResizeEdge::TopRight),
        (true, _, _, true) => Some(ResizeEdge::BottomLeft),
        (_, true, _, true) => Some(ResizeEdge::BottomRight),
        (true, _, _, _) => Some(ResizeEdge::Left),
        (_, true, _, _) => Some(ResizeEdge::Right),
        (_, _, true, _) => Some(ResizeEdge::Top),
        (_, _, _, true) => Some(ResizeEdge::Bottom),
        _ => None,
    }
}

pub fn hamburger_icon() -> Div {
    div()
        .w(px(16.0))
        .h(px(14.0))
        .flex()
        .flex_col()
        .justify_between()
        .child(div().w_full().h(px(2.0)).bg(rgb(0xcfcfc2)))
        .child(div().w_full().h(px(2.0)).bg(rgb(0xcfcfc2)))
        .child(div().w_full().h(px(2.0)).bg(rgb(0xcfcfc2)))
}

type MenuSelectCallback = std::sync::Arc<dyn Fn(SharedString, &mut Window, &mut App) + Send + Sync>;

pub struct Titlebar {
    menu: Entity<DropdownMenuState>,
    on_select: MenuSelectCallback,
}

impl Titlebar {
    pub fn new(cx: &mut App) -> Self {
        let menu = DropdownMenuState::new(cx);
        let on_select =
            std::sync::Arc::new(|id: SharedString, _window: &mut Window, cx: &mut App| {
                if let Some(action) = menus::action_from_id(id.as_ref()) {
                    menus::dispatch(action, cx);
                }
            });
        let callback = on_select.clone();
        menu.update(cx, |state, _| {
            state.set_dismiss_on_outside_click(true);
            state.set_items(menu_items(current_platform_kind()));
            state.set_on_select(move |id, window, cx| callback(id, window, cx));
        });
        Self { menu, on_select }
    }

    pub fn render(
        &self,
        title: &str,
        plan: crate::platform::ChromePlan,
        is_maximized: bool,
        window: &mut Window,
        cx: &mut Context<NeoVioletApp>,
    ) -> AnyElement {
        let left_padding = if current_platform_kind() == PlatformKind::MacOs {
            px(72.0)
        } else {
            px(8.0)
        };
        let menu = self.menu.clone();
        let mut drag_area = div()
            .id("aria:titlebar:drag-area")
            .h_full()
            .flex()
            .items_center()
            .flex_grow()
            .min_w(px(0.0))
            .pl(left_padding)
            .child(
                div()
                    .id("aria:titlebar:title")
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_sm()
                    .text_color(crate::theme_colors::secondary(cx))
                    .text_ellipsis()
                    .whitespace_nowrap()
                    .overflow_hidden()
                    .child(title.to_string()),
            );
        if plan.drag_via_control_area {
            drag_area = drag_area.window_control_area(WindowControlArea::Drag);
        }
        drag_area = drag_area.on_mouse_down(MouseButton::Left, move |event, window, _| {
            if event.click_count > 1 {
                window.zoom_window();
            } else if !plan.drag_via_control_area {
                window.start_window_move();
            }
        });

        let mut row = div()
            .id("aria:region:titlebar")
            .relative()
            .w_full()
            .h(px(30.0))
            .bg(crate::theme_colors::sunken(cx))
            .flex()
            .flex_row()
            .items_center()
            .child(drag_area);

        if plan.menu_button {
            let trigger = hamburger_icon()
                .id("aria:titlebar:menu-trigger")
                .on_click({
                    let menu = menu.clone();
                    move |_, _, cx| {
                        if button_action(TitlebarButton::Menu) == TitlebarAction::ToggleMenu {
                            menu.update(cx, |state, cx| {
                                state.toggle();
                                cx.notify();
                            });
                        }
                    }
                })
                .into_any_element();
            row = row.child(
                dropdown_menu("aria:titlebar:menu", menu.clone())
                    .trigger(trigger)
                    .content(
                        menu_content(
                            current_platform_kind(),
                            menu.clone(),
                            self.on_select.clone(),
                        )
                        .into_any_element(),
                    )
                    .render(cx),
            );
        }

        if plan.window_buttons {
            let minimize = icon_button("aria:titlebar:minimize", cx)
                .icon(IconSource::Builtin("minimize".into()))
                .icon_size(px(14.0))
                .on_click(
                    |_, window, _| match button_action(TitlebarButton::Minimize) {
                        TitlebarAction::Minimize => window.minimize_window(),
                        _ => unreachable!(),
                    },
                )
                .render(cx);
            let maximize_name = if is_maximized {
                "maximize-off"
            } else {
                "maximize-on"
            };
            let maximize = icon_button("aria:titlebar:maximize", cx)
                .icon(IconSource::Builtin(maximize_name.into()))
                .icon_size(px(14.0))
                .on_click(
                    |_, window, _| match button_action(TitlebarButton::MaximizeRestore) {
                        TitlebarAction::Zoom => window.zoom_window(),
                        _ => unreachable!(),
                    },
                )
                .render(cx);
            let close = icon_button("aria:titlebar:close", cx)
                .icon(IconSource::Builtin("close".into()))
                .icon_size(px(14.0))
                .on_click(|_, _window, cx| {
                    if button_action(TitlebarButton::Close) != TitlebarAction::RequestClose {
                        return;
                    }
                    let eid = {
                        let state = cx.global::<AppState>();
                        *state.show_close.lock().unwrap() = true;
                        *state.root_entity_id.lock().unwrap()
                    };
                    if let Some(eid) = eid {
                        cx.notify(eid);
                    }
                })
                .render(cx);
            row = row.child(minimize).child(maximize).child(close);
        }

        row = row
            .on_mouse_down(MouseButton::Right, |event, window, _| {
                window.show_window_menu(event.position);
            })
            .border_b_1()
            .border_color(crate::theme_colors::tertiary(cx));
        if plan.frame {
            let edge = |id: &'static str, resize_edge| {
                div()
                    .id(id)
                    .absolute()
                    .on_mouse_down(MouseButton::Left, move |_, window, _| {
                        window.start_window_resize(resize_edge);
                    })
            };
            row = row
                .border_1()
                .border_color(crate::theme_colors::tertiary(cx))
                .child(
                    edge("aria:titlebar:resize-left", ResizeEdge::Left)
                        .left_0()
                        .top_0()
                        .w(px(6.0))
                        .h_full(),
                )
                .child(
                    edge("aria:titlebar:resize-right", ResizeEdge::Right)
                        .right_0()
                        .top_0()
                        .w(px(6.0))
                        .h_full(),
                )
                .child(
                    edge("aria:titlebar:resize-top", ResizeEdge::Top)
                        .left_0()
                        .top_0()
                        .w_full()
                        .h(px(6.0)),
                )
                .child(
                    edge("aria:titlebar:resize-bottom", ResizeEdge::Bottom)
                        .left_0()
                        .bottom_0()
                        .w_full()
                        .h(px(6.0)),
                )
                .child(
                    edge("aria:titlebar:resize-top-left", ResizeEdge::TopLeft)
                        .left_0()
                        .top_0()
                        .size(px(6.0)),
                )
                .child(
                    edge("aria:titlebar:resize-top-right", ResizeEdge::TopRight)
                        .right_0()
                        .top_0()
                        .size(px(6.0)),
                )
                .child(
                    edge("aria:titlebar:resize-bottom-left", ResizeEdge::BottomLeft)
                        .left_0()
                        .bottom_0()
                        .size(px(6.0)),
                )
                .child(
                    edge("aria:titlebar:resize-bottom-right", ResizeEdge::BottomRight)
                        .right_0()
                        .bottom_0()
                        .size(px(6.0)),
                );
        }
        let _ = window;
        row.into_any_element()
    }
}

fn menu_items(kind: PlatformKind) -> Vec<DropdownItem> {
    menus::layout(kind)
        .into_iter()
        .map(|group| {
            let mut items = Vec::new();
            for entry in group.entries {
                match entry {
                    MenuEntry::Action {
                        action,
                        label,
                        shortcut,
                    } => {
                        let mut item = DropdownMenuItem::new(menus::action_id(action), label);
                        if let Some(shortcut) = shortcut {
                            item = item.shortcut(shortcut.split('-').map(str::to_string).collect());
                        }
                        items.push(item);
                    }
                    MenuEntry::Separator | MenuEntry::Services => {}
                }
            }
            DropdownItem::Group(DropdownMenuGroup {
                label: group.title.into(),
                items,
            })
        })
        .collect()
}

fn menu_content(
    kind: PlatformKind,
    menu: Entity<DropdownMenuState>,
    on_select: MenuSelectCallback,
) -> Div {
    let mut content = div().flex().flex_col();
    for group in menu_items(kind) {
        if let DropdownItem::Group(group) = group {
            content = content.child(
                div()
                    .px(px(8.0))
                    .py(px(4.0))
                    .text_color(rgb(0x6272a4))
                    .text_size(px(11.0))
                    .child(group.label.to_string()),
            );
            for item in group.items {
                let id = item.id.clone();
                let label = item.label.to_string();
                let menu = menu.clone();
                let on_select = on_select.clone();
                content = content.child(
                    div()
                        .id(ElementId::Name(format!("titlebar-menu-{id}").into()))
                        .w_full()
                        .px(px(10.0))
                        .py(px(6.0))
                        .hover(|style| style.bg(rgb(0x44475a)))
                        .child(label)
                        .on_click(move |_, window, cx| {
                            menu.update(cx, |state, _| state.close());
                            on_select(id.clone(), window, cx);
                        }),
                );
            }
        }
    }
    content
}

#[cfg(test)]
mod tests {
    use super::{TitlebarAction, TitlebarButton, button_action, edge_for_point};
    use gpui::ResizeEdge;

    #[test]
    fn the_close_button_requests_the_dialog_instead_of_quitting() {
        assert_eq!(
            button_action(TitlebarButton::Close),
            TitlebarAction::RequestClose
        );
    }

    #[test]
    fn every_button_maps_to_a_window_operation() {
        assert_eq!(
            button_action(TitlebarButton::Minimize),
            TitlebarAction::Minimize
        );
        assert_eq!(
            button_action(TitlebarButton::MaximizeRestore),
            TitlebarAction::Zoom
        );
        assert_eq!(
            button_action(TitlebarButton::Menu),
            TitlebarAction::ToggleMenu
        );
    }

    #[test]
    fn edges_are_hit_within_the_grab_thickness() {
        let (w, h, t) = (400.0, 300.0, 6.0);
        assert_eq!(edge_for_point(2.0, 150.0, w, h, t), Some(ResizeEdge::Left));
        assert_eq!(
            edge_for_point(398.0, 150.0, w, h, t),
            Some(ResizeEdge::Right)
        );
        assert_eq!(edge_for_point(200.0, 2.0, w, h, t), Some(ResizeEdge::Top));
        assert_eq!(
            edge_for_point(200.0, 298.0, w, h, t),
            Some(ResizeEdge::Bottom)
        );
        assert_eq!(edge_for_point(2.0, 2.0, w, h, t), Some(ResizeEdge::TopLeft));
        assert_eq!(
            edge_for_point(398.0, 298.0, w, h, t),
            Some(ResizeEdge::BottomRight)
        );
        assert_eq!(
            edge_for_point(2.0, 298.0, w, h, t),
            Some(ResizeEdge::BottomLeft)
        );
        assert_eq!(
            edge_for_point(398.0, 2.0, w, h, t),
            Some(ResizeEdge::TopRight)
        );
    }

    #[test]
    fn the_middle_of_the_window_is_not_an_edge() {
        assert_eq!(edge_for_point(200.0, 150.0, 400.0, 300.0, 6.0), None);
        assert_eq!(
            edge_for_point(6.0, 150.0, 400.0, 300.0, 6.0),
            Some(ResizeEdge::Left)
        );
    }
}
