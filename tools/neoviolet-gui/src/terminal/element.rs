//! Terminal GPUI Element — custom `Element` implementation that renders
//! alacritty_terminal cells directly via `window.paint_quad` and batched
//! text shaping. Also contains the `InputHandler` for IME support.
//!
//! Extracted from `app.rs` to keep the file sizes manageable.

use std::ops::Range;

use alacritty_terminal::{
    term::cell::Flags,
    vte::ansi::{Color as AnsiColor, CursorShape, NamedColor},
};
use gpui::{
    App, Bounds, Element, ElementId, Entity, FocusHandle, Font, FontStyle, FontWeight,
    GlobalElementId, Hsla, InspectorElementId, IntoElement, LayoutId, Pixels, Point, Rgba,
    SharedString, StrikethroughStyle, Style, TextRun, TextStyle, UnderlineStyle, UTF16Selection,
    Window, fill, point, px, rgb, size,
};
use yororen_ui::theme::ActiveTheme as _;

use crate::app::TerminalApp;
use crate::terminal::{
    custom_blocks::{is_custom_block_supported, paint_custom_block},
    RenderSnapshot, ViewportSelection,
};

// ── Metrics & layout types ──

#[derive(Clone, Copy)]
struct TerminalMetrics {
    cell_width: Pixels,
    line_height: Pixels,
}

#[derive(Clone)]
struct LayoutRect {
    row: i32,
    col: i32,
    cells: usize,
    color: Hsla,
}

impl LayoutRect {
    fn paint(&self, origin: Point<Pixels>, metrics: TerminalMetrics, window: &mut Window) {
        let position = point(
            origin.x + metrics.cell_width * self.col as f32,
            origin.y + metrics.line_height * self.row as f32,
        );
        let s = size(metrics.cell_width * self.cells as f32, metrics.line_height);
        window.paint_quad(fill(Bounds::new(position, s), self.color));
    }
}

#[derive(Clone)]
struct BatchedTextRun {
    row: i32,
    col: i32,
    cell_count: usize,
    text: String,
    style: TextRun,
    font_size: Pixels,
}

impl BatchedTextRun {
    fn new(row: i32, col: i32, ch: char, style: TextRun, font_size: Pixels) -> Self {
        Self { row, col, cell_count: 1, text: ch.to_string(), style, font_size }
    }

    fn can_append(&self, other: &TextRun, row: i32, col: i32) -> bool {
        self.row == row
            && self.col + self.cell_count as i32 == col
            && self.style.font == other.font
            && self.style.color == other.color
            && self.style.background_color == other.background_color
            && self.style.underline == other.underline
            && self.style.strikethrough == other.strikethrough
    }

    fn append(&mut self, ch: char, zerowidth: Option<&[char]>) {
        self.text.push(ch);
        self.cell_count += 1;
        self.style.len += ch.len_utf8();
        if let Some(chars) = zerowidth {
            for c in chars {
                self.text.push(*c);
                self.style.len += c.len_utf8();
            }
        }
    }

    fn paint(&self, origin: Point<Pixels>, metrics: TerminalMetrics, window: &mut Window, cx: &mut App) {
        let pos = point(
            origin.x + metrics.cell_width * self.col as f32,
            origin.y + metrics.line_height * self.row as f32,
        );
        window
            .text_system()
            .shape_line(
                self.text.clone().into(),
                self.font_size,
                std::slice::from_ref(&self.style),
                Some(metrics.cell_width),
            )
            .paint(pos, metrics.line_height, window, cx)
            .ok();
    }
}

#[derive(Clone, Copy)]
struct CursorLayout {
    row: usize,
    col: usize,
    shape: CursorShape,
    color: Hsla,
}

#[derive(Clone)]
struct LayoutCustomBlock {
    c: char,
    row: i32,
    col: i32,
    cells: usize,
    color: Hsla,
}

// ── TerminalElement ──

