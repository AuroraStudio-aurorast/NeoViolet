package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// optionalPicker only clarifies the existing optional field's navigation help.
type optionalPicker struct{ *huh.FilePicker }

func (p *optionalPicker) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	_, cmd := p.FilePicker.Update(msg)
	return p, cmd
}

func (p *optionalPicker) KeyBinds() []key.Binding {
	if p.Zoom() {
		return p.FilePicker.KeyBinds()
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "skip")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "back")),
	}
}

func validateSoundfont(path string) error {
	if !strings.EqualFold(filepath.Ext(path), ".sf2") {
		return fmt.Errorf("choose a .sf2 SoundFont file")
	}
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect SoundFont: %w", err)
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("SoundFont must be a regular file")
	}
	// #nosec G304 -- path is the user-selected SoundFont, validated as a regular .sf2 file.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open SoundFont: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close SoundFont: %w", err)
	}

	return nil
}
