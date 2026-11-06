//! Blend custom chrome into the terminal when it is not being used.
use super::State;
use iced::Color;
use iced::animation::{Animation, Easing};
use iced::time::{Duration, Instant};

#[derive(Debug)]
pub(super) struct Appearance {
    pub(super) hovered: bool,
    pub(super) opacity: f32,
    transition: Animation<bool>,
    now: Instant,
}

impl Default for Appearance {
    fn default() -> Self {
        Self {
            hovered: false,
            opacity: 1.0,
            transition: Animation::new(false)
                .duration(Duration::from_millis(180))
                .easing(Easing::EaseInOut),
            now: Instant::now(),
        }
    }
}

impl State {
    pub(crate) fn animate_chrome(&mut self, now: Instant) {
        self.appearance.now = now;
        let active = !self.fullscreen
            && (self.appearance.hovered
                || self.caption_hover.is_some()
                || self.menu_open
                || self.press.is_some());
        if active != self.appearance.transition.value() {
            self.appearance.transition.go_mut(active, now);
        }
    }

    pub(crate) fn chrome_animating(&self) -> bool {
        self.appearance.transition.is_animating(self.appearance.now)
    }

    pub(super) fn chrome_color(&self) -> Color {
        let blend = self
            .appearance
            .transition
            .interpolate(0.0, 1.0, self.appearance.now);
        let base = crate::app::TERMINAL_BACKGROUND;
        let hover = Color::from_rgb8(43, 43, 43);
        Color::from_rgba(
            base.r + (hover.r - base.r) * blend,
            base.g + (hover.g - base.g) * blend,
            base.b + (hover.b - base.b) * blend,
            self.appearance.opacity + (1.0 - self.appearance.opacity) * blend,
        )
    }
}
