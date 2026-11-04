//! Native lyrics and settings windows; playback remains in the TUI.
use iced::widget::{
    button, column, container, mouse_area, rich_text, row, rule, scrollable, slider, space, span,
    text, toggler, tooltip,
};
use iced::{Color, Element, Font, Length, Point, Size, Task, window};

use crate::ipc::Request;
use crate::lyrics::{Frame, Highlight, Position, Preferences};
use crate::lyrics_text::{emphasized, outlined};

const TOOLBAR_SIZE: Size = Size::new(240.0, 88.0);
const TOOLBAR_OFFSET: f32 = 80.0;

#[derive(Debug, Clone)]
pub enum Event {
    Opened(window::Id),
    SettingsOpened(window::Id),
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
    CloseSettings,
    ResetPosition,
    Hover(bool),
    ToolbarHover(bool),
    HideToolbar(u64),
    Drag,
    DragEnded,
    PlayPause,
    Locked(bool),
    FontSize(f32),
    Opacity(f32),
    Highlight(Highlight),
    AdditionalLines(bool),
    NextLine(bool),
}

#[derive(Default)]
pub struct Overlay {
    pub frame: Frame,
    pub window: Option<window::Id>,
    pub toolbar_window: Option<window::Id>,
    pub settings_window: Option<window::Id>,
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

    pub fn display_size(&self, preferences: &Preferences) -> Size {
        let frame = self.display_frame();
        let active = frame.display_lines();
        let toolbar_height = if self.embedded_toolbar {
            TOOLBAR_OFFSET
        } else {
            0.0
        };
        let next = preferences.next_line.then(|| frame.next_line()).flatten();
        if active.is_empty() && next.is_none() {
            return Size::new(300.0, 50.0 + toolbar_height);
        }
        let measure = |width| {
            let mut bounds = Size::ZERO;
            let mut groups = 0;
            for line in &active {
                if groups > 0 {
                    bounds.height += 12.0;
                }
                groups += 1;
                let primary = measure_primary(line, preferences.font_size as f32, width);
                bounds.width = bounds.width.max(primary.width);
                bounds.height += primary.height;
                if preferences.additional_lines {
                    for additional in line
                        .additional_lines()
                        .iter()
                        .filter(|line| !line.trim().is_empty())
                    {
                        let size =
                            measure_caption(additional, preferences.font_size as f32 * 0.46, width);
                        bounds.width = bounds.width.max(size.width);
                        bounds.height += 5.0 + size.height;
                    }
                }
            }
            if active.is_empty() {
                let quiet = measure_caption("···", 17.0, width);
                bounds.width = bounds.width.max(quiet.width);
                bounds.height += quiet.height;
                groups += 1;
            }
            if let Some(next) = next {
                let size = measure_caption(
                    &format!("{}{}", next.prefix, next.first_text()),
                    13.0,
                    width,
                );
                bounds.width = bounds.width.max(size.width);
                bounds.height += if groups > 0 { 12.0 } else { 0.0 };
                bounds.height += size.height;
            }
            bounds
        };
        let width = (measure(f32::INFINITY).width.ceil() + 40.0).clamp(300.0, 880.0);
        let height = (measure(width - 40.0).height.ceil() + 16.0).clamp(50.0, 420.0);
        Size::new(width, height + toolbar_height)
    }

