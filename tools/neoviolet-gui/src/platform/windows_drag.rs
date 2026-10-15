//! Moving the window from our own title bar, on Windows.
//!
//! Windows moves a window itself, out of the caption area the title bar
//! declares: gpui hands the non-client mouse messages to the app and, when no
//! element consumes one, passes it on to `DefWindowProc`, which is what runs the
//! move. This module only covers the case where that did not happen — gpui
//! implements `start_window_move` on macOS, X11 and Wayland but leaves it an
//! empty default on Windows — and a drag started here is a `WM_NCLBUTTONDOWN` on
//! `HTCAPTION`, which runs the shell's own move loop for us.
//!
//! The few declarations needed are written out here rather than taken from the
//! `windows` crate, so the GUI keeps no extra direct dependency (the same reason
//! the neighbouring `macos_tis` and `windows_console` modules declare their own).

use std::ffi::c_void;

/// A window handle (`HWND`).
type Hwnd = *mut c_void;
/// A Win32 boolean (`BOOL`).
type Bool = i32;

const WM_NCLBUTTONDOWN: u32 = 0x00a1;
const HTCAPTION: usize = 2;
const SW_RESTORE: i32 = 9;
/// `GA_ROOT`: the top-level window a child belongs to.
const GA_ROOT: u32 = 2;

#[link(name = "user32")]
unsafe extern "system" {
    fn GetActiveWindow() -> Hwnd;
    fn GetCursorPos(point: *mut Point) -> Bool;
    fn WindowFromPoint(point: Point) -> Hwnd;
    fn GetAncestor(hwnd: Hwnd, flags: u32) -> Hwnd;
    fn ReleaseCapture() -> Bool;
    fn SendMessageW(hwnd: Hwnd, msg: u32, wparam: usize, lparam: isize) -> isize;
    /// Only used to put a maximized window back before a fallback drag; the
    /// window's own maximize and restore are the platform's business.
    fn IsZoomed(hwnd: Hwnd) -> Bool;
    fn ShowWindow(hwnd: Hwnd, cmd: i32) -> Bool;
}

/// A point in screen coordinates (`POINT`).
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct Point {
    x: i32,
    y: i32,
}

/// Start dragging our window, the way a press on a system caption does.
///
/// The message runs the shell's move loop, so it returns once the drag is over.
///
/// A maximized window is un-maximized and left there. The shell would do the same
/// on a caption drag, but then it would drag the restored window to wherever the
/// move loop's grab point says, and the only grab point we can hand it is the
/// pointer's current one — the restored window would jump to the pointer. Getting
/// it to follow the pointer the way the shell does needs the pointer's place in
/// the maximized window mapped onto the restored one, which is a whole extra
/// behaviour; restoring and stopping means the same gesture never teleports a
/// window, and a second drag moves it.
pub(super) fn start_window_drag() {
    let Some(hwnd) = our_window("drag") else {
        return;
    };
    if unsafe { IsZoomed(hwnd) } != 0 {
        log::debug!("[gui] restoring the window instead of dragging a maximized one");
        unsafe {
            ShowWindow(hwnd, SW_RESTORE);
        }
        return;
    }

    let mut cursor = Point::default();
    if unsafe { GetCursorPos(&mut cursor) } == 0 {
        log::warn!("[gui] could not read the pointer position, so no drag started");
        return;
    }

    log::debug!("[gui] handing the drag over to the window shell");
    unsafe {
        ReleaseCapture();
        SendMessageW(
            hwnd,
            WM_NCLBUTTONDOWN,
            HTCAPTION,
            pack_point((cursor.x, cursor.y)),
        );
    }
}

/// The window a title-bar gesture belongs to, or a warning saying there is none.
///
/// Both gestures are only ever asked for from a pointer press inside our own
/// window, so the window under the pointer is the one to act on. Asking for the
/// *active* window instead can answer with nothing at all — and a gesture that
/// quietly does nothing is exactly what went unnoticed for a while here — so the
/// active window is only the fallback, and a miss is logged rather than dropped.
fn our_window(gesture: &'static str) -> Option<Hwnd> {
    unsafe {
        let mut cursor = Point::default();
        if GetCursorPos(&mut cursor) != 0 {
            let under_pointer = GetAncestor(WindowFromPoint(cursor), GA_ROOT);
            if !under_pointer.is_null() {
                return Some(under_pointer);
            }
        }
        let active = GetActiveWindow();
        if active.is_null() {
            log::warn!("[gui] no window to {gesture}");
            None
        } else {
            Some(active)
        }
    }
}

/// A point the way `WM_NCLBUTTONDOWN` carries one: two 16-bit halves, which the
/// shell reads back as signed.
fn pack_point((x, y): (i32, i32)) -> isize {
    (((y as u16) as u32) << 16 | (x as u16) as u32) as isize
}
