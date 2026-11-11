package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
)

type pageSettingRow struct {
	label string
	field int
}

func (p *pageState) settingRows() ([]pageSettingRow, int) {
	rows := make([]pageSettingRow, 0, len(p.settings)+len(pageSettingGroups))
	selected := 0
	previous := -1
	for i, f := range p.settings {
		if previous != f.group {
			rows = append(rows, pageSettingRow{label: pageSettingGroups[f.group], field: -1})
			previous = f.group
		}
		if i == p.cursors[3] {
			selected = len(rows)
		}
		rows = append(rows, pageSettingRow{label: f.label, field: i})
	}
	return rows, selected
}

// Keep the preceding group title visible when its first field reaches the
// viewport's upper edge. Headers remain context, never selectable items.
func (p *pageState) settingsWindowStart(rows []pageSettingRow, selected, capacity int) int {
	start := p.viewports.settings.start(selected, len(rows), capacity)
	if capacity > 1 && selected > 0 && selected < len(rows) && rows[selected-1].field < 0 && start == selected {
		start--
	}
	return start
}

func (p *pageState) renderSettings(c *pageCanvas, focused bool) {
	w, h := c.width, len(c.rows)
	status := "Saved configuration"
	if p.dirty {
		status = "Unsaved changes"
	}
	c.pair(0, 1, w-2, "Settings", status)
	current := p.settings[p.cursors[3]]
	c.dim(1, 1, fmt.Sprintf("%d / %d  %s", current.group+1, len(pageSettingGroups), pageSettingGroups[current.group]))
	c.rule(2, 1, w-2)
	rows, selected := p.settingRows()
	available := listCapacity(h, 3, 1, 1)
	start := p.settingsWindowStart(rows, selected, available)
	for i, row := range rows[start:min(len(rows), start+available)] {
		y := 3 + i
		if row.field < 0 {
			c.rule(y, 1, w-2)
			c.label(y, 1, row.label+" ")
			continue
		}
		f := p.settings[row.field]
		marker := "  "
		if row.field == p.cursors[3] {
			marker = "▸ "
		}
		c.pair(y, 1, w-2, marker+f.label, truncateLine(f.value, max(0, w*43/100)))
		if row.field == p.cursors[3] && focused {
			c.highlight(y, 1, w-2)
		}
	}
	c.dim(h-1, 1, "Enter Edit · s Save · 1-8 Group · r Default")
}

func (p *pageState) jumpGroup(group int) {
	for i, f := range p.settings {
		if f.group == group {
			p.cursors[3] = i
			return
		}
	}
}

func (p *pageState) editSetting() tea.Cmd {
	index := p.cursors[3]
	f := &p.settings[index]
	if f.kind == "folders" {
		return nil
	}
	if f.kind == "file" {
		return p.openEditor(f.label, "setting", f.value, index)
	}
	if f.kind != "" {
		p.picker, p.pickerField, p.pickerIndex = f.kind, index, 0
		p.pickerError = ""
		if f.kind == "formats" {
			p.pickerFormats = slices.Clone(p.formats)
			p.pickerEnabled = slices.Clone(p.enabled)
		}
		return nil
	}
	if len(f.options) > 0 {
		_ = p.setDraftField(index, f.options[(slices.Index(f.options, f.value)+1)%len(f.options)])
		return nil
	}
	return p.openEditor(f.label, "setting", f.value, index)
}

func (p *pageState) pickerItems() []string { return p.formats }

func (p *pageState) renderPicker(c *pageCanvas) {
	title := "Lyric format priority"
	c.label(0, 1, "Settings / "+title)
	c.dim(1, 1, "Select enabled formats in priority order")
	c.rule(2, 1, c.width-2)
	items := p.pickerItems()
	available := listCapacity(len(c.rows), 4, 1, 1)
	start := p.viewports.formats.start(p.pickerIndex, len(items), available)
	for i, item := range items[start:min(len(items), start+available)] {
		index := start + i
		marker := "  "
		if index == p.pickerIndex {
			marker = "▸ "
		}
		if p.picker == "formats" {
			enabled := "[ ]"
			if p.enabled[index] {
				enabled = "[x]"
			}
			item = fmt.Sprintf("%s %d. %s", enabled, index+1, item)
		}
		c.text(4+i, 1, marker+item)
		if index == p.pickerIndex {
			c.highlight(4+i, 1, c.width-2)
		}
	}
	hint := "Enter Choose sample · Esc Cancel"
	if p.picker == "formats" {
		hint = "Space Enable · J/K Order · Enter Confirm · Esc Cancel"
	}
	if p.pickerError != "" {
		hint = p.pickerError
	}
	c.dim(len(c.rows)-1, 1, hint)
}

func (p *pageState) updatePicker(k string) {
	items := p.pickerItems()
	switch k {
	case "esc", "ctrl+g":
		if p.picker == "formats" {
			p.formats, p.enabled = p.pickerFormats, p.pickerEnabled
		}
		p.pickerFormats, p.pickerEnabled = nil, nil
		p.picker = ""
	case "up", "k":
		p.pickerIndex = max(0, p.pickerIndex-1)
	case "down", "j":
		p.pickerIndex = min(len(items)-1, p.pickerIndex+1)
	case "space":
		if p.picker == "formats" {
			p.enabled[p.pickerIndex] = !p.enabled[p.pickerIndex]
		}
	case "J", "K":
		p.reorderFormat(k)
	case "enter":
		order := []string{}
		for i, name := range p.formats {
			if p.enabled[i] {
				order = append(order, name)
			}
		}
		if len(order) == 0 {
			p.pickerError = "Enable at least one format · Esc Cancel"
			return
		}
		p.setFormatDraft(order)
		p.pickerFormats, p.pickerEnabled = nil, nil
		p.picker = ""
	}
}

func (p *pageState) reorderFormat(k string) {
	if p.picker != "formats" {
		return
	}
	i := p.pickerIndex
	j := i + 1
	if k == "K" {
		j = i - 1
	}
	if j < 0 || j >= len(p.formats) {
		return
	}
	p.formats[i], p.formats[j] = p.formats[j], p.formats[i]
	p.enabled[i], p.enabled[j] = p.enabled[j], p.enabled[i]
	p.pickerIndex = j
}
