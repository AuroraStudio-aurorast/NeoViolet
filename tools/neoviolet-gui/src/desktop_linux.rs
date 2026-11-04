//! X11 utility window hints. Wayland placement and stacking remain compositor-owned.
use iced::window::{self, raw_window_handle::RawWindowHandle};
use x11rb::connection::Connection;
use x11rb::protocol::xproto::{AtomEnum, ClientMessageEvent, ConnectionExt, EventMask, PropMode};
use x11rb::wrapper::ConnectionExt as _;

#[path = "desktop_lyrics_linux.rs"]
mod lyrics;

pub fn link_lyrics(lyrics: window::Id, toolbar: window::Id) -> iced::Task<()> {
    window::run(lyrics, native_window).then(move |lyrics| {
        lyrics.map_or_else(iced::Task::none, move |lyrics| {
            window::run(toolbar, move |handle| {
                if let Some(toolbar) = native_window(handle)
                    && let Err(error) = self::lyrics::follow_toolbar(lyrics, toolbar)
                {
                    eprintln!("NeoViolet GUI: could not link X11 lyric windows: {error}");
                }
            })
        })
    })
}

fn native_window(handle: &dyn window::Window) -> Option<u32> {
    match handle.window_handle().ok()?.as_raw() {
        RawWindowHandle::Xlib(handle) => Some(handle.window as u32),
        RawWindowHandle::Xcb(handle) => Some(handle.window.get()),
        _ => None,
    }
}

pub fn configure_lyrics(id: window::Id) -> iced::Task<()> {
    window::run(id, |handle| {
        let Some(xid) = native_window(handle) else {
            return;
        };
        if let Err(error) = utility_hints(xid) {
            eprintln!("NeoViolet GUI: could not configure X11 lyric window: {error}");
        }
    })
}

fn utility_hints(window: u32) -> Result<(), Box<dyn std::error::Error>> {
    let (connection, screen) = x11rb::connect(None)?;
    let atom = |name: &[u8]| -> Result<u32, Box<dyn std::error::Error>> {
        Ok(connection.intern_atom(false, name)?.reply()?.atom)
    };
    let role = atom(b"_NET_WM_WINDOW_TYPE")?;
    let utility = atom(b"_NET_WM_WINDOW_TYPE_UTILITY")?;
    connection
        .change_property32(PropMode::REPLACE, window, role, AtomEnum::ATOM, &[utility])?
        .check()?;
    let state = atom(b"_NET_WM_STATE")?;
    let skip_taskbar = atom(b"_NET_WM_STATE_SKIP_TASKBAR")?;
    let skip_pager = atom(b"_NET_WM_STATE_SKIP_PAGER")?;
    let above = atom(b"_NET_WM_STATE_ABOVE")?;
    let root = connection.setup().roots[screen].root;
    for pair in [[skip_taskbar, skip_pager], [above, 0]] {
        // These windows are already mapped: ask the WM to update state rather
        // than replacing its state property behind its back.
        let event = ClientMessageEvent::new(32, window, state, [1, pair[0], pair[1], 1, 0]);
        connection
            .send_event(
                false,
                root,
                EventMask::SUBSTRUCTURE_REDIRECT | EventMask::SUBSTRUCTURE_NOTIFY,
                event,
            )?
            .check()?;
    }
    connection.flush()?;
    Ok(())
}