pub struct TerminalElement {
    snapshot: RenderSnapshot,
    marked_text: Option<String>,
    font_family: SharedString,
    font_size: Pixels,
    line_height: Pixels,
    cell_width: Pixels,
    view: Entity<TerminalApp>,
    focus_handle: FocusHandle,
}

pub struct PrepaintState {
    bounds: Bounds<Pixels>,
    metrics: TerminalMetrics,
    rects: Vec<LayoutRect>,
    runs: Vec<BatchedTextRun>,
    custom_blocks: Vec<LayoutCustomBlock>,
    cursor: Option<CursorLayout>,
}

// ── InputHandler ──

struct TerminalInputHandler {
    view: Entity<TerminalApp>,
    element_bounds: Bounds<Pixels>,
    cell_width: f32,
    line_height: f32,
}

impl gpui::InputHandler for TerminalInputHandler {
    fn selected_text_range(
        &mut self, _ignore_disabled_input: bool, _window: &mut Window, cx: &mut App,
    ) -> Option<UTF16Selection> {
        self.view.read(cx).terminal_accepts_text_input().then_some(UTF16Selection {
            range: 0..0,
            reversed: false,
        })
    }

    fn marked_text_range(&mut self, _window: &mut Window, cx: &mut App) -> Option<Range<usize>> {
        self.view.read(cx).terminal_marked_text_range()
    }

    fn text_for_range(
        &mut self, _range_utf16: Range<usize>, _adjusted_range: &mut Option<Range<usize>>,
        _window: &mut Window, _cx: &mut App,
    ) -> Option<String> {
        None
    }

    fn replace_text_in_range(
        &mut self, _replacement_range: Option<Range<usize>>, text: &str,
        window: &mut Window, cx: &mut App,
    ) {
        self.view.update(cx, |view, cx| view.commit_terminal_ime_text(text, window, cx));
    }

    fn replace_and_mark_text_in_range(
        &mut self, _range_utf16: Option<Range<usize>>, new_text: &str,
        _new_selected_range: Option<Range<usize>>, window: &mut Window, cx: &mut App,
    ) {
        self.view.update(cx, |view, cx| {
            view.set_terminal_marked_text(new_text.to_string(), window, cx);
        });
    }

    fn unmark_text(&mut self, window: &mut Window, cx: &mut App) {
        self.view.update(cx, |view, cx| view.clear_terminal_marked_text(window, cx));
    }

    fn bounds_for_range(
        &mut self, range_utf16: Range<usize>, _window: &mut Window, cx: &mut App,
    ) -> Option<Bounds<Pixels>> {
        self.view.read(cx).terminal_ime_bounds_for_range(
            range_utf16, self.element_bounds, self.cell_width, self.line_height,
        )
    }

    fn character_index_for_point(
        &mut self, _point: Point<Pixels>, _window: &mut Window, _cx: &mut App,
    ) -> Option<usize> {
        None
    }

    fn accepts_text_input(&mut self, _window: &mut Window, cx: &mut App) -> bool {
        self.view.read(cx).terminal_accepts_text_input()
    }

    fn apple_press_and_hold_enabled(&mut self) -> bool {
        false
    }
}

// ── TerminalElement impl ──

impl TerminalElement {
    pub fn new(
        snapshot: RenderSnapshot,
        marked_text: Option<String>,
        font_family: SharedString,
        font_size: Pixels,
        line_height: Pixels,
        cell_width: Pixels,
        view: Entity<TerminalApp>,
        focus_handle: FocusHandle,
    ) -> Self {
        Self { snapshot, marked_text, font_family, font_size, line_height, cell_width, view, focus_handle }
    }

    fn base_text_style(&self, cx: &App) -> TextStyle {
        TextStyle {
            color: cx.theme().content.primary,
            font_family: self.font_family.clone(),
            font_size: self.font_size.into(),
            line_height: self.line_height.into(),
            ..Default::default()
        }
    }

