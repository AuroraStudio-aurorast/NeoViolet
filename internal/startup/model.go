// Package startup owns one Bubble Tea program and IPC session from setup to playback.
package startup

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

type guiWaitExpired struct{}
type settingsSaved struct {
	Config *config.Config
	Err    error
}

// Model owns startup phases; the embedded player continues owning all playback state.
type Model struct {
	server                *ipc.Server
	session               uint64
	phase, lastPhase      string
	phaseSession          uint64
	setup                 *wizard.Model
	player                *ui.Model
	cfg                   *config.Config
	path                  string
	seek                  time.Duration
	volume                int
	width, height         int
	awaitGUI, gui, native bool
	saving                bool
	saveError             string
	accepted              *config.Config
	pickerID              string
	guiCapabilities       []string
	controller            mediactl.Controller
	deferredLoad          *ui.LoadTrackMsg
	closed                bool
}

// New creates the IPC endpoint before setup, without creating a second input loop.
func New(path string, cfg *config.Config, needsWizard bool, seek time.Duration, volume int) *Model {
	m := &Model{path: path, cfg: cfg, seek: seek, volume: volume, width: 80, height: 24}
	hint := os.Getenv("TERM_PROGRAM") == "neoviolet-gui"
	if hint {
		var err error
		m.server, err = ipc.NewServer()
		if err != nil {
			logger.Warn("Startup IPC unavailable", "err", err)
		}
	}
	if needsWizard {
		m.setup = wizard.New()
		m.awaitGUI = hint && m.server != nil
		m.phase = "wizard"
		if m.awaitGUI {
			m.phase = "starting"
		}
	} else {
		m.activate(cfg)
	}
	return m
}

// Player returns nil until setup has been accepted and saved successfully.
func (m *Model) Player() *ui.Model { return m.player }

func (m *Model) activate(cfg *config.Config) {
	m.cfg = cfg
	if m.volume > 0 {
		cfg.DefaultVolume = min(1, float64(m.volume)/100)
	}
	if m.path == "" && m.deferredLoad != nil {
		m.path = m.deferredLoad.Path
	}
	m.player = ui.NewModelWithIPC(m.path, cfg, m.server, m.seek)
	m.player.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	if m.controller != nil {
		m.player.Update(ui.MediaCtlReadyMsg{Controller: m.controller})
		m.controller = nil
	}
	if m.session != 0 {
		m.player.Update(ipc.Event{Kind: "connected", Session: m.session})
		m.player.Update(ipc.Event{Kind: "request", Session: m.session, Message: ipc.Message{Type: "client_hello", Client: "neoviolet-gui", Protocol: 1, Capabilities: m.guiCapabilities}})
	}
	m.phase = "ready"
	if m.player.Loading {
		m.phase = "initial_loading"
	}
	m.setup = nil
}

// Init starts one socket reader and the current embedded model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.waitIPC()}
	if m.setup != nil {
		cmds = append(cmds, m.setup.Init())
	} else {
		cmds = append(cmds, m.player.Init())
	}
	if m.awaitGUI {
		cmds = append(cmds, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return guiWaitExpired{} }))
	}
	return tea.Batch(cmds...)
}

// Close is also called when Program.Run fails before a normal quit update.
func (m *Model) Close() {
	if m.closed {
		return
	}
	m.closed = true
	if m.player != nil {
		m.player.Close()
	}
	if m.controller != nil {
		if err := m.controller.Close(); err != nil {
			logger.Debug("Startup media cleanup", "err", err)
		}
	}
	if m.server != nil {
		m.server.Close()
	}
}

// View preserves a single terminal owner across wizard, initial load and player.
func (m *Model) View() tea.View {
	if m.setup != nil {
		if m.awaitGUI || m.saving || m.saveError != "" {
			text := "Starting NeoViolet…"
			if m.saving {
				text = "Saving settings…"
			}
			if m.saveError != "" {
				text = "Could not save settings: " + m.saveError + "\nEnter retry · Esc quit"
			}
			view := tea.NewView(text)
			view.AltScreen = m.gui
			if m.gui {
				view.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, text)
			}
			return view
		}
		return m.setup.View()
	}
	view := m.player.View()
	if m.gui && m.phase == "initial_loading" {
		view.AltScreen = true
	}
	return view
}
