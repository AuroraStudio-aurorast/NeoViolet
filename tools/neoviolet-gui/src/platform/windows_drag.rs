//! Moving and maximizing the window from our own title bar, on Windows.
//!
//! gpui has no way to start a window drag here: `start_window_move` is an empty
//! default on Windows — only macOS and Linux implement it — and `zoom_window` can
//! only maximize a window, never put one back. Both are reachable through the
//! shell instead. Dragging a window by its caption is a `WM_NCLBUTTONDOWN` on
//! `HTCAPTION`, which runs the shell's own move loop; toggling maximize is
//! `ShowWindow` with `SW_MAXIMIZE` or `SW_RESTORE`.
//!
//! The shell has its own opinion about a press in the title bar: gpui hands us
//! the non-client mouse messages and, when an element does not consume one,
//! passes it on to `DefWindowProc`, which starts *its own* caption drag. Two move
//! loops for one press fight over the mouse capture, so the title bar consumes
//! the press (`WindowControlArea` is declared, but the handler stops
//! propagation) and this module is the only thing that starts a drag.
//!
//! The few declarations needed are written out here rather than taken from the
//! `windows` crate, so the GUI keeps no extra direct dependency (the same reason
//! the neighbouring `macos_tis` and `windows_console` modules declare their own).

use std::ffi::c_void;
use std::mem::size_of;
use std::sync::OnceLock;

/// A window handle (`HWND`).
type Hwnd = *mut c_void;
/// A Win32 boolean (`BOOL`).
type Bool = i32;

const WM_NCLBUTTONDOWN: u32 = 0x00a1;
const HTCAPTION: usize = 2;
const SW_MAXIMIZE: i32 = 3;
const SW_RESTORE: i32 = 9;
/// `GA_ROOT`: the top-level window a child belongs to.
const GA_ROOT: u32 = 2;

/// Windows 11's first build; before it, the caption font has another name.
const WINDOWS_11_BUILD: u32 = 22_000;

#[link(name = "user32")]
unsafe extern "system" {
    fn GetActiveWindow() -> Hwnd;
    fn GetCursorPos(point: *mut Point) -> Bool;
    fn WindowFromPoint(point: Point) -> Hwnd;
    fn GetAncestor(hwnd: Hwnd, flags: u32) -> Hwnd;
    fn ReleaseCapture() -> Bool;
    fn SendMessageW(hwnd: Hwnd, msg: u32, wparam: usize, lparam: isize) -> isize;
    fn IsZoomed(hwnd: Hwnd) -> Bool;
    fn ShowWindow(hwnd: Hwnd, cmd: i32) -> Bool;
}

#[link(name = "ntdll")]
unsafe extern "system" {
    fn RtlGetVersion(info: *mut OsVersionInfo) -> i32;
}

/// A point in screen coordinates (`POINT`).
#[repr(C)]
#[derive(Clone, Copy, Default)]
struct Point {
    x: i32,
    y: i32,
}

/// The head of `OSVERSIONINFOW`, which is all we need from it: the build number
/// says which caption font this Windows has.
#[repr(C)]
struct OsVersionInfo {
    size: u32,
    major: u32,
    minor: u32,
    build: u32,
    platform_id: u32,
    service_pack: [u16; 128],
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

/// Maximize our window, or put it back if it is already maximized.
pub(super) fn toggle_zoom() {
    let Some(hwnd) = our_window("maximize or restore") else {
        return;
    };
    let maximized = unsafe { IsZoomed(hwnd) } != 0;
    log::debug!(
        "[gui] {} the window",
        if maximized { "restoring" } else { "maximizing" }
    );
    let command = if maximized { SW_RESTORE } else { SW_MAXIMIZE };
    unsafe {
        ShowWindow(hwnd, command);
    }
}

/// The font this Windows draws caption glyphs with.
pub(super) fn caption_font() -> &'static str {
    caption_font_for_build(windows_build())
}

/// Windows 11 renamed the caption font; the glyphs are the same.
fn caption_font_for_build(build: u32) -> &'static str {
    if build >= WINDOWS_11_BUILD {
        "Segoe Fluent Icons"
    } else {
        "Segoe MDL2 Assets"
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

fn windows_build() -> u32 {
    static BUILD: OnceLock<u32> = OnceLock::new();
    *BUILD.get_or_init(|| {
        let mut info = OsVersionInfo {
            size: size_of::<OsVersionInfo>() as u32,
            major: 0,
            minor: 0,
            build: 0,
            platform_id: 0,
            service_pack: [0; 128],
        };
        let status = unsafe { RtlGetVersion(&mut info) };
        if status == 0 { info.build } else { 0 }
    })
}
