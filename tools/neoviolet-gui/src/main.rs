#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]

mod actions;
mod app;
mod config;
mod desktop;
mod ipc;
mod launch;
mod lyrics;
mod lyrics_text;
mod lyrics_window;
mod view;

fn main() -> iced::Result {
    let (settings, warning) = config::Config::load();
    let desktop = desktop::Desktop::install();
    iced::daemon(
        move || app::App::boot(settings.clone(), warning.clone(), desktop.clone()),
        app::App::update,
        app::App::view,
    )
    .title(app::App::title_for_window)
    .theme(|app: &app::App, id| {
        if id == app.main_window {
            app.theme()
        } else {
            iced::Theme::Dark
        }
    })
    .style(|_state, theme| iced::theme::Style {
        background_color: iced::Color::TRANSPARENT,
        text_color: theme.palette().text,
    })
    .subscription(app::App::subscription)
    .run()
}
