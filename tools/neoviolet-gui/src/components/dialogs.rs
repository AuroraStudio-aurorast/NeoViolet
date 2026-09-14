use gpui::{
    AnyElement, App, Bounds, ClickEvent, Entity, IntoElement, Window, WindowBackgroundAppearance,
    WindowBounds, WindowDecorations, WindowKind, WindowOptions, div, point, prelude::*, px, size,
};
use yororen_ui::headless::button::button;
use yororen_ui::headless::modal::{ModalState, modal};

use crate::config::DesktopLyricsConfig;
use crate::state::AppState;
use crate::theme_colors;

// ── Shared helpers ──

/// Build WindowOptions for a desktop lyrics overlay window from config.
pub fn lyrics_window_options(cfg: &DesktopLyricsConfig) -> WindowOptions {
    WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(Bounds::new(
            point(
                px(cfg.position_x.unwrap_or(100) as f32),
                px(cfg.position_y.unwrap_or(100) as f32),
            ),
            size(px(cfg.window_width as f32), px(cfg.window_height as f32)),
        ))),
        titlebar: None,
        focus: false,
        window_background: WindowBackgroundAppearance::Transparent,
        kind: if cfg!(target_os = "macos") {
            WindowKind::Normal
        } else {
            WindowKind::PopUp
        },
        window_decorations: if cfg!(target_os = "linux") {
            Some(WindowDecorations::Client)
        } else {
            None
        },
        ..Default::default()
    }
}

// ── Public helpers ──

pub fn open_about(cx: &mut App) {
    let eid = {
        let state = cx.global::<AppState>();
        *state.show_about.lock().unwrap() = true;
        *state.root_entity_id.lock().unwrap()
    };
    if let Some(eid) = eid {
        cx.notify(eid);
    }
}

pub fn open_close(cx: &mut App) {
    let eid = {
        let state = cx.global::<AppState>();
        *state.show_close.lock().unwrap() = true;
        *state.root_entity_id.lock().unwrap()
    };
    if let Some(eid) = eid {
        cx.notify(eid);
    }
}

// ── Dialog shell ──

/// The one place that knows how yororen-ui 0.3 composes a dialog.
///
/// Two things changed from 0.2 and both would fail quietly if missed. The modal
/// renderer draws only the panel now — the scrim and the centring are the
/// caller's job — and it renders *nothing at all* unless its `ModalState` entity
/// has been opened, so the entity is passed in rather than created here.
fn dialog_shell(
    cx: &mut App,
    state: &Entity<ModalState>,
    id: &'static str,
    width: f32,
    title: &str,
    content: impl IntoElement,
    actions: Vec<AnyElement>,
) -> AnyElement {
    // The panel's own background, border and padding come from the theme via
    // the renderer; only the width is still ours to choose.
    let panel = div().w(px(width)).child(
        modal(format!("{id}:modal"), state.clone())
            .child(
                div()
                    .id(format!("{id}:title"))
                    .text_lg()
                    .font_weight(gpui::FontWeight::BOLD)
                    .text_color(theme_colors::primary(cx))
                    .child(title.to_string()),
            )
            .child(content)
            .child(
                div()
                    .flex()
                    .flex_row()
                    .justify_end()
                    .gap_2()
                    .children(actions),
            )
            .render(cx),
    );

    div()
        .id(id)
        .absolute()
        .size_full()
        .bg(gpui::rgba(0x000000cc))
        .flex()
        .items_center()
        .justify_center()
        .child(panel)
        .into_any_element()
}

// ── About dialog ──

pub fn render_about_dialog(
    cx: &mut App,
    state: &Entity<ModalState>,
    on_dismiss: impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static,
) -> impl IntoElement {
    let gui_ver = crate::menus::GUI_VER;
    let cli_ver = cx.global::<AppState>().cli_version.lock().unwrap().clone();

    let content = div()
        .flex()
        .flex_col()
        .gap_2()
        .child(
            div()
                .id("aria:about:version")
                .text_sm()
                .text_color(theme_colors::secondary(cx))
                .child(format!("GUI {}; CLI {}", gui_ver, cli_ver)),
        )
        .child(
            div()
                .id("aria:about:credits")
                .text_sm()
                .text_color(theme_colors::tertiary(cx))
                .child("GUI Credits:\nGPUI-CE, Yororen UI, Alacritty, etc."),
        )
        .child(
            div()
                .id("aria:about:license")
                .text_sm()
                .text_color(theme_colors::tertiary(cx))
                .child("GUI License: GPL-3.0\nCLI License: MIT License"),
        );

    let actions = vec![
        button("aria:btn:about-ok", cx)
            .caption("OK")
            .on_click(on_dismiss)
            .render(cx)
            .into_any_element(),
    ];

    dialog_shell(
        cx,
        state,
        "aria:dialog:about",
        400.,
        "NeoViolet GUI",
        content,
        actions,
    )
}

