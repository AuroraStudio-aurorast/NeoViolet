//! Logical caption regions shared by native hit testing and its platform-independent tests.
use iced::{Point, Size};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Hit {
    Client,
    Caption,
    Minimize,
    Maximize,
    Close,
    North,
    South,
    West,
    East,
    NorthWest,
    NorthEast,
    SouthWest,
    SouthEast,
}

#[derive(Default)]
pub struct CaptionPress(Option<Hit>);

impl CaptionPress {
    pub fn begin(&mut self, hit: Hit) -> bool {
        self.0 = matches!(hit, Hit::Minimize | Hit::Maximize | Hit::Close).then_some(hit);
        self.0.is_some()
    }

    pub fn release(&mut self, hit: Hit) -> Option<crate::actions::Action> {
        if self.0.take() != Some(hit) {
            return None;
        }
        match hit {
            Hit::Minimize => Some(crate::actions::Action::Minimize),
            Hit::Maximize => Some(crate::actions::Action::ZoomWindow),
            Hit::Close => Some(crate::actions::Action::Quit),
            _ => None,
        }
    }

    pub fn cancel(&mut self) {
        self.0 = None;
    }
    pub fn capture_changed(&mut self, retained: bool) {
        if !retained {
            self.cancel();
        }
    }
    pub fn active(&self) -> bool {
        self.0.is_some()
    }
}

pub fn hit(point: Point, size: Size, maximized: bool, fullscreen: bool, menu_open: bool) -> Hit {
    if fullscreen
        || menu_open
        || !point.x.is_finite()
        || !point.y.is_finite()
        || point.x < 0.0
        || point.y < 0.0
        || point.x >= size.width
        || point.y >= size.height
    {
        return Hit::Client;
    }
    if !maximized {
        let left = point.x < 5.0;
        let right = point.x >= size.width - 5.0;
        let top = point.y < 5.0;
        let bottom = point.y >= size.height - 5.0;
        match (left, right, top, bottom) {
            (true, _, true, _) => return Hit::NorthWest,
            (_, true, true, _) => return Hit::NorthEast,
            (true, _, _, true) => return Hit::SouthWest,
            (_, true, _, true) => return Hit::SouthEast,
            (true, _, _, _) => return Hit::West,
            (_, true, _, _) => return Hit::East,
            (_, _, true, _) => return Hit::North,
            (_, _, _, true) => return Hit::South,
            _ => {}
        }
    }
    if point.y >= crate::titlebar::HEIGHT || point.x < crate::titlebar::MENU_BUTTON_WIDTH {
        return Hit::Client;
    }
    if point.x >= size.width - 42.0 {
        Hit::Close
    } else if point.x >= size.width - 84.0 {
        Hit::Maximize
    } else if point.x >= size.width - 126.0 {
        Hit::Minimize
    } else {
        Hit::Caption
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::actions::Action;

    #[test]
    fn caption_actions_require_release_over_the_pressed_button_and_fire_once() {
        let mut press = CaptionPress::default();
        assert!(!press.active());
        for (hit, action) in [
            (Hit::Minimize, Action::Minimize),
            (Hit::Maximize, Action::ZoomWindow),
            (Hit::Close, Action::Quit),
        ] {
            assert!(press.begin(hit));
            assert!(press.active());
            assert_eq!(
                press.release(hit).map(|a| std::mem::discriminant(&a)),
                Some(std::mem::discriminant(&action))
            );
            assert!(press.release(hit).is_none());
        }
        assert!(press.begin(Hit::Maximize));
        assert!(press.release(Hit::Caption).is_none());
        assert!(press.begin(Hit::Minimize));
        press.cancel();
        assert!(press.release(Hit::Minimize).is_none());
        assert!(!press.begin(Hit::Client));
    }

    #[test]
    fn retaining_native_capture_preserves_the_press_and_losing_capture_cancels_it() {
        let mut press = CaptionPress::default();
        assert!(press.begin(Hit::Maximize));
        press.capture_changed(true);
        assert!(press.active());
        assert!(matches!(
            press.release(Hit::Maximize),
            Some(Action::ZoomWindow)
        ));
        assert!(press.begin(Hit::Minimize));
        press.capture_changed(false);
        assert!(!press.active());
        assert!(press.release(Hit::Minimize).is_none());
    }
    #[test]
    fn caption_regions_preserve_menu_client_input_and_all_native_controls() {
        let size = Size::new(800.0, 600.0);
        let at = |x, y| hit(Point::new(x, y), size, false, false, false);
        assert_eq!(at(15.0, 18.0), Hit::Client);
        assert_eq!(at(62.0, 18.0), Hit::Client);
        assert_eq!(at(150.0, 18.0), Hit::Caption);
        assert_eq!(at(690.0, 18.0), Hit::Minimize);
        assert_eq!(at(735.0, 18.0), Hit::Maximize);
        assert_eq!(at(775.0, 18.0), Hit::Close);
        assert_eq!(at(2.0, 2.0), Hit::NorthWest);
        assert_eq!(at(798.0, 598.0), Hit::SouthEast);
        assert_eq!(at(400.0, 598.0), Hit::South);
        assert_eq!(at(400.0, 300.0), Hit::Client);
    }
    #[test]
    fn fullscreen_and_menu_capture_disable_native_caption_actions() {
        let point = Point::new(740.0, 18.0);
        let size = Size::new(800.0, 600.0);
        assert_eq!(hit(point, size, false, true, false), Hit::Client);
        assert_eq!(hit(point, size, false, false, true), Hit::Client);
        assert_eq!(
            hit(Point::new(2.0, 2.0), size, true, false, false),
            Hit::Client
        );
    }
}
