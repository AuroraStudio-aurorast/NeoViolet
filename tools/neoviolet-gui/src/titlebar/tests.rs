use super::*;
use iced::widget::{container, text};
use iced::{Color, Length, Theme};

#[test]
fn windows_caption_hover_survives_a_client_cursor_leave_and_clears_on_native_leave() {
    use iced::advanced::{
        mouse,
        renderer::{Headless, Style},
    };
    let size = Size::new(800.0, 600.0);
    let mut state = State::new(size);
    let render = |state: &State| {
        let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
            iced::Font::DEFAULT,
            14.0.into(),
        ));
        let content = container(text("Terminal"))
            .width(Length::Fill)
            .height(Length::Fill);
        let view = state.wrap_for(content.into(), "NeoViolet", true, Platform::Windows);
        let mut ui = iced_runtime::UserInterface::build(
            view,
            size,
            iced_runtime::user_interface::Cache::new(),
            &mut renderer,
        );
        ui.draw(
            &mut renderer,
            &Theme::Dark,
            &Style {
                text_color: Color::WHITE,
            },
            mouse::Cursor::Unavailable,
        );
        renderer.screenshot(Size::new(800, 600), 1.0, Color::BLACK)
    };
    let idle = render(&state);
    state.caption_hover = Some(CaptionControl::Close);
    let close = render(&state);
    assert_ne!(
        idle, close,
        "native hover must paint even without a client-area cursor"
    );
    let index = (18 * 800 + 770) * 4;
    assert!(
        close[index] > 200 && close[index + 1] < 60,
        "Close must have its Windows red hover"
    );
    state.caption_hover = Some(CaptionControl::Maximize);
    assert_ne!(idle, render(&state));
    state.caption_hover = None;
    assert_eq!(idle, render(&state));
}

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