    fn cell_run_style(&self, cell: &alacritty_terminal::term::cell::Cell, cx: &App) -> TextRun {
        let mut fg = color_to_hsla(cell.fg, true, cx);
        let mut bg = color_to_hsla(cell.bg, false, cx);
        if cell.flags.contains(Flags::INVERSE) {
            std::mem::swap(&mut fg, &mut bg);
        }
        if cell.flags.contains(Flags::DIM) {
            fg.a *= 0.7;
        }

        let underline = cell.flags.intersects(Flags::ALL_UNDERLINES).then(|| UnderlineStyle {
            color: Some(fg),
            thickness: px(1.0),
            wavy: cell.flags.contains(Flags::UNDERCURL),
        });
        let strikethrough = cell.flags.contains(Flags::STRIKEOUT).then(|| StrikethroughStyle {
            color: Some(fg),
            thickness: px(1.0),
        });

        let weight = if cell.flags.intersects(Flags::BOLD) { FontWeight::BOLD } else { FontWeight::NORMAL };
        let style = if cell.flags.intersects(Flags::ITALIC) { FontStyle::Italic } else { FontStyle::Normal };

        TextRun {
            len: cell.c.len_utf8(),
            color: fg,
            background_color: None,
            font: Font { family: self.font_family.clone(), weight, style, ..Font::default() },
            underline,
            strikethrough,
        }
    }

    fn layout_grid(&self, cx: &App) -> (Vec<LayoutRect>, Vec<BatchedTextRun>, Vec<LayoutCustomBlock>) {
        let mut rects = Vec::new();
        let mut runs = Vec::new();
        let mut custom_blocks = Vec::new();
        let mut current_run: Option<BatchedTextRun> = None;

        for render_cell in &self.snapshot.cells {
            let cell = &render_cell.cell;
            if cell.flags.intersects(Flags::HIDDEN | Flags::WIDE_CHAR_SPACER | Flags::LEADING_WIDE_CHAR_SPACER) {
                continue;
            }

            let selected = self.snapshot.selection.is_some_and(|selection| {
                selection_contains(selection, render_cell.row, render_cell.col)
            });
            let bg = color_to_hsla(cell.bg, false, cx);
            if selected || !is_default_bg(cell.bg) {
                rects.push(LayoutRect {
                    row: render_cell.row,
                    col: render_cell.col,
                    cells: if cell.flags.contains(Flags::WIDE_CHAR) { 2 } else { 1 },
                    color: if selected {
                        cx.theme().content.primary
                    } else if cell.flags.contains(Flags::INVERSE) {
                        color_to_hsla(cell.fg, true, cx)
                    } else {
                        bg
                    },
                });
            }

            if is_blank(cell) {
                if let Some(run) = current_run.take() { runs.push(run); }
                continue;
            }

            let style = self.cell_run_style(cell, cx);

            if is_custom_block_supported(cell.c) {
                if let Some(run) = current_run.take() { runs.push(run); }
                custom_blocks.push(LayoutCustomBlock {
                    c: cell.c,
                    row: render_cell.row,
                    col: render_cell.col,
                    cells: if cell.flags.contains(Flags::WIDE_CHAR) { 2 } else { 1 },
                    color: style.color,
                });
                continue;
            }

            if let Some(run) = current_run.as_mut()
                && run.can_append(&style, render_cell.row, render_cell.col)
            {
                run.append(cell.c, cell.zerowidth());
                continue;
            }

            if let Some(run) = current_run.take() { runs.push(run); }

            let mut run = BatchedTextRun::new(render_cell.row, render_cell.col, cell.c, style, self.font_size);
            if let Some(chars) = cell.zerowidth() {
                for ch in chars {
                    run.text.push(*ch);
                    run.style.len += ch.len_utf8();
                }
            }
            current_run = Some(run);
        }

        if let Some(run) = current_run { runs.push(run); }

        (merge_rects(rects), runs, custom_blocks)
    }

