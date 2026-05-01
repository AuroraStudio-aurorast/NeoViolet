package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
)

func updateDispatcher(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		return handleTick(m)
	case tea.WindowSizeMsg:
		return handleResize(m, msg)
	case tea.KeyPressMsg:
		return handleKeyPress(m, msg)
	case tea.MouseMsg:
		return handleMouse(m, msg)
	case progress.FrameMsg:
		return handleProgressFrame(m, msg)
	case VolumeMsg:
		return handleVolume(m, msg)
	case SeekMsg:
		return handleSeek(m, msg)
	case ErrorMsg:
		return handleError(m, msg)
	default:
		return m, nil
	}
}

func handleTick(m *Model) (tea.Model, tea.Cmd) {
	m.updatePlaybackState()

	if m.Error.Visible && m.Error.Timer > 0 {
		m.Error.Timer--
		if m.Error.Timer <= 0 {
			m.Error.Visible = false
			m.Error.Message = ""
		}
	}

	return m, tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

func handleResize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.UI.Width = msg.Width
	m.UI.Height = msg.Height
	
	// Recalculate tab width based on available space
	m.UI.tabWidth = (msg.Width - 4) / len(m.UI.Tabs)
	if m.UI.tabWidth > 20 {
		m.UI.tabWidth = 20
	}
	
	return m, nil
}

func handleProgressFrame(m *Model, msg progress.FrameMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Components.ProgressBar, cmd = m.Components.ProgressBar.Update(msg)
	return m, cmd
}

func handleVolume(m *Model, msg VolumeMsg) (tea.Model, tea.Cmd) {
	if msg.Delta != 0 {
		m.adjustVolume(msg.Delta)
	} else if msg.Level >= 0 && msg.Level <= 1.0 {
		m.Audio.Volume = msg.Level
		if m.Audio.Player != nil {
			m.Audio.Player.SetVolume(m.Audio.Volume)
		}
		m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	}
	return m, nil
}

func handleSeek(m *Model, msg SeekMsg) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		return m, nil
	}
	
	currentPos := m.Audio.Player.Position()
	var newPos time.Duration
	if msg.Relative {
		newPos = currentPos + msg.Position
	} else {
		newPos = msg.Position
	}
	
	if newPos < 0 {
		newPos = 0
	}
	if newPos > m.Audio.Duration {
		newPos = m.Audio.Duration
	}
	
	if err := m.Audio.Player.Seek(newPos); err != nil {
		m.Error.Message = fmt.Sprintf("Seek failed: %v", err)
		m.Error.Timer = 120
		m.Error.Visible = true
	}
	
	return m, nil
}

func handleError(m *Model, msg ErrorMsg) (tea.Model, tea.Cmd) {
	m.Error.Message = msg.Message
	m.Error.Timer = msg.Timer
	m.Error.Visible = true
	return m, nil
}