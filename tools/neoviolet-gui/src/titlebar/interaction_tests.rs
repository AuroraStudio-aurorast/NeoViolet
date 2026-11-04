use super::*;
use iced::Length;
use iced::widget::button;

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
        for (size, position) in [
            (Size::new(480.0, 320.0), Point::new(195.0, 122.0)),
            (Size::new(1000.0, 700.0), Point::new(195.0, 172.0)),
        ] {
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
