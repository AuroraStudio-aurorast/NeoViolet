package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

type pageEditor struct {
	input          textinput.Model
	title, purpose string
	field          int
}

// pageState keeps page interaction and an isolated configuration draft.
// Playback, library, queue and effects remain samples until their wiring batches.
type pageState struct {
	cursors                  [4]int
	scope                    int
	filter                   string
	saved, detail            bool
	collection               int
	queue                    []int
	removed, removedAt       int
	playing                  int
	effect                   int
	gains                    [5]float64
	preamp, pan              float64
	mono, swap, bypass       bool
	settings                 []pageSetting
	draft                    config.Config
	changed                  map[string]bool
	dirty                    bool
	quitSettings             bool
	editor                   *pageEditor
	picker                   string
	pickerIndex, pickerField int
	formats                  []string
	enabled                  []bool
	baseFormats              []string
	pickerError              string
	pickerFormats            []string
	pickerEnabled            []bool
}

func newPageState() *pageState {
	return &pageState{
		queue: []int{0, 1, 2, 3, 4, 5}, removed: -1, playing: 0, effect: 0, preamp: -3,
		settings: samplePageSettings(),
		formats:  []string{"embedded", "lrc", "spl", "ttml", "qrc", "yrc", "eslrc", "lys", "online"},
		enabled:  []bool{true, true, true, true, true, true, true, true, true},
	}
}

func (m *Model) pageNotice() {
	m.Info.Set("Preview only: sample content, not connected", max(90, m.Config.Error.Duration))
}

func (p *pageState) openEditor(title, purpose, value string, field int) tea.Cmd {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 512
	input.SetValue(value)
	p.editor = &pageEditor{input: input, title: title, purpose: purpose, field: field}
	return p.editor.input.Focus()
}

func (m *Model) pageOwnsInput() bool {
	return m.pages != nil && !m.animVisible() && m.UI.Mode == ModeNormal && m.UI.Focus == FocusContent &&
		(m.pages.editor != nil || m.pages.picker != "" || m.pages.quitSettings)
}

func (m *Model) updatePageInput(msg tea.KeyPressMsg) tea.Cmd {
	p := m.pages
	if p.quitSettings {
		return m.updateSettingsQuit(msg)
	}
	if m.configSave != nil {
		m.Info.Set("Saving configuration…", 90)
		return nil
	}
	if p.editor == nil {
		p.updatePicker(msg.String())
		return nil
	}
	switch msg.String() {
	case "esc", "ctrl+g":
		p.editor = nil
		return nil
	case "enter":
		editor := p.editor
		switch editor.purpose {
		case "filter":
			p.filter = editor.input.Value()
			p.cursors[0] = 0
		case "setting":
			if err := p.setDraftField(editor.field, editor.input.Value()); err != nil {
				m.Info.Visible = false
				m.Error.Set(err.Error(), max(90, m.Config.Error.Duration))
				return nil
			}
		default:
			m.pageNotice()
		}
		p.editor = nil
		return nil
	default:
		var cmd tea.Cmd
		p.editor.input, cmd = p.editor.input.Update(msg)
		return cmd
	}
}

func (m *Model) updatePageKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if m.pages == nil || m.animVisible() {
		return false, nil
	}
	p := m.pages
	k := msg.String()
	switch k {
	case "up", "k":
		p.move(m.UI.ActiveTab, -1)
		return true, nil
	case "down", "j":
		p.move(m.UI.ActiveTab, 1)
		return true, nil
	case "home", "g":
		p.move(m.UI.ActiveTab, -1000)
		return true, nil
	case "end", "G":
		p.move(m.UI.ActiveTab, 1000)
		return true, nil
	case "pgup":
		p.move(m.UI.ActiveTab, -5)
		return true, nil
	case "pgdown":
		p.move(m.UI.ActiveTab, 5)
		return true, nil
	case "enter":
		return true, m.activatePage()
	case "esc":
		p.detail = false
		return true, nil
	}
	switch m.UI.ActiveTab {
	case 0:
		switch k {
		case "left", "right":
			delta := 1
			if k == "left" {
				delta = -1
			}
			p.scope = (p.scope + len(pageScopes) + delta) % len(pageScopes)
			p.cursors[0] = 0
		case "f":
			return true, p.openEditor("Filter tracks", "filter", p.filter, 0)
		case "o":
			return true, p.openEditor("Open audio", "open", "/Music/song.flac", 0)
		case "a", "r":
			m.pageNotice()
		default:
			return false, nil
		}
	case 1:
		switch k {
		case "v":
			p.saved = !p.saved
			p.detail = false
			p.collection = 0
		case "d":
			p.removeEntry()
			m.pageNotice()
		case "u":
			p.restoreEntry()
			m.pageNotice()
		case "J", "K":
			p.reorder(k)
		case "s":
			return true, p.openEditor("Save as playlist", "playlist", "Evening queue", 0)
		case "a":
			m.pageNotice()
		default:
			return false, nil
		}
	case 2:
		switch k {
		case "left":
			p.move(2, -1)
		case "right":
			p.move(2, 1)
		case "+", "=":
			p.adjustEffect(1)
			m.pageNotice()
		case "-":
			p.adjustEffect(-1)
			m.pageNotice()
		case "r":
			p.resetEffect()
		default:
			return false, nil
		}
	case 3:
		if m.configSave != nil {
			m.Info.Set("Saving configuration…", 90)
			return true, nil
		}
		switch {
		case k == "s":
			return true, m.saveSettings(false)
		case k == "r":
			if err := p.resetSetting(p.cursors[3]); err != nil {
				m.Error.Set(err.Error(), 90)
			}
		case len(k) == 1 && k >= "1" && k <= "6":
			p.jumpGroup(int(k[0] - '1'))
		default:
			return false, nil
		}
	default:
		return false, nil
	}
	return true, nil
}