    fn cursor_layout(&self, cx: &App) -> Option<CursorLayout> {
        self.snapshot.cursor.map(|cursor| CursorLayout {
            row: cursor.row,
            col: cursor.col,
            shape: cursor.shape,
            color: cx.theme().content.primary,
        })
    }
}

impl IntoElement for TerminalElement {
    type Element = Self;
    fn into_element(self) -> Self::Element { self }
}

impl Element for TerminalElement {
    type RequestLayoutState = ();
    type PrepaintState = PrepaintState;

    fn id(&self) -> Option<ElementId> { None }

    fn source_location(&self) -> Option<&'static std::panic::Location<'static>> { None }

    fn request_layout(
        &mut self, _id: Option<&GlobalElementId>, _inspector_id: Option<&InspectorElementId>,
        window: &mut Window, cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        let mut style = Style::default();
        style.size.width = gpui::relative(1.).into();
        style.size.height = gpui::relative(1.).into();
        (window.request_layout(style, None, cx), ())
    }

    fn prepaint(
        &mut self, _id: Option<&GlobalElementId>, _inspector_id: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>, _request_layout: &mut Self::RequestLayoutState,
        _window: &mut Window, cx: &mut App,
    ) -> Self::PrepaintState {
        let _ = self.base_text_style(cx);
        let (rects, runs, custom_blocks) = self.layout_grid(cx);

        self.view.update(cx, |view, _cx| {
            view.terminal_bounds = bounds;
            let cell_width = view.terminal_cell_width();
            let line_height = view.terminal_line_height();
            let cols = ((f32::from(bounds.size.width) / cell_width).floor() as u16).max(1);
            let rows = ((f32::from(bounds.size.height) / line_height).floor() as u16).max(1);
            view.pending_resize = Some((cols, rows));
            view.apply_pending_resize();
        });

        self.view.update(cx, |view, _cx| {
            self.snapshot = view.tab.render_snapshot();
        });

        PrepaintState {
            bounds,
            metrics: TerminalMetrics { cell_width: self.cell_width, line_height: self.line_height },
            rects,
            runs,
            custom_blocks,
            cursor: self.cursor_layout(cx),
        }
    }

