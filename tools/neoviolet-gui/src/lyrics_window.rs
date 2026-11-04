//! Native lyrics and settings windows; playback remains in the TUI.
use iced::widget::{
    button, column, container, mouse_area, rich_text, row, rule, scrollable, slider, space, span,
    text, toggler, tooltip,
};
use iced::{Color, Element, Font, Length, Point, Size, Task, window};

use crate::ipc::Request;
use crate::lyrics::{Frame, Highlight, Position, Preferences};
use crate::lyrics_text::outlined;

#[derive(Debug, Clone)]
pub enum Event {
    Opened(window::Id),
    SettingsOpened(window::Id),
    Ready,
    #[cfg(target_os = "macos")]
    Capture(window::Id, u64),
    #[cfg(target_os = "macos")]
    Mask(window::Id, u64, crate::lyrics_hit_test::Mask),
    Sized(window::Id, Point, Size, Size),
    Visible(bool),
    Settings,
    CloseSettings,
    ResetPosition,
    Hover(bool),
    Drag,
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
    pub settings_window: Option<window::Id>,
    hovered: bool,
    size: Size,
    opened: bool,
    #[cfg(target_os = "macos")]
    mask_revision: u64,
}

impl Overlay {
    pub fn display_size(&self, preferences: &Preferences) -> Size {
        let active = self.frame.active_lines();
        let mut rows = Vec::new();
        for line in active {
            rows.push((
                format!("{}{}", line.prefix, line.first_text()),
                preferences.font_size as f32,
            ));
            if preferences.additional_lines {
                rows.extend(
                    line.additional_lines()
                        .iter()
                        .filter(|line| !line.trim().is_empty())
                        .map(|line| (line.clone(), preferences.font_size as f32 * 0.46)),
                );
            }
        }
        if preferences.next_line
            && let Some(next) = self.frame.next_line()
        {
            rows.push((format!("{}{}", next.prefix, next.first_text()), 13.0));
        }
        if rows.is_empty() {
            return Size::new(300.0, 130.0);
        }
        let width = (rows
            .iter()
            .map(|(line, size)| measure_text(line, *size, f32::INFINITY).width)
            .fold(0.0_f32, f32::max)
            .ceil()
            + 40.0)
            .clamp(300.0, 880.0);
        let height = rows
            .iter()
            .map(|(line, size)| measure_text(line, *size, width - 40.0).height)
            .sum::<f32>()
            + 100.0
            + (rows.len() - 1) as f32 * 8.0;
        Size::new(width, height.ceil().clamp(130.0, 500.0))
    }

    pub fn fit(&mut self, preferences: &Preferences) -> Task<Event> {
        if !self.opened {
            return Task::none();
        }
        let Some(id) = self.window else {
            return Task::none();
        };
        let size = self.display_size(preferences);
        if size == self.size {
            return Task::none();
        }
        self.size = size;
        window::position(id).then(move |position| {
            window::size(id).map(move |previous| {
                Event::Sized(id, position.unwrap_or(Point::ORIGIN), previous, size)
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
                    (monitor.height * 0.65 - window.height / 2.0).max(0.0),
                )
            }),
            |point| window::Position::Specific(Point::new(point.x, point.y)),
        );
        self.size = self.display_size(preferences);
        self.opened = false;
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
    }

