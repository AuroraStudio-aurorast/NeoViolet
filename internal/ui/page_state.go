package ui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

type pageEditor struct {
	input          textinput.Model
	title, purpose string
	field          int
	playlistID     string
	tracks         []library.Track
}

// pageState keeps page interaction and an isolated configuration draft.
// Library, queue and saved playlist snapshots belong to Model. Effects are session values.
type pageState struct {
	viewports                pageViewports
	visible                  []library.Track
	scopes                   []string
	artists, albums          int
	rootEditor               bool
	rootCursor               int
	browser                  *fileBrowser
	cursors                  [4]int
	scope                    int
	filter                   string
	saved, detail            bool
	collection               int
	savedTrack               int
	deletePlaylist           string
	effect                   int
	effects                  audio.EffectsSettings
	settings                 []pageSetting
	draft                    config.Config
	baseline                 config.Config
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
		scopes: []string{"All tracks"}, visible: []library.Track{},
		effect:   0,
		settings: samplePageSettings(),
		formats:  []string{"embedded", "lrc", "spl", "ttml", "qrc", "yrc", "eslrc", "lys", "online"},
		enabled:  []bool{true, true, true, true, true, true, true, true, true},
	}
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
		(m.pages.editor != nil || m.pages.picker != "" || m.pages.quitSettings || m.pages.browser != nil || m.pages.rootEditor || m.pages.deletePlaylist != "")
}

func (m *Model) updatePageInput(msg tea.KeyPressMsg) tea.Cmd {
	p := m.pages
	if p.quitSettings {
		return m.updateSettingsQuit(msg)
	}
	if p.deletePlaylist != "" {
		return m.updatePlaylistDelete(msg)
	}
	if m.playlists.save != nil {
		m.Info.Set("Saving playlist…", 90)
		return nil
	}
	if m.configSave != nil {
		m.Info.Set("Saving configuration…", 90)
		return nil
	}
	if p.editor == nil && p.browser != nil {
		return m.updateFileBrowser(msg)
	}
	if p.editor == nil && p.rootEditor {
		return m.updateLibraryRoots(msg)
	}
	if p.editor == nil {
		p.updatePicker(msg.String())
		return nil
	}
	switch msg.String() {
	case "esc", "ctrl+g":
		if p.browser != nil {
			p.browser.generation++
			p.browser.loading = false
		}
		p.editor = nil
		return nil
	case "enter":
		editor := p.editor
		switch editor.purpose {
		case "playlist_create", "playlist_queue", "playlist_rename":
			return m.submitPlaylistName()
		case "filter":
			p.filter = editor.input.Value()
			p.cursors[0] = 0
			m.refreshLibraryView()
		case "browser_location":
			if p.browser != nil && p.browser.loading {
				return nil
			}
			return m.fileBrowserLocation(editor.input.Value())
		case "open":
			value := editor.input.Value()
			return m.openFileBrowser("audio", -1, value)
		case "setting":
			if p.settings[editor.field].kind == "file" && editor.input.Value() != "" {
				return m.openFileBrowser(settingFilePurpose(p.settings[editor.field].key), editor.field, editor.input.Value())
			}
			if err := p.setDraftField(editor.field, editor.input.Value()); err != nil {
				m.Info.Visible = false
				m.Error.Set(err.Error(), max(90, m.Config.Error.Duration))
				return nil
			}
		default:
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
		m.movePageCursor(m.UI.ActiveTab, -1)
		return true, nil
	case "down", "j":
		m.movePageCursor(m.UI.ActiveTab, 1)
		return true, nil
	case "home", "g":
		m.movePageCursor(m.UI.ActiveTab, -m.pageItemCount())
		return true, nil
	case "end", "G":
		m.movePageCursor(m.UI.ActiveTab, m.pageItemCount())
		return true, nil
	case "pgup":
		m.movePageCursor(m.UI.ActiveTab, -5)
		return true, nil
	case "pgdown":
		m.movePageCursor(m.UI.ActiveTab, 5)
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
			p.scope = (p.scope + len(p.scopes) + delta) % len(p.scopes)
			p.cursors[0] = 0
			m.refreshLibraryView()
		case "f":
			return true, p.openEditor("Filter tracks", "filter", p.filter, 0)
		case "o":
			return true, m.openFilePicker("audio", -1, "")
		case "O":
			return true, p.openEditor("Open audio path or URL", "open", "", -1)
		case "r":
			return true, m.rescanLibrary()
		case "a":
			if len(p.visible) > 0 {
				m.appendQueue(p.visible[p.cursors[0]])
			}
		default:
			return false, nil
		}
	case 1:
		return m.updatePlaylistKey(k)
	case 2:
		switch k {
		case "left":
			m.movePageCursor(2, -1)
		case "right":
			m.movePageCursor(2, 1)
		case "+", "=":
			m.adjustEffect(1)
		case "-":
			m.adjustEffect(-1)
		case "r":
			m.resetEffect()
		default:
			return false, nil
		}
	case 3:
		if m.configSave != nil {
			m.Info.Set("Saving configuration…", 90)
			return true, nil
		}
		switch {
		case k == "e" && p.settings[p.cursors[3]].kind == "file":
			return true, p.openEditor(p.settings[p.cursors[3]].label+" path", "setting", p.settings[p.cursors[3]].value, p.cursors[3])
		case k == "x" && p.settings[p.cursors[3]].kind == "file":
			_ = p.setDraftField(p.cursors[3], "")
		case k == "s":
			return true, m.saveSettings(false)
		case k == "r":
			if err := p.resetSetting(p.cursors[3]); err != nil {
				m.Error.Set(err.Error(), 90)
			}
		case len(k) == 1 && k >= "1" && k <= "8":
			p.jumpGroup(int(k[0] - '1'))
		default:
			return false, nil
		}
	default:
		return false, nil
	}
	return true, nil
}