// ── Close confirmation dialog ──

pub fn render_close_dialog(
    cx: &mut App,
    state: &Entity<ModalState>,
    on_cancel: impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static,
    on_quit: impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static,
) -> impl IntoElement {
    let content = div()
        .id("aria:close:message")
        .text_sm()
        .text_color(theme_colors::secondary(cx))
        .child(
            "NeoViolet is still running. Are you sure you want to quit?\n\
             Quit means the audio playback will stop.",
        );

    let actions = vec![
        button("aria:btn:cancel-quit", cx)
            .caption("Cancel")
            .on_click(on_cancel)
            .render(cx)
            .into_any_element(),
        button("aria:btn:confirm-quit", cx)
            .caption("Quit")
            .on_click(on_quit)
            .render(cx)
            .into_any_element(),
    ];

    dialog_shell(
        cx,
        state,
        "aria:dialog:close-confirm",
        400.,
        "Quit NeoViolet?",
        content,
        actions,
    )
}

// ── Exit error / no-terminal dialog (NOT Esc-dismissable) ──

pub fn render_error_dialog(
    cx: &mut App,
    state: &Entity<ModalState>,
    title: &str,
    message: &str,
    button_label: &str,
    on_action: impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static,
    on_dismiss: Option<impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static>,
) -> impl IntoElement {
    let message = message.to_string();
    let button_label = button_label.to_string();

    let mut actions: Vec<AnyElement> = Vec::new();
    if let Some(dismiss) = on_dismiss {
        actions.push(
            button("aria:btn:dismiss", cx)
                .caption("Close")
                .on_click(dismiss)
                .render(cx)
                .into_any_element(),
        );
    }
    actions.push(
        button("aria:btn:restart", cx)
            .caption(button_label)
            .on_click(on_action)
            .render(cx)
            .into_any_element(),
    );

    let content = div()
        .id("aria:error:message")
        .text_sm()
        .text_color(theme_colors::secondary(cx))
        .child(message);

    dialog_shell(
        cx,
        state,
        "aria:dialog:error",
        420.,
        title,
        content,
        actions,
    )
}

// ── Bad-args dialog — displays captured help text (NOT Esc-dismissable) ──

pub fn render_bad_args_dialog(
    cx: &mut App,
    state: &Entity<ModalState>,
    output_text: &str,
    on_quit: impl Fn(&ClickEvent, &mut Window, &mut App) + Send + Sync + 'static,
) -> impl IntoElement {
    let output_text = output_text.to_string();

    let content = div()
        .flex()
        .flex_col()
        .gap_2()
        .child(
            div()
                .id("aria:bad-args:error-banner")
                .text_sm()
                .text_color(theme_colors::tertiary(cx))
                .child(
                    "The arguments you provided were not recognized by NeoViolet.\n\
                     See the help output below:",
                ),
        )
        .child(
            div()
                .id("aria:bad-args:help-output")
                .max_h(px(300.))
                .overflow_y_scroll()
                .px_2()
                .py_1()
                .font_family("Menlo")
                .text_xs()
                .text_color(theme_colors::secondary(cx))
                .bg(theme_colors::sunken(cx))
                .rounded_md()
                .child(output_text),
        );

    let actions = vec![
        button("aria:btn:bad-args-close", cx)
            .caption("Close")
            .on_click(on_quit)
            .render(cx)
            .into_any_element(),
    ];

    dialog_shell(
        cx,
        state,
        "aria:dialog:bad-args",
        640.,
        "NeoViolet: Invalid Arguments",
        content,
        actions,
    )
}
