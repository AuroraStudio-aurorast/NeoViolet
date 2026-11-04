use super::*;
use crate::app::{Dialog, Message};
use iced::advanced::mouse;
use iced::{Length, Point, Size};

#[test]
fn keyboard_navigation_keeps_quit_safe_and_reaches_both_actions() {
    use iced::keyboard::{Key, key::Named};
    let mut focus = Navigation::default();
    assert!(matches!(
        focus.key(&Dialog::Quit, true, &Key::Named(Named::Enter), false),
        Some(Message::Dismiss)
    ));
    assert!(!focus.visible());
    assert!(
        focus
            .key(&Dialog::Quit, true, &Key::Named(Named::Tab), false)
            .is_none()
    );
    assert!(focus.visible());
    assert!(matches!(
        focus.key(&Dialog::Quit, true, &Key::Named(Named::Enter), false),
        Some(Message::ConfirmQuit)
    ));
    focus.key(&Dialog::Quit, true, &Key::Named(Named::Tab), true);
    assert!(matches!(
        focus.key(&Dialog::Quit, true, &Key::Named(Named::Space), false),
        Some(Message::Dismiss)
    ));
}

#[test]
fn stopped_player_prefers_restart_and_single_action_dialogs_do_not_escape_focus() {
    use iced::keyboard::{Key, key::Named};
    let mut focus = Navigation::default();
    let error = Dialog::Error("Player stopped".into());
    assert!(matches!(
        focus.key(&error, false, &Key::Named(Named::Enter), false),
        Some(Message::Restart)
    ));
    for shift in [true, false] {
        focus.reset();
        focus.key(&Dialog::About, true, &Key::Named(Named::Tab), shift);
        assert!(matches!(
            focus.key(&Dialog::About, true, &Key::Named(Named::Enter), false),
            Some(Message::Dismiss)
        ));
    }
}

#[test]
fn long_errors_keep_the_restart_action_clickable_at_the_minimum_window_size() {
    let message = "An audio file could not be opened. ".repeat(100);
    let dialog = Dialog::Error(message);
    let size = Size::new(480.0, 320.0);
    let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    let navigation = Navigation::default();
    let view = iced::widget::stack![
        iced::widget::button("Terminal")
            .width(Length::Fill)
            .height(Length::Fill)
            .on_press(Message::ConfirmQuit),
        overlay(&dialog, "NeoViolet test", false, &navigation),
    ];
    let mut ui = iced_runtime::UserInterface::build(
        view,
        size,
        iced_runtime::user_interface::Cache::new(),
        &mut renderer,
    );
    let mut clipboard = iced::advanced::clipboard::Null;
    let mut messages = Vec::new();
    // The action row stays below the bounded, independently scrolling details.
    let position = Point::new(400.0, 259.0);
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
        messages
            .iter()
            .any(|message| matches!(message, Message::Restart))
    );
    assert!(
        !messages
            .iter()
            .any(|message| matches!(message, Message::ConfirmQuit))
    );
}
