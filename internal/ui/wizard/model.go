package wizard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// PickFileMsg asks the host to delegate the optional SoundFont chooser.
type PickFileMsg struct{ ID string }

// Selection is a reply scoped by the host to the current authenticated session.
type Selection struct{ ID, Status, Path, Error string }

type selectionChecked struct {
	ID, Path string
	Err      error
}

// Model embeds the unchanged form in a single application input lifecycle.
type Model struct {
	form          *huh.Form
	icons         *huh.Select[iconOption]
	logo          *logoNote
	picker        *optionalPicker
	selected      iconOption
	sfPath        string
	width, height int
	gui, native   bool
	pending       string
	sequence      uint64
	notice        string
}

// Init starts Huh's normal field focus and terminal queries.
func (m *Model) Init() tea.Cmd { return m.form.Init() }

// SetGUI upgrades presentation only after the host negotiated a GUI client.
// Losing native capabilities cancels the pending request, keeping all answers.
func (m *Model) SetGUI(gui, native bool) {
	m.gui, m.native = gui, native
	if !native {
		m.pending = ""
	}
}

// Update preserves Huh navigation, including Tab skipping the optional last field.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		cmd := m.resize()
		return m, cmd
	case Selection:
		if msg.ID != m.pending || m.pending == "" {
			return m, nil
		}
		if msg.Status == "selected" {
			return m, func() tea.Msg { return selectionChecked{ID: msg.ID, Path: msg.Path, Err: validateSoundfont(msg.Path)} }
		}
		m.pending = ""
		if msg.Status == "failed" {
			m.notice = msg.Error
		}
		return m, nil
	case selectionChecked:
		if msg.ID != m.pending || m.pending == "" {
			return m, nil
		}
		m.pending = ""
		if msg.Err != nil {
			m.notice = msg.Err.Error()
			return m, nil
		}
		m.sfPath = msg.Path
		m.notice = ""
		return m, m.form.NextField()
	case tea.KeyPressMsg:
		if m.pending != "" && msg.String() != "ctrl+c" {
			return m, nil
		}
		if m.form.GetFocusedField() == m.picker && msg.Code == tea.KeyTab && !msg.Mod.Contains(tea.ModShift) {
			return m, m.form.NextField()
		}
		if m.native && m.form.GetFocusedField() == m.picker && (msg.Code == tea.KeyEnter || msg.String() == "l" || msg.Code == tea.KeyRight) {
			m.sequence++
			m.pending = fmt.Sprintf("soundfont-%d", m.sequence)
			m.notice = ""
			id := m.pending
			return m, func() tea.Msg { return PickFileMsg{ID: id} }
		}
	}
	_, cmd := m.form.Update(msg)
	if m.form.State == huh.StateAborted {
		return m, tea.Quit
	}
	return m, cmd
}

func (m *Model) resize() tea.Cmd {
	width := m.width
	if m.gui {
		width = min(width, 80)
	}
	// Huh shrinks child fields to fit small startup frames; restore their
	// natural sizes before remeasuring a later, larger PTY viewport.
	m.icons.Height(0)
	m.logo.Height(0)
	m.picker.Height(8)
	m.form.WithWidth(max(1, width))
	// Let Huh measure and scroll its viewport within the current terminal.
	_, cmd := m.form.Update(tea.WindowSizeMsg{Width: max(1, width), Height: max(1, m.height)})
	return cmd
}

// Result returns accepted values only once Huh completes the form.
func (m *Model) Result() (*config.Config, bool) {
	if m.form.State != huh.StateCompleted {
		return nil, false
	}
	cfg := config.DefaultConfig()
	cfg.IconTheme = m.selected.ConfigValue()
	cfg.SoundfontPath = m.sfPath
	return &cfg, true
}

// Aborted distinguishes a cancelled setup from an accepted empty SoundFont.
func (m *Model) Aborted() bool { return m.form.State == huh.StateAborted }

// View keeps the form's own colors and half-block logo; only cell padding changes.
func (m *Model) View() tea.View {
	text := m.form.View()
	if m.notice != "" {
		text += "\n" + strings.TrimSpace(m.notice)
	}
	view := tea.NewView(text)
	view.ReportFocus = true
	view.AltScreen = m.gui
	if m.gui && m.width > 0 && m.height > 0 {
		view.Content = centerForm(m.width, m.height, text)
	}
	return view
}

// PendingPicker prevents delayed host commands from reopening a cancelled request.
func (m *Model) PendingPicker(id string) bool { return id != "" && id == m.pending && m.native }
