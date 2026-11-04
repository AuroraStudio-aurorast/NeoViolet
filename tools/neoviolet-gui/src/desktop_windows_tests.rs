//! Native capture regression using a hidden Windows control, without moving the cursor.
use super::*;

struct HiddenWindow(HWND);

impl Drop for HiddenWindow {
    fn drop(&mut self) {
        // SAFETY: This window and its subclass were created on the current test thread.
        unsafe {
            WINDOWS.with(|windows| windows.borrow_mut().remove(&(self.0 as isize)));
            if GetCapture() == self.0 {
                ReleaseCapture();
            }
            RemoveWindowSubclass(self.0, Some(procedure), SUBCLASS_ID);
            DestroyWindow(self.0);
        }
    }
}

#[test]
fn same_window_capture_notification_never_releases_capture_or_cancels_a_caption_press() {
    let class: Vec<u16> = "STATIC\0".encode_utf16().collect();
    // SAFETY: STATIC is a system-provided class. No WS_VISIBLE flag is used, and
    // no event loop, desktop interaction or cursor movement is needed.
    let window = HiddenWindow(unsafe {
        CreateWindowExW(
            0,
            class.as_ptr(),
            std::ptr::null(),
            WS_OVERLAPPEDWINDOW,
            0,
            0,
            480,
            320,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            std::ptr::null(),
        )
    });
    assert!(!window.0.is_null());
    let (sender, _receiver) = mpsc::unbounded_channel();
    let mut press = CaptionPress::default();
    assert!(press.begin(Hit::Maximize));
    WINDOWS.with(|windows| {
        windows.borrow_mut().insert(
            window.0 as isize,
            WindowState {
                input: InputState::default(),
                id: iced::window::Id::unique(),
                sender,
                hovered: None,
                press,
            },
        );
    });
    // SAFETY: All window operations and the synchronous callback stay on this thread.
    unsafe {
        assert_ne!(
            SetWindowSubclass(window.0, Some(procedure), SUBCLASS_ID, 0),
            0
        );
        SetCapture(window.0);
        assert_eq!(GetCapture(), window.0);
        // Mirrors SetCapture(hwnd) retaining an existing capture while winit is locked.
        SendMessageW(window.0, WM_CAPTURECHANGED, 0, window.0 as LPARAM);
        assert_eq!(
            GetCapture(),
            window.0,
            "retained capture must not be released recursively"
        );
        assert!(WINDOWS.with(|windows| windows.borrow()[&(window.0 as isize)].press.active()));
        ReleaseCapture();
        assert!(!WINDOWS.with(|windows| windows.borrow()[&(window.0 as isize)].press.active()));
    }
}
