//! Native lyrics overlay and toolbar; settings and playback remain in the TUI.
mod controls;
mod geometry;
mod update;
mod view;

#[cfg(test)]
mod layout_tests;
#[cfg(test)]
mod window_tests;

use iced::{Point, Size, Task, window};

use crate::lyrics::{Frame, Highlight, Position, Preferences};

const TOOLBAR_SIZE: Size = Size::new(240.0, 88.0);
const TOOLBAR_OFFSET: f32 = 80.0;

#[derive(Debug, Clone)]
pub enum Event {
    Opened(window::Id),
    Ready,
    #[cfg(target_os = "macos")]
    Capture(window::Id, u64),
    #[cfg(target_os = "macos")]
    Mask(window::Id, u64, crate::lyrics_hit_test::Mask),
    Sized(window::Id, u64, Point, Size, Size),
    ToolbarRequested(window::Id, Option<Point>, Size),
    #[cfg(target_os = "macos")]
    Linked(window::Id, bool),
    Visible(bool),
    Settings,
    ResetPosition,
    Hover(bool),
    ToolbarHover(bool),
    HideToolbar(u64),
    Drag,
    DragEnded,
    PlayPause,
    Locked(bool),
}

#[derive(Default)]
pub struct Overlay {
    pub frame: Frame,
    pub playback: Option<bool>,
    pub window: Option<window::Id>,
    pub toolbar_window: Option<window::Id>,
    hovered: bool,
    toolbar_hovered: bool,
    toolbar_visible: bool,
    toolbar_opened: bool,
    #[cfg(target_os = "macos")]
    native_linked: bool,
    toolbar_position: Option<Point>,
    hover_revision: u64,
    size: Size,
    opened: bool,
    locating: bool,
    dragging: bool,
    embedded_toolbar: bool,
    drag_snapshot: Option<Frame>,
    fit_revision: u64,
    #[cfg(target_os = "macos")]
    mask_revision: u64,
}

impl Overlay {
    fn display_frame(&self) -> &Frame {
        self.drag_snapshot.as_ref().unwrap_or(&self.frame)
    }

    pub fn show(&mut self, preferences: &Preferences) -> Task<Event> {
        if !preferences.enabled {
            return Task::none();
        }
        if self.window.is_some() {
            return self.passthrough(preferences.locked);
        }
        let position = preferences.position.map_or(
            window::Position::SpecificWith(|window, monitor| {
                Point::new(
                    ((monitor.width - window.width) / 2.0).max(0.0),
                    (monitor.height * 0.65 - window.height / 2.0 + TOOLBAR_OFFSET / 2.0).max(0.0),
                )
            }),
            |point| window::Position::Specific(Point::new(point.x, point.y + TOOLBAR_OFFSET)),
        );
        self.size = self.display_size(preferences);
        self.opened = false;
        self.locating = true;
        let (id, open) = window::open(window::Settings {
            size: self.size,
            position,
            decorations: false,
            transparent: true,
            resizable: false,
            minimizable: false,
            closeable: false,
            level: window::Level::AlwaysOnTop,
            exit_on_close_request: false,
            ..Default::default()
        });
        self.window = Some(id);
        open.map(Event::Opened)
    }

    pub fn clear(&mut self) {
        self.frame = Frame::default();
        self.drag_snapshot = None;
    }

    pub fn contains(&self, id: window::Id) -> bool {
        self.window == Some(id) || self.toolbar_window == Some(id)
    }

    pub fn moved(
        &mut self,
        id: window::Id,
        point: Point,
        preferences: &mut Preferences,
    ) -> Task<Event> {
        if !point.x.is_finite() || !point.y.is_finite() {
            return Task::none();
        }
        if self.toolbar_window == Some(id) {
            self.toolbar_position = Some(point);
            let position = self.lyrics_position(point, self.size);
            preferences.position = Some(Position {
                x: position.x,
                y: position.y - TOOLBAR_OFFSET,
            });
            #[cfg(target_os = "macos")]
            if self.native_linked {
                // AppKit already moved the child during the native drag.
                return Task::none();
            }
            return self
                .window
                .map_or_else(Task::none, |id| window::move_to(id, position));
        }
        if self.window == Some(id) {
            preferences.position = Some(Position {
                x: point.x,
                y: point.y - TOOLBAR_OFFSET,
            });
        }
        Task::none()
    }

    fn close(&mut self) -> Task<Event> {
        self.opened = false;
        self.locating = false;
        self.toolbar_opened = false;
        self.toolbar_visible = false;
        self.toolbar_hovered = false;
        self.toolbar_position = None;
        #[cfg(target_os = "macos")]
        {
            self.native_linked = false;
        }
        self.drag_snapshot = None;
        self.embedded_toolbar = false;
        Task::batch([
            self.window
                .take()
                .map_or_else(Task::none, crate::desktop::close_lyrics),
            self.toolbar_window
                .take()
                .map_or_else(Task::none, crate::desktop::close_lyrics),
        ])
    }

    fn sync_toolbar(&mut self, locked: bool) -> Task<Event> {
        self.hover_revision = self.hover_revision.wrapping_add(1);
        if !locked && (self.hovered || self.toolbar_hovered || self.dragging) {
            self.toolbar_visible = true;
            self.passthrough(locked)
        } else if locked {
            self.toolbar_visible = false;
            self.passthrough(locked)
        } else {
            let revision = self.hover_revision;
            // Crossing between the two windows must not briefly hide the controls.
            Task::perform(
                async { tokio::time::sleep(std::time::Duration::from_millis(80)).await },
                move |_| Event::HideToolbar(revision),
            )
        }
    }

    fn passthrough(&self, locked: bool) -> Task<Event> {
        Task::batch(
            [self.window, self.toolbar_window]
                .into_iter()
                .flatten()
                .map(|id| {
                    let locked =
                        locked || (self.toolbar_window == Some(id) && !self.toolbar_visible);
                    #[cfg(target_os = "macos")]
                    {
                        crate::desktop::set_lyrics_locked(id, locked).map(|_| Event::Ready)
                    }
                    #[cfg(not(target_os = "macos"))]
                    {
                        if locked {
                            window::enable_mouse_passthrough(id)
                        } else {
                            window::disable_mouse_passthrough(id)
                        }
                    }
                }),
        )
    }

    pub fn refresh_mask(&mut self) -> Task<Event> {
        #[cfg(target_os = "macos")]
        {
            self.mask_revision = self.mask_revision.wrapping_add(1);
            let revision = self.mask_revision;
            Task::batch(
                [
                    self.window.filter(|_| self.opened),
                    self.toolbar_window.filter(|_| self.toolbar_opened),
                ]
                .into_iter()
                .flatten()
                .map(|id| {
                    // Let Iced draw the changed content before reading its alpha buffer.
                    Task::perform(
                        async { tokio::time::sleep(std::time::Duration::from_millis(30)).await },
                        move |_| Event::Capture(id, revision),
                    )
                }),
            )
        }
        #[cfg(not(target_os = "macos"))]
        {
            Task::none()
        }
    }
}