    pub fn fit(&mut self, preferences: &Preferences) -> Task<Event> {
        if !self.opened || self.locating {
            return Task::none();
        }
        let Some(id) = self.window else {
            return Task::none();
        };
        self.fit_revision = self.fit_revision.wrapping_add(1);
        let size = self.display_size(preferences);
        if size == self.size {
            return Task::none();
        }
        let revision = self.fit_revision;
        if let Some(position) = self.toolbar_position {
            return Task::done(Event::Sized(id, revision, position, self.size, size));
        }
        window::position(id).then(move |position| {
            window::size(id).map(move |previous| {
                Event::Sized(
                    id,
                    revision,
                    position.unwrap_or(Point::ORIGIN),
                    previous,
                    size,
                )
            })
        })
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

    fn lyrics_position(&self, toolbar: Point, size: Size) -> Point {
        Point::new(
            toolbar.x + (TOOLBAR_SIZE.width - size.width) / 2.0,
            toolbar.y + TOOLBAR_OFFSET,
        )
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

    pub fn view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let alpha = preferences.opacity;
        let sung = match preferences.highlight {
            Highlight::Violet => Color::from_rgb8(224, 189, 255),
            Highlight::White => Color::WHITE,
        }
        .scale_alpha(alpha);
        let frame = self.display_frame();
        let active = frame.display_lines();
        let mut lines = column![]
            .spacing(12)
            .align_x(iced::Center)
            .width(Length::Fill);
        if active.is_empty() {
            let label = if frame.next_line().is_some() || !frame.lines.is_empty() {
                "···"
            } else if frame.title.is_empty() && frame.artist.is_empty() {
                "Waiting for music"
            } else {
                "No lyrics"
            };
            lines = lines.push(caption(label.to_owned(), 17.0, 0.85 * alpha, alpha));
        }
        for line in active {
            let mut fragments = Vec::new();
            let mut emphasis = Vec::new();
            if !line.prefix.is_empty() {
                fragments.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(sung),
                );
                emphasis.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(Color::TRANSPARENT),
                );
            }
            for (fragment, intensity) in line.fragments(frame.elapsed, |text| {
                measure_text(text, preferences.font_size as f32, f32::INFINITY).width
            }) {
                let grey = Color::from_rgb8(181, 177, 188);
                fragments.push(span(fragment).color(Color {
                    r: grey.r + (sung.r - grey.r) * intensity,
                    g: grey.g + (sung.g - grey.g) * intensity,
                    b: grey.b + (sung.b - grey.b) * intensity,
                    a: alpha,
                }));
                emphasis.push(span(fragment).color(Color {
                    r: sung.r * intensity,
                    g: sung.g * intensity,
                    b: sung.b * intensity,
                    a: 0.22 * alpha,
                }));
            }
            let shadow = fragments
                .iter()
                .cloned()
                .map(|fragment| {
                    fragment.color(Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * alpha))
                })
                .collect::<Vec<_>>();
            let make_text = |fragments| {
                rich_text::<(), _, _, _>(fragments)
                    .size(preferences.font_size)
                    .font(primary_font())
                    .align_x(iced::Center)
                    .width(Length::Fill)
            };
            let primary = emphasized(make_text(fragments), make_text(shadow), make_text(emphasis));
            let mut group = column![primary]
                .spacing(5)
                .align_x(iced::Center)
                .width(Length::Fill);
            if preferences.additional_lines {
                for additional in line
                    .additional_lines()
                    .iter()
                    .filter(|line| !line.trim().is_empty())
                {
                    let additional_text = |color| {
                        text(additional.as_str())
                            .size(preferences.font_size as f32 * 0.46)
                            .shaping(iced::advanced::text::Shaping::Advanced)
                            .color(color)
                            .align_x(iced::Center)
                            .width(Length::Fill)
                    };
                    group = group.push(outlined(
                        additional_text(Color::WHITE.scale_alpha(0.95 * alpha)),
                        additional_text(Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * alpha)),
                        0.7,
                    ));
                }
            }
            let agent = if line.agent_name.is_empty() {
                &line.agent
            } else {
                &line.agent_name
            };
            let group: Element<'_, Event> = if agent.is_empty() {
                group.into()
            } else {
                hint(group, agent.as_str(), tooltip::Position::Bottom)
            };
            lines = lines.push(group);
        }
        if preferences.next_line
            && let Some(next) = frame.next_line()
        {
            lines = lines.push(caption(
                format!("{}{}", next.prefix, next.first_text()),
                13.0,
                0.8 * alpha,
                alpha,
            ));
        }
        let content: Element<'_, Event> = if self.embedded_toolbar {
            column![
                space().height(30),
                container(self.toolbar_content(preferences))
                    .height(34)
                    .center_x(Length::Fill),
                scrollable(lines).height(Length::Shrink),
            ]
            .spacing(8)
            .into()
        } else {
            scrollable(lines).height(Length::Shrink).into()
        };
        let area = mouse_area(
            container(content)
                .padding([8, 20])
                .center_x(Length::Fill)
                .height(Length::Fill),
        );
        #[cfg(not(target_os = "macos"))]
        let area = area
            .on_enter(Event::Hover(true))
            .on_exit(Event::Hover(false));
        area.into()
    }

    fn toolbar_content(&self, preferences: &Preferences) -> Element<'_, Event> {
        if self.toolbar_visible && !preferences.locked {
            container(
                row![
                    hint(
                        mouse_area(container(icon(Icon::Move)).padding(6)).on_press(Event::Drag),
                        "Drag to move",
                        tooltip::Position::Top
                    ),
                    icon_button(Icon::Playback, "Play / Pause", Event::PlayPause),
                    icon_button(Icon::Lock, "Lock position", Event::Locked(true)),
                    icon_button(Icon::Settings, "Lyrics settings", Event::Settings),
                ]
                .spacing(2)
                .align_y(iced::Center),
            )
            .padding(3)
            .style(toolbar_style)
            .into()
        } else {
            space().height(34).into()
        }
    }

    pub fn toolbar_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let toolbar = self.toolbar_content(preferences);
        let area = mouse_area(
            container(column![space().height(30), toolbar].spacing(8))
                .padding([8, 0])
                .center_x(Length::Fill)
                .height(Length::Fill),
        );
        #[cfg(not(target_os = "macos"))]
        let area = area
            .on_enter(Event::ToolbarHover(true))
            .on_exit(Event::ToolbarHover(false));
        area.into()
    }

    pub fn settings_view(&self, preferences: &Preferences) -> Element<'_, Event> {
        let label = |name, value| {
            row![
                text(name).size(12).color(Color::from_rgb8(206, 204, 214)),
                space().width(Length::Fill),
                text(value).size(12).color(Color::from_rgb8(228, 220, 242)),
            ]
            .align_y(iced::Center)
        };
        let heading = |name| {
            text(name)
                .size(12)
                .font(Font {
                    weight: iced::font::Weight::Semibold,
                    ..Font::DEFAULT
                })
                .color(Color::from_rgb8(236, 232, 242))
        };
        let toggle = |title, checked, event: fn(bool) -> Event| {
            row![
                text(title).size(12).color(Color::from_rgb8(206, 204, 214)),
                space().width(Length::Fill),
                toggler(checked)
                    .size(16)
                    .on_toggle(event)
                    .style(|theme, status| {
                        let mut style = toggler::default(theme, status);
                        if matches!(
                            status,
                            toggler::Status::Active { is_toggled: true }
                                | toggler::Status::Hovered { is_toggled: true }
                        ) {
                            style.background = Color::from_rgb8(170, 146, 205).into();
                        }
                        style
                    }),
            ]
            .align_y(iced::Center)
        };
        let color_button = |name, color, selected| {
            button(
                row![
                    text(if selected { "✓" } else { "" }).size(11).width(12),
                    text(name).size(12)
                ]
                .spacing(5),
            )
            .padding([5, 12])
            .on_press(Event::Highlight(color))
            .style(move |_, status| control_style(selected, status))
        };
        let appearance = column![
            heading("Appearance"),
            label("Text size", format!("{} px", preferences.font_size)),
            slider(22.0..=48.0, preferences.font_size as f32, Event::FontSize)
                .step(1.0_f32)
                .height(14)
                .style(slider_style),
            label("Opacity", format!("{:.0}%", preferences.opacity * 100.0)),
            slider(0.4..=1.0, preferences.opacity, Event::Opacity)
                .step(0.01_f32)
                .height(14)
                .style(slider_style),
            text("Highlight color")
                .size(12)
                .color(Color::from_rgb8(206, 204, 214)),
            row![
                color_button(
                    "Violet",
                    Highlight::Violet,
                    matches!(preferences.highlight, Highlight::Violet)
                ),
                color_button(
                    "White",
                    Highlight::White,
                    matches!(preferences.highlight, Highlight::White)
                )
            ]
            .spacing(6),
        ]
        .spacing(8);
        let lines = column![
            heading("Lines"),
            toggle(
                "Additional lines",
                preferences.additional_lines,
                Event::AdditionalLines
            ),
            toggle("Next line", preferences.next_line, Event::NextLine),
        ]
        .spacing(10);
        let window = column![
            heading("Window"),
            toggle("Lock position", preferences.locked, Event::Locked)
        ]
        .spacing(10);
        let content = column![
            toggle("Show lyrics", preferences.enabled, Event::Visible),
            rule::horizontal(1).style(rule::weak),
            appearance,
            rule::horizontal(1).style(rule::weak),
            lines,
            rule::horizontal(1).style(rule::weak),
            window,
        ]
        .spacing(12);
        let header = column![
            text("Desktop Lyrics").size(17).font(Font {
                weight: iced::font::Weight::Semibold,
                ..Font::DEFAULT
            }),
            text("Display settings")
                .size(11)
                .color(Color::from_rgb8(160, 157, 170))
        ]
        .spacing(3);
        let footer = row![
            button(text("Reset position").size(11))
                .on_press(Event::ResetPosition)
                .padding([7, 0])
                .style(button::text),
            space().width(Length::Fill),
            button(text("Close").size(12))
                .on_press(Event::CloseSettings)
                .padding([7, 18])
                .style(|_, status| control_style(true, status)),
        ]
        .align_y(iced::Center);
        container(
            column![
                header,
                scrollable(container(content).padding(iced::Padding::ZERO.right(8)))
                    .height(Length::Fill),
                rule::horizontal(1).style(rule::weak),
                footer,
            ]
            .spacing(14),
        )
        .padding(20)
        .width(Length::Fill)
        .height(Length::Fill)
        .style(|_| container::Style {
            background: Some(Color::from_rgb8(32, 32, 35).into()),
            ..Default::default()
        })
        .into()
    }
}

