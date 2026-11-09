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

#[test]
fn setup_menus_keep_all_items_visible_but_disable_mouse_and_keyboard_dispatch() {
    use iced::advanced::mouse;
    let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    for platform in [Platform::Windows, Platform::Linux] {
        let size = Size::new(800.0, 600.0);
        let mut state = State::new(size);
        let available = state.groups(platform, true);
        state.commands_disabled = true;
        let disabled = state.groups(platform, true);
        assert_eq!(available.len(), disabled.len());
        for (before, during) in available.iter().zip(disabled.iter()) {
            assert_eq!(before.items.len(), during.items.len());
            for (before, during) in before.items.iter().zip(during.items.iter()) {
                assert_eq!(before.label, during.label);
                assert!(!state.item_enabled(during, true));
            }
        }
        for (group, items) in disabled.iter().enumerate() {
            state.update(Message::SelectGroup(group));
            for item in 0..items.items.len() {
                state.update(Message::HoverItem(item));
                assert!(state.key(&Key::Named(Named::Enter), true).is_none());
                assert!(state.key(&Key::Named(Named::Space), true).is_none());
                assert!(
                    state
                        .update(Message::Action(items.items[item].action))
                        .is_none()
                );
            }
        }
        state.update(Message::SelectGroup(0));
        let position = Point::new(195.0, 96.0);
        let content = button("Terminal sentinel")
            .on_press(crate::app::Message::ConfirmQuit)
            .width(Length::Fill)
            .height(Length::Fill);
        let mut ui = iced_runtime::UserInterface::build(
            state.wrap_for(content.into(), "Setup", true, platform),
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
            !messages.iter().any(|message| matches!(
                message,
                crate::app::Message::Titlebar(Message::Action(_))
                    | crate::app::Message::ConfirmQuit
            )),
            "disabled menu must neither dispatch nor click through"
        );
    }
}

#[test]
fn startup_menus_keep_zoom_and_help_enabled_but_block_open_and_lyrics() {
    let mut state = State::new(Size::new(800.0, 600.0));
    state.player_unavailable = true;
    for platform in [Platform::Windows, Platform::Linux] {
        let groups = state.groups(platform, true);
        for group in &groups {
            for item in &group.items {
                if matches!(
                    item.action,
                    Action::ZoomIn | Action::About | Action::Quit | Action::Copy
                ) {
                    assert!(
                        state.item_enabled(item, true),
                        "basic command disabled: {}",
                        item.label
                    );
                }
                if matches!(item.action, Action::Open | Action::DesktopLyrics) {
                    assert!(
                        !state.item_enabled(item, true),
                        "playback command enabled: {}",
                        item.label
                    );
                }
            }
        }
    }
}

#[test]
fn startup_menu_keyboard_skips_unavailable_player_commands() {
    let mut state = State::new(Size::new(800.0, 600.0));
    state.player_unavailable = true;
    state.update(Message::SelectGroup(0));
    state.update(Message::HoverItem(0));
    assert!(state.key(&Key::Named(Named::Enter), true).is_none());
    assert!(state.update(Message::Action(Action::Open)).is_none());
    state.key(&Key::Named(Named::ArrowDown), true);
    assert!(matches!(
        state.key(&Key::Named(Named::Enter), true),
        Some(Action::Quit)
    ));
    state.update(Message::SelectGroup(2));
    state.update(Message::HoverItem(0));
    state.key(&Key::Named(Named::End), true);
    assert!(matches!(
        state.key(&Key::Named(Named::Enter), true),
        Some(Action::ZoomReset)
    ));
    state.update(Message::SelectGroup(4));
    state.update(Message::HoverItem(1));
    assert!(matches!(
        state.key(&Key::Named(Named::Enter), true),
        Some(Action::About)
    ));
}

#[test]
fn macos_title_input_starts_after_the_measured_native_button_reservation() {
    use iced::advanced::mouse;
    let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    for (inset, x, title_input) in [
        (None, 80.0, true),
        (Some(86.0), 82.0, false),
        (Some(86.0), 90.0, true),
    ] {
        let mut state = State::new(Size::new(480.0, 320.0));
        state.mac_title_inset = inset;
        let content = iced::widget::container(iced::widget::Space::new())
            .width(Length::Fill)
            .height(Length::Fill);
        let view = state.wrap_for(content.into(), "NeoViolet", false, Platform::MacOs);
        let mut ui = iced_runtime::UserInterface::build(
            view,
            state.size,
            iced_runtime::user_interface::Cache::new(),
            &mut renderer,
        );
        let position = Point::new(x, 17.0);
        let mut messages = Vec::new();
        let _ = ui.update(
            &[iced::Event::Mouse(mouse::Event::ButtonPressed(
                mouse::Button::Left,
            ))],
            mouse::Cursor::Available(position),
            &mut renderer,
            &mut iced::advanced::clipboard::Null,
            &mut messages,
        );
        assert_eq!(
            messages.iter().any(|message| matches!(
                message,
                crate::app::Message::Titlebar(Message::PressTitle)
            )),
            title_input,
            "the button reservation must remain outside the draggable title at x={x}"
        );
    }
}