    fn paint(
        &mut self, _id: Option<&GlobalElementId>, _inspector_id: Option<&InspectorElementId>,
        _bounds: Bounds<Pixels>, _request_layout: &mut Self::RequestLayoutState,
        prepaint: &mut Self::PrepaintState, window: &mut Window, cx: &mut App,
    ) {
        for rect in &prepaint.rects {
            rect.paint(prepaint.bounds.origin, prepaint.metrics, window);
        }
        for run in &prepaint.runs {
            run.paint(prepaint.bounds.origin, prepaint.metrics, window, cx);
        }
        for block in &prepaint.custom_blocks {
            let x = f32::from(prepaint.bounds.origin.x)
                + block.col as f32 * f32::from(prepaint.metrics.cell_width);
            let y = f32::from(prepaint.bounds.origin.y)
                + block.row as f32 * f32::from(prepaint.metrics.line_height);
            paint_custom_block(
                window, block.c, x, y,
                f32::from(prepaint.metrics.cell_width) * block.cells as f32,
                f32::from(prepaint.metrics.line_height),
                block.color,
            );
        }

        window.handle_input(
            &self.focus_handle,
            TerminalInputHandler {
                view: self.view.clone(),
                element_bounds: prepaint.bounds,
                cell_width: f32::from(prepaint.metrics.cell_width),
                line_height: f32::from(prepaint.metrics.line_height),
            },
            cx,
        );

        if let Some(marked_text) = self.marked_text.as_ref().filter(|text| !text.is_empty()) {
            if let Some(cursor) = prepaint.cursor {
                let pos = point(
                    prepaint.bounds.origin.x + prepaint.metrics.cell_width * cursor.col as f32,
                    prepaint.bounds.origin.y + prepaint.metrics.line_height * cursor.row as f32,
                );
                let mut base_style = self.base_text_style(cx);
                base_style.underline = Some(UnderlineStyle {
                    color: Some(base_style.color), thickness: px(1.0), wavy: false,
                });
                let shaped = window.text_system().shape_line(
                    marked_text.clone().into(),
                    self.font_size,
                    &[TextRun {
                        len: marked_text.len(),
                        font: Font { family: self.font_family.clone(), ..Font::default() },
                        color: base_style.color,
                        underline: base_style.underline,
                        ..Default::default()
                    }],
                    None,
                );
                let bg_bounds = Bounds::new(pos, size(shaped.width, prepaint.metrics.line_height));
                window.paint_quad(fill(bg_bounds, cx.theme().surface.canvas));
                shaped.paint(pos, prepaint.metrics.line_height, window, cx).ok();
            }
        }

        if let Some(cursor) = prepaint.cursor {
            if self.marked_text.as_ref().is_some_and(|text| !text.is_empty()) {
                return;
            }
            let x = prepaint.bounds.origin.x + prepaint.metrics.cell_width * cursor.col as f32;
            let y = prepaint.bounds.origin.y + prepaint.metrics.line_height * cursor.row as f32;
            match cursor.shape {
                CursorShape::Hidden => {}
                CursorShape::Beam => {
                    window.paint_quad(fill(
                        Bounds::new(point(x, y), size(px(2.), prepaint.metrics.line_height)),
                        cursor.color,
                    ));
                }
                CursorShape::Underline => {
                    window.paint_quad(fill(
                        Bounds::new(
                            point(x, y + prepaint.metrics.line_height - px(2.)),
                            size(prepaint.metrics.cell_width, px(2.)),
                        ),
                        cursor.color,
                    ));
                }
                CursorShape::Block | CursorShape::HollowBlock => {
                    let alpha = if matches!(cursor.shape, CursorShape::HollowBlock) { 0.18 } else { 0.32 };
                    window.paint_quad(fill(
                        Bounds::new(point(x, y), size(prepaint.metrics.cell_width, prepaint.metrics.line_height)),
                        cursor.color.opacity(alpha),
                    ));
                }
            }
        }
    }
}

// ── Helper functions ──

fn merge_rects(mut rects: Vec<LayoutRect>) -> Vec<LayoutRect> {
    rects.sort_by_key(|rect| (rect.row, rect.col));
    let mut merged: Vec<LayoutRect> = Vec::with_capacity(rects.len());
    for rect in rects {
        if let Some(last) = merged.last_mut()
            && last.row == rect.row
            && last.color == rect.color
            && last.col + last.cells as i32 == rect.col
        {
            last.cells += rect.cells;
            continue;
        }
        merged.push(rect);
    }
    merged
}

fn selection_contains(selection: ViewportSelection, row: i32, col: i32) -> bool {
    let row = row.max(0) as usize;
    let col = col.max(0) as usize;
    if row < selection.start_row || row > selection.end_row { return false; }
    if selection.is_block { return col >= selection.start_col && col <= selection.end_col; }
    let after_start = row > selection.start_row || col >= selection.start_col;
    let before_end = row < selection.end_row || col <= selection.end_col;
    after_start && before_end
}

fn is_blank(cell: &alacritty_terminal::term::cell::Cell) -> bool {
    cell.c == ' '
        && cell.zerowidth().is_none()
        && !cell.flags.intersects(Flags::ALL_UNDERLINES | Flags::STRIKEOUT)
}

fn is_default_bg(color: alacritty_terminal::vte::ansi::Color) -> bool {
    matches!(color, alacritty_terminal::vte::ansi::Color::Named(NamedColor::Background))
}

fn color_to_hsla(color: alacritty_terminal::vte::ansi::Color, foreground: bool, cx: &App) -> Hsla {
    match color {
        AnsiColor::Spec(rgb) => Hsla::from(Rgba { r: rgb.r as f32 / 255.0, g: rgb.g as f32 / 255.0, b: rgb.b as f32 / 255.0, a: 1.0 }),
        AnsiColor::Indexed(index) => ansi_index_color(index, cx),
        AnsiColor::Named(named) => named_color(named, foreground, cx),
    }
}