fn caption<'a>(label: String, size: f32, opacity: f32, outline_opacity: f32) -> Element<'a, Event> {
    let make_text = |label, color| {
        text(label)
            .size(size)
            .shaping(iced::advanced::text::Shaping::Advanced)
            .color(color)
            .align_x(iced::Center)
            .width(Length::Fill)
    };
    outlined(
        make_text(label.clone(), Color::WHITE.scale_alpha(opacity)),
        make_text(
            label,
            Color::from_rgb8(20, 18, 25).scale_alpha(0.92 * outline_opacity),
        ),
        0.7,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hover_must_not_force_the_next_lyric_into_the_previous_lines_width() {
        let mut overlay = Overlay {
            window: Some(window::Id::unique()),
            opened: true,
            size: Size::new(300.0, 50.0),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let _ = overlay.update(Event::Hover(true), &mut preferences);
        overlay.frame = serde_json::from_value(serde_json::json!({
            "elapsed":1,"lines":[{"time":0,"text":"A longer lyric that needs its natural width"}]
        }))
        .unwrap();
        assert!(overlay.display_size(&preferences).width > 300.0);
        assert!(
            iced_runtime::task::into_stream(overlay.fit(&preferences)).is_some(),
            "hovering must not prevent the lyric window from growing"
        );
    }

    async fn window_actions(
        task: Task<Event>,
        position: Point,
        size: Size,
    ) -> Vec<iced_runtime::Action<Event>> {
        use iced::futures::StreamExt;
        let mut actions = Vec::new();
        if let Some(mut stream) = iced_runtime::task::into_stream(task) {
            while let Some(action) = stream.next().await {
                match action {
                    iced_runtime::Action::Window(iced_runtime::window::Action::GetPosition(
                        _,
                        sender,
                    )) => {
                        sender.send(Some(position)).unwrap();
                    }
                    iced_runtime::Action::Window(iced_runtime::window::Action::GetSize(
                        _,
                        sender,
                    )) => {
                        sender.send(size).unwrap();
                    }
                    action @ iced_runtime::Action::Output(Event::Sized(..)) => {
                        actions.push(action);
                        break;
                    }
                    action => actions.push(action),
                }
            }
        }
        actions
    }

    #[tokio::test]
    async fn new_lyrics_resize_only_the_lyric_window_while_the_toolbar_is_dragged() {
        let id = window::Id::unique();
        let toolbar = window::Id::unique();
        let initial = Size::new(300.0, 50.0);
        let mut overlay = Overlay {
            window: Some(id),
            toolbar_window: Some(toolbar),
            toolbar_position: Some(Point::new(430.0, 250.0)),
            opened: true,
            size: initial,
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let _ = overlay.update(Event::Hover(true), &mut preferences);
        let _ = overlay.update(Event::Drag, &mut preferences);
        assert!(overlay.dragging);
        overlay.frame = serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
            {"time":0,"text":"A longer lyric that needs its natural width"}
        ]}))
        .unwrap();
        let expected = overlay.display_size(&preferences);
        assert!(expected.width > initial.width);
        let actions = window_actions(overlay.fit(&preferences), Point::ORIGIN, initial).await;
        let iced_runtime::Action::Output(sized) = actions.into_iter().next().unwrap() else {
            panic!("lyrics must grow even while dragging the controls");
        };
        let moved = overlay.moved(toolbar, Point::new(530.0, 350.0), &mut preferences);
        let _ = window_actions(moved, Point::ORIGIN, initial).await;
        let (task, _) = overlay.update(sized, &mut preferences);
        let actions = window_actions(task, Point::ORIGIN, initial).await;
        assert!(actions.iter().any(|action| matches!(action,
            iced_runtime::Action::Window(iced_runtime::window::Action::Resize(window, size))
                if *window == id && *size == expected)));
        assert!(actions.iter().any(|action| matches!(action,
            iced_runtime::Action::Window(iced_runtime::window::Action::Move(window, point))
                if *window == id && point.y == 430.0
                    && (point.x + expected.width / 2.0 - 650.0).abs() < 0.1)));
        assert!(!actions.iter().any(|action| matches!(action,
            iced_runtime::Action::Window(iced_runtime::window::Action::Move(window, _)
                | iced_runtime::window::Action::Resize(window, _)) if *window == toolbar)));
        assert_eq!(preferences.font_size, 32);
        assert_eq!(overlay.toolbar_position, Some(Point::new(530.0, 350.0)));
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn native_pair_movement_saves_position_without_sending_a_delayed_move_back_to_appkit() {
        let lyrics = window::Id::unique();
        let toolbar = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(lyrics),
            toolbar_window: Some(toolbar),
            size: Size::new(300.0, 50.0),
            native_linked: true,
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let task = overlay.moved(toolbar, Point::new(430.0, 300.0), &mut preferences);
        assert!(iced_runtime::task::into_stream(task).is_none());
        assert_eq!(
            preferences.position.map(|p| (p.x, p.y)),
            Some((400.0, 300.0))
        );
        let _ = overlay.update(Event::Linked(window::Id::unique(), false), &mut preferences);
        assert!(
            overlay.native_linked,
            "a closed toolbar must not replace the active link"
        );
        let _ = overlay.close();
        assert!(!overlay.native_linked);
    }

    #[tokio::test]
    async fn returning_to_the_current_size_cancels_a_pending_expansion() {
        let id = window::Id::unique();
        let initial = Size::new(300.0, 50.0);
        let mut overlay = Overlay {
            window: Some(id),
            opened: true,
            size: initial,
            frame: serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"text":"A very long lyric ".repeat(40)}
            ]}))
            .unwrap(),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let pending = overlay.fit(&preferences);
        let mut actions = window_actions(pending, Point::ORIGIN, initial).await;
        let iced_runtime::Action::Output(stale) = actions.pop().unwrap() else {
            panic!("a size query must produce a sizing event");
        };
        overlay.clear();
        assert!(iced_runtime::task::into_stream(overlay.fit(&preferences)).is_none());
        let (task, _) = overlay.update(stale, &mut preferences);
        assert!(iced_runtime::task::into_stream(task).is_none());
        assert_eq!(overlay.size, initial);
    }

    #[test]
    fn crossing_windows_and_dragging_do_not_hide_the_toolbar() {
        let mut overlay = Overlay {
            window: Some(window::Id::unique()),
            toolbar_window: Some(window::Id::unique()),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let _ = overlay.update(Event::Hover(true), &mut preferences);
        assert!(overlay.toolbar_visible);
        let _ = overlay.update(Event::Hover(false), &mut preferences);
        let old_revision = overlay.hover_revision;
        let _ = overlay.update(Event::ToolbarHover(true), &mut preferences);
        let _ = overlay.update(Event::HideToolbar(old_revision), &mut preferences);
        assert!(overlay.toolbar_visible);
        let _ = overlay.update(Event::Drag, &mut preferences);
        let _ = overlay.update(Event::ToolbarHover(false), &mut preferences);
        let _ = overlay.update(Event::HideToolbar(overlay.hover_revision), &mut preferences);
        assert!(overlay.toolbar_visible);
        let _ = overlay.update(Event::DragEnded, &mut preferences);
        let _ = overlay.update(Event::HideToolbar(overlay.hover_revision), &mut preferences);
        assert!(!overlay.toolbar_visible);
        let _ = overlay.update(Event::Visible(false), &mut preferences);
        assert!(overlay.window.is_none());
        assert!(overlay.toolbar_window.is_none());
        assert!(overlay.toolbar_position.is_none());
    }

    #[test]
    fn unavailable_absolute_position_uses_one_window_and_restores_the_latest_cue_after_drag() {
        let id = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            opened: true,
            locating: true,
            size: Size::new(300.0, 50.0),
            frame: serde_json::from_value(serde_json::json!({
                "elapsed":1,"lines":[{"time":0,"text":"Short"}]
            }))
            .unwrap(),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let _ = overlay.update(
            Event::ToolbarRequested(id, None, overlay.size),
            &mut preferences,
        );
        assert!(overlay.embedded_toolbar);
        assert!(overlay.toolbar_window.is_none());
        let size = overlay.display_size(&preferences);
        let _ = overlay.update(Event::Drag, &mut preferences);
        overlay.frame = serde_json::from_value(serde_json::json!({
            "elapsed":2,"lines":[{"time":1,"text":"A considerably longer next lyric"}]
        }))
        .unwrap();
        assert_eq!(
            overlay.display_frame().display_lines()[0].first_text(),
            "Short"
        );
        assert_eq!(overlay.display_size(&preferences), size);
        let _ = overlay.update(Event::DragEnded, &mut preferences);
        assert_eq!(overlay.display_frame().elapsed, 2.0);
        assert!(overlay.display_size(&preferences).width > size.width);
        assert!(overlay.drag_snapshot.is_none());
    }

    #[test]
    fn lyrics_have_their_own_content_area_without_toolbar_layout_constraints() {
        use iced::advanced::{layout, widget};
        #[derive(Default)]
        struct Bounds(Vec<(iced::Rectangle, iced::Rectangle)>);
        impl widget::Operation for Bounds {
            fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
                operate(self);
            }
            fn scrollable(
                &mut self,
                _: Option<&widget::Id>,
                bounds: iced::Rectangle,
                content: iced::Rectangle,
                _: iced::Vector,
                _: &mut dyn widget::operation::Scrollable,
            ) {
                self.0.push((bounds, content));
            }
        }
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
        let preferences = Preferences::default();
        for size in [Size::new(300.0, 50.0), Size::new(880.0, 500.0)] {
            for lyric in ["Short".to_owned(), "这是一句非常长的歌词 ".repeat(60)] {
                let overlay = Overlay {
                    hovered: true,
                    frame: serde_json::from_value(serde_json::json!({
                        "elapsed":1,"lines":[{"time":0,"text":lyric,
                            "parts":[lyric, "A long additional translation ".repeat(60)]}]
                    }))
                    .unwrap(),
                    ..Default::default()
                };
                let mut view = overlay.view(&preferences);
                let mut tree = widget::Tree::new(&view);
                let node = view.as_widget_mut().layout(
                    &mut tree,
                    &renderer,
                    &layout::Limits::new(size, size),
                );
                let mut bounds = Bounds::default();
                view.as_widget_mut().operate(
                    &mut tree,
                    layout::Layout::new(&node),
                    &renderer,
                    &mut bounds,
                );
                let [(viewport, _)] = bounds.0.as_slice() else {
                    panic!("the lyrics must have one bounded viewport");
                };
                assert_eq!(
                    viewport.y, 8.0,
                    "lyrics must use their own window content area"
                );
                assert!(viewport.y + viewport.height <= size.height - 8.0 + 0.5);
                assert!(viewport.height > 0.0);
            }
        }
    }

    #[test]
    fn ordinary_longer_bilingual_cues_keep_two_text_rows_while_dragging() {
        use iced::advanced::{layout, widget};
        struct Content(f32);
        impl widget::Operation for Content {
            fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
                operate(self);
            }
            fn scrollable(
                &mut self,
                _: Option<&widget::Id>,
                _: iced::Rectangle,
                content: iced::Rectangle,
                _: iced::Vector,
                _: &mut dyn widget::operation::Scrollable,
            ) {
                self.0 = content.height;
            }
        }
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
        let preferences = Preferences::default();
        let mut overlay = Overlay {
            dragging: true,
            hovered: true,
            ..Default::default()
        };
        for lyric in ["Short", "A longer phrase in its natural width"] {
            overlay.frame = serde_json::from_value(serde_json::json!({
                "elapsed":1,"lines":[{"time":0,"text":lyric,
                    "parts":[lyric,"保持原本的字号和自然排版"]}]
            }))
            .unwrap();
            let size = overlay.display_size(&preferences);
            assert!(
                size.width < 880.0,
                "this cue fits within the normal width limit"
            );
            let mut view = overlay.view(&preferences);
            let mut tree = widget::Tree::new(&view);
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
            let mut content = Content(0.0);
            view.as_widget_mut().operate(
                &mut tree,
                layout::Layout::new(&node),
                &renderer,
                &mut content,
            );
            assert!(
                (60.0..70.0).contains(&content.0),
                "one 32px lyric plus its translation must stay on two rows: {}",
                content.0
            );
        }
    }

    #[test]
    fn word_sweep_changes_pixels_without_shifting_english_or_bilingual_layout() {
        use iced::advanced::{
            layout, mouse,
            renderer::{Headless, Style},
            widget,
        };
        fn geometry(node: &layout::Node, out: &mut Vec<iced::Rectangle>) {
            out.push(node.bounds());
            for child in node.children() {
                geometry(child, out);
            }
        }
        for phrase in [
            "Let the quiet carry what we left unsaid",
            "把未说完的话，交给夜色",
        ] {
            let mut overlay = Overlay {
                frame: serde_json::from_value(serde_json::json!({
                    "elapsed":0,"lines":[{"time":0,"end":8,"text":phrase,
                        "parts":[phrase,"A quieter additional line"],
                        "words":[{"time":0,"end":8,"text":phrase}]}]
                }))
                .unwrap(),
                ..Default::default()
            };
            let preferences = Preferences::default();
            let size = overlay.display_size(&preferences);
            let mut reference = None;
            let mut images = Vec::new();
            for elapsed in [0.0, 3.7, 7.999] {
                overlay.frame.elapsed = elapsed;
                assert_eq!(overlay.display_size(&preferences), size);
                let mut renderer = iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(
                    Font::DEFAULT,
                    16.0.into(),
                ));
                let mut view = overlay.view(&preferences);
                let mut tree = widget::Tree::new(&view);
                let node = view.as_widget_mut().layout(
                    &mut tree,
                    &renderer,
                    &layout::Limits::new(size, size),
                );
                let mut bounds = Vec::new();
                geometry(&node, &mut bounds);
                if let Some(reference) = &reference {
                    assert_eq!(
                        &bounds, reference,
                        "word color must not alter wrapping or baselines"
                    );
                } else {
                    reference = Some(bounds);
                }
                let mut ui = iced_runtime::UserInterface::build(
                    view,
                    size,
                    iced_runtime::user_interface::Cache::new(),
                    &mut renderer,
                );
                ui.draw(
                    &mut renderer,
                    &iced::Theme::Dark,
                    &Style {
                        text_color: Color::WHITE,
                    },
                    mouse::Cursor::Unavailable,
                );
                images.push(renderer.screenshot(
                    Size::new(size.width as u32, size.height as u32),
                    1.0,
                    Color::TRANSPARENT,
                ));
            }
            assert_eq!(
                images[0].iter().skip(3).step_by(4).collect::<Vec<_>>(),
                images[1].iter().skip(3).step_by(4).collect::<Vec<_>>(),
                "the click-through alpha mask must not change with word progress"
            );
            assert_eq!(
                images[1].iter().skip(3).step_by(4).collect::<Vec<_>>(),
                images[2].iter().skip(3).step_by(4).collect::<Vec<_>>()
            );
            assert_ne!(
                images[0], images[1],
                "the emphasis front must advance within a word"
            );
            assert_ne!(
                images[1], images[2],
                "the completed phrase must have a different color"
            );
        }
    }

    #[tokio::test]
    async fn the_toolbar_opens_at_a_fixed_size_above_the_lyrics_without_moving_them() {
        use iced::futures::StreamExt;
        let root = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(root),
            opened: true,
            locating: true,
            size: Size::new(300.0, 50.0),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let (task, _) = overlay.update(
            Event::ToolbarRequested(root, Some(Point::new(400.0, 380.0)), overlay.size),
            &mut preferences,
        );
        let mut stream = iced_runtime::task::into_stream(task).unwrap();
        let Some(iced_runtime::Action::Window(iced_runtime::window::Action::Open(id, settings, _))) =
            stream.next().await
        else {
            panic!("a separate toolbar must open");
        };
        assert_eq!(Some(id), overlay.toolbar_window);
        assert_eq!(settings.size, Size::new(240.0, 88.0));
        assert_eq!(overlay.toolbar_position, Some(Point::new(430.0, 300.0)));
        assert_eq!(
            overlay.lyrics_position(Point::new(430.0, 300.0), overlay.size),
            Point::new(400.0, 380.0)
        );
        let _ = overlay.moved(root, Point::new(400.0, 380.0), &mut preferences);
        assert_eq!(
            preferences.position.map(|p| (p.x, p.y)),
            Some((400.0, 300.0))
        );
    }

    #[cfg(target_os = "macos")]
    #[tokio::test]
    async fn paused_hover_captures_both_windows_and_cancels_stale_masks() {
        use iced::futures::StreamExt;
        async fn capture(task: Task<Event>) -> (window::Id, u64) {
            let mut stream = iced_runtime::task::into_stream(task).unwrap();
            while let Some(action) = stream.next().await {
                if let iced_runtime::Action::Output(Event::Capture(id, revision)) = action {
                    return (id, revision);
                }
            }
            panic!("a visual change must capture the current alpha mask");
        }
        let id = window::Id::unique();
        let toolbar = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            toolbar_window: Some(toolbar),
            opened: true,
            toolbar_opened: true,
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        overlay.size = overlay.display_size(&preferences);
        let (task, _) = overlay.update(Event::Hover(true), &mut preferences);
        let (window, old_revision) = capture(task).await;
        assert!(window == id || window == toolbar);
        let (task, _) = overlay.update(Event::Hover(false), &mut preferences);
        let (_, revision) = capture(task).await;
        assert_ne!(revision, old_revision);
        for id in [id, toolbar] {
            let (task, _) = overlay.update(Event::Capture(id, old_revision), &mut preferences);
            assert!(iced_runtime::task::into_stream(task).is_none());
        }
        let actions = window_actions(overlay.refresh_mask(), Point::ORIGIN, overlay.size).await;
        assert_eq!(
            actions
                .iter()
                .filter(|action| matches!(
                    action,
                    iced_runtime::Action::Output(Event::Capture(_, _))
                ))
                .count(),
            2
        );
    }

    #[test]
    fn settings_groups_fit_at_the_default_size_and_footer_stays_outside_scroll_content() {
        use iced::advanced::{layout, widget};
        #[derive(Default)]
        struct Bounds(Vec<(iced::Rectangle, iced::Rectangle)>);
        impl widget::Operation for Bounds {
            fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
                operate(self);
            }
            fn scrollable(
                &mut self,
                _: Option<&widget::Id>,
                bounds: iced::Rectangle,
                content: iced::Rectangle,
                _: iced::Vector,
                _: &mut dyn widget::operation::Scrollable,
            ) {
                self.0.push((bounds, content));
            }
        }
        let overlay = Overlay::default();
        let preferences = Preferences::default();
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 14.0.into()));
        let size = Size::new(320.0, 520.0);
        let mut view = overlay.settings_view(&preferences);
        let mut tree = widget::Tree::new(&view);
        let layout =
            view.as_widget_mut()
                .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
        let mut bounds = Bounds::default();
        view.as_widget_mut().operate(
            &mut tree,
            layout::Layout::new(&layout),
            &renderer,
            &mut bounds,
        );
        assert_eq!(bounds.0.len(), 1);
        let (viewport, content) = bounds.0[0];
        assert!(
            content.height <= viewport.height + 0.5,
            "settings content {} exceeds {}",
            content.height,
            viewport.height
        );
        assert!(
            viewport.y + viewport.height < size.height - 55.0,
            "footer must be separate from scrolling preferences"
        );
    }

    #[test]
    fn ordinary_bilingual_lyrics_fit_below_tooltip_space_without_scrollbars() {
        use iced::advanced::{layout, widget};
        #[derive(Default)]
        struct Bounds(Vec<(iced::Rectangle, iced::Rectangle)>);
        impl widget::Operation for Bounds {
            fn traverse(&mut self, operate: &mut dyn FnMut(&mut dyn widget::Operation)) {
                operate(self);
            }
            fn scrollable(
                &mut self,
                _: Option<&widget::Id>,
                bounds: iced::Rectangle,
                content: iced::Rectangle,
                _: iced::Vector,
                _: &mut dyn widget::operation::Scrollable,
            ) {
                self.0.push((bounds, content));
            }
        }
        let frames = [
            serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"text":"把未说完的话，交给夜色",
                 "parts":["把未说完的话，交给夜色","Let the quiet carry what we left unsaid."]}
            ]}),
            serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"text":"First voice"},
                {"time":0,"text":"Second voice"},
                {"time":0,"text":"Third voice"}
            ]}),
            serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"prefix":"Voice A: ","text":"Hello, world!",
                 "words":[{"time":0,"end":0.5,"text":"Hello, "},{"time":0.7,"end":1.5,"text":"world!"}],
                 "parts":["Hello, world!","A longer translation with punctuation and spaces.","另一个翻译。"]},
                {"time":2,"end":4,"text":"Next cue"}
            ]}),
        ];
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
        for frame in frames {
            let overlay = Overlay {
                frame: serde_json::from_value(frame).unwrap(),
                ..Default::default()
            };
            for font_size in [22, 32, 48] {
                let preferences = Preferences {
                    font_size,
                    next_line: true,
                    ..Default::default()
                };
                let size = overlay.display_size(&preferences);
                let mut view = overlay.view(&preferences);
                let mut tree = widget::Tree::new(&view);
                let node = view.as_widget_mut().layout(
                    &mut tree,
                    &renderer,
                    &layout::Limits::new(size, size),
                );
                let mut bounds = Bounds::default();
                view.as_widget_mut().operate(
                    &mut tree,
                    layout::Layout::new(&node),
                    &renderer,
                    &mut bounds,
                );
                assert!(!bounds.0.is_empty());
                for (viewport, content) in bounds.0 {
                    assert!(
                        content.height <= viewport.height + 0.5,
                        "font size {font_size}: content {} exceeds viewport {}",
                        content.height,
                        viewport.height
                    );
                }
            }
        }
    }

    #[test]
    fn short_gaps_keep_overlay_geometry_until_the_next_cue_and_clear_removes_it() {
        let mut overlay=Overlay {frame:serde_json::from_value(serde_json::json!({
            "elapsed":1.9,"lines":[
                {"time":1,"end":2,"text":"A sufficiently long line to size the window", "parts":["A sufficiently long line to size the window","Additional translation"]},
                {"time":2.15,"end":4,"text":"Next"}
            ]
        })).unwrap(),..Default::default()};
        let preferences = Preferences::default();
        let before = overlay.display_size(&preferences);
        overlay.frame.elapsed = 2.05;
        assert_eq!(overlay.display_size(&preferences), before);
        overlay.frame.elapsed = 2.15;
        assert_eq!(overlay.frame.display_lines()[0].text, "Next");
        overlay.clear();
        assert!(overlay.frame.display_lines().is_empty());
        assert_eq!(overlay.display_size(&preferences), Size::new(300.0, 50.0));
    }

    #[test]
    fn overlay_bounds_follow_lyrics_and_optional_sublines_without_growing_on_progress() {
        let mut overlay = Overlay { frame: serde_json::from_value(serde_json::json!({
            "elapsed": 1, "lines": [{"time":0,"text":"Short line", "parts":["Short line","A second line"]}]
        })).unwrap(), ..Default::default() };
        let mut preferences = Preferences {
            additional_lines: false,
            ..Default::default()
        };
        let short = overlay.display_size(&preferences);
        preferences.additional_lines = true;
        let bilingual = overlay.display_size(&preferences);
        assert!(bilingual.height > short.height);
        overlay.frame.lines[0].parts[0] =
            "A considerably longer line with more words to read on the desktop".into();
        let long = overlay.display_size(&preferences);
        assert!(long.width > short.width);
        overlay.frame.elapsed = 2.0;
        assert_eq!(overlay.display_size(&preferences), long);
    }

    #[tokio::test]
    async fn frames_received_during_opening_are_sized_after_the_window_is_ready() {
        let id = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            size: Size::new(300.0, 50.0),
            frame: serde_json::from_value(serde_json::json!({"elapsed":1,"lines":[
                {"time":0,"text":"A considerably longer line than the initial waiting message"}
            ]}))
            .unwrap(),
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        let initial = overlay.size;
        let _ = overlay.fit(&preferences);
        assert_eq!(overlay.size, initial);
        let expected = overlay.display_size(&preferences);
        assert!(expected.width > initial.width);
        let _ = overlay.update(Event::Opened(id), &mut preferences);
        assert!(overlay.opened);
        assert!(overlay.locating);
        assert!(iced_runtime::task::into_stream(overlay.fit(&preferences)).is_none());
        let (task, _) = overlay.update(
            Event::ToolbarRequested(id, Some(Point::new(400.0, 380.0)), initial),
            &mut preferences,
        );
        assert_eq!(overlay.toolbar_position, Some(Point::new(430.0, 300.0)));
        let actions = window_actions(task, Point::ORIGIN, initial).await;
        let event = actions
            .into_iter()
            .find_map(|action| match action {
                iced_runtime::Action::Output(event @ Event::Sized(..)) => Some(event),
                _ => None,
            })
            .expect("the ready window must request its latest lyric size");
        let _ = overlay.update(event, &mut preferences);
        assert_eq!(overlay.size, expected);
    }
}

