use super::*;
use iced::advanced::widget::operation::Focusable;
use iced::futures::StreamExt;

#[derive(Default)]
struct FocusState(bool);

impl Focusable for FocusState {
    fn is_focused(&self) -> bool {
        self.0
    }
    fn focus(&mut self) {
        self.0 = true;
    }
    fn unfocus(&mut self) {
        self.0 = false;
    }
}

async fn focuses_terminal(app: &App, task: Task<Message>) -> bool {
    let target = app.terminal.as_ref().unwrap().widget_id().clone();
    let mut focus = FocusState::default();
    if let Some(mut stream) = iced_runtime::task::into_stream(task) {
        while let Some(action) = stream.next().await {
            match action {
                iced_runtime::Action::Widget(mut operation) => {
                    operation.focusable(Some(&target), iced::Rectangle::default(), &mut focus);
                }
                iced_runtime::Action::Window(window::Action::GetMaximized(_, reply)) => {
                    let _ = reply.send(false);
                }
                iced_runtime::Action::Window(window::Action::GetMode(_, reply)) => {
                    let _ = reply.send(window::Mode::Windowed);
                }
                _ => {}
            }
        }
    }
    focus.0
}

fn with_terminal() -> App {
    let mut app = app();
    app.dialog = None;
    app.terminal = Some(Terminal::new(2, iced_term::settings::Settings::default()).unwrap());
    app
}

#[test]
fn preferences_requests_tui_settings_without_changing_playback() {
    let mut app = with_terminal();
    app.ipc_connected = true;
    app.handshake.capabilities = vec!["settings_navigation".into(), "results".into()];
    let (sender, mut receiver) = mpsc::channel(4);
    app.connection = Some(sender);
    app.playback = Some(ipc::PlaybackState {
        has_track: true,
        playing: true,
        ..Default::default()
    });
    let _ = app.action(Action::Preferences);
    let request = receiver
        .try_recv()
        .expect("Preferences must send a TUI request");
    assert!(matches!(request.request, ipc::Request::OpenSettings));
    assert!(!request.id.is_empty());
    assert!(app.playback.as_ref().unwrap().playing);
    assert!(app.dialog.is_none());
}

#[test]
fn modal_preferences_actions_cannot_navigate_behind_the_dialog() {
    let mut app = with_terminal();
    app.ipc_connected = true;
    app.handshake.capabilities = vec!["settings_navigation".into()];
    let (sender, mut receiver) = mpsc::channel(4);
    app.connection = Some(sender);
    app.dialog = Some(Dialog::Quit);
    let _ = app.action(Action::Preferences);
    assert!(receiver.try_recv().is_err());
    assert!(matches!(app.dialog, Some(Dialog::Quit)));
}

#[tokio::test]
async fn activating_or_reopening_the_main_window_restores_terminal_input() {
    for reopen in [false, true] {
        let mut app = with_terminal();
        let message = if reopen {
            Message::Desktop(desktop::Event::Reopen)
        } else {
            Message::Native(app.main_window, iced::Event::Window(window::Event::Focused))
        };
        let task = app.update(message);
        assert!(focuses_terminal(&app, task).await, "reopen={reopen}");
    }
}

#[tokio::test]
async fn releasing_passive_main_window_chrome_restores_terminal_input() {
    let mut app = with_terminal();
    let task = app.update(Message::Native(
        app.main_window,
        iced::Event::Mouse(iced::mouse::Event::ButtonReleased(
            iced::mouse::Button::Left,
        )),
    ));
    assert!(focuses_terminal(&app, task).await);
}

#[tokio::test]
async fn preferences_focus_does_not_wait_for_request_timeout() {
    let mut app = with_terminal();
    app.ipc_connected = true;
    app.handshake.capabilities = vec!["settings_navigation".into(), "results".into()];
    let (sender, _receiver) = mpsc::channel(4);
    app.connection = Some(sender);
    let task = app.action(Action::Preferences);
    let target = app.terminal.as_ref().unwrap().widget_id().clone();
    let immediate = async {
        let mut focus = FocusState::default();
        if let Some(mut stream) = iced_runtime::task::into_stream(task) {
            while let Some(action) = stream.next().await {
                match action {
                    iced_runtime::Action::Widget(mut operation) => {
                        operation.focusable(Some(&target), iced::Rectangle::default(), &mut focus);
                        if focus.0 {
                            return true;
                        }
                    }
                    iced_runtime::Action::Output(message) => {
                        let next = app.update(message);
                        if focuses_terminal(&app, next).await {
                            return true;
                        }
                    }
                    _ => {}
                }
            }
        }
        false
    };
    assert!(
        tokio::time::timeout(std::time::Duration::from_millis(250), immediate)
            .await
            .unwrap_or(false),
        "Preferences must restore focus independently of its 15-second receipt timer"
    );
}

#[tokio::test]
async fn focus_restoration_does_not_steal_modal_menu_picker_or_other_window_input() {
    for owner in ["dialog", "menu", "picker", "closing", "other-window"] {
        let mut app = with_terminal();
        match owner {
            "dialog" => app.dialog = Some(Dialog::Quit),
            "menu" => {
                app.titlebar.update(titlebar::Message::ToggleMenu);
            }
            "picker" => {
                app.ipc_connected = true;
                app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
                app.handshake.capabilities = vec!["file_picker".into()];
                let _ = app.setup_picker("audio".into(), "picker-1".into());
                assert!(app.picker.is_some());
            }
            "closing" => app.closing = true,
            _ => {}
        }
        let id = if owner == "other-window" {
            window::Id::unique()
        } else {
            app.main_window
        };
        let task = app.update(Message::Native(
            id,
            iced::Event::Window(window::Event::Focused),
        ));
        assert!(!focuses_terminal(&app, task).await, "owner={owner}");
        if owner != "other-window" {
            assert!(
                !focuses_terminal(&app, app.focus()).await,
                "direct owner={owner}"
            );
            let task = app.update(Message::RestoreTerminalFocus);
            assert!(
                !focuses_terminal(&app, task).await,
                "deferred restoration owner={owner}"
            );
        }
    }
}
