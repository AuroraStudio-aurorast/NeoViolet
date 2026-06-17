use alacritty_terminal::event::{Event, EventListener, Notify};
use alacritty_terminal::vte::ansi::Rgb;
use futures::channel::mpsc::UnboundedSender;
use std::borrow::Cow;
use std::sync::Arc;

/// Events forwarded from the alacritty terminal to the main thread.
pub enum TermEvent {
    ClipboardStore(String),
    ClipboardLoad(Arc<dyn Fn(&str) -> String + Sync + Send + 'static>),
    ColorQuery(usize, Arc<dyn Fn(Rgb) -> String + Sync + Send + 'static>),
}

/// Custom event listener that captures:
/// - Title changes (OSC 0/2)
/// - Child exit events
/// - Clipboard operations (OSC 52)
/// - Color query responses
/// - PTY write-back (via Notify)
///
/// Also sends a wake notification on every event so the render loop
/// can be event-driven instead of polling at a fixed frame rate.
pub struct TermEventListener {
    pub title_tx: std::sync::mpsc::Sender<String>,
    pub child_exit_tx: std::sync::mpsc::Sender<Option<i32>>,
    pub term_event_tx: std::sync::mpsc::Sender<TermEvent>,
    pub pty_write_tx: std::sync::mpsc::Sender<Vec<u8>>,
    /// Wake channel — sent on every terminal event so the main thread
    /// can refresh the display (event-driven, async-compatible).
    pub wake_tx: UnboundedSender<()>,
}

impl EventListener for TermEventListener {
    fn send_event(&self, event: Event) {
        match event {
            Event::Title(title) => {
                let _ = self.title_tx.send(title);
            }
            Event::ChildExit(status) => {
                let _ = self.child_exit_tx.send(status.code());
            }
            Event::ClipboardStore(_ty, text) => {
                let _ = self.term_event_tx.send(TermEvent::ClipboardStore(text));
            }
            Event::ClipboardLoad(_ty, formatter) => {
                let _ = self
                    .term_event_tx
                    .send(TermEvent::ClipboardLoad(formatter));
            }
            Event::ColorRequest(index, formatter) => {
                let _ = self
                    .term_event_tx
                    .send(TermEvent::ColorQuery(index, formatter));
            }
            _ => {}
        }
        // Wake the render loop — any terminal event means content changed.
        let _ = self.wake_tx.unbounded_send(());
    }
}

impl Notify for TermEventListener {
    fn notify<B: Into<Cow<'static, [u8]>>>(&self, data: B) {
        let _ = self.pty_write_tx.send(data.into().into_owned());
    }
}