fn toolbar_style(_: &iced::Theme) -> container::Style {
    container::Style {
        background: Some(Color::from_rgba8(34, 34, 37, 0.94).into()),
        border: iced::Border {
            color: Color::WHITE.scale_alpha(0.09),
            width: 1.0,
            radius: 8.0.into(),
        },
        ..Default::default()
    }
}

fn control_style(selected: bool, status: button::Status) -> button::Style {
    let hovered = matches!(status, button::Status::Hovered | button::Status::Pressed);
    button::Style {
        background: Some(
            if selected {
                Color::from_rgb8(56, 52, 63)
            } else if hovered {
                Color::from_rgb8(48, 46, 52)
            } else {
                Color::TRANSPARENT
            }
            .into(),
        ),
        text_color: if selected {
            Color::from_rgb8(221, 203, 248)
        } else {
            Color::from_rgb8(180, 176, 190)
        },
        border: iced::Border {
            radius: 5.0.into(),
            ..Default::default()
        },
        ..Default::default()
    }
}

fn slider_style(theme: &iced::Theme, status: slider::Status) -> slider::Style {
    let mut style = slider::default(theme, status).with_circular_handle(5.0);
    style.rail.width = 3.0;
    style.rail.backgrounds = (
        Color::from_rgb8(176, 153, 213).into(),
        Color::from_rgb8(68, 62, 76).into(),
    );
    style.handle.background = Color::from_rgb8(217, 200, 247).into();
    style
}

