package ui

// setVolume keeps all input paths consistent with keyboard volume changes.
func (m *Model) setVolume(volume float64) {
	before := m.Audio.Volume
	m.Audio.SetVolume(volume)
	if m.Audio.Volume != before {
		m.volumeRevision++
	}
	if p := m.pages; p != nil {
		p.baseline.DefaultVolume = m.Audio.Volume
		if !p.changed["default_volume"] {
			p.draft.DefaultVolume = m.Audio.Volume
		}
		if p.draft.DefaultVolume == m.Audio.Volume {
			delete(p.changed, "default_volume")
		}
		p.dirty = len(p.changed) > 0
		for i := range p.settings {
			if p.settings[i].key == "default_volume" {
				p.settings[i].value = formatSettingValue("default_volume", p.draft.DefaultVolume)
				p.settings[i].savedValue = formatSettingValue("default_volume", m.Audio.Volume)
			}
		}
	}
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	m.saveVolumeConfig()
}
