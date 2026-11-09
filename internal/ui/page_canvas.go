package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type pageSelection struct{ row, x, width int }

// pageCanvas writes into the content interior only. All coordinates are cells;
// ANSI-aware cuts keep long names and double-width characters inside the box.
type pageCanvas struct {
	width                 int
	rows                  []string
	normal, muted, accent lipgloss.Style
	selected              color.RGBA
	selections            []pageSelection
}

func newPageCanvas(width, height int, m *Model) *pageCanvas {
	palette := newInteractionPalette(m.Accent)
	c := &pageCanvas{
		width: max(0, width), rows: make([]string, max(0, height)),
		normal: lipgloss.NewStyle().Foreground(lipgloss.Color("#d9d9d9")),
		muted:  lipgloss.NewStyle().Foreground(lipgloss.Color("#999099")),
		accent: lipgloss.NewStyle().Foreground(palette.prompt), selected: palette.selected,
		selections: []pageSelection{},
	}
	for i := range c.rows {
		c.rows[i] = strings.Repeat(" ", c.width)
	}
	return c
}

func (c *pageCanvas) write(y, x int, text string, style lipgloss.Style) {
	if y < 0 || y >= len(c.rows) || x < 0 || x >= c.width {
		return
	}
	text = ansi.Truncate(strings.ReplaceAll(text, "\n", " "), c.width-x, "…")
	c.rows[y] = pageCellSlice(c.rows[y], 0, x) + style.Render(text) + pageCellSlice(c.rows[y], x+lipgloss.Width(text), c.width)
}

// A cut through half of a wide glyph is blank space, still occupying its cell.
func pageCellSlice(row string, start, end int) string {
	segment := ansi.Truncate(ansi.Cut(row, start, end), max(0, end-start), "")
	return segment + strings.Repeat(" ", max(0, end-start-lipgloss.Width(segment)))
}

func (c *pageCanvas) text(y, x int, s string) {
	c.write(y, x, s, c.normal)
}
func (c *pageCanvas) dim(y, x int, s string) {
	c.write(y, x, s, c.muted)
}
func (c *pageCanvas) label(y, x int, s string) {
	c.write(y, x, s, c.accent)
}
func (c *pageCanvas) rule(y, x, width int) {
	c.dim(y, x, strings.Repeat("─", max(0, min(width, c.width-x))))
}
func (c *pageCanvas) divider(x, y, height int) {
	for i := range max(0, height) {
		c.dim(y+i, x, "│")
	}
}
func (c *pageCanvas) pair(y, x, width int, left, right string) {
	right = ansi.Truncate(right, max(0, width-2), "…")
	c.text(y, x, ansi.Truncate(left, max(0, width-lipgloss.Width(right)-1), "…"))
	c.text(y, x+width-lipgloss.Width(right), right)
}
func (c *pageCanvas) highlight(y, x, width int) {
	c.selections = append(c.selections, pageSelection{row: y, x: x, width: width})
}
func (c *pageCanvas) String() string {
	for _, s := range c.selections {
		if s.row < 0 || s.row >= len(c.rows) || s.x < 0 || s.x >= c.width {
			continue
		}
		end := min(c.width, s.x+s.width)
		row := c.rows[s.row]
		c.rows[s.row] = pageCellSlice(row, 0, s.x) + paintBackground(pageCellSlice(row, s.x, end), []color.RGBA{c.selected}) + pageCellSlice(row, end, c.width)
	}
	return strings.Join(c.rows, "\n")
}

func renderTabPage(m *Model, plan layoutPlan) string {
	c := newPageCanvas(plan.ContentInnerW, plan.ContentInnerH, m)
	if m.pages == nil {
		return c.String()
	}
	p := m.pages
	if p.quitSettings {
		c.label(0, 1, "Unsaved settings")
		c.text(2, 1, "Save your changes before quitting?")
		c.dim(4, 1, "s Save and quit · d Discard and quit · Esc Cancel")
		return c.String()
	}
	if p.editor != nil {
		p.renderEditor(c)
		return c.String()
	}
	if p.picker != "" {
		p.renderPicker(c)
		return c.String()
	}
	switch m.UI.ActiveTab {
	case 0:
		p.renderHome(c, m.UI.Focus == FocusContent)
	case 1:
		p.renderPlaylists(c, m.UI.Focus == FocusContent)
	case 2:
		p.renderEffects(c, m.UI.Focus == FocusContent)
	case 3:
		p.renderSettings(c, m.UI.Focus == FocusContent)
	}
	return c.String()
}
