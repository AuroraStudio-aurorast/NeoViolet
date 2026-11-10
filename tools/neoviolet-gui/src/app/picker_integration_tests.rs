use super::*;
use crate::actions::Action;

fn ready_picker() -> (App, mpsc::Receiver<ipc::Command>) {
    let mut app = app();
    app.dialog = None;
    app.pending.clear();
    app.handshake.capabilities = vec![
        "file_picker".into(),
        "settings_file_picker".into(),
        "startup".into(),
    ];
    app.ipc_connected = true;
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Ready);
    let (sender, receiver) = mpsc::channel(32);
    app.connection = Some(sender);
    (app, receiver)
}

#[test]
fn settings_choosers_are_ready_only_and_reply_without_editing_gui_config() {
    for purpose in ["soundfont", "library_directory", "player_binary"] {
        let (mut app, mut receiver) = ready_picker();
        let _ = app.setup_picker(purpose.into(), "settings-1".into());
        assert!(app.picker.is_some(), "missing {purpose} dialog");
        assert!(!app.terminal_input_available());
        let _ = app.setup_picked(
            app.session,
            app.connection_epoch,
            "settings-1".into(),
            Ok(Some("/tmp/a file 雪".into())),
        );
        assert!(
            matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{purpose:got,status,path:Some(path),..} if got==purpose && status=="selected" && path==std::path::Path::new("/tmp/a file 雪"))
        );
        assert!(app.config.neoviolet_path.is_none());
        assert!(app.picker.is_some());
        let _ = app.finish_picker_reply("settings-1", "succeeded", "");
        assert!(app.picker.is_none());
    }
}

#[test]
fn gui_open_delegates_chooser_ownership_to_go() {
    let (mut app, mut receiver) = ready_picker();
    let _ = app.action(Action::Open);
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::OpenPicker
    ));
    assert!(app.picker.is_none(), "Go must authorize the dialog first");
}

#[test]
fn settings_chooser_keeps_owner_stage_and_ignores_late_replies() {
    let (mut app, mut receiver) = ready_picker();
    let _ = app.setup_picker("soundfont".into(), "runtime".into());
    assert!(app.picker.is_some());
    let _ = app.startup_phase(ipc::StartupPhase::Wizard);
    let _ = app.setup_picked(
        app.session,
        app.connection_epoch,
        "runtime".into(),
        Ok(Some("/tmp/bank.sf2".into())),
    );
    assert!(
        receiver.try_recv().is_err(),
        "runtime reply crossed into wizard"
    );
}

#[test]
fn quitting_cancels_go_selection_before_late_native_result() {
    let (mut app, mut receiver) = ready_picker();
    app.handshake.capabilities.push("request_quit".into());
    let _ = app.setup_picker("library_directory".into(), "folder".into());
    let _ = app.window_action(Action::Quit);
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{request_id,status,..} if request_id=="folder" && status=="cancelled")
    );
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::CheckQuit
    ));
    let _ = app.setup_picked(
        app.session,
        app.connection_epoch,
        "folder".into(),
        Ok(Some("/tmp/ignored".into())),
    );
    while let Ok(command) = receiver.try_recv() {
        assert!(
            !matches!(
                command.request,
                ipc::Request::FilePicked { .. } | ipc::Request::Open(_)
            ),
            "quit accepted a late selection"
        );
    }
}

#[test]
fn quit_during_picker_drains_queued_zoom_before_checking_quit() {
    let (mut app, mut receiver) = ready_picker();
    app.handshake.capabilities.extend([
        "results".into(),
        "settings_preferences".into(),
        "request_quit".into(),
    ]);
    let _ = app.action(Action::ZoomIn);
    let first = receiver.try_recv().unwrap();
    let _ = app.action(Action::ZoomIn);
    let _ = app.setup_picker("audio".into(), "audio".into());
    let _ = app.window_action(Action::Quit);
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{status,..} if status=="cancelled")
    );
    assert!(
        receiver.try_recv().is_err(),
        "quit must wait for pending preference saves"
    );
    let _ = app.command_result(first.id, "succeeded".into(), String::new());
    let last = receiver.try_recv().unwrap();
    assert!(matches!(last.request,ipc::Request::SetPreference{value,..} if value==18));
    let _ = app.command_result(last.id, "succeeded".into(), String::new());
    assert!(matches!(
        receiver.try_recv().unwrap().request,
        ipc::Request::CheckQuit
    ));
}

