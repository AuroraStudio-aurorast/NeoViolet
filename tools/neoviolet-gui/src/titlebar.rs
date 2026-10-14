use gpui::prelude::*;
use gpui::*;
use yororen_ui::headless::dropdown_menu::{
    DropdownItem, DropdownMenuGroup, DropdownMenuItem, DropdownMenuState, dropdown_menu,
};
use yororen_ui::headless::icon::IconSource;
use yororen_ui::headless::icon_button::icon_button;
use yororen_ui::headless::menu::MenuState;

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

/// Which edge or corner of a `width` × `height` window the point `(x, y)` grabs.
///
/// `(x, y)` is relative to the window's top-left corner. Corners win over
/// edges: a point that is inside a corner strip is that corner, never either of
/// the two edges it also touches. A window no wider (or no taller) than two
/// strips has no distinguishable edges left, and its overlapping strips simply
/// make one of the corners cover it, so the answer stays defined rather than
/// bailing out — that is what keeps the outermost pixels of a tiny window
/// grabbable at all.
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

pub fn hamburger_icon(color: Hsla) -> Div {
    div()
        .w(px(16.0))
        .h(px(14.0))
        .flex()
        .flex_col()
        .justify_between()
        .child(div().w_full().h(px(2.0)).bg(color))
        .child(div().w_full().h(px(2.0)).bg(color))
        .child(div().w_full().h(px(2.0)).bg(color))
}

/// The eight invisible grab strips around a `width` × `height` window, in
/// window coordinates, `thickness` deep.
///
/// Edges come first and corners last, so a corner strip is drawn over — and so
/// hit before — the two edges it sits on, which is the same verdict
/// [`edge_for_point`] reaches. `thickness` is capped at half of each axis, so a
/// window too small to carry two full strips still gets strips that stay inside
/// its own bounds instead of overlapping into a negative size.
pub fn edge_strips(width: f32, height: f32, thickness: f32) -> Vec<(ResizeEdge, Bounds<Pixels>)> {
    let thickness = thickness
        .max(0.0)
        .min(width.max(0.0) / 2.0)
        .min(height.max(0.0) / 2.0);
    let strip =
        |x: f32, y: f32, w: f32, h: f32| Bounds::new(point(px(x), px(y)), size(px(w), px(h)));
    let far_x = width - thickness;
    let far_y = height - thickness;
    vec![
        (ResizeEdge::Top, strip(0.0, 0.0, width, thickness)),
        (ResizeEdge::Bottom, strip(0.0, far_y, width, thickness)),
        (ResizeEdge::Left, strip(0.0, 0.0, thickness, height)),
        (ResizeEdge::Right, strip(far_x, 0.0, thickness, height)),
        (ResizeEdge::TopLeft, strip(0.0, 0.0, thickness, thickness)),
        (
            ResizeEdge::TopRight,
            strip(far_x, 0.0, thickness, thickness),
        ),
        (
            ResizeEdge::BottomLeft,
            strip(0.0, far_y, thickness, thickness),
        ),
        (
            ResizeEdge::BottomRight,
            strip(far_x, far_y, thickness, thickness),
        ),
    ]
}

/// Depth of the grab strips along each window edge.
const FRAME_THICKNESS: f32 = 6.0;

