package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
)

func (m *Model) requestAudioPicker() tea.Cmd {
	if m.ipcPickerID != "" {
		return nil
	}
	m.ipcPickerSequence++
	id := fmt.Sprintf("audio-%d", m.ipcPickerSequence)
	if err := m.ipcServer.SendTo(m.ipcSession, ipc.Message{Type: "pick_file", Purpose: "audio", RequestID: id}); err != nil {
		m.Error.Set("Native file picker unavailable. Use :open <path>.", m.Config.Error.Duration)
		return nil
	}
	m.ipcPickerID = id
	return nil
}

func (m *Model) audioPicked(message ipc.Message) tea.Cmd {
	if !m.ipcAudioPicker || message.Purpose != "audio" || m.ipcPickerID == "" || message.RequestID != m.ipcPickerID {
		return nil
	}
	m.ipcPickerID = ""
	switch message.Status {
	case "cancelled":
		return nil
	case "selected":
		if isValidAudioPath(message.Path) {
			_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: message.Path})
			return cmd
		}
		m.Error.Set("Invalid or unsupported audio file: "+message.Path, m.Config.Error.Duration)
	case "failed":
		reason := message.Error
		if reason == "" {
			reason = "Native file picker unavailable. Use :open <path>."
		}
		m.Error.Set(reason, m.Config.Error.Duration)
	default:
		m.Error.Set("Invalid native file picker result", m.Config.Error.Duration)
	}
	return nil
}
