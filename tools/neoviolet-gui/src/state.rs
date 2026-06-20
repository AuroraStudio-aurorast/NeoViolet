//! Shared application state — registered as a gpui `Global`.

use gpui::{EntityId, Global, WeakEntity};
use std::sync::{Arc, Mutex};
use std::time::Instant;

use crate::config::GuiConfig;
use crate::app::TerminalApp;
use crate::ipc::IpcClient;

pub struct AppState {
    pub config: GuiConfig,
    pub launch_args: Arc<Mutex<Vec<String>>>,

    // ── Terminal state ──
    pub terminal_child: Arc<Mutex<Option<WeakEntity<TerminalApp>>>>,

    // ── PTY output buffer (for bad-args / crash diagnostics) ──
    pub recent_output: Arc<Mutex<Vec<u8>>>,
    pub process_start: Arc<Mutex<Option<Instant>>>,

    // ── Font (mutated by zoom handlers, read by TerminalApp on restart) ──
    pub font_size: Arc<Mutex<u32>>,

    // ── Dialog visibility ──
    pub show_about: Arc<Mutex<bool>>,
    pub show_close: Arc<Mutex<bool>>,
    pub show_exit_error: Arc<Mutex<bool>>,

    // ── Notification target ──
    pub root_entity_id: Arc<Mutex<Option<EntityId>>>,

    // ── CLI version cache ──
    pub cli_version: Arc<Mutex<String>>,

    // ── Pending file paths from drag-and-drop / open-file events ──
    /// File paths waiting to be forwarded to the PTY process as launch args.
    /// Set by `on_open_urls` / `FileDropEvent` handlers, consumed by
    /// `NeoVioletApp::render()`.
    pub pending_file_paths: Arc<Mutex<Vec<String>>>,

    // ── Child PID (for IPC control file path) ──
    pub child_pid: Arc<Mutex<Option<u32>>>,

    // ── IPC client (GUI → TUI communication) ──
    pub ipc: Arc<IpcClient>,
}

impl Global for AppState {}

impl AppState {
    pub fn new(config: GuiConfig, launch_args: Vec<String>) -> Self {
        let font_size = config.font_size;
        Self {
            config,
            launch_args: Arc::new(Mutex::new(launch_args)),
            terminal_child: Arc::new(Mutex::new(None)),
            recent_output: Arc::new(Mutex::new(Vec::new())),
            process_start: Arc::new(Mutex::new(None)),
            font_size: Arc::new(Mutex::new(font_size)),
            show_about: Arc::new(Mutex::new(false)),
            show_close: Arc::new(Mutex::new(false)),
            show_exit_error: Arc::new(Mutex::new(false)),
            root_entity_id: Arc::new(Mutex::new(None)),
            cli_version: Arc::new(Mutex::new(String::new())),
            pending_file_paths: Arc::new(Mutex::new(Vec::new())),
            child_pid: Arc::new(Mutex::new(None)),
            ipc: Arc::new(IpcClient::new()),
        }
    }
}
