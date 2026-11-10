package ui

import (
	"encoding/json"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func (m *Model) pushGUIPreferences() {
	if !m.ipcPreferences || m.ipcServer == nil || m.ipcSession == 0 || m.exiting {
		return
	}
	m.ipcPreferencesRevision++
	frame := ipc.Preferences{GUI: m.Config.GUI, DesktopLyrics: m.Config.DesktopLyrics}
	_ = m.ipcServer.SendTo(m.ipcSession, ipc.Message{Type: "preferences", Revision: m.ipcPreferencesRevision, Preferences: &frame})
}

// Only runtime controls may write here; the full form remains a TUI draft.
func (m *Model) saveGUIPreference(message ipc.Message) (tea.Cmd, error) {
	if m.ipcPickerID != "" || m.configSave != nil || m.pages == nil || m.pages.editor != nil || m.pages.picker != "" || m.pages.browser != nil || m.pages.rootEditor || m.pages.quitSettings || m.pages.changed[message.Key] {
		return nil, fmt.Errorf("finish or cancel the current edit or save before changing this preference")
	}
	var value any
	switch message.Key {
	case "gui.font_size":
		var size int
		if err := json.Unmarshal(message.Value, &size); err != nil {
			return nil, fmt.Errorf("font size must be an integer")
		}
		parsed, err := parseSetting(pageSetting{key: message.Key}, strconv.Itoa(size))
		if err != nil {
			return nil, err
		}
		value = parsed
	case "desktop_lyrics.enabled", "desktop_lyrics.locked":
		var enabled *bool
		if err := json.Unmarshal(message.Value, &enabled); err != nil || enabled == nil {
			return nil, fmt.Errorf("value must be a boolean")
		}
		value = *enabled
	default:
		return nil, fmt.Errorf("unsupported runtime preference: %s", message.Key)
	}
	candidate := m.Config.Clone()
	values := configValues(candidate)
	setSettingValue(values, message.Key, value)
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &candidate); err != nil {
		return nil, err
	}
	cmd := m.startConfigSave(candidate, map[string]bool{message.Key: true}, false)
	m.configSave.receipt = &ipcOpenRequest{id: message.RequestID, session: m.ipcSession}
	m.configSave.preserveDraft = true
	return cmd, nil
}
