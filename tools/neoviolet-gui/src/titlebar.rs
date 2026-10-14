use std::cell::Cell;
use std::rc::Rc;

use gpui::prelude::*;
use gpui::*;
use yororen_ui::headless::icon::{IconSource, icon};

use crate::menus::{self, MenuAction, MenuEntry};
use crate::neo_violet_app::NeoVioletApp;
use crate::platform::{ChromePlan, PlatformKind, current_platform_kind};
use crate::state::AppState;
use crate::window_controls;

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

/// Height of the title bar, in points.
const TITLEBAR_HEIGHT: f32 = 30.0;

/// Width of the panel the menu drops down.
const MENU_PANEL_WIDTH: f32 = 240.0;

/// Gap between the panel and the trigger it hangs from.
const MENU_PANEL_GAP: f32 = 4.0;

/// Least distance the panel keeps from the window's edges.
const MENU_PANEL_MARGIN: f32 = 8.0;

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
    /// What the drawn menu remembers: whether it is open, and which row the
    /// keyboard is on.
    menu: Entity<DrawnMenu>,
    menu_focus_handle: FocusHandle,
    /// Where the menu trigger was painted, which is what the panel hangs from.
    /// gpui hands these bounds out while painting, so the panel follows the
    /// trigger one frame behind rather than waiting for a measurement pass of its
    /// own.
    trigger_bounds: Rc<Cell<Option<Bounds<Pixels>>>>,
    /// Whether a left press is waiting to become a window drag. The drag starts on
    /// the first movement rather than on the press, the way a system caption does
    /// it, so that a click which only selects or maximizes never moves anything.
    should_move: Rc<Cell<bool>>,
}

impl Titlebar {
    pub fn new(cx: &mut App) -> Self {
        Self {
            menu: cx.new(|_| DrawnMenu::new()),
            menu_focus_handle: cx.focus_handle(),
            trigger_bounds: Rc::new(Cell::new(None)),
            should_move: Rc::new(Cell::new(false)),
        }
    }

    pub fn menu_is_open(&self, cx: &App) -> bool {
        self.menu.read(cx).open
    }

