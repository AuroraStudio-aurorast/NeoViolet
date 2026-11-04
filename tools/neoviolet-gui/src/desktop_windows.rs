//! Keep native caption operations while Iced paints the main window's chrome.
use super::Event;
use crate::chrome_hit_test::{self, CaptionPress, Hit};
use crate::titlebar::CaptionControl;
use iced::futures::SinkExt;
use iced::window::raw_window_handle::RawWindowHandle;
use std::cell::RefCell;
use std::collections::HashMap;
use std::hash::{Hash, Hasher};
use std::sync::Arc;
use tokio::sync::{Mutex, mpsc};
use windows_sys::Win32::Foundation::{HWND, LPARAM, LRESULT, POINT, RECT, WPARAM};
use windows_sys::Win32::Graphics::Dwm::{
    DWMWA_USE_IMMERSIVE_DARK_MODE, DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND,
    DwmExtendFrameIntoClientArea, DwmSetWindowAttribute,
};
use windows_sys::Win32::Graphics::Gdi::ScreenToClient;
use windows_sys::Win32::UI::Controls::{MARGINS, WM_MOUSELEAVE};
use windows_sys::Win32::UI::HiDpi::{GetDpiForWindow, GetSystemMetricsForDpi};
use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    GetCapture, ReleaseCapture, SetCapture, TME_LEAVE, TME_NONCLIENT, TRACKMOUSEEVENT,
    TrackMouseEvent,
};
use windows_sys::Win32::UI::Shell::{DefSubclassProc, RemoveWindowSubclass, SetWindowSubclass};
use windows_sys::Win32::UI::WindowsAndMessaging::*;

const SUBCLASS_ID: usize = 0x4e56;
#[derive(Default, Clone, Copy)]
struct InputState {
    menu: bool,
    fullscreen: bool,
}
struct WindowState {
    input: InputState,
    id: iced::window::Id,
    sender: mpsc::UnboundedSender<Event>,
    hovered: Option<CaptionControl>,
    press: CaptionPress,
}
thread_local! { static WINDOWS: RefCell<HashMap<isize, WindowState>> = RefCell::new(HashMap::new()); }

#[derive(Clone)]
struct Events(Arc<Mutex<mpsc::UnboundedReceiver<Event>>>);
impl Hash for Events {
    fn hash<H: Hasher>(&self, state: &mut H) {
        Arc::as_ptr(&self.0).hash(state);
    }
}

#[derive(Clone)]
pub struct Desktop {
    pub(super) sender: mpsc::UnboundedSender<Event>,
    events: Events,
}
impl Desktop {
    pub fn install() -> Result<Self, String> {
        let (sender, receiver) = mpsc::unbounded_channel();
        Ok(Self {
            sender,
            events: Events(Arc::new(Mutex::new(receiver))),
        })
    }
    pub fn subscription(&self) -> iced::Subscription<Event> {
        iced::Subscription::run_with(self.events.clone(), |events| {
            let events = events.clone();
            iced::stream::channel(16, async move |mut output| {
                let mut receiver = events.0.lock().await;
                while let Some(event) = receiver.recv().await {
                    if output.send(event).await.is_err() {
                        return;
                    }
                }
            })
        })
    }
    pub fn sync_lyrics(&self, _: bool) {}
    pub fn configure_lyrics_window(&self, _: iced::window::Id) -> iced::Task<()> {
        iced::Task::none()
    }
}

fn hover(hwnd: HWND, control: Option<CaptionControl>) {
    WINDOWS.with(|windows| {
        if let Some(state) = windows.borrow_mut().get_mut(&(hwnd as isize)) {
            let control = control.filter(|_| !state.input.menu && !state.input.fullscreen);
            if state.hovered != control {
                state.hovered = control;
                let _ = state.sender.send(Event::CaptionHover(state.id, control));
            }
        }
    });
}