/// The eight invisible strips that start a resize, sized for a window of
/// `size`.
///
/// This has to be the **last** child of the window root. gpui dispatches mouse
/// events to the elements painted last first, and everything below it — the
/// terminal above all — paints before it. That ordering is what makes the
/// outermost few pixels the ones the user grabs and keeps those clicks out of
/// the terminal. Callers mount it only where the platform lets the client start
/// the resize itself, since elsewhere the strips would swallow clicks without
/// resizing anything.
pub fn window_frame(size: Size<Pixels>) -> Div {
    let width = f32::from(size.width);
    let height = f32::from(size.height);
    let mut frame = div()
        .absolute()
        .left_0()
        .top_0()
        .w(size.width)
        .h(size.height);
    {
        for (edge, bounds) in edge_strips(width, height, FRAME_THICKNESS) {
            frame = frame.child(
                div()
                    .id(strip_id(edge))
                    .absolute()
                    .left(bounds.origin.x)
                    .top(bounds.origin.y)
                    .w(bounds.size.width)
                    .h(bounds.size.height)
                    .cursor(resize_cursor(edge))
                    .on_mouse_down(MouseButton::Left, move |event, window, cx| {
                        let Some(edge) = edge_for_point(
                            f32::from(event.position.x),
                            f32::from(event.position.y),
                            width,
                            height,
                            FRAME_THICKNESS,
                        ) else {
                            return;
                        };
                        window.start_window_resize(edge);
                        // The strips sit on top of the terminal, which would
                        // otherwise receive the same click.
                        cx.stop_propagation();
                    }),
            );
        }
    }
    frame
}

/// The element id of `edge`'s strip, in the app's `aria:` naming scheme.
fn strip_id(edge: ResizeEdge) -> &'static str {
    match edge {
        ResizeEdge::Top => "aria:window:resize-top",
        ResizeEdge::TopRight => "aria:window:resize-top-right",
        ResizeEdge::Right => "aria:window:resize-right",
        ResizeEdge::BottomRight => "aria:window:resize-bottom-right",
        ResizeEdge::Bottom => "aria:window:resize-bottom",
        ResizeEdge::BottomLeft => "aria:window:resize-bottom-left",
        ResizeEdge::Left => "aria:window:resize-left",
        ResizeEdge::TopLeft => "aria:window:resize-top-left",
    }
}

/// The resize cursor that points along `edge`.
///
/// gpui has no per-corner cursors, so each corner takes the diagonal it lies on:
/// `ResizeUpLeftDownRight` is the top-left/bottom-right diagonal and
/// `ResizeUpRightDownLeft` the top-right/bottom-left one.
fn resize_cursor(edge: ResizeEdge) -> CursorStyle {
    match edge {
        ResizeEdge::Top => CursorStyle::ResizeUp,
        ResizeEdge::Bottom => CursorStyle::ResizeDown,
        ResizeEdge::Left => CursorStyle::ResizeLeft,
        ResizeEdge::Right => CursorStyle::ResizeRight,
        ResizeEdge::TopLeft | ResizeEdge::BottomRight => CursorStyle::ResizeUpLeftDownRight,
        ResizeEdge::TopRight | ResizeEdge::BottomLeft => CursorStyle::ResizeUpRightDownLeft,
    }
}

pub struct Titlebar {
    /// The dropdown shell: open/close, positioning and outside-click dismissal.
    menu: Entity<DropdownMenuState>,
    /// The menu itself. The registered menu renderer paints the panel and the
    /// rows from these items, so this entity owns the highlight and the
    /// selection callback those rows fire.
    menu_body: Entity<MenuState>,
    menu_focus_handle: FocusHandle,
}

impl Titlebar {
    pub fn new(cx: &mut App) -> Self {
        let menu = DropdownMenuState::new(cx);
        let menu_body = MenuState::new(cx);
        let menu_focus_handle = cx.focus_handle();
        menu.update(cx, |state, _| {
            state.set_dismiss_on_outside_click(true);
        });
        let menu_for_close = menu.clone();
        menu_body.update(cx, |state, _| {
            state.set_items(menu_items(current_platform_kind()));
            state.set_on_select(move |id, _window, cx| {
                // The shell fires the item's callback without closing itself.
                menu_for_close.update(cx, |state, cx| {
                    state.close();
                    cx.notify();
                });
                if let Some(action) = menus::action_from_id(id.as_ref()) {
                    menus::dispatch(action, cx);
                }
            });
        });
        Self {
            menu,
            menu_body,
            menu_focus_handle,
        }
    }