func (p *pageState) move(tab, delta int) {
	switch tab {
	case 0:
		p.cursors[0] = max(0, min(len(p.tracks())-1, p.cursors[0]+delta))
	case 1:
		if p.saved {
			p.collection = max(0, min(len(pageCollections)-1, p.collection+delta))
			return
		}
		p.cursors[1] = max(0, min(len(p.queue)-1, p.cursors[1]+delta))
	case 2:
		p.effect = max(0, min(9, p.effect+delta))
	case 3:
		p.cursors[3] = max(0, min(len(p.settings)-1, p.cursors[3]+delta))
	}
}

func (m *Model) activatePage() tea.Cmd {
	p := m.pages
	switch m.UI.ActiveTab {
	case 0:
		items := p.tracks()
		if len(items) > 0 {
			p.playing = items[min(p.cursors[0], len(items)-1)]
		}
	case 1:
		if p.saved {
			p.detail = true
			return nil
		}
		if len(p.queue) > 0 {
			p.playing = p.queue[min(p.cursors[1], len(p.queue)-1)]
		}
	case 2:
		if p.effect >= 6 && p.effect <= 7 || p.effect == 9 {
			p.adjustEffect(1)
		}
	case 3:
		if m.configSave != nil {
			return nil
		}
		if p.settings[p.cursors[3]].key == "" {
			m.Info.Set("Music folders will be connected with the library", 90)
			return nil
		}
		return p.editSetting()
	}
	m.pageNotice()
	return nil
}

func (p *pageState) removeEntry() {
	if p.saved || len(p.queue) == 0 {
		return
	}
	i := min(p.cursors[1], len(p.queue)-1)
	p.removed, p.removedAt = p.queue[i], i
	p.queue = append(p.queue[:i], p.queue[i+1:]...)
	p.cursors[1] = max(0, min(i, len(p.queue)-1))
}
func (p *pageState) restoreEntry() {
	if p.saved || p.removed < 0 {
		return
	}
	i := min(p.removedAt, len(p.queue))
	p.queue = append(p.queue, 0)
	copy(p.queue[i+1:], p.queue[i:])
	p.queue[i] = p.removed
	p.removed = -1
}
func (p *pageState) reorder(k string) {
	if p.saved || len(p.queue) == 0 {
		return
	}
	i := p.cursors[1]
	j := i + 1
	if k == "K" {
		j = i - 1
	}
	if j >= 0 && j < len(p.queue) {
		p.queue[i], p.queue[j] = p.queue[j], p.queue[i]
		p.cursors[1] = j
	}
}

func (p *pageState) renderEditor(c *pageCanvas) {
	e := p.editor
	c.label(0, 1, e.title)
	if e.purpose == "setting" {
		c.dim(1, 1, "Enter validates the draft; s saves after closing this editor")
	} else {
		c.dim(1, 1, "Sample input / not connected")
	}
	c.rule(2, 1, c.width-2)
	e.input.SetWidth(max(1, c.width-6))
	c.text(4, 2, e.input.View())
	c.dim(len(c.rows)-1, 1, "Enter Confirm · Esc Cancel")
}

func (p *pageState) tracks() []int {
	out := make([]int, 0, len(pageTracks))
	query := strings.ToLower(p.filter)
	for i, t := range pageTracks {
		matches := strings.Contains(strings.ToLower(t.title+" "+t.artist+" "+t.album), query)
		if matches && (p.scope == 0 || t.format == pageScopes[p.scope]) {
			out = append(out, i)
		}
	}
	return out
}

func pageCount(n int, unit string) string { return fmt.Sprintf("%d %s", n, unit) }