#[derive(Clone, Copy)]
enum Icon {
    Move,
    Playback,
    Lock,
    Settings,
}

fn icon(kind: Icon) -> iced::widget::Canvas<Glyph, Event> {
    iced::widget::canvas(Glyph(kind)).width(16).height(16)
}

fn hint<'a>(
    content: impl Into<Element<'a, Event>>,
    label: &'a str,
    position: tooltip::Position,
) -> Element<'a, Event> {
    tooltip(
        content,
        container(text(label).size(11).color(Color::from_rgb8(235, 232, 241))).padding([6, 9]),
        position,
    )
    .gap(7)
    .padding(0)
    .delay(std::time::Duration::from_millis(400))
    .style(|_| container::Style {
        background: Some(Color::from_rgba8(30, 29, 34, 0.98).into()),
        border: iced::Border {
            color: Color::WHITE.scale_alpha(0.14),
            width: 1.0,
            radius: 6.0.into(),
        },
        shadow: iced::Shadow {
            color: Color::BLACK.scale_alpha(0.2),
            offset: iced::Vector::new(0.0, 2.0),
            blur_radius: 6.0,
        },
        ..Default::default()
    })
    .into()
}

fn icon_button(kind: Icon, label: &'static str, event: Event) -> Element<'static, Event> {
    hint(
        button(icon(kind))
            .padding(6)
            .on_press(event)
            .style(|_, status| control_style(false, status)),
        label,
        tooltip::Position::Top,
    )
}

struct Glyph(Icon);

impl iced::widget::canvas::Program<Event> for Glyph {
    type State = ();
    fn draw(
        &self,
        _: &(),
        renderer: &iced::Renderer,
        _: &iced::Theme,
        bounds: iced::Rectangle,
        _: iced::mouse::Cursor,
    ) -> Vec<iced::widget::canvas::Geometry> {
        use iced::widget::canvas::{Frame, Path, Stroke};
        let mut frame = Frame::new(renderer, bounds.size());
        let path = Path::new(|p| {
            let mut line = |a, b| {
                p.move_to(Point::from(a));
                p.line_to(Point::from(b));
            };
            match self.0 {
                Icon::Move => {
                    for y in [4.0, 8.0, 12.0] {
                        line((5.0, y), (5.5, y));
                        line((10.5, y), (11.0, y));
                    }
                }
                Icon::Playback => {
                    line((2.0, 3.0), (8.0, 8.0));
                    line((8.0, 8.0), (2.0, 13.0));
                    line((2.0, 13.0), (2.0, 3.0));
                    line((11.0, 3.0), (11.0, 13.0));
                    line((14.0, 3.0), (14.0, 13.0));
                }
                Icon::Lock => {
                    line((3.0, 7.0), (13.0, 7.0));
                    line((13.0, 7.0), (13.0, 14.0));
                    line((13.0, 14.0), (3.0, 14.0));
                    line((3.0, 14.0), (3.0, 7.0));
                    line((5.0, 7.0), (5.0, 4.0));
                    line((5.0, 4.0), (8.0, 2.0));
                    line((8.0, 2.0), (11.0, 4.0));
                    line((11.0, 4.0), (11.0, 7.0));
                    line((8.0, 10.0), (8.0, 12.0));
                }
                Icon::Settings => {
                    line((2.0, 5.0), (14.0, 5.0));
                    line((2.0, 11.0), (14.0, 11.0));
                    line((6.0, 3.0), (6.0, 7.0));
                    line((11.0, 9.0), (11.0, 13.0));
                }
            }
        });
        frame.stroke(
            &path,
            Stroke::default()
                .with_width(1.3)
                .with_color(Color::from_rgb8(205, 201, 216)),
        );
        vec![frame.into_geometry()]
    }
}

fn primary_font() -> Font {
    Font {
        weight: iced::font::Weight::Semibold,
        ..Font::DEFAULT
    }
}

fn measure_primary(line: &crate::lyrics::Line, size: f32, width: f32) -> Size {
    use iced::advanced::text::{Paragraph, Shaping};
    let mut spans: Vec<iced::advanced::text::Span<'_, (), Font>> = Vec::new();
    if !line.prefix.is_empty() {
        spans.push(span(line.prefix.as_str()).size(size * 0.5));
    }
    spans.extend(
        line.fragments(0.0, |_| 1.0)
            .into_iter()
            .map(|(text, _)| span(text)),
    );
    iced::advanced::graphics::text::paragraph::Paragraph::with_spans(iced::advanced::Text {
        content: spans.as_slice(),
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: Default::default(),
        font: primary_font(),
        align_x: iced::advanced::text::Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Default::default(),
    })
    .min_bounds()
}

fn measure_caption(content: &str, size: f32, width: f32) -> Size {
    measure_with_font(content, size, width, Font::DEFAULT)
}

fn measure_text(content: &str, size: f32, width: f32) -> Size {
    measure_with_font(content, size, width, primary_font())
}

fn measure_with_font(content: &str, size: f32, width: f32, font: Font) -> Size {
    use iced::advanced::text::{Alignment, LineHeight, Paragraph, Shaping};
    iced::advanced::graphics::text::paragraph::Paragraph::with_text(iced::advanced::Text {
        content,
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: LineHeight::default(),
        font,
        align_x: Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Default::default(),
    })
    .min_bounds()
}