    pub fn menu_is_open(&self, cx: &App) -> bool {
        self.menu.read(cx).is_open()
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
            let menu_focus_handle = self.menu_focus_handle.clone();
            let trigger = hamburger_icon(crate::theme_colors::secondary(cx))
                .id("aria:titlebar:menu-trigger")
                .on_click({
                    let menu = menu.clone();
                    let menu_focus_handle = menu_focus_handle.clone();
                    move |_, window, cx| {
                        if button_action(TitlebarButton::Menu) == TitlebarAction::ToggleMenu {
                            let is_open = menu.update(cx, |state, cx| {
                                state.toggle();
                                cx.notify();
                                state.is_open()
                            });
                            if is_open {
                                window.focus(&menu_focus_handle);
                            }
                        }
                    }
                })
                .into_any_element();
            let menu_focus_handle = self.menu_focus_handle.clone();
            let menu_body_for_keys = self.menu_body.clone();
            let menu_for_close = menu.clone();
            row = row.child(
                div()
                    .track_focus(&menu_focus_handle)
                    .on_key_down(move |event, window, cx| {
                        let key = event.keystroke.key.to_ascii_lowercase();
                        let step = match key.as_str() {
                            "up" | "arrowup" => Some(false),
                            "down" | "arrowdown" => Some(true),
                            _ => None,
                        };
                        if let Some(forward) = step {
                            let target = {
                                let state = menu_body_for_keys.read(cx);
                                next_row_index(&state.items, state.highlighted_index, forward)
                            };
                            menu_body_for_keys.update(cx, |state, cx| {
                                if let Some(index) = target {
                                    state.highlighted_index = Some(index);
                                }
                                cx.notify();
                            });
                            cx.stop_propagation();
                            return;
                        }
                        match key.as_str() {
                            "enter" | "return" => menu_body_for_keys.update(cx, |state, cx| {
                                state.select_highlighted(window, cx);
                                cx.notify();
                            }),
                            "escape" => menu_for_close.update(cx, |state, cx| {
                                state.close();
                                cx.notify();
                            }),
                            _ => return,
                        }
                        cx.stop_propagation();
                    })
                    .child(
                        dropdown_menu("aria:titlebar:menu", menu.clone())
                            .trigger(trigger)
                            .content(
                                yororen_ui::headless::menu::menu(
                                    "aria:titlebar:menu-body",
                                    self.menu_body.clone(),
                                )
                                .render(cx)
                                .into_any_element(),
                            )
                            .render(cx),
                    ),
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
            row = row
                .child(
                    div()
                        .window_control_area(WindowControlArea::Min)
                        .child(minimize),
                )
                .child(
                    div()
                        .window_control_area(WindowControlArea::Max)
                        .child(maximize),
                )
                .child(
                    div()
                        .window_control_area(WindowControlArea::Close)
                        .child(close),
                );
        }

        row = row
            .on_mouse_down(MouseButton::Right, |event, window, _| {
                window.show_window_menu(event.position);
            })
            .border_b_1()
            .border_color(crate::theme_colors::tertiary(cx));
        let _ = window;
        row.into_any_element()
    }
}

/// The rows the drawn menu shows, in the order the renderer paints them.
///
/// The shell's menu renderer paints a `Group` as a section heading rather than
/// as a container, so the layout's groups are flattened into a heading followed
/// by their own rows.
/// The next row the user can actually activate, wrapping around.
///
/// The shell's `MenuState` treats only separators as unselectable, and the menu
/// renderer paints a section heading as plain text that never highlights, so
/// stepping with the library's own navigation would look like a lost keypress
/// whenever it landed on a heading or a separator.
fn next_row_index(items: &[DropdownItem], from: Option<usize>, forward: bool) -> Option<usize> {
    let len = items.len();
    if len == 0 {
        return None;
    }
    let mut index = match (from, forward) {
        (Some(index), true) => (index + 1) % len,
        (Some(index), false) => (index + len - 1) % len,
        (None, true) => 0,
        (None, false) => len - 1,
    };
    for _ in 0..len {
        if matches!(items.get(index), Some(DropdownItem::Item(_))) {
            return Some(index);
        }
        index = if forward {
            (index + 1) % len
        } else {
            (index + len - 1) % len
        };
    }
    None
}

