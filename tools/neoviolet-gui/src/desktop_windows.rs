//! Keep native caption operations while Iced paints the main window's chrome.
use super::Event;
use crate::chrome_hit_test::{self, Hit};
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
struct WindowState {
    input: InputState,
    id: iced::window::Id,
    sender: mpsc::UnboundedSender<Event>,
    hovered: Option<CaptionControl>,
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
        }
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
        let state = WINDOWS.with(|windows| {
            windows
                .borrow()
                .get(&(hwnd as isize))
                .map(|state| state.input)
        });
        if let Some(state) = state {
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
            // Iced still needs pointer position for its caption hover painting. Button presses
            // stay non-client so DefWindowProc owns activation, snapping, and the close request.
            if message == WM_NCMOUSEMOVE {
                hover(
                    hwnd,
                    match wparam as u32 {
                        HTMINBUTTON => Some(CaptionControl::Minimize),
                        HTMAXBUTTON => Some(CaptionControl::Maximize),
                        HTCLOSE => Some(CaptionControl::Close),
                        _ => None,
                    },
                );
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
