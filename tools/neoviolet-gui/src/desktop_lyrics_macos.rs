//! Let AppKit move the lyric window together with its fixed-size toolbar.
use objc2_app_kit::{NSWindow, NSWindowOrderingMode};
use objc2_foundation::{NSPoint, NSRect, NSSize};

pub fn attach(lyrics: &NSWindow, toolbar: &NSWindow, offset: f64) {
    detach(lyrics);
    // SAFETY: Both windows are live, main-thread AppKit objects. The toolbar owns
    // the native drag; the child keeps its relative position during that drag.
    unsafe { toolbar.addChildWindow_ordered(lyrics, NSWindowOrderingMode::Below) };
    resize(lyrics, lyrics.frame().size, offset);
}

pub fn resize(lyrics: &NSWindow, size: NSSize, offset: f64) {
    let frame = lyrics.frame();
    let origin = if let Some(toolbar) = lyrics.parentWindow() {
        let toolbar = toolbar.frame();
        NSPoint::new(
            toolbar.origin.x + (toolbar.size.width - size.width) / 2.0,
            toolbar.origin.y + toolbar.size.height - offset - size.height,
        )
    } else {
        NSPoint::new(
            frame.origin.x,
            frame.origin.y + frame.size.height - size.height,
        )
    };
    lyrics.setFrame_display(NSRect::new(origin, size), true);
}

pub fn detach(window: &NSWindow) {
    if let Some(parent) = window.parentWindow() {
        parent.removeChildWindow(window);
    }
    if let Some(children) = window.childWindows() {
        for child in &children {
            window.removeChildWindow(&child);
        }
    }
}
