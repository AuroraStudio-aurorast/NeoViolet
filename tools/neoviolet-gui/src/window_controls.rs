//! The minimize / maximize / close buttons a self-drawn title bar needs.
//!
//! Each platform has its own idea of what these look like, and users notice when
//! it is not the one they are used to. Windows draws them flush against each
//! other as full-height rectangles whose glyphs come from the system caption
//! font, and its close button turns red under the pointer; Linux desktops draw
//! small round ghost buttons with air around them. Sizes, glyphs and hit-test
//! areas are decided here per platform, so the drawing and the platform-specific
//! window behaviour stay in one place.

use gpui::{
    AnyElement, App, CursorStyle, Hsla, MouseButton, WindowControlArea, WindowControls, div,
    prelude::*, px,
};
use yororen_ui::headless::icon::{IconSource, icon};

use crate::platform::PlatformKind;
use crate::state::AppState;

/// A button a title bar can draw, in the state it is currently in.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CaptionButton {
    Minimize,
    /// Grow the window until it fills the screen.
    Maximize,
    /// Put a maximized window back the way it was.
    Restore,
    Close,
}

/// The measurements a platform's caption buttons are drawn with.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CaptionMetrics {
    /// Width of one button.
    pub button_width: f32,
    /// Height of one button.
    pub button_height: f32,
    /// Gap between buttons.
    pub gap: f32,
    /// Padding on the outer edge of the group.
    pub padding: f32,
    /// Corner radius; half the button height makes it a circle.
    pub rounded: f32,
    /// Size of the icon drawn in a button.
    pub icon_size: f32,
}

/// The red Windows turns its close button when the pointer is over it.
pub const CLOSE_HOVER_RED: u32 = 0x00e8_1123;

/// Windows draws caption buttons as full-height rectangles of a fixed width —
/// the same ones its own title bar would use — and they sit flush against the
/// trailing edge.
const WINDOWS_CAPTION_WIDTH: f32 = 36.0;
const WINDOWS_CAPTION_ICON: f32 = 12.0;

/// Linux desktops draw small round ghost buttons, with a little air between them
/// and around the group.
const LINUX_CAPTION_SIZE: f32 = 20.0;
const LINUX_CAPTION_GAP: f32 = 12.0;
const LINUX_CAPTION_PADDING: f32 = 12.0;
const LINUX_CAPTION_ICON: f32 = 16.0;

/// The measurements for one platform's buttons, in a title bar of this height.
pub fn caption_metrics(kind: PlatformKind, titlebar_height: f32) -> CaptionMetrics {
    match kind {
        PlatformKind::Windows => CaptionMetrics {
            button_width: WINDOWS_CAPTION_WIDTH,
            button_height: titlebar_height,
            gap: 0.0,
            padding: 0.0,
            rounded: 0.0,
            icon_size: WINDOWS_CAPTION_ICON,
        },
        // macOS keeps its own traffic lights, so it never draws these; it is
        // measured like Linux so that the numbers stay defined everywhere.
        PlatformKind::MacOs | PlatformKind::Linux => CaptionMetrics {
            button_width: LINUX_CAPTION_SIZE,
            button_height: LINUX_CAPTION_SIZE,
            gap: LINUX_CAPTION_GAP,
            padding: LINUX_CAPTION_PADDING,
            rounded: LINUX_CAPTION_SIZE / 2.0,
            icon_size: LINUX_CAPTION_ICON,
        },
    }
}

/// The icon a button is drawn with, from the app's own icon set.
///
/// Windows has its own caption font — "Segoe Fluent Icons" on 11, "Segoe MDL2
/// Assets" before it — and Zed draws its Windows controls with those private-use
/// codepoints. Rendering one here came out as a missing-glyph box instead, on and
/// off, so the buttons use the drawn icons everywhere: they cannot go missing,
/// they look the same on all three platforms, and one set of shapes is one thing
/// to keep right.
pub fn caption_icon(button: CaptionButton) -> &'static str {
    match button {
        CaptionButton::Minimize => "minimize",
        CaptionButton::Maximize => "maximize-off",
        CaptionButton::Restore => "maximize-on",
        CaptionButton::Close => "close",
    }
}

/// The element id a button is drawn with, inside the title bar.
pub fn caption_id(button: CaptionButton) -> &'static str {
    match button {
        CaptionButton::Minimize => "minimize",
        CaptionButton::Maximize | CaptionButton::Restore => "maximize",
        CaptionButton::Close => "close",
    }
}

/// The part of the window a button answers for, so the platform can run its own
/// window behaviour — snapping, its window menu — over that spot.
pub fn caption_area(button: CaptionButton) -> WindowControlArea {
    match button {
        CaptionButton::Minimize => WindowControlArea::Min,
        CaptionButton::Maximize | CaptionButton::Restore => WindowControlArea::Max,
        CaptionButton::Close => WindowControlArea::Close,
    }
}

