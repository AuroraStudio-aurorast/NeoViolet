//! Positioning the Windows input method.
//!
//! gpui computes the caret rect for the platform and hands it over, but its Windows
//! backend drops it, so the input method is only told where the caret is once, when a
//! composition starts. A composition that is already running keeps that position, which
//! leaves every candidate window at the corner of the window. An application therefore
//! has to place the input method itself.
//!
//! The bindings are declared here rather than taken from a crate, the same way the
//! macOS input-method detection works.

use std::ffi::c_void;

/// A window handle (`HWND`).
type Hwnd = *mut c_void;
/// An input-method context handle (`HIMC`).
type Himc = *mut c_void;

#[derive(Clone, Copy)]
#[repr(C)]
struct Point {
    x: i32,
    y: i32,
}

#[derive(Clone, Copy)]
#[repr(C)]
struct Rect {
    left: i32,
    top: i32,
    right: i32,
    bottom: i32,
}

#[repr(C)]
struct CompositionForm {
    style: u32,
    current_pos: Point,
    area: Rect,
}

#[repr(C)]
struct CandidateForm {
    index: u32,
    style: u32,
    current_pos: Point,
    area: Rect,
}

/// `CFS_POINT`: the composition window is placed at `current_pos`.
const CFS_POINT: u32 = 0x0002;
/// `CFS_CANDIDATEPOS`: the candidate window's top-left corner is `current_pos`.
const CFS_CANDIDATEPOS: u32 = 0x0040;

#[link(name = "imm32")]
unsafe extern "system" {
    fn ImmGetContext(hwnd: Hwnd) -> Himc;
    fn ImmSetCompositionWindow(himc: Himc, form: *const CompositionForm) -> i32;
    fn ImmSetCandidateWindow(himc: Himc, form: *const CandidateForm) -> i32;
    fn ImmReleaseContext(hwnd: Hwnd, himc: Himc) -> i32;
}

#[link(name = "user32")]
unsafe extern "system" {
    fn GetActiveWindow() -> Hwnd;
}

/// Ask the input method of the active window where the caret is.
///
/// `x` and `y` are client-area physical pixels. Failing to place the windows is not
/// worth reporting: the input method falls back to its own position, which is what it
/// would have used anyway.
pub(super) fn move_caret_to(x: i32, y: i32) {
    let point = Point { x, y };
    let composition = CompositionForm {
        style: CFS_POINT,
        current_pos: point,
        area: Rect {
            left: 0,
            top: 0,
            right: 0,
            bottom: 0,
        },
    };
    let candidate = CandidateForm {
        index: 0,
        style: CFS_CANDIDATEPOS,
        current_pos: point,
        area: Rect {
            left: 0,
            top: 0,
            right: 0,
            bottom: 0,
        },
    };

    unsafe {
        // A composition only runs while its own window holds the focus, so the active
        // window is the one that owns it.
        let window = GetActiveWindow();
        let context = ImmGetContext(window);
        if context.is_null() {
            return;
        }
        ImmSetCompositionWindow(context, &composition);
        ImmSetCandidateWindow(context, &candidate);
        ImmReleaseContext(window, context);
    }
}
