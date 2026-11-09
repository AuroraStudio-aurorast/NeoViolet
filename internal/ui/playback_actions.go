package ui

// setVolume keeps all input paths consistent with keyboard volume changes.
func (m *Model) setVolume(volume float64) {
	before := m.Audio.Volume
	m.Audio.SetVolume(volume)
	if m.Audio.Volume != before {
		m.volumeRevision++
	}
	if m.pages != nil && !m.pages.changed["default_volume"] {
		m.pages.draft.DefaultVolume = m.Audio.Volume
		for i := range m.pages.settings {
			if m.pages.settings[i].key == "default_volume" {
				value := formatSettingValue("default_volume", m.Audio.Volume)
				m.pages.settings[i].value = value
				m.pages.settings[i].savedValue = value
			}
		}
	}
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	m.saveVolumeConfig()
}