/// The buttons to draw, given what the window is doing and what the platform
/// says it can do — a Wayland compositor, for instance, may not offer a minimize
/// button, and then we must not draw one either.
pub fn caption_buttons(is_maximized: bool, controls: WindowControls) -> Vec<CaptionButton> {
    let mut buttons = Vec::new();
    if controls.minimize {
        buttons.push(CaptionButton::Minimize);
    }
    if controls.maximize {
        buttons.push(if is_maximized {
            CaptionButton::Restore
        } else {
            CaptionButton::Maximize
        });
    }
    buttons.push(CaptionButton::Close);
    buttons
}

/// What pressing a button does.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CaptionAction {
    Minimize,
    /// Grow the window, or put it back if it is already as large as it gets.
    ToggleZoom,
    /// Ask the app to close the window, so that it can confirm first.
    RequestClose,
}

/// Pressing a button with a caption on it. Close asks for the app's confirmation
/// dialog rather than ending the process, here as in the menu and the window's
/// own close button.
pub fn caption_action(button: CaptionButton) -> CaptionAction {
    match button {
        CaptionButton::Minimize => CaptionAction::Minimize,
        CaptionButton::Maximize | CaptionButton::Restore => CaptionAction::ToggleZoom,
        CaptionButton::Close => CaptionAction::RequestClose,
    }
}

/// Whether the buttons have to act themselves on this platform.
///
/// Windows turns a control area into a real window button — it runs minimize,
/// maximize/restore and close from the areas declared below, and those are also
/// what give the buttons their snap layouts and proper activation. Acting here
/// as well would run every press twice. Everywhere else gpui does none of that,
/// so the buttons do it.
pub fn buttons_act_themselves(kind: PlatformKind) -> bool {
    kind != PlatformKind::Windows
}

/// Draw the caption buttons for this platform, at the trailing end of the title
/// bar.
pub fn caption_row(
    kind: PlatformKind,
    titlebar_height: f32,
    is_maximized: bool,
    controls: WindowControls,
    cx: &mut App,
) -> impl IntoElement {
    let metrics = caption_metrics(kind, titlebar_height);
    let buttons = caption_buttons(is_maximized, controls);
    let mut row = div()
        .id("aria:titlebar:window-controls")
        .flex()
        .flex_row()
        .items_center()
        .h_full()
        .gap(px(metrics.gap))
        .px(px(metrics.padding));
    if buttons_act_themselves(kind) {
        // A press on a button is a press on a button, not the start of a window
        // drag. On Windows the press has to reach the window procedure instead,
        // which is what answers the control areas declared below.
        row = row.on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation());
    }
    for button in buttons {
        row = row.child(caption_button(kind, button, metrics, cx));
    }
    row
}

fn caption_button(
    kind: PlatformKind,
    button: CaptionButton,
    metrics: CaptionMetrics,
    cx: &mut App,
) -> impl IntoElement {
    let (hover_bg, hover_fg) = hover_colors(kind, button, cx);
    // Windows closes in red under the pointer, where the icon has to stay legible;
    // everywhere else the icon takes the bar's own quiet colour.
    let icon_color = if kind == PlatformKind::Windows && button == CaptionButton::Close {
        gpui::white()
    } else {
        crate::theme_colors::secondary(cx)
    };
    let drawn_icon: AnyElement = icon(
        format!("aria:titlebar:{}-icon", caption_id(button)),
        IconSource::Builtin(caption_icon(button).into()),
        cx,
    )
    .size(px(metrics.icon_size))
    .color(icon_color)
    .render(cx);

    div()
        .id(format!("aria:titlebar:{}", caption_id(button)))
        .flex()
        .items_center()
        .justify_center()
        .w(px(metrics.button_width))
        .h(px(metrics.button_height))
        .rounded(px(metrics.rounded))
        .occlude()
        .text_color(icon_color)
        .cursor(CursorStyle::PointingHand)
        .hover(move |this| this.bg(hover_bg).text_color(hover_fg))
        .active(move |this| this.bg(hover_bg).text_color(hover_fg))
        .window_control_area(caption_area(button))
        .child(drawn_icon)
        .when(buttons_act_themselves(kind), |this| {
            // On Windows the platform presses this button for us, out of the
            // control area declared above, so there is nothing to do here.
            this.on_click(move |event, window, cx| {
                // A double click is still one press of this button: acting on both
                // halves of it would maximize a window and put it straight back.
                if event.click_count() > 1 {
                    return;
                }
                match caption_action(button) {
                    CaptionAction::Minimize => window.minimize_window(),
                    CaptionAction::ToggleZoom => window.zoom_window(),
                    CaptionAction::RequestClose => request_close(cx),
                }
            })
        })
}

/// The colours a button takes under the pointer: Windows closes with a red
/// button, everything else with the usual ghost highlight.
fn hover_colors(kind: PlatformKind, button: CaptionButton, cx: &mut App) -> (Hsla, Hsla) {
    match (kind, button) {
        (PlatformKind::Windows, CaptionButton::Close) => {
            (gpui::rgb(CLOSE_HOVER_RED).into(), gpui::white())
        }
        _ => (
            crate::theme_colors::hover(cx),
            crate::theme_colors::primary(cx),
        ),
    }
}

