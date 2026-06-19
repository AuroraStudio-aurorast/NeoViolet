//! Shared application state — registered as a gpui `Global`.

use gpui::{EntityId, Global, WeakEntity};
use std::sync::{Arc, Mutex};

use crate::config::GuiConfig;
use crate::app::TerminalApp;

pub struct AppState {
    pub config: GuiConfig,
    pub launch_args: Vec<String>,

    // ── Terminal state ──
    pub terminal_child: Arc<Mutex<Option<WeakEntity<TerminalApp>>>>,

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
}

impl Global for AppState {}

impl AppState {
    pub fn new(config: GuiConfig, launch_args: Vec<String>) -> Self {
        let font_size = config.font_size;
        Self {
            config,
            launch_args,
            terminal_child: Arc::new(Mutex::new(None)),
            font_size: Arc::new(Mutex::new(font_size)),
            show_about: Arc::new(Mutex::new(false)),
            show_close: Arc::new(Mutex::new(false)),
            show_exit_error: Arc::new(Mutex::new(false)),
            root_entity_id: Arc::new(Mutex::new(None)),
            cli_version: Arc::new(Mutex::new(String::new())),
        }
    }
}
