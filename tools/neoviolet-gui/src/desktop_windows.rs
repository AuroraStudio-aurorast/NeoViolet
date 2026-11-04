//! Keep native caption operations while Iced paints the main window's chrome.
use crate::chrome_hit_test::{self, Hit};
use iced::window::raw_window_handle::RawWindowHandle;
use std::cell::RefCell;
use std::collections::HashMap;
use windows_sys::Win32::Foundation::{HWND, LPARAM, LRESULT, POINT, RECT, WPARAM};
use windows_sys::Win32::Graphics::Gdi::ScreenToClient;
use windows_sys::Win32::UI::Controls::WM_MOUSELEAVE;
use windows_sys::Win32::UI::HiDpi::GetDpiForWindow;
use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    TME_LEAVE, TME_NONCLIENT, TRACKMOUSEEVENT, TrackMouseEvent,
};
use windows_sys::Win32::UI::Shell::{DefSubclassProc, RemoveWindowSubclass, SetWindowSubclass};
use windows_sys::Win32::UI::WindowsAndMessaging::*;

const SUBCLASS_ID: usize = 0x4e56;
#[derive(Default, Clone, Copy)]
struct InputState {
    menu: bool,
    fullscreen: bool,
}
thread_local! { static WINDOWS: RefCell<HashMap<isize, InputState>> = RefCell::new(HashMap::new()); }

pub fn configure(id: iced::window::Id) -> iced::Task<()> {
    iced::window::run(id, |handle| {
        let Ok(handle) = handle.window_handle() else {
            return;
        };
        let RawWindowHandle::Win32(handle) = handle.as_raw() else {
            return;
        };
        let hwnd = handle.hwnd.get() as HWND;
        // SAFETY: Iced provides a live HWND on its owning thread; the subclass is removed at destruction.
        unsafe {
            if SetWindowSubclass(hwnd, Some(procedure), SUBCLASS_ID, 0) == 0 {
                eprintln!("NeoViolet GUI: could not install native title bar hit testing");
                return;
            }
            WINDOWS.with(|windows| {
                windows
                    .borrow_mut()
                    .insert(hwnd as isize, InputState::default())
            });
            let style = GetWindowLongW(hwnd, GWL_STYLE) as u32;
            SetWindowLongW(
                hwnd,
                GWL_STYLE,
                (style | WS_THICKFRAME | WS_MAXIMIZEBOX | WS_MINIMIZEBOX | WS_SYSMENU) as i32,
            );
            SetWindowPos(
                hwnd,
                std::ptr::null_mut(),
                0,
                0,
                0,
                0,
                SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_NOACTIVATE | SWP_FRAMECHANGED,
            );
        }
    })
}

pub fn input_state(id: iced::window::Id, menu: bool, fullscreen: bool) -> iced::Task<()> {
    iced::window::run(id, move |handle| {
        let Ok(handle) = handle.window_handle() else {
            return;
        };
        let RawWindowHandle::Win32(handle) = handle.as_raw() else {
            return;
        };
        WINDOWS.with(|windows| {
            if let Some(state) = windows.borrow_mut().get_mut(&handle.hwnd.get()) {
                *state = InputState { menu, fullscreen };
            }
        });
    })
}

unsafe extern "system" fn procedure(
    hwnd: HWND,
    message: u32,
    wparam: WPARAM,
    lparam: LPARAM,
    _: usize,
    _: usize,
) -> LRESULT {
    // SAFETY: All API calls use this subclass's live HWND and stack-allocated output pointers.
    unsafe {
        let state = WINDOWS.with(|windows| windows.borrow().get(&(hwnd as isize)).copied());
        if let Some(state) = state {
            if message == WM_NCHITTEST {
                let mut point = POINT {
                    x: lparam as i16 as i32,
                    y: (lparam >> 16) as i16 as i32,
                };
                let mut rect = RECT::default();
                if ScreenToClient(hwnd, &mut point) != 0 && GetClientRect(hwnd, &mut rect) != 0 {
                    let scale = GetDpiForWindow(hwnd).max(96) as f32 / 96.0;
                    let hit = chrome_hit_test::hit(
                        iced::Point::new(point.x as f32 / scale, point.y as f32 / scale),
                        iced::Size::new(
                            (rect.right - rect.left) as f32 / scale,
                            (rect.bottom - rect.top) as f32 / scale,
                        ),
                        IsZoomed(hwnd) != 0,
                        state.fullscreen,
                        state.menu,
                    );
                    return match hit {
                        Hit::Client => HTCLIENT,
                        Hit::Caption => HTCAPTION,
                        Hit::Minimize => HTMINBUTTON,
                        Hit::Maximize => HTMAXBUTTON,
                        Hit::Close => HTCLOSE,
                        Hit::North => HTTOP,
                        Hit::South => HTBOTTOM,
                        Hit::West => HTLEFT,
                        Hit::East => HTRIGHT,
                        Hit::NorthWest => HTTOPLEFT,
                        Hit::NorthEast => HTTOPRIGHT,
                        Hit::SouthWest => HTBOTTOMLEFT,
                        Hit::SouthEast => HTBOTTOMRIGHT,
                    } as LRESULT;
                }
            }
            // Iced still needs pointer position for its caption hover painting. Button presses
            // stay non-client so DefWindowProc owns activation, snapping, and the close request.
            if message == WM_NCMOUSEMOVE {
                let mut tracking = TRACKMOUSEEVENT {
                    cbSize: std::mem::size_of::<TRACKMOUSEEVENT>() as u32,
                    dwFlags: TME_LEAVE | TME_NONCLIENT,
                    hwndTrack: hwnd,
                    dwHoverTime: 0,
                };
                TrackMouseEvent(&mut tracking);
                let mut point = POINT {
                    x: lparam as i16 as i32,
                    y: (lparam >> 16) as i16 as i32,
                };
                if ScreenToClient(hwnd, &mut point) != 0 {
                    let position =
                        (point.x as u16 as u32 | ((point.y as u16 as u32) << 16)) as LPARAM;
                    PostMessageW(hwnd, WM_MOUSEMOVE, 0, position);
                }
            }
        }
        if message == WM_NCMOUSELEAVE {
            PostMessageW(hwnd, WM_MOUSELEAVE, 0, 0);
        }
        if message == WM_NCDESTROY {
            WINDOWS.with(|windows| windows.borrow_mut().remove(&(hwnd as isize)));
            RemoveWindowSubclass(hwnd, Some(procedure), SUBCLASS_ID);
        }
        DefSubclassProc(hwnd, message, wparam, lparam)
    }
}
