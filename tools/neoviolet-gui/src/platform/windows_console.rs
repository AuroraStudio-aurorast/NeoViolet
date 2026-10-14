//! Giving the process a console to write its log to.
//!
//! The GUI links as a GUI program so that Windows does not open a console window
//! beside the window (see `main.rs`). Windows therefore never gives this process a
//! console, and a launch from a terminal would have nowhere to write; attaching to
//! that terminal's console puts the standard streams back.
//!
//! The symbols are declared here rather than taken from a crate: this is a handful
//! of calls, and a dependency would not earn its keep.
use std::ffi::c_void;

/// A handle (`HANDLE`).
type Handle = *mut c_void;

/// The console of the process that started this one (`ATTACH_PARENT_PROCESS`, -1).
const ATTACH_PARENT_PROCESS: u32 = 0xFFFF_FFFF;

/// The three standard streams (`STD_INPUT_HANDLE` / `_OUTPUT_` / `_ERROR_`, -10 to -12).
const STD_INPUT_HANDLE: u32 = 0xFFFF_FFF6;
const STD_OUTPUT_HANDLE: u32 = 0xFFFF_FFF5;
const STD_ERROR_HANDLE: u32 = 0xFFFF_FFF4;

/// How the console's device files are opened.
const GENERIC_READ: u32 = 0x8000_0000;
const GENERIC_WRITE: u32 = 0x4000_0000;
const FILE_SHARE_READ: u32 = 0x0000_0001;
const FILE_SHARE_WRITE: u32 = 0x0000_0002;
const OPEN_EXISTING: u32 = 3;

/// What `CreateFileW` returns when it could not open the file.
const INVALID_HANDLE_VALUE: isize = -1;

#[link(name = "kernel32")]
unsafe extern "system" {
    fn AttachConsole(process_id: u32) -> i32;
    fn CreateFileW(
        name: *const u16,
        access: u32,
        share: u32,
        security: *const c_void,
        creation: u32,
        flags: u32,
        template: Handle,
    ) -> Handle;
    fn SetStdHandle(which: u32, handle: Handle) -> i32;
}

/// Attach to the console this process was started from and point the standard
/// streams at it, so that a terminal launch can be read.
///
/// Returns immediately when there is no console to attach to — a launch from
/// Explorer, which is also the case where no console should appear.
pub(super) fn attach_parent_console() {
    // SAFETY: every call gets values of its documented type; the device names are
    // NUL-terminated and outlive the call that reads them; a handle that could not
    // be opened is never installed.
    unsafe {
        if AttachConsole(ATTACH_PARENT_PROCESS) == 0 {
            return;
        }
        // Windows leaves the standard handles at null for a GUI program, so they
        // are opened against the console just attached and installed by hand.
        for (which, name, access) in [
            (STD_OUTPUT_HANDLE, "CONOUT$", GENERIC_WRITE),
            (STD_ERROR_HANDLE, "CONOUT$", GENERIC_WRITE),
            (STD_INPUT_HANDLE, "CONIN$", GENERIC_READ),
        ] {
            let name = wide(name);
            let handle = CreateFileW(
                name.as_ptr(),
                access,
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                std::ptr::null(),
                OPEN_EXISTING,
                0,
                std::ptr::null_mut(),
            );
            if handle as isize != INVALID_HANDLE_VALUE {
                SetStdHandle(which, handle);
            }
        }
    }
}

/// A NUL-terminated UTF-16 copy of `name`, as the W-suffixed APIs take.
fn wide(name: &str) -> Vec<u16> {
    name.encode_utf16().chain(std::iter::once(0)).collect()
}
