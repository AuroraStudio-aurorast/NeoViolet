#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]

mod actions;
mod app;
mod config;
mod desktop;
mod ipc;
mod launch;
mod view;

fn main() -> iced::Result {
    let (settings, warning) = config::Config::load();
    let window = iced::window::Settings {
        size: iced::Size::new(settings.window_width as f32, settings.window_height as f32),
        min_size: Some(iced::Size::new(480.0, 320.0)),
        transparent: settings.opacity < 1.0,
        exit_on_close_request: false,
        ..Default::default()
    };
    let desktop = desktop::Desktop::install();
    iced::application(
        move || app::App::boot(settings.clone(), warning.clone(), desktop.clone()),
        app::App::update,
        app::App::view,
    )
    .title(app::App::title)
    .window(window)
    .theme(app::App::theme)
    .style(|_state, theme| iced::theme::Style {
        background_color: iced::Color::TRANSPARENT,
        text_color: theme.palette().text,
    })
    .subscription(app::App::subscription)
    .run()
}