fn menu_items(kind: PlatformKind) -> Vec<DropdownItem> {
    let mut items = Vec::new();
    for (index, group) in menus::layout(kind).into_iter().enumerate() {
        if index > 0 {
            items.push(DropdownItem::Separator);
        }
        items.push(DropdownItem::Group(DropdownMenuGroup {
            label: group.title.into(),
            items: Vec::new(),
        }));
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
                    items.push(DropdownItem::Item(item));
                }
                MenuEntry::Separator => items.push(DropdownItem::Separator),
                MenuEntry::Services => {}
            }
        }
    }
    items
}

#[cfg(test)]
mod tests {
    use super::{
        TitlebarAction, TitlebarButton, button_action, edge_for_point, edge_strips, menu_items,
        next_row_index,
    };
    use crate::platform::PlatformKind;
    use gpui::ResizeEdge;
    use yororen_ui::headless::dropdown_menu::{DropdownItem, DropdownMenuGroup, DropdownMenuItem};

    #[test]
    fn the_drawn_menu_keeps_separators_and_shortcuts() {
        let items = menu_items(PlatformKind::Windows);
        assert!(
            items
                .iter()
                .any(|item| matches!(item, DropdownItem::Separator))
        );
        let shortcut = items.iter().find_map(|item| match item {
            DropdownItem::Item(item) => {
                (item.label.as_ref() == "Open File…").then(|| item.shortcut.clone())
            }
            _ => None,
        });
        assert_eq!(
            shortcut,
            Some(Some(vec!["ctrl".to_string(), "o".to_string()]))
        );
    }

    #[test]
    fn the_drawn_menu_heads_each_group_with_its_title() {
        let items = menu_items(PlatformKind::Windows);
        let headings: Vec<String> = items
            .iter()
            .filter_map(|item| match item {
                DropdownItem::Group(group) => Some(group.label.to_string()),
                _ => None,
            })
            .collect();
        assert_eq!(headings, vec!["File", "Edit", "View", "Help"]);
        // The renderer paints a `Group` as a heading, not as a container, so a
        // heading must not carry rows of its own — they would never be painted.
        assert!(items.iter().all(|item| match item {
            DropdownItem::Group(group) => group.items.is_empty(),
            _ => true,
        }));
        // Flattening must not lose or duplicate a row.
        let rows = items
            .iter()
            .filter(|item| matches!(item, DropdownItem::Item(_)))
            .count();
        let actions: usize = crate::menus::layout(PlatformKind::Windows)
            .iter()
            .map(|group| {
                group
                    .entries
                    .iter()
                    .filter(|entry| matches!(entry, crate::menus::MenuEntry::Action { .. }))
                    .count()
            })
            .sum();
        assert_eq!(rows, actions);
    }

    fn rows_with_headings() -> Vec<DropdownItem> {
        vec![
            DropdownItem::Group(DropdownMenuGroup {
                label: "File".into(),
                items: Vec::new(),
            }),
            DropdownItem::Item(DropdownMenuItem::new("open-file", "Open File…")),
            DropdownItem::Item(DropdownMenuItem::new("quit", "Exit")),
            DropdownItem::Separator,
            DropdownItem::Group(DropdownMenuGroup {
                label: "Help".into(),
                items: Vec::new(),
            }),
            DropdownItem::Item(DropdownMenuItem::new("about", "About")),
        ]
    }

