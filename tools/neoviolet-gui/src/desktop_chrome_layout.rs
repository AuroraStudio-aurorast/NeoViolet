//! Windowed traffic-light geometry; AppKit still owns button sizes and behavior.
#[derive(Clone, Copy)]
pub(super) struct Layout {
    gaps: Option<[f64; 2]>,
}

impl Layout {
    pub(super) fn capture(major: isize, frames: [(f64, f64); 3]) -> Option<Self> {
        if major < 26 {
            return Some(Self { gaps: None });
        }
        if frames
            .iter()
            .any(|(x, width)| !x.is_finite() || !width.is_finite() || *width <= 0.0)
        {
            return None;
        }
        let gaps = [
            frames[1].0 - frames[0].0 - frames[0].1,
            frames[2].0 - frames[1].0 - frames[1].1,
        ];
        gaps.iter()
            .all(|gap| gap.is_finite() && *gap > 0.0)
            .then_some(Self { gaps: Some(gaps) })
    }

    pub(super) fn modern(self) -> bool {
        self.gaps.is_some()
    }

    pub(super) fn origins(self, widths: [f64; 3]) -> [f64; 3] {
        match self.gaps {
            Some([first, second]) => [
                14.0,
                14.0 + widths[0] + first,
                14.0 + widths[0] + first + widths[1] + second,
            ],
            None => [14.0, 34.0, 54.0],
        }
    }

    pub(super) fn title_inset(self, widths: [f64; 3]) -> f32 {
        if self.gaps.is_none() {
            return 78.0;
        }
        (self.origins(widths)[2] + widths[2] + 12.0).max(78.0) as f32
    }
}

#[cfg(test)]
mod tests {
    #[allow(unused_imports)]
    // Also included by the native regression executable without a test harness.
    use super::*;

    #[test]
    fn older_systems_keep_button_positions_and_title_reservation() {
        let layout = Layout::capture(15, [(9.0, 14.0), (32.0, 14.0), (55.0, 14.0)]).unwrap();
        assert_eq!(layout.origins([14.0; 3]), [14.0, 34.0, 54.0]);
        assert_eq!(layout.origins([16.0; 3]), [14.0, 34.0, 54.0]);
        assert_eq!(layout.title_inset([16.0; 3]), 78.0);
    }

    #[test]
    fn modern_systems_keep_native_gaps_when_button_sizes_change() {
        for major in [26, 27] {
            let layout = Layout::capture(major, [(9.0, 14.0), (32.0, 14.0), (55.0, 14.0)]).unwrap();
            assert_eq!(layout.origins([14.0; 3]), [14.0, 37.0, 60.0]);
            assert_eq!(layout.title_inset([14.0; 3]), 86.0);
            assert_eq!(layout.origins([18.0, 16.0, 20.0]), [14.0, 41.0, 66.0]);
            assert_eq!(layout.title_inset([18.0, 16.0, 20.0]), 98.0);
        }
    }

    #[test]
    fn incomplete_or_overlapping_native_layout_is_not_saved() {
        assert!(Layout::capture(27, [(0.0, 0.0); 3]).is_none());
        assert!(Layout::capture(27, [(9.0, 14.0), (9.0, 14.0), (55.0, 14.0)]).is_none());
        assert!(Layout::capture(27, [(f64::NAN, 14.0), (32.0, 14.0), (55.0, 14.0)]).is_none());
    }
}
