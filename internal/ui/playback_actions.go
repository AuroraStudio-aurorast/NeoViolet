package ui

// setVolume keeps all input paths consistent with keyboard volume changes.
func (m *Model) setVolume(volume float64) {
	m.Audio.SetVolume(volume)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	m.saveVolumeConfig()
}
