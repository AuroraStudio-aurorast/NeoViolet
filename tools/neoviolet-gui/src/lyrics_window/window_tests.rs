use super::*;

#[cfg(target_os = "linux")]
#[tokio::test]
async fn compositor_sized_lyrics_update_fixed_size_hints_before_resizing() {
    let id = window::Id::unique();
    let size = Size::new(650.0, 210.0);
    let mut overlay = Overlay {
        window: Some(id),
        embedded_toolbar: true,
        ..Default::default()
    };
    let (task, _) = overlay.update(
        Event::Sized(id, 0, Point::ORIGIN, Size::new(300.0, 50.0), size),
        &mut Preferences::default(),
    );
    let actions = window_actions(task, Point::ORIGIN, size).await;
    assert!(matches!(actions.as_slice(), [
        iced_runtime::Action::Window(iced_runtime::window::Action::SetMaxSize(_, None)),
        iced_runtime::Action::Window(iced_runtime::window::Action::SetMinSize(_, Some(min))),
        iced_runtime::Action::Window(iced_runtime::window::Action::SetMaxSize(_, Some(max))),
        iced_runtime::Action::Window(iced_runtime::window::Action::Resize(_, resized)),
    ] if *min == size && *max == size && *resized == size));
}

#[test]
fn hover_must_not_force_the_next_lyric_into_the_previous_lines_width() {
    let mut overlay = Overlay {
        window: Some(window::Id::unique()),
        opened: true,
        size: Size::new(300.0, 50.0),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let _ = overlay.update(Event::Hover(true), &mut preferences);
    overlay.frame = serde_json::from_value(serde_json::json!({
        "elapsed":1,"lines":[{"time":0,"text":"A longer lyric that needs its natural width"}]
    }))
    .unwrap();
    assert!(overlay.display_size(&preferences).width > 300.0);
    assert!(
        iced_runtime::task::into_stream(overlay.fit(&preferences)).is_some(),
        "hovering must not prevent the lyric window from growing"
    );
}

async fn window_actions(
    task: Task<Event>,
    position: Point,
    size: Size,
) -> Vec<iced_runtime::Action<Event>> {
    use iced::futures::StreamExt;
    let mut actions = Vec::new();
    if let Some(mut stream) = iced_runtime::task::into_stream(task) {
        while let Some(action) = stream.next().await {
            match action {
                iced_runtime::Action::Window(iced_runtime::window::Action::GetPosition(
                    _,
                    sender,
                )) => {
                    sender.send(Some(position)).unwrap();
                }
                iced_runtime::Action::Window(iced_runtime::window::Action::GetSize(_, sender)) => {
                    sender.send(size).unwrap();
                }
                action @ iced_runtime::Action::Output(Event::Sized(..)) => {
                    actions.push(action);
                    break;
                }
                action => actions.push(action),
            }
        }
    }
    actions
}

#[tokio::test]
async fn new_lyrics_resize_only_the_lyric_window_while_the_toolbar_is_dragged() {
    let id = window::Id::unique();
    let toolbar = window::Id::unique();
    let initial = Size::new(300.0, 50.0);
    let mut overlay = Overlay {
        window: Some(id),
        toolbar_window: Some(toolbar),
        toolbar_position: Some(Point::new(430.0, 250.0)),
        opened: true,
        size: initial,
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let _ = overlay.update(Event::Hover(true), &mut preferences);
    let _ = overlay.update(Event::Drag, &mut preferences);
    assert!(overlay.dragging);
    overlay.frame = serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
        {"time":0,"text":"A longer lyric that needs its natural width"}
    ]}))
    .unwrap();
    let expected = overlay.display_size(&preferences);
    assert!(expected.width > initial.width);
    let actions = window_actions(overlay.fit(&preferences), Point::ORIGIN, initial).await;
    let iced_runtime::Action::Output(sized) = actions.into_iter().next().unwrap() else {
        panic!("lyrics must grow even while dragging the controls");
    };
    let moved = overlay.moved(toolbar, Point::new(530.0, 350.0), &mut preferences);
    let _ = window_actions(moved, Point::ORIGIN, initial).await;
    let (task, _) = overlay.update(sized, &mut preferences);
    let actions = window_actions(task, Point::ORIGIN, initial).await;
    assert!(actions.iter().any(|action| matches!(action,
        iced_runtime::Action::Window(iced_runtime::window::Action::Resize(window, size))
            if *window == id && *size == expected)));
    assert!(actions.iter().any(|action| matches!(action,
        iced_runtime::Action::Window(iced_runtime::window::Action::Move(window, point))
            if *window == id && point.y == 430.0
                && (point.x + expected.width / 2.0 - 650.0).abs() < 0.1)));
    assert!(!actions.iter().any(|action| matches!(action,
        iced_runtime::Action::Window(iced_runtime::window::Action::Move(window, _)
            | iced_runtime::window::Action::Resize(window, _)) if *window == toolbar)));
    assert_eq!(preferences.font_size, 32);
    assert_eq!(overlay.toolbar_position, Some(Point::new(530.0, 350.0)));
}