#[test]
fn native_completion_waits_for_go_before_returning_terminal_input() {
    let (mut app, mut receiver) = ready_picker();
    let _ = app.setup_picker("audio".into(), "cancel".into());
    let _ = app.setup_picked(app.session, app.connection_epoch, "cancel".into(), Ok(None));
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{status,..} if status=="cancelled")
    );
    assert!(
        app.picker.is_some(),
        "input returned before Go acknowledged cancellation"
    );
    assert!(!app.terminal_input_available());
    let _ = app.ipc_event(
        app.session,
        ipc::Event::Received(ipc::Incoming::Result {
            request_id: "cancel".into(),
            status: "cancelled".into(),
            error: String::new(),
        }),
    );
    assert!(app.picker.is_none());
}

#[test]
fn wizard_chooser_uses_startup_phase_instead_of_runtime_reply_barrier() {
    let (mut app, mut receiver) = ready_picker();
    app.startup = super::super::startup::Stage::Phase(ipc::StartupPhase::Wizard);
    let _ = app.setup_picker("soundfont".into(), "wizard".into());
    let _ = app.setup_picked(app.session, app.connection_epoch, "wizard".into(), Ok(None));
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{status,..} if status=="cancelled")
    );
    assert!(
        app.picker.is_none(),
        "wizard does not emit the runtime selection receipt"
    );
}

#[test]
fn selection_reply_timeout_cancels_go_and_discards_late_success() {
    let (mut app, mut receiver) = ready_picker();
    let _ = app.setup_picker("soundfont".into(), "slow".into());
    let _ = app.setup_picked(
        app.session,
        app.connection_epoch,
        "slow".into(),
        Ok(Some("/tmp/a.sf2".into())),
    );
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{status,..} if status=="selected")
    );
    let _ = app.update(Message::RequestTimeout {
        session: app.session,
        epoch: app.connection_epoch,
        id: "slow".into(),
        accepted: false,
    });
    assert!(
        matches!(receiver.try_recv().unwrap().request,ipc::Request::FilePicked{status,..} if status=="cancelled")
    );
    assert!(app.picker.is_some(), "cancellation is not acknowledged yet");
    let _ = app.finish_picker_reply("slow", "cancelled", "");
    assert!(app.picker.is_none());
    assert!(
        app.notice
            .as_ref()
            .is_some_and(|notice| notice.contains("did not confirm"))
    );
    assert!(app.finish_picker_reply("slow", "succeeded", "").is_none());
}

#[test]
fn programmatic_cancel_requires_both_physical_completion_and_go_ack() {
    for ack_first in [false, true] {
        let (mut app, mut receiver) = ready_picker();
        let _ = app.setup_picker("soundfont".into(), "cancel-programmatically".into());
        let _ = app.cancel_native_picker();
        let _ = receiver.try_recv().unwrap();
        if ack_first {
            let _ = app.finish_picker_reply("cancel-programmatically", "cancelled", "");
            assert!(app.picker.is_some());
        }
        let _ = app.setup_picked(
            app.session,
            app.connection_epoch,
            "cancel-programmatically".into(),
            Ok(Some("/tmp/late.sf2".into())),
        );
        if !ack_first {
            assert!(
                app.picker.is_some(),
                "physical completion released input before Go ack"
            );
            let _ = app.finish_picker_reply("cancel-programmatically", "cancelled", "");
        }
        assert!(app.picker.is_none());
    }
}
