//! Keep separate X11 lyric windows together during window-manager drags.
use x11rb::connection::Connection;
use x11rb::protocol::Event;
use x11rb::protocol::xproto::{
    ChangeWindowAttributesAux, ClientMessageEvent, ConnectionExt, EventMask, Window,
};
use x11rb::rust_connection::RustConnection;

type Error = Box<dyn std::error::Error>;

pub(super) fn follow_toolbar(lyrics: Window, toolbar: Window) -> Result<(), Error> {
    let (connection, screen) = x11rb::connect(None)?;
    let root = connection.setup().roots[screen].root;
    let offset =
        root_position(&connection, lyrics, root)?.1 - root_position(&connection, toolbar, root)?.1;
    let moveresize = connection
        .intern_atom(false, b"_NET_MOVERESIZE_WINDOW")?
        .reply()?
        .atom;
    for window in [lyrics, toolbar] {
        connection
            .change_window_attributes(
                window,
                &ChangeWindowAttributesAux::new().event_mask(EventMask::STRUCTURE_NOTIFY),
            )?
            .check()?;
    }
    // Reparenting WMs move their frame during the drag and may send the client
    // its synthetic root-coordinate ConfigureNotify only when the drag ends.
    connection
        .change_window_attributes(
            root,
            &ChangeWindowAttributesAux::new().event_mask(EventMask::SUBSTRUCTURE_NOTIFY),
        )?
        .check()?;
    let frame = root_child(&connection, toolbar, root)?;
    connection.flush()?;
    std::thread::Builder::new()
        .name("neoviolet-x11-lyrics".into())
        .spawn(move || {
            if let Err(error) = track(
                &connection,
                root,
                toolbar,
                lyrics,
                frame,
                moveresize,
                offset,
            ) {
                eprintln!("NeoViolet GUI: X11 lyric tracking stopped: {error}");
            }
        })?;
    Ok(())
}

fn track(
    connection: &RustConnection,
    root: Window,
    toolbar: Window,
    lyrics: Window,
    mut frame: Window,
    moveresize: u32,
    offset: i32,
) -> Result<(), Error> {
    let mut previous = None;
    let mut lyrics_width = connection.get_geometry(lyrics)?.reply()?.width;
    loop {
        let event = connection.wait_for_event()?;
        let changed = match event {
            Event::ConfigureNotify(event) if event.window == toolbar || event.window == frame => {
                true
            }
            Event::ConfigureNotify(event) if event.window == lyrics => {
                let changed = lyrics_width != event.width;
                lyrics_width = event.width;
                changed
            }
            Event::ReparentNotify(event) if event.window == toolbar => {
                frame = root_child(connection, toolbar, root)?;
                true
            }
            Event::DestroyNotify(event) if event.window == toolbar || event.window == lyrics => {
                return Ok(());
            }
            _ => false,
        };
        if !changed {
            continue;
        }
        let (x, y) = root_position(connection, toolbar, root)?;
        let width = connection.get_geometry(toolbar)?.reply()?.width;
        let position = follower_position(x, y, width, lyrics_width, offset);
        if previous == Some(position) {
            continue;
        }
        previous = Some(position);
        // Static gravity, x/y fields present, source = application. Leave size
        // to Iced so changing lyric length does not move or resize the toolbar.
        let message = ClientMessageEvent::new(
            32,
            lyrics,
            moveresize,
            [
                10 | (1 << 8) | (1 << 9) | (1 << 12),
                position.0 as u32,
                position.1 as u32,
                0,
                0,
            ],
        );
        connection
            .send_event(
                false,
                root,
                EventMask::SUBSTRUCTURE_REDIRECT | EventMask::SUBSTRUCTURE_NOTIFY,
                message,
            )?
            .check()?;
        connection.flush()?;
    }
}

fn root_child(
    connection: &RustConnection,
    mut window: Window,
    root: Window,
) -> Result<Window, Error> {
    loop {
        let parent = connection.query_tree(window)?.reply()?.parent;
        if parent == root || parent == 0 {
            return Ok(window);
        }
        window = parent;
    }
}

fn root_position(
    connection: &RustConnection,
    window: Window,
    root: Window,
) -> Result<(i32, i32), Error> {
    let position = connection
        .translate_coordinates(window, root, 0, 0)?
        .reply()?;
    Ok((i32::from(position.dst_x), i32::from(position.dst_y)))
}

fn follower_position(
    x: i32,
    y: i32,
    toolbar_width: u16,
    lyrics_width: u16,
    offset: i32,
) -> (i32, i32) {
    (
        x + (i32::from(toolbar_width) - i32::from(lyrics_width)) / 2,
        y + offset,
    )
}

#[cfg(test)]
mod tests {
    use super::follower_position;

    #[test]
    fn lyrics_stay_centered_under_a_moving_toolbar_as_their_width_changes() {
        assert_eq!(follower_position(700, 350, 240, 880, 80), (380, 430));
        assert_eq!(follower_position(700, 350, 240, 300, 80), (670, 430));
        assert_eq!(follower_position(-200, 350, 480, 1760, 160), (-840, 510));
    }
}
