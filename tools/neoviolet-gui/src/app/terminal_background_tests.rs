//! Pixel regression for PTY backgrounds at non-integral terminal grid sizes.
use super::*;
use iced::advanced::{
    mouse,
    renderer::{Headless, Style},
};
use iced::widget::container;
use iced::{Color, Element, Length, Size};
use iced_term::{BackendCommand, Terminal, TerminalView, settings};

const PAINT: &str = concat!(
    "stty -echo; read ready; printf '",
    r"\033[?25l\033[48;2;10;20;30m\033[2J",
    r"\033[2;1H\033[48;2;28;22;41m\033[2K",
    r"\033[3;1H\033[48;2;60;40;80m\033[2K\033[0m",
    "'; read done",
);

fn render(terminal: &Terminal, size: Size<u32>) -> Vec<u8> {
    let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    let terminal: Element<'_, Message> =
        TerminalView::show_with_input(terminal, false).map(Message::Terminal);
    let view: Element<'_, Message> = container(terminal)
        .padding([7, 11])
        .width(Length::Fill)
        .height(Length::Fill)
        .into();
    let mut ui = iced_runtime::UserInterface::build(
        view,
        Size::new(size.width as f32, size.height as f32),
        iced_runtime::user_interface::Cache::new(),
        &mut renderer,
    );
    ui.draw(
        &mut renderer,
        &iced::Theme::Dark,
        &Style {
            text_color: Color::WHITE,
        },
        mouse::Cursor::Unavailable,
    );
    renderer.screenshot(size, 1.0, Color::TRANSPARENT)
}

fn pixel(image: &[u8], size: Size<u32>, x: u32, y: u32) -> &[u8] {
    let offset = ((y * size.width + x) * 4) as usize;
    &image[offset..offset + 4]
}

#[tokio::test]
async fn terminal_background_reaches_fractional_grid_edges_without_spilling_outside_the_widget() {
    for opacity in [1.0, 0.65] {
        for inner in [Size::new(103, 65), Size::new(127, 83), Size::new(100, 60)] {
            let mut terminal = Terminal::new(
                712,
                settings::Settings {
                    backend: settings::BackendSettings {
                        program: "/bin/sh".into(),
                        args: vec!["-c".into(), PAINT.into()],
                        ..Default::default()
                    },
                    theme: settings::ThemeSettings::new(Box::new(iced_term::ColorPalette {
                        background: "#000000".into(),
                        ..Default::default()
                    })),
                    ..Default::default()
                },
            )
            .unwrap();
            terminal.set_opacity(opacity);
            terminal.handle(iced_term::Command::ProxyToBackend(BackendCommand::Resize(
                Some(Size::new(inner.width as f32, inner.height as f32)),
                Some(Size::new(10.0, 20.0)),
            )));
            terminal.handle(iced_term::Command::ProxyToBackend(BackendCommand::Write(
                b"ready\n".to_vec(),
            )));
            let size = Size::new(inner.width + 22, inner.height + 14);
            let image = tokio::time::timeout(std::time::Duration::from_secs(3), async {
                loop {
                    terminal.handle(iced_term::Command::ProxyToBackend(
                        BackendCommand::ProcessAlacrittyEvent(iced_term::AlacrittyEvent::Wakeup),
                    ));
                    let image = render(&terminal, size);
                    if pixel(&image, size, 16, 12)[0] > 0 {
                        break image;
                    }
                    tokio::time::sleep(std::time::Duration::from_millis(10)).await;
                }
            })
            .await
            .expect("fixture must reach the terminal before pixel assertions");
            let grid_width = inner.width / 10 * 10;
            let grid_height = inner.height / 20 * 20;
            for row in 0..inner.height / 20 {
                assert_eq!(
                    pixel(&image, size, 11 + inner.width - 1, 7 + row * 20 + 10),
                    pixel(&image, size, 11 + grid_width - 2, 7 + row * 20 + 10),
                    "right gutter at opacity {opacity}, size {inner:?}, row {row}"
                );
            }
            for column in 0..inner.width / 10 {
                assert_eq!(
                    pixel(&image, size, 11 + column * 10 + 5, 7 + inner.height - 1),
                    pixel(&image, size, 11 + column * 10 + 5, 7 + grid_height - 2),
                    "bottom gutter at opacity {opacity}, size {inner:?}, column {column}"
                );
            }
            assert_eq!(
                pixel(&image, size, 11 + inner.width - 1, 7 + inner.height - 1),
                pixel(&image, size, 11 + grid_width - 2, 7 + grid_height - 2),
                "bottom-right corner must be painted exactly once"
            );
            for (x, y) in [(0, 0), (10, 10), (size.width - 1, size.height - 1), (15, 6)] {
                assert_eq!(
                    pixel(&image, size, x, y),
                    [0, 0, 0, 0],
                    "terminal background escaped its widget"
                );
            }
        }
    }
}
