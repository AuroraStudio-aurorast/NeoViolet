use gpui::{Bounds, Pixels};

/// Structured terminal boundary calculations.
///
/// Encapsulates cell width, line height, and the available pixel bounds
/// so num_lines()/num_columns() are computed in one place with proper
/// float precision handling (inspired by Zed's `TerminalBounds`).
#[derive(Clone, Copy, Debug)]
pub struct TerminalBounds {
    /// Height of a single line in pixels.
    pub line_height: Pixels,
    /// Width of a single cell in pixels.
    pub cell_width: Pixels,
    /// Available pixel bounds for the terminal grid.
    pub bounds: Bounds<Pixels>,
}

impl TerminalBounds {
    /// Number of visible lines that fit in the bounds.
    pub fn num_lines(&self) -> usize {
        let h: f32 = self.bounds.size.height.into();
        let lh: f32 = self.line_height.into();
        (h / lh).floor().max(1.0) as usize
    }

    /// Number of visible columns that fit in the bounds.
    pub fn num_columns(&self) -> usize {
        let w: f32 = self.bounds.size.width.into();
        let cw: f32 = self.cell_width.into();
        (w / cw).floor().max(1.0) as usize
    }

    /// Convert a pixel position (relative to the terminal view origin) to grid coordinates.
    /// Returns (column, row) — both 0-based.
    pub fn grid_point(&self, pos: gpui::Point<Pixels>) -> (usize, usize) {
        let x: f32 = pos.x.into();
        let y: f32 = pos.y.into();
        let cw: f32 = self.cell_width.into();
        let lh: f32 = self.line_height.into();
        let col = ((x / cw).floor().max(0.0)) as usize;
        let row = ((y / lh).floor().max(0.0)) as usize;
        let max_col = self.num_columns().saturating_sub(1);
        let max_row = self.num_lines().saturating_sub(1);
        (col.min(max_col), row.min(max_row))
    }
}