pub fn configure(id: iced::window::Id, sender: mpsc::UnboundedSender<Event>) -> iced::Task<()> {
    iced::window::run(id, move |handle| {
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
                windows.borrow_mut().insert(
                    hwnd as isize,
                    WindowState {
                        input: InputState::default(),
                        id,
                        sender,
                        hovered: None,
                        press: CaptionPress::default(),
                    },
                )
            });
            let style = GetWindowLongW(hwnd, GWL_STYLE) as u32;
            SetWindowLongW(
                hwnd,
                GWL_STYLE,
                (style | WS_THICKFRAME | WS_MAXIMIZEBOX | WS_MINIMIZEBOX | WS_SYSMENU) as i32,
            );
            let dark: i32 = 1;
            DwmSetWindowAttribute(
                hwnd,
                DWMWA_USE_IMMERSIVE_DARK_MODE as u32,
                (&dark as *const i32).cast(),
                std::mem::size_of_val(&dark) as u32,
            );
            DwmSetWindowAttribute(
                hwnd,
                DWMWA_WINDOW_CORNER_PREFERENCE as u32,
                (&DWMWCP_ROUND as *const i32).cast(),
                std::mem::size_of_val(&DWMWCP_ROUND) as u32,
            );
            let margins = MARGINS {
                cxLeftWidth: 0,
                cxRightWidth: 0,
                cyTopHeight: 1,
                cyBottomHeight: 0,
            };
            DwmExtendFrameIntoClientArea(hwnd, &margins);
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
                state.input = InputState { menu, fullscreen };
            }
        });
        if menu || fullscreen {
            hover(handle.hwnd.get() as HWND, None);
            cancel_press(handle.hwnd.get() as HWND);
        }
    })
}

fn cancel_press(hwnd: HWND) {
    let owned = WINDOWS.with(|windows| {
        if let Some(state) = windows.borrow_mut().get_mut(&(hwnd as isize)) {
            let active = state.press.active();
            state.press.cancel();
            active
        } else {
            false
        }
    });
    // SAFETY: Releasing capture is confined to this window's owning thread.
    unsafe {
        if owned && GetCapture() == hwnd {
            ReleaseCapture();
        }
    }
}

fn caption_control(hit: Hit) -> Option<CaptionControl> {
    match hit {
        Hit::Minimize => Some(CaptionControl::Minimize),
        Hit::Maximize => Some(CaptionControl::Maximize),
        Hit::Close => Some(CaptionControl::Close),
        _ => None,
    }
}

fn control_hit(value: WPARAM) -> Hit {
    match value as u32 {
        HTMINBUTTON => Hit::Minimize,
        HTMAXBUTTON => Hit::Maximize,
        HTCLOSE => Hit::Close,
        _ => Hit::Client,
    }
}

