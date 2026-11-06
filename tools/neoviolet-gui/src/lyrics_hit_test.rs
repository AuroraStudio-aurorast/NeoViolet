//! Hit-test the actual rendered alpha, including gaps between glyphs and toolbar icons.
use iced::{Point, window};

#[derive(Debug, Clone)]
pub struct Mask(pub window::Screenshot);

impl Mask {
    pub fn contains(&self, point: Point) -> bool {
        let image = &self.0;
        if !point.x.is_finite()
            || !point.y.is_finite()
            || point.x < 0.0
            || point.y < 0.0
            || !image.scale_factor.is_finite()
            || image.scale_factor <= 0.0
        {
            return false;
        }
        let x = (point.x * image.scale_factor).floor() as u32;
        let y = (point.y * image.scale_factor).floor() as u32;
        if x >= image.size.width || y >= image.size.height {
            return false;
        }
        let index = (u64::from(y) * u64::from(image.size.width) + u64::from(x)) * 4 + 3;
        usize::try_from(index)
            .ok()
            .and_then(|index| image.rgba.get(index))
            .is_some_and(|alpha| *alpha >= 8)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn transparent_pixels_and_glyph_gaps_pass_through_at_retina_scale() {
        let mask = Mask(window::Screenshot::new(
            vec![
                255, 255, 255, 0, 255, 255, 255, 255, 255, 255, 255, 0, 255, 255, 255, 0, 255, 255,
                255, 0, 255, 255, 255, 100,
            ],
            iced::Size::new(3, 2),
            2.0,
        ));
        assert!(!mask.contains(Point::new(0.1, 0.1)));
        assert!(mask.contains(Point::new(0.6, 0.1)));
        assert!(!mask.contains(Point::new(0.6, 0.6)));
        assert!(mask.contains(Point::new(1.1, 0.6)));
        for point in [
            Point::new(-0.1, 0.0),
            Point::new(1.5, 0.0),
            Point::new(0.0, 1.0),
            Point::new(f32::NAN, 0.0),
        ] {
            assert!(!mask.contains(point));
        }
    }
}
