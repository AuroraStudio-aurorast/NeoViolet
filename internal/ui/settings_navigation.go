package ui

import "fmt"

func (m *Model) openSettings() error {
	p := m.pages
	if p == nil {
		return fmt.Errorf("settings are unavailable")
	}
	if m.UI.Mode == ModeCommand || p.editor != nil || p.picker != "" || p.browser != nil || p.rootEditor || p.quitSettings || p.deletePlaylist != "" || m.ipcPickerID != "" || m.configSave != nil || m.playlists.save != nil {
		return fmt.Errorf("finish or cancel the current input or save before opening settings")
	}
	if m.Anim != nil {
		m.Anim.Close()
	}
	m.UI.ActiveTab = 3
	m.UI.Focus = FocusContent
	m.UI.Mode = ModeNormal
	return nil
}

func (m *Model) openSettingsSection(section string) error {
	group := -1
	switch section {
	case "":
	case "gui":
		group = 5
	case "desktop_lyrics":
		group = 6
	default:
		return fmt.Errorf("unknown settings section: %s", section)
	}
	if err := m.openSettings(); err != nil {
		return err
	}
	if group >= 0 {
		m.pages.jumpGroup(group)
	}
	return nil
}
