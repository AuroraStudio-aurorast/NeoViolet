use gpui::{div, prelude::*, px, App, ClickEvent, IntoElement, Window};
use yororen_ui::component::{button, modal, modal_actions_row};
use yororen_ui::theme::ActiveTheme;

use crate::state::AppState;

// ── Public helpers to toggle dialog flags ──

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

// ── About dialog ──

pub fn render_about_dialog(
    cx: &mut App,
    on_dismiss: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> impl IntoElement {
    let theme = cx.theme();
    let state = cx.global::<AppState>();
    let gui_ver = crate::menus::GUI_VER;
    let cli_ver = state.cli_version.lock().unwrap().clone();

    div()
        .absolute()
        .size_full()
        .bg(gpui::rgba(0x000000cc))
        .flex()
        .items_center()
        .justify_center()
        .child(
            modal()
                .title("NeoViolet GUI")
                .width(px(400.))
                .bg(theme.surface.raised)
                .content(
                    div()
                        .flex()
                        .flex_col()
                        .gap_2()
                        .child(
                            div()
                                .text_sm()
                                .text_color(theme.content.secondary)
                                .child(format!("GUI {}; CLI {}", gui_ver, cli_ver)),
                        )
                        .child(
                            div()
                                .text_sm()
                                .text_color(theme.content.tertiary)
                                .child("GUI Credits:\nGPUI-CE, Yororen UI, Alacritty, etc."),
                        ),
                )
                .actions(
                    modal_actions_row(
                        [button("about-ok").child("OK").on_click(on_dismiss).into_any_element()],
                    ),
                ),
        )
}

// ── Close confirmation dialog ──

pub fn render_close_dialog(
    cx: &mut App,
    on_cancel: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    on_quit: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> impl IntoElement {
    let theme = cx.theme();

    div()
        .absolute()
        .size_full()
        .bg(gpui::rgba(0x000000cc))
        .flex()
        .items_center()
        .justify_center()
        .child(
            modal()
                .title("Quit NeoViolet?")
                .width(px(400.))
                .bg(theme.surface.raised)
                .content(
                    div()
                        .text_sm()
                        .text_color(theme.content.secondary)
                        .child("NeoViolet is still running. Are you sure you want to quit?\nQuit means the audio playback will stop."),
                )
                .actions(
                    modal_actions_row([
                        button("cancel-btn")
                            .child("Cancel")
                            .on_click(on_cancel)
                            .into_any_element(),
                        button("quit-btn")
                            .child("Quit")
                            .on_click(on_quit)
                            .into_any_element(),
                    ]),
                ),
        )
}

// ── Exit error / no-terminal dialog ──

pub fn render_error_dialog(
    cx: &mut App,
    title: &str,
    message: &str,
    button_label: &str,
    on_action: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    on_dismiss: Option<impl Fn(&ClickEvent, &mut Window, &mut App) + 'static>,
) -> impl IntoElement {
    let theme = cx.theme();
    // Convert to owned Strings to avoid borrowed-data-escapes errors
    let title = title.to_string();
    let message = message.to_string();
    let button_label = button_label.to_string();

    let mut actions: Vec<gpui::AnyElement> = Vec::new();
    if let Some(dismiss) = on_dismiss {
        actions.push(
            button("dismiss-btn")
                .child("Close")
                .on_click(dismiss)
                .into_any_element(),
        );
    }
    actions.push(
        button("restart-btn")
            .child(button_label)
            .on_click(on_action)
            .into_any_element(),
    );

    div()
        .absolute()
        .size_full()
        .bg(gpui::rgba(0x000000cc))
        .flex()
        .items_center()
        .justify_center()
        .child(
            modal()
                .title(title)
                .width(px(420.))
                .bg(theme.surface.raised)
                .content(
                    div()
                        .text_sm()
                        .text_color(theme.content.secondary)
                        .child(message),
                )
                .actions(modal_actions_row(actions)),
        )
}
