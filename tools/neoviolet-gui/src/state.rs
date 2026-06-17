use gpui::{EntityId, Global};
use std::sync::Arc;
use std::sync::Mutex;

use crate::config::GuiConfig;
use crate::modes::Modes;
use crate::term::TerminalState;

/// Shared application state — registered as a gpui `Global`.
///
/// All mutable fields use `Arc<Mutex<...>>` so menu handlers and async
/// callbacks can safely write while the render thread reads.
pub struct AppState {
    /// Config loaded at startup (read-only after init).
    pub config: GuiConfig,
    /// Current terminal instance (None if spawning failed).
    pub terminal: Arc<Mutex<Option<TerminalState>>>,
    /// Cached CLI version string.
    pub cli_version: Arc<Mutex<String>>,
    /// Original CLI args — never mutated, used for terminal restarts.
    pub launch_args: Vec<String>,

    // ── Font ──
    pub font_size: Arc<Mutex<u32>>,
    pub font_family: Arc<Mutex<String>>,

    // ── Dialog visibility ──
    pub show_about: Arc<Mutex<bool>>,
    pub show_close: Arc<Mutex<bool>>,
    pub show_exit_error: Arc<Mutex<bool>>,

    // ── Notification target ──
    /// The root component's EntityId — used by menu handlers to trigger
    /// a re-render after mutating state.
    pub root_entity_id: Arc<Mutex<Option<EntityId>>>,

    // ── Locally-cached terminal modes (avoids FairMutex lock) ──
    pub modes: Arc<Mutex<Modes>>,

    // ── Direct PTY sender (avoids mpsc buffering for keystrokes) ──
    /// Cloned from TerminalState::input_sender(). The keystroke observer
    /// sends directly to the PTY, bypassing the render-loop drain.
    pub pty_sender: Arc<Mutex<Option<alacritty_terminal::event_loop::EventLoopSender>>>,
}

impl Global for AppState {}

impl AppState {
    pub fn new(
        config: GuiConfig,
        launch_args: Vec<String>,
    ) -> Self {
        let font_size = config.font_size;
        let font_family = config.monospace_font.clone();
        Self {
            config,
            terminal: Arc::new(Mutex::new(None)),
            cli_version: Arc::new(Mutex::new(String::new())),
            launch_args,
            font_size: Arc::new(Mutex::new(font_size)),
            font_family: Arc::new(Mutex::new(font_family)),
            show_about: Arc::new(Mutex::new(false)),
            show_close: Arc::new(Mutex::new(false)),
            show_exit_error: Arc::new(Mutex::new(false)),
            root_entity_id: Arc::new(Mutex::new(None)),
            modes: Arc::new(Mutex::new(Modes::empty())),
            pty_sender: Arc::new(Mutex::new(None)),
        }
    }

    /// Read the locally-cached terminal modes without acquiring FairMutex.
    pub fn cached_modes(&self) -> Modes {
        *self.modes.lock().unwrap()
    }

    /// Send raw bytes directly to the PTY (for keystrokes).
    /// Returns false if no PTY sender is available.
    pub fn send_to_pty(&self, data: Vec<u8>) -> bool {
        if let Some(ref sender) = *self.pty_sender.lock().unwrap() {
            sender.send(alacritty_terminal::event_loop::Msg::Input(
                std::borrow::Cow::Owned(data),
            )).is_ok()
        } else {
            false
        }
    }

    /// Set the IME composition (preedit) text on the terminal.
    pub fn set_ime_preedit(&self, text: Option<String>) {
        let term = self.terminal.lock().unwrap();
        if let Some(ref t) = *term {
            t.set_ime_preedit(text);
        }
    }
}