#[cfg(target_os = "macos")]
#[test]
fn native_pair_movement_saves_position_without_sending_a_delayed_move_back_to_appkit() {
    let lyrics = window::Id::unique();
    let toolbar = window::Id::unique();
    let mut overlay = Overlay {
        window: Some(lyrics),
        toolbar_window: Some(toolbar),
        size: Size::new(300.0, 50.0),
        native_linked: true,
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let task = overlay.moved(toolbar, Point::new(430.0, 300.0), &mut preferences);
    assert!(iced_runtime::task::into_stream(task).is_none());
    assert_eq!(
        preferences.position.map(|p| (p.x, p.y)),
        Some((400.0, 300.0))
    );
    let _ = overlay.update(Event::Linked(window::Id::unique(), false), &mut preferences);
    assert!(
        overlay.native_linked,
        "a closed toolbar must not replace the active link"
    );
    let _ = overlay.close();
    assert!(!overlay.native_linked);
}

#[tokio::test]
async fn returning_to_the_current_size_cancels_a_pending_expansion() {
    let id = window::Id::unique();
    let initial = Size::new(300.0, 50.0);
    let mut overlay = Overlay {
        window: Some(id),
        opened: true,
        size: initial,
        frame: serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"text":"A very long lyric ".repeat(40)}
        ]}))
        .unwrap(),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let pending = overlay.fit(&preferences);
    let mut actions = window_actions(pending, Point::ORIGIN, initial).await;
    let iced_runtime::Action::Output(stale) = actions.pop().unwrap() else {
        panic!("a size query must produce a sizing event");
    };
    overlay.clear();
    assert!(iced_runtime::task::into_stream(overlay.fit(&preferences)).is_none());
    let (task, _) = overlay.update(stale, &mut preferences);
    assert!(iced_runtime::task::into_stream(task).is_none());
    assert_eq!(overlay.size, initial);
}

