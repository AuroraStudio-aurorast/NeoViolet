package startup

import (
	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

// Update serializes setup, IPC and player events in the same Bubble Tea loop.
func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.closed {
		if loaded, ok := message.(ui.AudioLoadedMsg); ok {
			// Let the player discard any prepared audio using its existing ownership path.
			if m.player != nil {
				m.player.Update(loaded)
			} else if loaded.Player != nil {
				if err := loaded.Player.Close(); err != nil {
					logger.Debug("Discard late startup audio", "err", err)
				}
			}
		}
		if ready, ok := message.(ui.MediaCtlReadyMsg); ok && ready.Controller != nil {
			if err := ready.Controller.Close(); err != nil {
				logger.Debug("Discard late media controller", "err", err)
			}
		}
		return m, nil
	}
	switch msg := message.(type) {
	case ipc.Event:
		cmd := m.ipcEvent(msg)
		m.publishPhase(false)
		return m, tea.Batch(m.waitIPC(), cmd)
	case guiWaitExpired:
		if m.awaitGUI {
			m.awaitGUI = false
			m.phase = "wizard"
		}
	case settingsSaved:
		m.saving = false
		if msg.Err != nil {
			m.saveError = msg.Err.Error()
			return m, nil
		}
		m.activate(msg.Config)
		m.publishPhase(false)
		return m, m.player.Init()
	case wizard.PickFileMsg:
		if m.setup != nil && m.native && m.session != 0 && m.setup.PendingPicker(msg.ID) {
			m.pickerID = msg.ID
			if err := m.server.SendTo(m.session, ipc.Message{Type: "pick_file", Purpose: "soundfont", RequestID: msg.ID}); err != nil {
				m.pickerID = ""
				return m, m.selection(wizard.Selection{ID: msg.ID, Status: "failed", Error: "Native file picker unavailable. Try the terminal picker."})
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
	case ui.MediaCtlReadyMsg:
		if m.player == nil {
			m.controller = msg.Controller
			return m, nil
		}
	case ui.LoadTrackMsg:
		if m.player == nil {
			m.deferredLoad = &msg
			return m, nil
		}
	}
	if m.setup != nil {
		if key, ok := message.(tea.KeyPressMsg); ok {
			if m.saveError != "" {
				switch key.String() {
				case "enter":
					m.saveError = ""
					return m, m.save()
				case "esc", "ctrl+c":
					return m, tea.Quit
				}
				return m, nil
			}
			if m.awaitGUI && key.String() != "ctrl+c" {
				return m, nil
			}
		}
		if m.saving {
			return m, nil
		}
		_, cmd := m.setup.Update(message)
		if cfg, done := m.setup.Result(); done {
			m.accepted = cfg
			return m, m.save()
		}
		m.publishPhase(false)
		return m, cmd
	}
	_, cmd := m.player.Update(message)
	if m.phase == "initial_loading" && !m.player.Loading {
		m.phase = "ready"
	}
	m.publishPhase(false)
	return m, cmd
}

func (m *Model) save() tea.Cmd {
	m.saving = true
	cfg := m.accepted
	return func() tea.Msg { return settingsSaved{Config: cfg, Err: cfg.Save()} }
}

func (m *Model) selection(reply wizard.Selection) tea.Cmd {
	if m.setup == nil {
		return nil
	}
	_, cmd := m.setup.Update(reply)
	return cmd
}
