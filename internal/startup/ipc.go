package startup

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

func (m *Model) waitIPC() tea.Cmd {
	if m.server == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-m.server.Incoming
		if !ok {
			return nil
		}
		return event
	}
}

func (m *Model) publishPhase(force bool) {
	if m.server == nil || m.session == 0 || (!force && m.lastPhase == m.phase && m.phaseSession == m.session) {
		return
	}
	if err := m.server.SendTo(m.session, ipc.Message{Type: "phase", Phase: m.phase}); err == nil {
		m.lastPhase = m.phase
		m.phaseSession = m.session
	}
}

func (m *Model) result(id, status, reason string) {
	if id == "" {
		return
	}
	if err := m.server.SendTo(m.session, ipc.Message{Type: "result", RequestID: id, Status: status, Error: reason}); err != nil {
		logger.Debug("Startup IPC result unavailable", "err", err)
	}
}

func (m *Model) ipcEvent(event ipc.Event) tea.Cmd {
	switch event.Kind {
	case "connected":
		if m.server == nil || event.Session != m.server.Session() {
			return nil
		}
		m.session = event.Session
		m.native = false
		m.guiCapabilities = nil
		m.publishPhase(true)
		if m.player != nil {
			_, cmd := m.player.Update(event)
			return cmd
		}
	case "disconnected":
		if event.Session != m.session {
			return nil
		}
		m.session = 0
		m.native = false
		m.guiCapabilities = nil
		m.pickerID = ""
		if m.setup != nil {
			m.setup.SetGUI(m.gui, false)
		}
		if m.player != nil {
			_, cmd := m.player.Update(event)
			return cmd
		}
	case "request":
		if m.server == nil || event.Session != m.session || event.Session != m.server.Session() {
			return nil
		}
		message := event.Message
		switch message.Type {
		case "client_hello":
			if message.Client != "neoviolet-gui" || message.Protocol != 1 {
				m.result(message.RequestID, "failed", "Unsupported GUI client")
				return nil
			}
			m.gui = true
			m.guiCapabilities = slices.Clone(message.Capabilities)
			m.native = slices.Contains(message.Capabilities, "file_picker")
			if m.setup != nil {
				m.awaitGUI = false
				m.phase = "wizard"
				m.setup.SetGUI(true, m.native)
			}
			if m.player != nil {
				_, cmd := m.player.Update(event)
				return cmd
			}
			m.result(message.RequestID, "succeeded", "")
			return nil
		case "file_picked":
			if message.Purpose != "soundfont" {
				break
			}
			if m.setup == nil || !m.native || message.RequestID != m.pickerID || m.pickerID == "" {
				return nil
			}
			m.pickerID = ""
			return m.selection(wizard.Selection{ID: message.RequestID, Status: message.Status, Path: message.Path, Error: message.Error})
		}
		if m.phase != "ready" {
			switch message.Type {
			case "quit":
				m.result(message.RequestID, "succeeded", "")
				m.Close()
				return tea.Quit
			case "request_quit":
				confirm := false
				if err := m.server.SendTo(event.Session, ipc.Message{Type: "quit", Dialog: &confirm}); err != nil {
					logger.Debug("Startup quit unavailable", "err", err)
				}
				m.result(message.RequestID, "succeeded", "")
			case "get_state":
				m.publishPhase(true)
				m.result(message.RequestID, "succeeded", "")
			default:
				m.result(message.RequestID, "failed", fmt.Sprintf("Player is not ready (%s)", m.phase))
			}
			return nil
		}
		if m.player != nil {
			_, cmd := m.player.Update(event)
			return cmd
		}
	}
	return nil
}