#[test]
fn crossing_windows_and_dragging_do_not_hide_the_toolbar() {
    let mut overlay = Overlay {
        window: Some(window::Id::unique()),
        toolbar_window: Some(window::Id::unique()),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let _ = overlay.update(Event::Hover(true), &mut preferences);
    assert!(overlay.toolbar_visible);
    let _ = overlay.update(Event::Hover(false), &mut preferences);
    let old_revision = overlay.hover_revision;
    let _ = overlay.update(Event::ToolbarHover(true), &mut preferences);
    let _ = overlay.update(Event::HideToolbar(old_revision), &mut preferences);
    assert!(overlay.toolbar_visible);
    let _ = overlay.update(Event::Drag, &mut preferences);
    let _ = overlay.update(Event::ToolbarHover(false), &mut preferences);
    let _ = overlay.update(Event::HideToolbar(overlay.hover_revision), &mut preferences);
    assert!(overlay.toolbar_visible);
    let _ = overlay.update(Event::DragEnded, &mut preferences);
    let _ = overlay.update(Event::HideToolbar(overlay.hover_revision), &mut preferences);
    assert!(!overlay.toolbar_visible);
    let _ = overlay.update(Event::Visible(false), &mut preferences);
    assert!(overlay.window.is_none());
    assert!(overlay.toolbar_window.is_none());
    assert!(overlay.toolbar_position.is_none());
}

#[test]
fn unavailable_absolute_position_uses_one_window_and_restores_the_latest_cue_after_drag() {
    let id = window::Id::unique();
    let mut overlay = Overlay {
        window: Some(id),
        opened: true,
        locating: true,
        size: Size::new(300.0, 50.0),
        frame: serde_json::from_value(serde_json::json!({
            "elapsed":1,"lines":[{"time":0,"text":"Short"}]
        }))
        .unwrap(),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let _ = overlay.update(
        Event::ToolbarRequested(id, None, overlay.size),
        &mut preferences,
    );
    assert!(overlay.embedded_toolbar);
    assert!(overlay.toolbar_window.is_none());
    let size = overlay.display_size(&preferences);
    let _ = overlay.update(Event::Drag, &mut preferences);
    overlay.frame = serde_json::from_value(serde_json::json!({
        "elapsed":2,"lines":[{"time":1,"text":"A considerably longer next lyric"}]
    }))
    .unwrap();
    assert_eq!(
        overlay.display_frame().display_lines()[0].first_text(),
        "Short"
    );
    assert_eq!(overlay.display_size(&preferences), size);
    let _ = overlay.update(Event::DragEnded, &mut preferences);
    assert_eq!(overlay.display_frame().elapsed, 2.0);
    assert!(overlay.display_size(&preferences).width > size.width);
    assert!(overlay.drag_snapshot.is_none());
}

#[tokio::test]
async fn the_toolbar_opens_at_a_fixed_size_above_the_lyrics_without_moving_them() {
    use iced::futures::StreamExt;
    let root = window::Id::unique();
    let mut overlay = Overlay {
        window: Some(root),
        opened: true,
        locating: true,
        size: Size::new(300.0, 50.0),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let (task, _) = overlay.update(
        Event::ToolbarRequested(root, Some(Point::new(400.0, 380.0)), overlay.size),
        &mut preferences,
    );
    let mut stream = iced_runtime::task::into_stream(task).unwrap();
    let Some(iced_runtime::Action::Window(iced_runtime::window::Action::Open(id, settings, _))) =
        stream.next().await
    else {
        panic!("a separate toolbar must open");
    };
    assert_eq!(Some(id), overlay.toolbar_window);
    assert_eq!(settings.size, Size::new(240.0, 88.0));
    assert_eq!(overlay.toolbar_position, Some(Point::new(430.0, 300.0)));
    assert_eq!(
        overlay.lyrics_position(Point::new(430.0, 300.0), overlay.size),
        Point::new(400.0, 380.0)
    );
    let _ = overlay.moved(root, Point::new(400.0, 380.0), &mut preferences);
    assert_eq!(
        preferences.position.map(|p| (p.x, p.y)),
        Some((400.0, 300.0))
    );
}

#[cfg(target_os = "macos")]
#[tokio::test]
async fn paused_hover_captures_both_windows_and_cancels_stale_masks() {
    use iced::futures::StreamExt;
    async fn capture(task: Task<Event>) -> (window::Id, u64) {
        let mut stream = iced_runtime::task::into_stream(task).unwrap();
        while let Some(action) = stream.next().await {
            if let iced_runtime::Action::Output(Event::Capture(id, revision)) = action {
                return (id, revision);
            }
        }
        panic!("a visual change must capture the current alpha mask");
    }
    let id = window::Id::unique();
    let toolbar = window::Id::unique();
    let mut overlay = Overlay {
        window: Some(id),
        toolbar_window: Some(toolbar),
        opened: true,
        toolbar_opened: true,
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    overlay.size = overlay.display_size(&preferences);
    let (task, _) = overlay.update(Event::Hover(true), &mut preferences);
    let (window, old_revision) = capture(task).await;
    assert!(window == id || window == toolbar);
    let (task, _) = overlay.update(Event::Hover(false), &mut preferences);
    let (_, revision) = capture(task).await;
    assert_ne!(revision, old_revision);
    for id in [id, toolbar] {
        let (task, _) = overlay.update(Event::Capture(id, old_revision), &mut preferences);
        assert!(iced_runtime::task::into_stream(task).is_none());
    }
    let actions = window_actions(overlay.refresh_mask(), Point::ORIGIN, overlay.size).await;
    assert_eq!(
        actions
            .iter()
            .filter(|action| matches!(action, iced_runtime::Action::Output(Event::Capture(_, _))))
            .count(),
        2
    );
}

#[tokio::test]
async fn frames_received_during_opening_are_sized_after_the_window_is_ready() {
    let id = window::Id::unique();
    let mut overlay = Overlay {
        window: Some(id),
        size: Size::new(300.0, 50.0),
        frame: serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"text":"A considerably longer line than the initial waiting message"}
        ]}))
        .unwrap(),
        ..Default::default()
    };
    let mut preferences = Preferences::default();
    let initial = overlay.size;
    let _ = overlay.fit(&preferences);
    assert_eq!(overlay.size, initial);
    let expected = overlay.display_size(&preferences);
    assert!(expected.width > initial.width);
    let _ = overlay.update(Event::Opened(id), &mut preferences);
    assert!(overlay.opened);
    assert!(overlay.locating);
    assert!(iced_runtime::task::into_stream(overlay.fit(&preferences)).is_none());
    let (task, _) = overlay.update(
        Event::ToolbarRequested(id, Some(Point::new(400.0, 380.0)), initial),
        &mut preferences,
    );
    assert_eq!(overlay.toolbar_position, Some(Point::new(430.0, 300.0)));
    let actions = window_actions(task, Point::ORIGIN, initial).await;
    let event = actions
        .into_iter()
        .find_map(|action| match action {
            iced_runtime::Action::Output(event @ Event::Sized(..)) => Some(event),
            _ => None,
        })
        .expect("the ready window must request its latest lyric size");
    let _ = overlay.update(event, &mut preferences);
    assert_eq!(overlay.size, expected);
}
