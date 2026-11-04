//! Lyrics window events and preference updates.
use iced::{Point, Size, Task, window};

use crate::ipc::Request;

use super::{Event, Overlay, Preferences, TOOLBAR_OFFSET, TOOLBAR_SIZE};

impl Overlay {
    pub fn update(
        &mut self,
        event: Event,
        preferences: &mut Preferences,
    ) -> (Task<Event>, Option<Request>) {
        #[cfg(target_os = "macos")]
        let visual_change = !matches!(
            &event,
            Event::Ready
                | Event::Capture(..)
                | Event::Mask(..)
                | Event::Linked(..)
                | Event::Drag
                | Event::PlayPause
                | Event::Settings
                | Event::SettingsOpened(_)
                | Event::CloseSettings
        );
        #[cfg(not(target_os = "macos"))]
        let visual_change = false;
        let task = match event {
            #[cfg(target_os = "macos")]
            Event::Capture(id, revision) => {
                if !self.contains(id) || self.mask_revision != revision {
                    return (Task::none(), None);
                }
                return (
                    window::screenshot(id).map(move |image| {
                        Event::Mask(id, revision, crate::lyrics_hit_test::Mask(image))
                    }),
                    None,
                );
            }
            #[cfg(target_os = "macos")]
            Event::Mask(id, revision, mask) => {
                if !self.contains(id) || self.mask_revision != revision {
                    return (Task::none(), None);
                }
                return (
                    crate::desktop::set_lyrics_mask(
                        id,
                        mask,
                        preferences.locked
                            || (self.toolbar_window == Some(id) && !self.toolbar_visible),
                    )
                    .map(|_| Event::Ready),
                    None,
                );
            }
            Event::Opened(id) if self.window == Some(id) => {
                self.opened = true;
                self.locating = true;
                return (
                    Task::batch([
                        self.passthrough(preferences.locked),
                        self.refresh_mask(),
                        window::position(id).then(move |position| {
                            window::size(id)
                                .map(move |size| Event::ToolbarRequested(id, position, size))
                        }),
                    ]),
                    None,
                );
            }
            Event::Opened(id) if self.toolbar_window == Some(id) => {
                self.toolbar_opened = true;
                let task = Task::batch([self.passthrough(preferences.locked), self.refresh_mask()]);
                #[cfg(target_os = "macos")]
                let task = task.chain(self.window.map_or_else(Task::none, |lyrics| {
                    crate::desktop::link_lyrics(lyrics, id, TOOLBAR_OFFSET)
                        .map(move |linked| Event::Linked(id, linked))
                }));
                #[cfg(target_os = "linux")]
                let task = task.chain(self.window.map_or_else(Task::none, |lyrics| {
                    crate::desktop::link_lyrics(lyrics, id).map(|_| Event::Ready)
                }));
                task
            }
            #[cfg(target_os = "macos")]
            Event::Linked(id, linked) => {
                if self.toolbar_window == Some(id) {
                    self.native_linked = linked;
                }
                Task::none()
            }
            Event::ToolbarRequested(id, position, size) => {
                if self.window != Some(id) || !self.locating || self.toolbar_window.is_some() {
                    return (Task::none(), None);
                }
                self.locating = false;
                let Some(position) = position else {
                    // Wayland cannot place two top-level windows relative to each other.
                    self.embedded_toolbar = true;
                    return (self.fit(preferences), None);
                };
                let position = Point::new(
                    position.x + (size.width - TOOLBAR_SIZE.width) / 2.0,
                    position.y - TOOLBAR_OFFSET,
                );
                self.toolbar_position = Some(position);
                let (id, open) = window::open(window::Settings {
                    size: TOOLBAR_SIZE,
                    position: window::Position::Specific(position),
                    decorations: false,
                    transparent: true,
                    resizable: false,
                    minimizable: false,
                    closeable: false,
                    level: window::Level::AlwaysOnTop,
                    #[cfg(target_os = "windows")]
                    platform_specific: window::settings::PlatformSpecific {
                        skip_taskbar: true,
                        ..Default::default()
                    },
                    exit_on_close_request: false,
                    ..Default::default()
                });
                self.toolbar_window = Some(id);
                open.map(Event::Opened)
            }
            Event::SettingsOpened(id) if self.settings_window == Some(id) => Task::none(),
            Event::Opened(id) | Event::SettingsOpened(id) => window::close(id),
            Event::Visible(enable) => {
                preferences.enabled = enable;
                self.hovered = false;
                self.dragging = false;
                self.fit_revision = self.fit_revision.wrapping_add(1);
                let task = if enable {
                    self.show(preferences)
                } else {
                    self.clear();
                    self.close()
                };
                return (
                    Task::batch([task, self.refresh_mask()]),
                    Some(Request::DesktopLyrics(enable)),
                );
            }
            Event::Settings => {
                if let Some(id) = self.settings_window {
                    window::gain_focus(id)
                } else {
                    let (id, open) = window::open(window::Settings {
                        size: Size::new(320.0, 520.0),
                        position: window::Position::Centered,
                        resizable: false,
                        minimizable: false,
                        exit_on_close_request: false,
                        level: window::Level::AlwaysOnTop,
                        ..Default::default()
                    });
                    self.settings_window = Some(id);
                    open.map(Event::SettingsOpened)
                }
            }
            Event::CloseSettings => self
                .settings_window
                .take()
                .map_or_else(Task::none, window::close),
            Event::ResetPosition => {
                preferences.position = None;
                self.hovered = false;
                self.dragging = false;
                let close = self.close();
                Task::batch([close, self.show(preferences)])
            }
            Event::Sized(id, revision, position, previous, size) => {
                if self.window != Some(id) || self.fit_revision != revision {
                    return (Task::none(), None);
                }
                self.size = size;
                #[cfg(target_os = "macos")]
                if self.native_linked {
                    return (
                        crate::desktop::resize_lyrics(id, size, TOOLBAR_OFFSET)
                            .map(|_| Event::Ready),
                        None,
                    );
                }
                if self.embedded_toolbar {
                    #[cfg(target_os = "linux")]
                    return (
                        window::set_max_size(id, None)
                            .chain(window::set_min_size(id, Some(size)))
                            .chain(window::set_max_size(id, Some(size)))
                            .chain(window::resize(id, size)),
                        None,
                    );
                    #[cfg(not(target_os = "linux"))]
                    return (window::resize(id, size), None);
                }
                let position = self.toolbar_position.map_or_else(
                    || Point::new(position.x + (previous.width - size.width) / 2.0, position.y),
                    |toolbar| self.lyrics_position(toolbar, size),
                );
                Task::batch([window::move_to(id, position), window::resize(id, size)])
            }
            Event::Hover(hovered) => {
                self.hovered = hovered;
                self.sync_toolbar(preferences.locked)
            }
            Event::ToolbarHover(hovered) => {
                self.toolbar_hovered = hovered;
                self.sync_toolbar(preferences.locked)
            }
            Event::HideToolbar(revision) => {
                if revision != self.hover_revision
                    || self.hovered
                    || self.toolbar_hovered
                    || self.dragging
                {
                    return (Task::none(), None);
                }
                self.toolbar_visible = false;
                self.passthrough(preferences.locked)
            }
            Event::Drag
                if !preferences.locked
                    && (self.toolbar_window.is_some() || self.embedded_toolbar) =>
            {
                self.dragging = true;
                self.toolbar_visible = true;
                if self.embedded_toolbar {
                    // Retain only the presentation during a compositor-owned move.
                    self.drag_snapshot = Some(self.frame.clone());
                }
                self.toolbar_window
                    .or(self.window)
                    .map_or_else(Task::none, window::drag)
            }
            Event::DragEnded => {
                if !self.dragging {
                    return (Task::none(), None);
                }
                self.dragging = false;
                self.drag_snapshot = None;
                self.sync_toolbar(preferences.locked)
            }
            Event::Locked(locked) => {
                preferences.locked = locked;
                self.hovered = false;
                self.toolbar_hovered = false;
                self.dragging = false;
                self.drag_snapshot = None;
                self.sync_toolbar(locked)
            }
            Event::PlayPause => return (Task::none(), Some(Request::PlayPause)),
            Event::FontSize(size) => {
                preferences.font_size = size.round() as u32;
                Task::none()
            }
            Event::Opacity(opacity) => {
                preferences.opacity = opacity;
                Task::none()
            }
            Event::Highlight(highlight) => {
                preferences.highlight = highlight;
                Task::none()
            }
            Event::AdditionalLines(enable) => {
                preferences.additional_lines = enable;
                Task::none()
            }
            Event::NextLine(enable) => {
                preferences.next_line = enable;
                Task::none()
            }
            Event::Ready | Event::Drag => Task::none(),
        };
        preferences.normalize();
        (
            Task::batch([
                task,
                self.fit(preferences),
                if visual_change {
                    self.refresh_mask()
                } else {
                    Task::none()
                },
            ]),
            None,
        )
    }
}