    pub fn render(
        &self,
        title: &str,
        plan: crate::platform::ChromePlan,
        is_maximized: bool,
        window: &mut Window,
        cx: &mut Context<NeoVioletApp>,
    ) -> AnyElement {
        let kind = current_platform_kind();
        let left_padding = if kind == PlatformKind::MacOs {
            px(72.0)
        } else {
            px(8.0)
        };
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
        // Windows moves a window through its control areas, and this bar is the
        // area it should use; the other platforms leave the area alone and are
        // moved by hand below.
        drag_area = drag_area
            .window_control_area(WindowControlArea::Drag)
            .on_mouse_down(MouseButton::Left, {
                let should_move = self.should_move.clone();
                move |event, window, _| {
                    if event.click_count > 1 {
                        toggle_zoom(plan, window);
                    } else {
                        should_move.set(true);
                    }
                }
            })
            .on_mouse_up(MouseButton::Left, {
                let should_move = self.should_move.clone();
                move |_, _, _| should_move.set(false)
            })
            .on_mouse_down_out({
                let should_move = self.should_move.clone();
                move |_, _, _| should_move.set(false)
            })
            .on_mouse_move({
                let should_move = self.should_move.clone();
                move |event, window, _| {
                    // A press only becomes a drag once the pointer moves, the way a
                    // system caption behaves: clicking the bar must be able to
                    // select and double-click without moving the window.
                    if !should_move.get() {
                        return;
                    }
                    should_move.set(false);
                    if event.pressed_button == Some(MouseButton::Left) {
                        start_window_drag(plan, window);
                    }
                }
            });

        let mut row = div()
            .id("aria:region:titlebar")
            .relative()
            .w_full()
            .h(px(TITLEBAR_HEIGHT))
            .bg(crate::theme_colors::sunken(cx))
            .flex()
            .flex_row()
            .items_center()
            .child(drag_area);

        if plan.menu_button {
            let rows = menu_rows(current_platform_kind(), lyrics_overlay_is_on(cx));
            let (is_open, highlighted) = {
                let state = self.menu.read(cx);
                (state.open, state.highlighted)
            };
            let menu_focus_handle = self.menu_focus_handle.clone();
            let trigger = hamburger_icon(if is_open {
                crate::theme_colors::primary(cx)
            } else {
                crate::theme_colors::secondary(cx)
            })
            // gpui hands these bounds out while it paints, a frame after the
            // panel would have needed them, so the panel follows the trigger
            // rather than waiting for it. This listener lives on the bare `Div`,
            // hence before the id that makes the element stateful.
            .on_children_prepainted({
                let trigger_bounds = self.trigger_bounds.clone();
                move |bounds, _window, _cx| {
                    if let Some(bounds) = bounds.first() {
                        trigger_bounds.set(Some(*bounds));
                    }
                }
            })
            .id("aria:titlebar:menu-trigger")
            .p(px(4.0))
            .rounded(px(4.0))
            .when(is_open, |this| this.bg(crate::theme_colors::hover(cx)))
            .hover(|this| this.bg(crate::theme_colors::hover(cx)))
            .on_click({
                let menu = self.menu.clone();
                let menu_focus_handle = menu_focus_handle.clone();
                move |_, window, cx| {
                    let opened = menu.update(cx, |state, cx| {
                        state.open = !state.open;
                        state.highlighted = None;
                        cx.notify();
                        state.open
                    });
                    if opened {
                        window.focus(&menu_focus_handle);
                    }
                }
            });

            let panel = deferred(
                anchored()
                    // The panel hangs off the trigger's bottom-right corner and
                    // grows leftwards and downwards, which is where a menu at the
                    // end of a title bar has room. gpui pulls it back inside the
                    // window when it would not fit; placing a panel by hand is how
                    // a menu ends up half past the window edge.
                    .anchor(Corner::TopRight)
                    .position(anchor_for_panel(
                        self.trigger_bounds.get(),
                        window.viewport_size(),
                    ))
                    .offset(point(px(0.0), px(MENU_PANEL_GAP)))
                    .snap_to_window_with_margin(px(MENU_PANEL_MARGIN))
                    .child(menu_panel(rows, highlighted, self.menu.clone(), window, cx)),
            );

            let menu_for_keys = self.menu.clone();
            row = row.child(
                div()
                    .track_focus(&menu_focus_handle)
                    .on_key_down(move |event, window, cx| {
                        let key = event.keystroke.key.to_ascii_lowercase();
                        let rows = menu_rows(current_platform_kind(), lyrics_overlay_is_on(cx));
                        let step = match key.as_str() {
                            "up" | "arrowup" => Some(false),
                            "down" | "arrowdown" => Some(true),
                            _ => None,
                        };
                        if let Some(forward) = step {
                            menu_for_keys.update(cx, |state, cx| {
                                state.highlighted =
                                    next_row_index(&rows, state.highlighted, forward);
                                cx.notify();
                            });
                            cx.stop_propagation();
                            return;
                        }
                        let chosen = match key.as_str() {
                            "enter" | "return" => menu_for_keys.update(cx, |state, cx| {
                                let chosen =
                                    state.highlighted.and_then(|index| match rows.get(index) {
                                        Some(MenuRow::Item { action, .. }) => Some(*action),
                                        _ => None,
                                    });
                                if chosen.is_some() {
                                    state.open = false;
                                    state.highlighted = None;
                                    cx.notify();
                                }
                                chosen
                            }),
                            "escape" => {
                                close_menu(&menu_for_keys, cx);
                                None
                            }
                            _ => return,
                        };
                        cx.stop_propagation();
                        if let Some(action) = chosen {
                            menus::dispatch(action, window, cx);
                        }
                    })
                    .child(trigger)
                    .when(is_open, |this| this.child(panel)),
            );
        }

        if plan.window_buttons {
            row = row.child(window_controls::caption_row(
                kind,
                TITLEBAR_HEIGHT,
                is_maximized,
                window.window_controls(),
                cx,
            ));
        }

        row = row.on_mouse_down(MouseButton::Right, |event, window, _| {
            window.show_window_menu(event.position);
        });
        let _ = window;
        row.into_any_element()
    }
}

