package wizard

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// setupForm sizes the real form before forwarding resize events. Huh's group
// appends a blank line before its help footer; reserve that line and the inline
// renderer's cursor row rather than filling the entire terminal height.
type setupForm struct {
	form    *huh.Form
	welcome *huh.Note
	picker  *huh.FilePicker
	logo    string
}

func (m *setupForm) Init() tea.Cmd { return m.form.Init() }

func (m *setupForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		title := m.logo
		if size.Height < 36 || size.Width < 70 {
			title = "Welcome to NeoViolet — a terminal music player."
		}
		if size.Width < 50 {
			title = "NeoViolet setup"
		}
		m.welcome.Title(title)
		m.picker.Height(min(8, max(1, size.Height-12)))
		m.form.WithWidth(max(1, size.Width)).WithHeight(max(1, size.Height-2))
	}
	_, cmd := m.form.Update(msg)
	return m, cmd
}

func (m *setupForm) View() tea.View {
	v := tea.NewView(m.form.View())
	v.ReportFocus = true
	return v
}

func (m *setupForm) Run() error {
	m.form.SubmitCmd = tea.Quit
	m.form.CancelCmd = tea.Interrupt
	_, err := tea.NewProgram(m).Run()
	if m.form.State == huh.StateAborted || errors.Is(err, tea.ErrInterrupted) {
		return huh.ErrUserAborted
	}
	return err
}