    #[test]
    fn keyboard_navigation_skips_headings_and_separators() {
        let items = rows_with_headings();
        assert_eq!(next_row_index(&items, None, true), Some(1));
        assert_eq!(next_row_index(&items, Some(1), true), Some(2));
        assert_eq!(
            next_row_index(&items, Some(2), true),
            Some(5),
            "跳过分隔符与组标题，直接落到最后一行"
        );
        assert_eq!(
            next_row_index(&items, Some(5), true),
            Some(1),
            "循环回到第一行"
        );
        assert_eq!(next_row_index(&items, Some(1), false), Some(5));
        assert_eq!(next_row_index(&items, Some(5), false), Some(2));
    }

    #[test]
    fn keyboard_navigation_gives_up_without_any_row() {
        let headings_only = vec![
            DropdownItem::Separator,
            DropdownItem::Group(DropdownMenuGroup {
                label: "File".into(),
                items: Vec::new(),
            }),
        ];
        assert_eq!(next_row_index(&headings_only, None, true), None);
        assert_eq!(next_row_index(&[], None, true), None);
    }

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
    fn corner_hits_win_over_edge_hits() {
        let (w, h, t) = (400.0, 300.0, 6.0);
        // 角上的点必须判定为角，而不是任意一条边
        assert_eq!(
            edge_for_point(t - 0.5, t - 0.5, w, h, t),
            Some(ResizeEdge::TopLeft)
        );
        assert_eq!(
            edge_for_point(w - t + 0.5, h - t + 0.5, w, h, t),
            Some(ResizeEdge::BottomRight)
        );
    }

    #[test]
    fn a_window_smaller_than_the_grab_thickness_still_resolves_deterministically() {
        let (w, h, t) = (8.0, 8.0, 6.0);
        // 不能 panic，且中心点必须落到某个角上而不是 None
        assert!(edge_for_point(4.0, 4.0, w, h, t).is_some());
    }

    #[test]
    fn the_side_strips_run_the_whole_window_height() {
        let strips = edge_strips(400.0, 300.0, 6.0);
        for edge in [ResizeEdge::Left, ResizeEdge::Right] {
            let bounds = strip_of(&strips, edge);
            assert_eq!(
                f32::from(bounds.size.height),
                300.0,
                "{edge:?} 必须纵向铺满整个窗口，而不是只铺标题栏那一行"
            );
        }
    }

    #[test]
    fn the_bottom_strip_sits_on_the_window_bottom() {
        let strips = edge_strips(400.0, 300.0, 6.0);
        let bounds = strip_of(&strips, ResizeEdge::Bottom);
        assert_eq!(
            f32::from(bounds.origin.y) + f32::from(bounds.size.height),
            300.0,
            "底边命中条必须落在窗口底部"
        );
    }

    #[test]
    fn strips_stay_inside_a_window_narrower_than_two_strips() {
        let (w, h, t) = (8.0, 8.0, 6.0);
        let strips = edge_strips(w, h, t);
        assert_eq!(strips.len(), 8, "四边四角各一条");
        for (edge, bounds) in &strips {
            let (x, y) = (f32::from(bounds.origin.x), f32::from(bounds.origin.y));
            let (sw, sh) = (f32::from(bounds.size.width), f32::from(bounds.size.height));
            assert!(sw >= 0.0 && sh >= 0.0, "{edge:?} 的尺寸不能为负");
            assert!(
                x >= 0.0 && y >= 0.0 && x + sw <= w && y + sh <= h,
                "{edge:?} 越出窗口"
            );
        }
    }

    fn strip_of(
        strips: &[(ResizeEdge, gpui::Bounds<gpui::Pixels>)],
        edge: ResizeEdge,
    ) -> gpui::Bounds<gpui::Pixels> {
        strips
            .iter()
            .find(|(candidate, _)| *candidate == edge)
            .map(|(_, bounds)| *bounds)
            .expect("每条边都该有命中条")
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