unsafe fn point_hit(hwnd: HWND, mut point: POINT, screen: bool, state: InputState) -> Hit {
    // SAFETY: Both output values are local, and hwnd belongs to the current callback.
    unsafe {
        let mut rect = RECT::default();
        if (screen && ScreenToClient(hwnd, &mut point) == 0) || GetClientRect(hwnd, &mut rect) == 0
        {
            return Hit::Client;
        }
        let scale = GetDpiForWindow(hwnd).max(96) as f32 / 96.0;
        chrome_hit_test::hit(
            iced::Point::new(point.x as f32 / scale, point.y as f32 / scale),
            iced::Size::new(rect.right as f32 / scale, rect.bottom as f32 / scale),
            IsZoomed(hwnd) != 0,
            state.fullscreen,
            state.menu,
        )
    }
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
        let state = WINDOWS.with(|windows| {
            windows
                .borrow()
                .get(&(hwnd as isize))
                .map(|state| state.input)
        });
        if let Some(state) = state {
            if message == WM_CAPTURECHANGED {
                // SetCapture can notify while winit holds its own window-state lock.
                // Never call ReleaseCapture here or disturb capture retained by this HWND.
                WINDOWS.with(|windows| {
                    if let Some(window) = windows.borrow_mut().get_mut(&(hwnd as isize)) {
                        window.press.capture_changed(lparam as HWND == hwnd);
                    }
                });
                if lparam as HWND != hwnd {
                    hover(hwnd, None);
                }
            }
            if message == WM_CANCELMODE
                || (message == WM_ACTIVATE && wparam & 0xffff == WA_INACTIVE as usize)
            {
                // DefWindowProc owns cancellation/deactivation. Do not enter another
                // capture-change callback while a native window transition is in progress.
                WINDOWS.with(|windows| {
                    if let Some(window) = windows.borrow_mut().get_mut(&(hwnd as isize)) {
                        window.press.cancel();
                    }
                });
                hover(hwnd, None);
            }
            if matches!(message, WM_NCLBUTTONDOWN | WM_NCLBUTTONDBLCLK)
                && matches!(
                    control_hit(wparam),
                    Hit::Minimize | Hit::Maximize | Hit::Close
                )
            {
                if state.menu || state.fullscreen {
                    return 0;
                }
                let captured = WINDOWS.with(|windows| {
                    windows
                        .borrow_mut()
                        .get_mut(&(hwnd as isize))
                        .is_some_and(|window| window.press.begin(control_hit(wparam)))
                });
                if captured {
                    SetCapture(hwnd);
                    if GetCapture() != hwnd {
                        cancel_press(hwnd);
                    }
                    return 0;
                }
            }
            if matches!(message, WM_LBUTTONUP | WM_NCLBUTTONUP) {
                // Use the release event's position, not a newer global cursor sample.
                let point = POINT {
                    x: lparam as i16 as i32,
                    y: (lparam >> 16) as i16 as i32,
                };
                let hit = point_hit(hwnd, point, message == WM_NCLBUTTONUP, state);
                let released = WINDOWS.with(|windows| {
                    let mut windows = windows.borrow_mut();
                    let window = windows.get_mut(&(hwnd as isize))?;
                    if !window.press.active() {
                        return None;
                    }
                    Some((window.id, window.sender.clone(), window.press.release(hit)))
                });
                if let Some((id, sender, action)) = released {
                    if GetCapture() == hwnd {
                        ReleaseCapture();
                    }
                    if let Some(action) = action {
                        let _ = sender.send(Event::CaptionAction(id, action));
                    }
                    return 0;
                }
            }
            if message == WM_MOUSEMOVE
                && WINDOWS.with(|windows| {
                    windows
                        .borrow()
                        .get(&(hwnd as isize))
                        .is_some_and(|window| window.press.active())
                })
            {
                let point = POINT {
                    x: lparam as i16 as i32,
                    y: (lparam >> 16) as i16 as i32,
                };
                hover(hwnd, caption_control(point_hit(hwnd, point, false, state)));
            }
            // Native styles change synchronously during fullscreen transitions;
            // the application learns the new mode only after this calculation.
            if message == WM_NCCALCSIZE && GetWindowLongW(hwnd, GWL_STYLE) as u32 & WS_CAPTION != 0
            {
                let rect = if wparam != 0 {
                    std::ptr::addr_of_mut!((*(lparam as *mut NCCALCSIZE_PARAMS)).rgrc[0])
                } else {
                    lparam as *mut RECT
                };
                let top = (*rect).top;
                // Preserve the native side/bottom frame; extend only the title area.
                DefSubclassProc(hwnd, message, wparam, lparam);
                let inset = if IsZoomed(hwnd) != 0 {
                    let dpi = GetDpiForWindow(hwnd);
                    GetSystemMetricsForDpi(SM_CYFRAME, dpi)
                        + GetSystemMetricsForDpi(SM_CXPADDEDBORDER, dpi)
                } else {
                    0
                };
                (*rect).top = top + inset;
                return 0;
            }
            if message == WM_NCHITTEST {
                let mut point = POINT {
                    x: lparam as i16 as i32,
                    y: (lparam >> 16) as i16 as i32,
                };
                let mut rect = RECT::default();
                if ScreenToClient(hwnd, &mut point) != 0 && GetClientRect(hwnd, &mut rect) != 0 {
                    if point.x < 0 || point.y < 0 || point.x >= rect.right || point.y >= rect.bottom
                    {
                        return DefSubclassProc(hwnd, message, wparam, lparam);
                    }
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
            // Keep native hit testing for snap layouts; custom buttons dispatch
            // through the application's normal command path after captured release.
            if message == WM_NCMOUSEMOVE {
                hover(hwnd, caption_control(control_hit(wparam)));
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
            hover(hwnd, None);
            PostMessageW(hwnd, WM_MOUSELEAVE, 0, 0);
        }
        if message == WM_NCDESTROY {
            WINDOWS.with(|windows| windows.borrow_mut().remove(&(hwnd as isize)));
            RemoveWindowSubclass(hwnd, Some(procedure), SUBCLASS_ID);
        }
        DefSubclassProc(hwnd, message, wparam, lparam)
    }
}

#[cfg(test)]
#[path = "desktop_windows_tests.rs"]
mod tests;