fn ansi_index_color(index: u8, _cx: &App) -> Hsla {
    const ANSI_16: [u32; 16] = [
        0x1f2430, 0xff5c57, 0x5af78e, 0xf3f99d, 0x57c7ff, 0xff6ac1, 0x9aedfe, 0xf1f1f0,
        0x686868, 0xff5c57, 0x5af78e, 0xf3f99d, 0x57c7ff, 0xff6ac1, 0x9aedfe, 0xffffff,
    ];
    if (index as usize) < ANSI_16.len() {
        return Hsla::from(rgb(ANSI_16[index as usize]));
    }
    if index >= 232 {
        let gray = 8 + (index - 232) * 10;
        return Hsla::from(Rgba { r: gray as f32 / 255.0, g: gray as f32 / 255.0, b: gray as f32 / 255.0, a: 1.0 });
    }
    let i = index - 16;
    let r = i / 36;
    let g = (i % 36) / 6;
    let b = i % 6;
    let conv = |v: u8| if v == 0 { 0 } else { 55 + v * 40 };
    Hsla::from(Rgba { r: conv(r) as f32 / 255.0, g: conv(g) as f32 / 255.0, b: conv(b) as f32 / 255.0, a: 1.0 })
}

fn named_color(named: NamedColor, _foreground: bool, cx: &App) -> Hsla {
    match named {
        NamedColor::Foreground => cx.theme().content.primary,
        NamedColor::Background => cx.theme().surface.canvas,
        NamedColor::Black => Hsla::from(rgb(0x1f2430)),
        NamedColor::Red => Hsla::from(rgb(0xff5c57)),
        NamedColor::Green => Hsla::from(rgb(0x5af78e)),
        NamedColor::Yellow => Hsla::from(rgb(0xf3f99d)),
        NamedColor::Blue => Hsla::from(rgb(0x57c7ff)),
        NamedColor::Magenta => Hsla::from(rgb(0xff6ac1)),
        NamedColor::Cyan => Hsla::from(rgb(0x9aedfe)),
        NamedColor::White => Hsla::from(rgb(0xf1f1f0)),
        NamedColor::BrightBlack => Hsla::from(rgb(0x686868)),
        NamedColor::BrightRed => Hsla::from(rgb(0xff5c57)),
        NamedColor::BrightGreen => Hsla::from(rgb(0x5af78e)),
        NamedColor::BrightYellow => Hsla::from(rgb(0xf3f99d)),
        NamedColor::BrightBlue => Hsla::from(rgb(0x57c7ff)),
        NamedColor::BrightMagenta => Hsla::from(rgb(0xff6ac1)),
        NamedColor::BrightCyan => Hsla::from(rgb(0x9aedfe)),
        NamedColor::BrightWhite => Hsla::from(rgb(0xffffff)),
        NamedColor::Cursor => cx.theme().content.primary,
        NamedColor::DimForeground => cx.theme().content.tertiary,
        NamedColor::BrightForeground => cx.theme().content.primary,
        NamedColor::DimBlack => Hsla::from(rgb(0x3b4252)),
        NamedColor::DimRed => Hsla::from(rgb(0xbf616a)),
        NamedColor::DimGreen => Hsla::from(rgb(0xa3be8c)),
        NamedColor::DimYellow => Hsla::from(rgb(0xebcb8b)),
        NamedColor::DimBlue => Hsla::from(rgb(0x81a1c1)),
        NamedColor::DimMagenta => Hsla::from(rgb(0xb48ead)),
        NamedColor::DimCyan => Hsla::from(rgb(0x88c0d0)),
        NamedColor::DimWhite => Hsla::from(rgb(0xe5e9f0)),
    }
}