func (m *Model) pageItemCount() int {
	p := m.pages
	switch m.UI.ActiveTab {
	case 0:
		return len(p.visible)
	case 1:
		if !p.saved {
			return len(m.session.entries)
		}
		if !p.detail {
			return len(m.playlists.lists)
		}
		if list := m.selectedPlaylist(); list != nil {
			return len(list.Tracks)
		}
	case 2:
		return len(effectLabels)
	case 3:
		return len(p.settings)
	}
	return 0
}

func (m *Model) movePageCursor(tab, delta int) {
	p := m.pages
	switch tab {
	case 0:
		p.cursors[0] = max(0, min(len(p.visible)-1, p.cursors[0]+delta))
	case 1:
		if p.saved {
			if p.detail {
				if list := m.selectedPlaylist(); list != nil {
					p.savedTrack = max(0, min(len(list.Tracks)-1, p.savedTrack+delta))
				}
			} else {
				p.collection = max(0, min(len(m.playlists.lists)-1, p.collection+delta))
				p.savedTrack = 0
			}
			return
		}
		p.cursors[1] = max(0, min(len(m.session.entries)-1, p.cursors[1]+delta))
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
		return m.playLibrarySelection()
	case 1:
		if p.saved {
			if !m.playlistsReady() {
				return nil
			}
			if p.detail {
				return m.playSavedPlaylist(p.savedTrack)
			}
			p.detail = true
			return nil
		}
		if len(m.session.entries) > 0 {
			return m.requestQueueLoad(m.session.entries[min(p.cursors[1], len(m.session.entries)-1)], nil, false)
		}
	case 2:
		if p.effect >= 6 && p.effect <= 7 || p.effect == 9 {
			m.adjustEffect(1)
		}
	case 3:
		if m.configSave != nil {
			return nil
		}
		if p.settings[p.cursors[3]].kind == "folders" {
			p.rootEditor = true
			p.rootCursor = 0
			return nil
		}
		if p.settings[p.cursors[3]].kind == "file" {
			return m.openFilePicker(settingFilePurpose(p.settings[p.cursors[3]].key), p.cursors[3], "")
		}
		return p.editSetting()
	}
	return nil
}

func (p *pageState) renderEditor(c *pageCanvas) {
	e := p.editor
	c.label(0, 1, e.title)
	switch e.purpose {
	case "open", "browser_location":
		c.dim(1, 1, "Enter a local path or HTTP(S) audio URL; spaces are preserved")
	case "playlist_create", "playlist_queue", "playlist_rename":
		c.dim(1, 1, "Enter saves this name to disk; Esc cancels")
	case "filter":
		c.dim(1, 1, "Match title, artist or album")
	case "setting":
		c.dim(1, 1, "Enter validates the draft; s saves after closing this editor")
	default:
		c.dim(1, 1, "Sample input / not connected")
	}
	c.rule(2, 1, c.width-2)
	e.input.SetWidth(max(1, c.width-6))
	c.text(4, 2, e.input.View())
	if p.browser != nil && p.browser.err != "" {
		c.label(len(c.rows)-2, 1, browserDisplayText(p.browser.err))
	}
	c.dim(len(c.rows)-1, 1, "Enter Confirm · Esc Cancel")
}

func pageCount(n int, unit string) string { return fmt.Sprintf("%d %s", n, unit) }