/// Start moving the window from the drag area.
///
/// Windows has no toolkit call for this at all — `start_window_move` is an empty
/// default there — so the shell has to run its own caption drag. macOS and Linux
/// do have the call, and no shell drag to ask for.
fn start_window_drag(plan: ChromePlan, window: &mut Window) {
    if plan.drag_via_control_area {
        crate::platform::start_window_drag();
    } else {
        window.start_window_move();
    }
}

/// Maximize the window, or put it back if it is already maximized.
///
/// gpui can only do the first half; the shell can do both, so on Windows — the
/// platform we hand the drag area to — the toggle goes through it.
fn toggle_zoom(plan: ChromePlan, window: &mut Window) {
    if plan.drag_via_control_area {
        crate::platform::toggle_window_zoom();
    } else {
        window.zoom_window();
    }
}

/// What the drawn menu remembers between frames.
pub struct DrawnMenu {
    open: bool,
    /// The row the keyboard is on, as an index into [`menu_rows`].
    highlighted: Option<usize>,
}

impl DrawnMenu {
    fn new() -> Self {
        Self {
            open: false,
            highlighted: None,
        }
    }
}

impl Render for DrawnMenu {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        div()
    }
}

/// One row of the panel the menu drops down.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum MenuRow {
    /// The title over a group of rows.
    Heading(&'static str),
    /// A row that runs `action`. `shortcut` is the key the layout advertises for
    /// it, which is what the tests read; the panel itself shows the binding gpui
    /// reports, so an alias the layout does not list still shows up.
    Item {
        action: MenuAction,
        label: &'static str,
        shortcut: Option<&'static str>,
        /// Whether the row stands for something that is currently on.
        checked: bool,
    },
    Separator,
}

/// The rows the panel shows, in the order it paints them.
///
/// Each group of the layout becomes a heading with its own rows under it — the
/// shape of a menu bar, flattened into the one panel a menu drawn by the app can
/// show. The separators a layout writes inside a group are kept; runs of them
/// collapse into one, and the list neither starts nor ends with one, since a gap
/// is meant to say "something else follows" and two gaps, or a gap against the
/// panel's own edge, say nothing.
///
/// `lyrics_shown` is whether the desktop lyrics overlay is on, which is what the
/// lyrics row's check mark reports.
pub fn menu_rows(kind: PlatformKind, lyrics_shown: bool) -> Vec<MenuRow> {
    let mut rows = Vec::new();
    for (index, group) in menus::layout(kind).into_iter().enumerate() {
        if index > 0 {
            push_separator(&mut rows);
        }
        rows.push(MenuRow::Heading(group.title));
        for entry in group.entries {
            match entry {
                MenuEntry::Action {
                    action,
                    label,
                    shortcut,
                } => rows.push(MenuRow::Item {
                    action,
                    label,
                    shortcut,
                    checked: action == MenuAction::ToggleDesktopLyrics && lyrics_shown,
                }),
                MenuEntry::Separator => push_separator(&mut rows),
                // macOS fills its own Services submenu in; there is nothing here
                // to draw for it.
                MenuEntry::Services => {}
            }
        }
    }
    while matches!(rows.last(), Some(MenuRow::Separator)) {
        rows.pop();
    }
    rows
}

/// Adds a separator unless the list is empty or already ends with one.
fn push_separator(rows: &mut Vec<MenuRow>) {
    if !rows.is_empty() && !matches!(rows.last(), Some(MenuRow::Separator)) {
        rows.push(MenuRow::Separator);
    }
}

