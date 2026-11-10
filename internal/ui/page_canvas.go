package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

type pageSelection struct{ row, x, width int }
type pageCell struct {
	text  string
	ink   uint16
	width uint8
}
type pageInk struct {
	style uv.Style
	link  uv.Link
}

// pageCanvas stores cells, never prior ANSI output. Overwrites retain only the
// visible glyph/style; each row is serialized once after all drawing is done.
type pageCanvas struct {
	width                 int
	rows                  [][]pageCell
	inks                  []pageInk
	normal, muted, accent uint16
	selected              color.RGBA
	selections            []pageSelection
}

func newPageCanvas(width, height int, m *Model) *pageCanvas {
	width, height = max(0, width), max(0, height)
	palette := newInteractionPalette(m.Accent)
	c := &pageCanvas{width: width, rows: make([][]pageCell, height), selected: palette.selected}
	c.inks = []pageInk{{style: uv.Style{Fg: color.RGBA{R: 217, G: 217, B: 217, A: 255}}}, {style: uv.Style{Fg: color.RGBA{R: 153, G: 144, B: 153, A: 255}}}, {style: uv.Style{Fg: palette.prompt}}}
	c.normal, c.muted, c.accent = 0, 1, 2
	cells := make([]pageCell, width*height)
	for i := range cells {
		cells[i].width = 1
	}
	for y := range c.rows {
		c.rows[y] = cells[y*width : (y+1)*width]
	}
	return c
}
func (c *pageCanvas) write(y, x int, text string, ink uint16) {
	if y < 0 || y >= len(c.rows) || x < 0 || x >= c.width {
		return
	}
	text = ansi.Truncate(strings.ReplaceAll(text, "\n", " "), c.width-x, "…")
	if strings.ContainsRune(text, '\x1b') {
		lines := uv.NewStyledString(text).Lines(ansi.GraphemeWidth)
		if len(lines) == 0 {
			return
		}
		for _, cell := range lines[0] {
			if cell.Style.Fg == nil {
				cell.Style.Fg = c.inks[ink].style.Fg
			}
			candidate := pageInk{style: cell.Style, link: cell.Link}
			index := 0
			for ; index < len(c.inks); index++ {
				if c.inks[index].style.Equal(&candidate.style) && c.inks[index].link == candidate.link {
					break
				}
			}
			if index == len(c.inks) {
				c.inks = append(c.inks, candidate)
			}
			c.setCell(y, x, cell.Content, cell.Width, uint16(index))
			x += cell.Width
		}
		return
	}
	for len(text) > 0 && x < c.width {
		cluster, width := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		if len(cluster) == 0 {
			break
		}
		c.setCell(y, x, cluster, width, ink)
		x += width
		text = text[len(cluster):]
	}
}
func (c *pageCanvas) setCell(y, x int, text string, width int, ink uint16) {
	if x < 0 || x >= c.width || width <= 0 || width > 255 {
		return
	}
	row := c.rows[y]
	for position := x; position < min(c.width, x+width); position++ {
		start := position
		for start > 0 && row[start].width == 0 {
			start--
		}
		oldWidth := max(1, int(row[start].width))
		for i := start; i < min(c.width, start+oldWidth); i++ {
			row[i] = pageCell{width: 1, ink: row[start].ink}
		}
	}
	if x+width > c.width {
		return
	}
	row[x] = pageCell{text: text, ink: ink, width: uint8(width)}
	for i := 1; i < width; i++ {
		row[x+i] = pageCell{}
	}
}
func (c *pageCanvas) text(y, x int, s string)  { c.write(y, x, s, c.normal) }
func (c *pageCanvas) dim(y, x int, s string)   { c.write(y, x, s, c.muted) }
func (c *pageCanvas) label(y, x int, s string) { c.write(y, x, s, c.accent) }
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
	lines := make([]string, len(c.rows))
	line := uv.NewLine(c.width)
	for y, row := range c.rows {
		for x, cell := range row {
			if cell.width == 0 {
				line[x] = uv.Cell{}
				continue
			}
			ink := c.inks[cell.ink]
			text := cell.text
			if text == "" {
				text = " "
			}
			line[x] = uv.Cell{Content: text, Width: int(cell.width), Style: ink.style, Link: ink.link}
		}
		for _, selection := range c.selections {
			if selection.row != y {
				continue
			}
			for x := max(0, selection.x); x < min(c.width, selection.x+selection.width); x++ {
				line[x].Style.Bg = c.selected
			}
		}
		lines[y] = line.Render()
	}
	return strings.Join(lines, "\n")
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
	if p.deletePlaylist != "" {
		c.label(0, 1, "Delete saved playlist?")
		if list := m.selectedPlaylist(); list != nil {
			c.text(2, 1, browserDisplayText(list.Name))
		}
		c.dim(4, 1, "Audio files and the session queue stay intact")
		c.dim(len(c.rows)-1, 1, "y Delete playlist · Esc Cancel")
		return c.String()
	}
	if p.editor != nil {
		p.renderEditor(c)
		if m.playlists.save != nil {
			c.label(len(c.rows)-2, 1, "Saving playlist…")
		}
		return c.String()
	}
	if p.browser != nil {
		p.browser.render(c)
		return c.String()
	}
	if p.picker != "" {
		p.renderPicker(c)
		return c.String()
	}
	if p.rootEditor {
		p.renderLibraryRoots(c)
		return c.String()
	}
	switch m.UI.ActiveTab {
	case 0:
		m.renderHome(c, m.UI.Focus == FocusContent)
	case 1:
		m.renderPlaylists(c, m.UI.Focus == FocusContent)
	case 2:
		p.renderEffects(c, m.UI.Focus == FocusContent, m.effectsStatus())
	case 3:
		p.renderSettings(c, m.UI.Focus == FocusContent)
	}
	return c.String()
}
