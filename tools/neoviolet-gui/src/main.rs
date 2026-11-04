#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]

mod actions;
mod app;
#[cfg(any(target_os = "windows", test))]
mod chrome_hit_test;
mod config;
mod desktop;
mod dialog;
mod ipc;
mod launch;
mod lyrics;
#[cfg(any(target_os = "macos", test))]
mod lyrics_hit_test;
mod lyrics_text;
mod lyrics_window;
mod titlebar;
mod view;

fn main() -> iced::Result {
    launch::prepare_console();
    if launch::help_requested(std::env::args().skip(1)) {
        println!("{}", launch::help());
        return Ok(());
    }
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