/// Ask for the window to close through the app's confirmation dialog — the same
/// thing the window's own close button and the menu's Quit do.
fn request_close(cx: &mut App) {
    let root = {
        let state = cx.global::<AppState>();
        *state.show_close.lock().unwrap() = true;
        *state.root_entity_id.lock().unwrap()
    };
    if let Some(root) = root {
        cx.notify(root);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn all_controls() -> WindowControls {
        WindowControls::default()
    }

    #[test]
    fn windows_buttons_are_caption_sized_rectangles() {
        let metrics = caption_metrics(PlatformKind::Windows, 30.0);
        assert_eq!(
            metrics.button_width, 36.0,
            "Windows caption buttons are 36 wide"
        );
        assert_eq!(metrics.button_height, 30.0, "and as tall as the title bar");
        assert_eq!(metrics.gap, 0.0, "they sit flush against each other");
        assert!(metrics.icon_size > 0.0, "the buttons draw an icon");
    }

    #[test]
    fn linux_buttons_are_round_ghost_buttons() {
        let metrics = caption_metrics(PlatformKind::Linux, 30.0);
        assert_eq!(metrics.button_width, 20.0);
        assert_eq!(metrics.button_height, 20.0, "a square box");
        assert_eq!(
            metrics.rounded, 10.0,
            "half the box makes it a circle rather than a rounded square"
        );
        assert!(
            metrics.gap > 0.0 && metrics.padding > 0.0,
            "with air around them"
        );
    }

    #[test]
    fn every_platform_draws_our_own_icons() {
        assert_eq!(caption_icon(CaptionButton::Minimize), "minimize");
        assert_eq!(
            caption_icon(CaptionButton::Maximize),
            "maximize-off",
            "the plain square is the maximize glyph"
        );
        assert_eq!(
            caption_icon(CaptionButton::Restore),
            "maximize-on",
            "the doubled square is the restore glyph"
        );
        assert_eq!(caption_icon(CaptionButton::Close), "close");

        // The icons are ours, so no button depends on a system icon font being
        // installed and none of them can come out as a missing-glyph box.
        for button in [
            CaptionButton::Minimize,
            CaptionButton::Maximize,
            CaptionButton::Close,
        ] {
            assert!(
                !caption_icon(button).is_empty(),
                "every button has a shape of its own"
            );
        }
    }

    #[test]
    fn the_close_button_hovers_in_the_windows_red() {
        assert_eq!(
            CLOSE_HOVER_RED, 0x00e8_1123,
            "#E81123, the system caption red"
        );
    }

    #[test]
    fn a_maximized_window_offers_restore_instead_of_maximize() {
        assert_eq!(
            caption_buttons(false, all_controls()),
            vec![
                CaptionButton::Minimize,
                CaptionButton::Maximize,
                CaptionButton::Close
            ]
        );
        assert_eq!(
            caption_buttons(true, all_controls()),
            vec![
                CaptionButton::Minimize,
                CaptionButton::Restore,
                CaptionButton::Close
            ]
        );
    }

    #[test]
    fn buttons_the_platform_does_not_have_are_not_drawn() {
        let no_minimize = WindowControls {
            minimize: false,
            ..all_controls()
        };
        assert_eq!(
            caption_buttons(false, no_minimize),
            vec![CaptionButton::Maximize, CaptionButton::Close]
        );
        let nothing_but_close = WindowControls {
            minimize: false,
            maximize: false,
            ..all_controls()
        };
        assert_eq!(
            caption_buttons(false, nothing_but_close),
            vec![CaptionButton::Close],
            "a window without decorations can still be closed"
        );
    }

    #[test]
    fn the_close_button_asks_the_app_instead_of_ending_it() {
        assert_eq!(
            caption_action(CaptionButton::Close),
            CaptionAction::RequestClose
        );
        assert_eq!(
            caption_action(CaptionButton::Minimize),
            CaptionAction::Minimize
        );
        assert_eq!(
            caption_action(CaptionButton::Maximize),
            CaptionAction::ToggleZoom
        );
        assert_eq!(
            caption_action(CaptionButton::Restore),
            CaptionAction::ToggleZoom,
            "restoring and maximizing are the same press"
        );
    }

    #[test]
    fn every_button_answers_for_its_own_window_area() {
        assert_eq!(
            caption_area(CaptionButton::Minimize),
            WindowControlArea::Min
        );
        assert_eq!(
            caption_area(CaptionButton::Maximize),
            WindowControlArea::Max
        );
        assert_eq!(
            caption_area(CaptionButton::Restore),
            WindowControlArea::Max,
            "restoring and maximizing are the same strip of the window"
        );
        assert_eq!(caption_area(CaptionButton::Close), WindowControlArea::Close);
    }

    #[test]
    fn only_windows_lets_the_platform_press_its_buttons() {
        assert!(!buttons_act_themselves(PlatformKind::Windows));
        assert!(buttons_act_themselves(PlatformKind::MacOs));
        assert!(buttons_act_themselves(PlatformKind::Linux));
    }
}