/// The next row the keyboard can run, wrapping around.
///
/// Headings and separators are skipped: neither runs anything, so stopping on one
/// would look like a lost keypress.
fn next_row_index(rows: &[MenuRow], from: Option<usize>, forward: bool) -> Option<usize> {
    let len = rows.len();
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
        if matches!(rows.get(index), Some(MenuRow::Item { .. })) {
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

/// Closes the menu and lets go of the highlighted row.
fn close_menu(menu: &Entity<DrawnMenu>, cx: &mut App) {
    menu.update(cx, |state, cx| {
        state.open = false;
        state.highlighted = None;
        cx.notify();
    });
}

/// Whether the desktop lyrics overlay is on.
fn lyrics_overlay_is_on(cx: &App) -> bool {
    *cx.global::<AppState>()
        .desktop_lyrics_enabled
        .lock()
        .unwrap()
}

/// Where the panel's top-right corner goes: under the trigger once its bounds are
/// known, and under the trailing edge of the title bar before that.
fn anchor_for_panel(trigger: Option<Bounds<Pixels>>, viewport: Size<Pixels>) -> Point<Pixels> {
    trigger
        .map(|bounds| bounds.corner(Corner::BottomRight))
        .unwrap_or_else(|| {
            point(
                px(f32::from(viewport.width) - MENU_PANEL_MARGIN),
                px(TITLEBAR_HEIGHT),
            )
        })
}

/// The panel the menu drops down.
///
/// The panel is ours rather than the UI library's dropdown because its position
/// has to come from gpui's anchored element — see [`anchor_for_panel`]. Its rows
/// come from [`menu_rows`], and its shortcut column from gpui's own answer for the
/// action, so the key a row displays is the key that is really bound.
fn menu_panel(
    rows: Vec<MenuRow>,
    highlighted: Option<usize>,
    menu: Entity<DrawnMenu>,
    window: &mut Window,
    cx: &mut App,
) -> impl IntoElement {
    let panel_bg = crate::theme_colors::elevated(cx);
    let border = crate::theme_colors::border(cx);
    let hover = crate::theme_colors::hover(cx);
    let label_color = crate::theme_colors::primary(cx);
    let dim = crate::theme_colors::tertiary(cx);

    let mut panel = div()
        .id("aria:titlebar:menu-panel")
        .w(px(MENU_PANEL_WIDTH))
        .p(px(4.0))
        .flex()
        .flex_col()
        .bg(panel_bg)
        .border_1()
        .border_color(border)
        .rounded(px(6.0))
        .shadow(vec![BoxShadow {
            color: hsla(0.0, 0.0, 0.0, 0.35),
            blur_radius: px(12.0),
            spread_radius: px(0.0),
            offset: point(px(0.0), px(4.0)),
        }])
        .on_mouse_down_out({
            let menu = menu.clone();
            move |_, _, cx| close_menu(&menu, cx)
        });

    for (index, row) in rows.into_iter().enumerate() {
        panel = match row {
            MenuRow::Heading(title) => panel.child(
                div()
                    .px(px(8.0))
                    .pt(px(6.0))
                    .pb(px(2.0))
                    .text_size(px(11.0))
                    .text_color(dim)
                    .child(title),
            ),
            MenuRow::Separator => panel.child(div().w_full().h(px(1.0)).my(px(4.0)).bg(border)),
            MenuRow::Item {
                action,
                label,
                checked,
                ..
            } => {
                let package = menus::boxed_action(action);
                let key_text = window.keystroke_text_for(package.as_ref());
                // An action with no binding answers with its own name, which is no
                // shortcut at all and does not belong in the column.
                let shortcut = (key_text != package.as_ref().name()).then_some(key_text);
                let action_id = menus::action_id(action);
                let menu_for_click = menu.clone();
                panel.child(
                    div()
                        .id(ElementId::Name(format!("aria:titlebar:{action_id}").into()))
                        .h(px(24.0))
                        .px(px(8.0))
                        .rounded(px(4.0))
                        .flex()
                        .flex_row()
                        .items_center()
                        .gap(px(8.0))
                        .cursor(CursorStyle::PointingHand)
                        .when(highlighted == Some(index), |this| this.bg(hover))
                        .hover(move |this| this.bg(hover))
                        .child(
                            // A column of its own, so labels line up whether or
                            // not a row is checked.
                            div()
                                .w(px(14.0))
                                .flex()
                                .items_center()
                                .justify_center()
                                .when(checked, |this| {
                                    this.child(
                                        icon(
                                            ElementId::Name(
                                                format!("aria:titlebar:{action_id}:check").into(),
                                            ),
                                            IconSource::Builtin("check".into()),
                                            cx,
                                        )
                                        .size(px(12.0))
                                        .color(label_color)
                                        .render(cx),
                                    )
                                }),
                        )
                        .child(
                            div()
                                .flex_grow()
                                .text_size(px(13.0))
                                .text_color(label_color)
                                .child(label),
                        )
                        .when_some(shortcut, |this, shortcut| {
                            this.child(div().text_size(px(11.0)).text_color(dim).child(shortcut))
                        })
                        .on_click(move |_, window, cx| {
                            close_menu(&menu_for_click, cx);
                            menus::dispatch(action, window, cx);
                        }),
                )
            }
        };
    }
    panel
}

#[cfg(test)]
mod tests {
    use super::{
        MenuRow, anchor_for_panel, edge_for_point, edge_strips, menu_rows, next_row_index,
    };
    use crate::menus::{MenuAction, MenuEntry};
    use crate::platform::PlatformKind;
    use gpui::{Bounds, ResizeEdge, point, px, size};

    /// The actions a layout lists, in order, to compare against the rows a panel
    /// would draw.
    fn listed_actions(kind: PlatformKind) -> Vec<MenuAction> {
        crate::menus::layout(kind)
            .into_iter()
            .flat_map(|group| group.entries)
            .filter_map(|entry| match entry {
                MenuEntry::Action { action, .. } => Some(action),
                _ => None,
            })
            .collect()
    }

    fn drawn_actions(rows: &[MenuRow]) -> Vec<MenuAction> {
        rows.iter()
            .filter_map(|row| match row {
                MenuRow::Item { action, .. } => Some(*action),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn the_drawn_menu_heads_each_group_with_its_title() {
        let rows = menu_rows(PlatformKind::Windows, false);
        let headings: Vec<&str> = rows
            .iter()
            .filter_map(|row| match row {
                MenuRow::Heading(title) => Some(*title),
                _ => None,
            })
            .collect();
        assert_eq!(headings, vec!["File", "Edit", "View", "Help"]);
    }

    #[test]
    fn the_drawn_menu_keeps_every_row_exactly_once() {
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            let rows = menu_rows(kind, false);
            assert_eq!(
                drawn_actions(&rows),
                listed_actions(kind),
                "{kind:?} 面板必须把 layout 的每一行都画出来，且只画一次"
            );
        }
    }

    #[test]
    fn the_drawn_menu_keeps_separators_where_they_separate_things() {
        for kind in [
            PlatformKind::MacOs,
            PlatformKind::Windows,
            PlatformKind::Linux,
        ] {
            let rows = menu_rows(kind, false);
            // A gap says "something else follows", so the panel must not open or
            // close with one, and two in a row say nothing at all.
            assert!(
                !matches!(rows.first(), Some(MenuRow::Separator)),
                "{kind:?}"
            );
            assert!(!matches!(rows.last(), Some(MenuRow::Separator)), "{kind:?}");
            assert!(
                rows.windows(2)
                    .all(|pair| !matches!(pair, [MenuRow::Separator, MenuRow::Separator])),
                "{kind:?} 连续分隔线没有意义"
            );
            // macOS's application menu separates about/hide/quit, so it keeps at
            // least the gaps the layout writes.
            if kind == PlatformKind::MacOs {
                assert!(rows.iter().any(|row| matches!(row, MenuRow::Separator)));
            }
        }
    }

    #[test]
    fn the_drawn_menu_keeps_the_layout_shortcuts() {
        let rows = menu_rows(PlatformKind::Windows, false);
        let shortcut = rows.iter().find_map(|row| match row {
            MenuRow::Item {
                label, shortcut, ..
            } if *label == "Open File…" => Some(*shortcut),
            _ => None,
        });
        assert_eq!(shortcut, Some(Some("ctrl-o")));
    }

    #[test]
    fn the_drawn_menu_leaves_services_to_macos() {
        // macOS fills its own Services submenu in, so the panel has nothing to
        // show for it.
        let rows = menu_rows(PlatformKind::MacOs, false);
        assert!(
            !rows
                .iter()
                .any(|row| matches!(row, MenuRow::Heading(title) if *title == "Services")),
            "Services 由 macOS 自己填，面板里不该出现"
        );
    }

    #[test]
    fn the_drawn_menu_checks_the_lyrics_row_when_the_overlay_is_on() {
        let checked_count = |lyrics_shown| {
            menu_rows(PlatformKind::Windows, lyrics_shown)
                .iter()
                .filter(|row| matches!(row, MenuRow::Item { checked: true, .. }))
                .count()
        };
        assert_eq!(checked_count(false), 0);
        assert_eq!(checked_count(true), 1, "只在歌词行上打勾");
        assert!(
            menu_rows(PlatformKind::Windows, true)
                .iter()
                .any(|row| matches!(
                    row,
                    MenuRow::Item {
                        action: MenuAction::ToggleDesktopLyrics,
                        checked: true,
                        ..
                    }
                ))
        );
    }

    #[test]
    fn the_panel_anchors_under_the_trigger() {
        let trigger = Bounds::new(point(px(700.0), px(0.0)), size(px(24.0), px(30.0)));
        assert_eq!(
            anchor_for_panel(Some(trigger), size(px(800.0), px(600.0))),
            point(px(724.0), px(30.0)),
            "面板挂在触发器的右下角"
        );
        // Before the trigger has been painted there is still a home for the panel:
        // the trailing edge of the title bar, a margin's width in.
        assert_eq!(
            anchor_for_panel(None, size(px(800.0), px(600.0))),
            point(px(792.0), px(30.0))
        );
    }

    fn rows_with_headings() -> Vec<MenuRow> {
        vec![
            MenuRow::Heading("File"),
            MenuRow::Item {
                action: MenuAction::OpenFile,
                label: "Open File…",
                shortcut: Some("ctrl-o"),
                checked: false,
            },
            MenuRow::Item {
                action: MenuAction::Quit,
                label: "Exit",
                shortcut: Some("ctrl-q"),
                checked: false,
            },
            MenuRow::Separator,
            MenuRow::Heading("Help"),
            MenuRow::Item {
                action: MenuAction::About,
                label: "About",
                shortcut: Some("ctrl-shift-a"),
                checked: false,
            },
        ]
    }

    #[test]
    fn keyboard_navigation_skips_headings_and_separators() {
        let rows = rows_with_headings();
        assert_eq!(next_row_index(&rows, None, true), Some(1));
        assert_eq!(next_row_index(&rows, Some(1), true), Some(2));
        assert_eq!(
            next_row_index(&rows, Some(2), true),
            Some(5),
            "跳过分隔符与组标题，直接落到最后一行"
        );
        assert_eq!(
            next_row_index(&rows, Some(5), true),
            Some(1),
            "循环回到第一行"
        );
        assert_eq!(next_row_index(&rows, Some(1), false), Some(5));
        assert_eq!(next_row_index(&rows, Some(5), false), Some(2));
    }

    #[test]
    fn keyboard_navigation_gives_up_without_any_row() {
        let headings_only = vec![MenuRow::Separator, MenuRow::Heading("File")];
        assert_eq!(next_row_index(&headings_only, None, true), None);
        assert_eq!(next_row_index(&[], None, true), None);
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
