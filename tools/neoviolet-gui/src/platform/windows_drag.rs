//! Moving and maximizing the window from our own title bar, on Windows.
//!
//! gpui has no way to start a window drag here: `start_window_move` is an empty
//! default on Windows — only macOS and Linux implement it — and `zoom_window` can
//! only maximize a window, never put one back. Both are reachable through the
//! shell instead. Dragging a window by its caption is a `WM_NCLBUTTONDOWN` on
//! `HTCAPTION`, which runs the shell's own move loop; toggling maximize is
//! `ShowWindow` with `SW_MAXIMIZE` or `SW_RESTORE`.
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

/// Windows 11's first build; before it, the caption font has another name.
const WINDOWS_11_BUILD: u32 = 22_000;

#[link(name = "user32")]
unsafe extern "system" {
    fn GetActiveWindow() -> Hwnd;
    fn ReleaseCapture() -> Bool;
    fn SendMessageW(hwnd: Hwnd, msg: u32, wparam: usize, lparam: isize) -> isize;
    fn IsZoomed(hwnd: Hwnd) -> Bool;
    fn ShowWindow(hwnd: Hwnd, cmd: i32) -> Bool;
}

#[link(name = "ntdll")]
unsafe extern "system" {
    fn RtlGetVersion(info: *mut OsVersionInfo) -> i32;
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

/// Start dragging the active window, the way a press on a system caption does.
///
/// The message runs the shell's move loop, so it returns once the drag is over.
pub(super) fn start_window_drag() {
    let hwnd = active_window();
    if hwnd.is_null() {
        return;
    }
    unsafe {
        ReleaseCapture();
        SendMessageW(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0);
    }
}

/// Maximize the active window, or put it back if it is already maximized.
pub(super) fn toggle_zoom() {
    let hwnd = active_window();
    if hwnd.is_null() {
        return;
    }
    let already_maximized = unsafe { IsZoomed(hwnd) } != 0;
    let command = if already_maximized {
        SW_RESTORE
    } else {
        SW_MAXIMIZE
    };
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

/// The window a press should act on. A drag or a maximize is only ever asked for
/// from a pointer press inside our own window, which is the active one by then.
fn active_window() -> Hwnd {
    unsafe { GetActiveWindow() }
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
