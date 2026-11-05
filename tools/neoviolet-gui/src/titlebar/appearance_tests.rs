use super::*;
use iced::advanced::{
    mouse,
    renderer::{Headless, Style},
};
use iced::time::{Duration, Instant};
use iced::widget::container;
use iced::{Color, Length, Theme};

fn render(state: &State, platform: Platform) -> Vec<u8> {
    let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
        iced::Font::DEFAULT,
        14.0.into(),
    ));
    let opacity = state.appearance.opacity;
    let terminal = container(iced::widget::Space::new())
        .width(Length::Fill)
        .height(Length::Fill)
        .style(move |_| container::Style {
            background: Some(Color::from_rgba8(32, 32, 32, opacity).into()),
            ..Default::default()
        });
    let view = state.wrap_for(terminal.into(), "NeoViolet", false, platform);
    let mut ui = iced_runtime::UserInterface::build(
        view,
        state.size,
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
    renderer.screenshot(Size::new(480, 320), 1.0, Color::TRANSPARENT)
}

fn pixel(image: &[u8], y: usize) -> &[u8] {
    &image[(y * 480 + 240) * 4..(y * 480 + 240) * 4 + 4]
}

#[test]
fn idle_titlebar_and_menu_bar_match_the_terminal_background() {
    for platform in [Platform::MacOs, Platform::Windows, Platform::Linux] {
        for opacity in [1.0, 0.65] {
            let mut state = State::new(Size::new(480.0, 320.0));
            state.configure(&crate::config::Config {
                opacity,
                ..Default::default()
            });
            state.menu_bar = platform != Platform::MacOs;
            let image = render(&state, platform);
            assert_eq!(
                pixel(&image, 3),
                pixel(&image, 100),
                "idle caption seam on {platform:?}, opacity {opacity}"
            );
            if state.menu_bar {
                assert_eq!(
                    pixel(&image, HEIGHT as usize + 3),
                    pixel(&image, 100),
                    "menu bar seam"
                );
            }
        }
    }
}

#[test]
fn hover_fades_both_ways_without_jumping_on_reversal_and_stops_redrawing() {
    let mut state = State::new(Size::new(480.0, 320.0));
    let start = Instant::now();
    assert!(!state.chrome_animating());
    state.update(Message::Hover(true));
    state.animate_chrome(start);
    assert!(state.chrome_animating());
    assert_eq!(
        pixel(&render(&state, Platform::MacOs), 3),
        [32, 32, 32, 255]
    );
    state.animate_chrome(start + Duration::from_millis(90));
    let middle = state.chrome_color();
    let image = render(&state, Platform::MacOs);
    assert!(pixel(&image, 3)[0] > 32 && pixel(&image, 3)[0] < 43);
    state.update(Message::Hover(false));
    state.animate_chrome(start + Duration::from_millis(90));
    assert_eq!(
        state.chrome_color(),
        middle,
        "reversing a hover must start at the displayed color"
    );
    state.animate_chrome(start + Duration::from_millis(280));
    assert_eq!(
        pixel(&render(&state, Platform::MacOs), 3),
        [32, 32, 32, 255]
    );
    assert!(!state.chrome_animating());
    state.update(Message::Hover(true));
    state.animate_chrome(start + Duration::from_millis(300));
    state.animate_chrome(start + Duration::from_millis(500));
    assert_eq!(
        pixel(&render(&state, Platform::MacOs), 3),
        [43, 43, 43, 255]
    );
    assert!(!state.chrome_animating());
}

#[test]
fn open_menus_drag_and_native_caption_hover_keep_chrome_active_until_finished() {
    let start = Instant::now();
    for mode in ["menu", "drag", "native caption"] {
        let mut state = State::new(Size::new(480.0, 320.0));
        match mode {
            "menu" => {
                state.update(Message::ToggleMenu);
            }
            "drag" => {
                state.update(Message::PressTitle);
            }
            _ => state.caption_hover = Some(CaptionControl::Maximize),
        }
        state.update(Message::Hover(false));
        state.animate_chrome(start);
        state.animate_chrome(start + Duration::from_millis(200));
        assert_eq!(
            pixel(&render(&state, Platform::Windows), 3),
            [43, 43, 43, 255],
            "{mode}"
        );
        state.close();
        state.caption_hover = None;
        state.animate_chrome(start + Duration::from_millis(220));
        state.animate_chrome(start + Duration::from_millis(420));
        assert_eq!(
            pixel(&render(&state, Platform::Windows), 3),
            [32, 32, 32, 255]
        );
    }
}

#[test]
fn moving_across_title_controls_and_menu_bar_does_not_exit_the_chrome() {
    for platform in [Platform::MacOs, Platform::Windows, Platform::Linux] {
        let mut state = State::new(Size::new(480.0, 320.0));
        state.menu_bar = platform != Platform::MacOs;
        let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
            iced::Font::DEFAULT,
            14.0.into(),
        ));
        let terminal = container(iced::widget::Space::new())
            .width(Length::Fill)
            .height(Length::Fill);
        let view = state.wrap_for(terminal.into(), "NeoViolet", false, platform);
        let mut ui = iced_runtime::UserInterface::build(
            view,
            state.size,
            iced_runtime::user_interface::Cache::new(),
            &mut renderer,
        );
        let mut clipboard = iced::advanced::clipboard::Null;
        for (index, position) in [
            Point::new(240.0, 17.0),
            Point::new(460.0, 17.0),
            Point::new(120.0, 17.0),
            Point::new(240.0, 100.0),
        ]
        .into_iter()
        .enumerate()
        {
            let mut messages = Vec::new();
            let _ = ui.update(
                &[iced::Event::Mouse(mouse::Event::CursorMoved { position })],
                mouse::Cursor::Available(position),
                &mut renderer,
                &mut clipboard,
                &mut messages,
            );
            let hovers: Vec<_> = messages
                .iter()
                .filter_map(|message| {
                    if let crate::app::Message::Titlebar(Message::Hover(hovered)) = message {
                        Some(*hovered)
                    } else {
                        None
                    }
                })
                .collect();
            assert_eq!(
                hovers,
                match index {
                    0 => vec![true],
                    3 => vec![false],
                    _ => vec![],
                },
                "{platform:?} at {position:?}"
            );
        }
    }
}