    pub fn moved(&self, id: window::Id, point: Point, preferences: &mut Preferences) {
        if self.window == Some(id) && point.x.is_finite() && point.y.is_finite() {
            preferences.position = Some(Position {
                x: point.x,
                y: point.y,
            });
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
                if self.window != Some(id) || self.mask_revision != revision {
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
                if self.window != Some(id) || self.mask_revision != revision {
                    return (Task::none(), None);
                }
                return (
                    crate::desktop::set_lyrics_mask(id, mask, preferences.locked)
                        .map(|_| Event::Ready),
                    None,
                );
            }
            Event::Opened(id) if self.window == Some(id) => {
                self.opened = true;
                return (
                    Task::batch([
                        self.passthrough(preferences.locked),
                        self.fit(preferences),
                        self.refresh_mask(),
                    ]),
                    None,
                );
            }
            Event::SettingsOpened(id) if self.settings_window == Some(id) => Task::none(),
            Event::Opened(id) | Event::SettingsOpened(id) => window::close(id),
            Event::Visible(enable) => {
                preferences.enabled = enable;
                self.hovered = false;
                let task = if enable {
                    self.show(preferences)
                } else {
                    self.opened = false;
                    self.clear();
                    self.window
                        .take()
                        .map_or_else(Task::none, crate::desktop::close_lyrics)
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
                let close = self
                    .window
                    .take()
                    .map_or_else(Task::none, crate::desktop::close_lyrics);
                Task::batch([close, self.show(preferences)])
            }
            Event::Sized(id, position, previous, size) => {
                if self.window != Some(id) || self.size != size {
                    return (Task::none(), None);
                }
                let position =
                    Point::new(position.x + (previous.width - size.width) / 2.0, position.y);
                Task::batch([window::move_to(id, position), window::resize(id, size)])
            }
            Event::Hover(hovered) => {
                self.hovered = hovered;
                Task::none()
            }
            Event::Drag if !preferences.locked => self.window.map_or_else(Task::none, window::drag),
            Event::Locked(locked) => {
                preferences.locked = locked;
                self.hovered = false;
                self.passthrough(locked)
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
        self.window.map_or_else(Task::none, |id| {
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
        })
    }

    pub fn refresh_mask(&mut self) -> Task<Event> {
        #[cfg(target_os = "macos")]
        {
            let Some(id) = self.window.filter(|_| self.opened) else {
                return Task::none();
            };
            self.mask_revision = self.mask_revision.wrapping_add(1);
            let revision = self.mask_revision;
            // Let Iced draw the changed content before reading its alpha buffer.
            Task::perform(
                async { tokio::time::sleep(std::time::Duration::from_millis(30)).await },
                move |_| Event::Capture(id, revision),
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
            Highlight::Violet => Color::from_rgb8(220, 205, 255),
            Highlight::White => Color::from_rgb8(245, 243, 250),
        }
        .scale_alpha(alpha);
        let active = self.frame.active_lines();
        let mut lines = column![]
            .spacing(12)
            .align_x(iced::Center)
            .width(Length::Fill);
        if active.is_empty() {
            let label = if self.frame.next_line().is_some() || !self.frame.lines.is_empty() {
                "···"
            } else if self.frame.title.is_empty() && self.frame.artist.is_empty() {
                "Waiting for music"
            } else {
                "No lyrics"
            };
            lines = lines.push(caption(label.to_owned(), 17.0, 0.85 * alpha, alpha));
        }
        for line in active {
            let mut fragments = Vec::new();
            if !line.prefix.is_empty() {
                fragments.push(
                    span(line.prefix.as_str())
                        .size(preferences.font_size as f32 * 0.5)
                        .color(sung),
                );
            }
            for (fragment, played) in line.fragments(self.frame.elapsed) {
                fragments.push(span(fragment).color(if played {
                    sung
                } else {
                    Color::from_rgb8(220, 218, 226).scale_alpha(0.95 * alpha)
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
                    .font(Font {
                        weight: iced::font::Weight::Medium,
                        ..Font::DEFAULT
                    })
                    .align_x(iced::Center)
                    .width(Length::Fill)
            };
            let primary = outlined(make_text(fragments), make_text(shadow), 0.95);
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
            && let Some(next) = self.frame.next_line()
        {
            lines = lines.push(caption(
                format!("{}{}", next.prefix, next.first_text()),
                13.0,
                0.8 * alpha,
                alpha,
            ));
        }
        let toolbar: Element<'_, Event> = if self.hovered && !preferences.locked {
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
        };
        let content = column![
            space().height(30),
            container(toolbar).height(34).center_x(Length::Fill),
            scrollable(lines).height(Length::Shrink),
        ]
        .spacing(8)
        .align_x(iced::Center);
        let area = mouse_area(
            container(content)
                .padding([8, 20])
                .center_x(Length::Fill)
                .center_y(Length::Fill),
        )
        .on_press(Event::Drag)
        .interaction(iced::mouse::Interaction::Grab);
        #[cfg(not(target_os = "macos"))]
        let area = area
            .on_enter(Event::Hover(true))
            .on_exit(Event::Hover(false));
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

    #[cfg(target_os = "macos")]
    #[tokio::test]
    async fn paused_hover_captures_the_changed_toolbar_and_cancels_stale_masks() {
        use iced::futures::StreamExt;
        let id = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            opened: true,
            ..Default::default()
        };
        let mut preferences = Preferences::default();
        overlay.size = overlay.display_size(&preferences);
        let (task, _) = overlay.update(Event::Hover(true), &mut preferences);
        let messages = iced_runtime::task::into_stream(task)
            .unwrap()
            .collect::<Vec<_>>()
            .await;
        let [iced_runtime::Action::Output(Event::Capture(window, old_revision))] =
            messages.as_slice()
        else {
            panic!("a hover change must queue a single capture even while paused");
        };
        assert_eq!(*window, id);
        let old_revision = *old_revision;
        let (task, _) = overlay.update(Event::Hover(false), &mut preferences);
        let messages = iced_runtime::task::into_stream(task)
            .unwrap()
            .collect::<Vec<_>>()
            .await;
        assert!(
            matches!(messages.as_slice(), [iced_runtime::Action::Output(Event::Capture(window,revision))]
            if *window == id && *revision != old_revision)
        );
        let (stale, _) = overlay.update(Event::Capture(id, old_revision), &mut preferences);
        assert!(iced_runtime::task::into_stream(stale).is_none());
        let (ready, _) = overlay.update(Event::Ready, &mut preferences);
        if let Some(stream) = iced_runtime::task::into_stream(ready) {
            assert!(stream.collect::<Vec<_>>().await.is_empty());
        }
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
        let overlay = Overlay {
            frame: serde_json::from_value(serde_json::json!({
                "elapsed":1,"lines":[{"time":0,"text":"把未说完的话，交给夜色",
                    "parts":["把未说完的话，交给夜色","Let the quiet carry what we left unsaid."]}]
            }))
            .unwrap(),
            ..Default::default()
        };
        let renderer =
            iced::Renderer::Secondary(iced_tiny_skia::Renderer::new(Font::DEFAULT, 16.0.into()));
        for font_size in [22, 32, 48] {
            let preferences = Preferences {
                font_size,
                ..Default::default()
            };
            let size = overlay.display_size(&preferences);
            let mut view = overlay.view(&preferences);
            let mut tree = widget::Tree::new(&view);
            let node =
                view.as_widget_mut()
                    .layout(&mut tree, &renderer, &layout::Limits::new(size, size));
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

    #[test]
    fn frames_received_during_opening_are_sized_after_the_window_is_ready() {
        let id = window::Id::unique();
        let mut overlay = Overlay {
            window: Some(id),
            size: Size::new(300.0, 130.0),
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

fn measure_text(content: &str, size: f32, width: f32) -> Size {
    use iced::advanced::text::{Alignment, LineHeight, Paragraph, Shaping, Wrapping};
    iced::advanced::graphics::text::paragraph::Paragraph::with_text(iced::advanced::Text {
        content,
        bounds: Size::new(width, f32::INFINITY),
        size: size.into(),
        line_height: LineHeight::Relative(1.3),
        font: Font {
            weight: iced::font::Weight::Medium,
            ..Font::DEFAULT
        },
        align_x: Alignment::Center,
        align_y: iced::alignment::Vertical::Top,
        shaping: Shaping::Advanced,
        wrapping: Wrapping::WordOrGlyph,
    })
    .min_bounds()
}
