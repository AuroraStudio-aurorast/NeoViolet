use iced::advanced::{Shell, clipboard, layout, mouse, widget};
use iced::{Event, Font, Rectangle, Size, window};
use iced_term::{BackendCommand, Terminal, TerminalView};

#[test]
fn terminal_resize_requests_a_frame_without_more_input() {
    for interactive in [true, false] {
        let terminal = Terminal::new(1, Default::default()).unwrap();
        let mut view = TerminalView::show_with_input(&terminal, interactive);
        let mut tree = widget::Tree::new(&view);
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::MONOSPACE, 14.0.into()));
        let mut clipboard = clipboard::Null;
        let event = Event::Window(window::Event::RedrawRequested(std::time::Instant::now()));

        // Resize from a maximized surface back to a regular window, without key/mouse events.
        for size in [Size::new(2560.0, 1387.0), Size::new(800.0, 600.0)] {
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
            let mut messages = Vec::new();
            let mut shell = Shell::new(&mut messages);
            view.as_widget_mut().update(
                &mut tree,
                &event,
                layout::Layout::new(&node),
                mouse::Cursor::Unavailable,
                &renderer,
                &mut clipboard,
                &mut shell,
                &Rectangle::with_size(size),
            );
            assert_eq!(shell.redraw_request(), window::RedrawRequest::NextFrame);
            assert!(matches!(
                messages.as_slice(),
                [iced_term::Event::BackendCall(1, BackendCommand::Resize(Some(actual), _))]
                    if *actual == size
            ));

            // The follow-up frame with unchanged bounds must settle back to idle.
            let mut messages = Vec::new();
            let mut shell = Shell::new(&mut messages);
            view.as_widget_mut().update(
                &mut tree,
                &event,
                layout::Layout::new(&node),
                mouse::Cursor::Unavailable,
                &renderer,
                &mut clipboard,
                &mut shell,
                &Rectangle::with_size(size),
            );
            assert_eq!(shell.redraw_request(), window::RedrawRequest::Wait);
            assert!(messages.is_empty());
        }
    }
}
