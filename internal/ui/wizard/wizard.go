package wizard

import (
	tea "charm.land/bubbletea/v2"
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

type iconOption int

// Icon theme options for the first-run wizard.
const (
	IconNerd iconOption = iota
	IconEmoji
	IconFallback
)

func (o iconOption) ConfigValue() string {
	switch o {
	case IconNerd:
		return "nerd"
	case IconEmoji:
		return "emoji"
	case IconFallback:
		return "fallback"
	}
	return "nerd"
}

// New constructs the existing Huh form for embedding in the application loop.
func New() *Model {
	m := &Model{}

	home, _ := os.UserHomeDir()

	// Detect Nerd Font support: preselect Nerd Font if terminal has it,
	// otherwise fall back to Fallback for maximum compatibility.
	m.selected = IconFallback
	if detectNerdFont() {
		m.selected = IconNerd
	}

	descriptions := map[iconOption]string{
		IconNerd:     "Nerd Font: Requires a Nerd Font. (Recommended)",
		IconEmoji:    "Emoji:     Uses emoji characters. No special font needed.",
		IconFallback: "Fallback:  Uses basic Unicode. Maximum compatibility.",
	}

	opts := []huh.Option[iconOption]{
		huh.NewOption(descriptions[IconNerd], IconNerd),
		huh.NewOption(descriptions[IconEmoji], IconEmoji),
		huh.NewOption(descriptions[IconFallback], IconFallback),
	}

	m.picker = &optionalPicker{FilePicker: huh.NewFilePicker().
		Title("Choose a SoundFont file (optional)").
		Description("Pick a .sf2 file for MIDI playback, or leave empty to skip.").
		AllowedTypes([]string{".sf2"}).CurrentDirectory(home).Height(8).Value(&m.sfPath)}
	m.logo = newLogoNote()
	m.icons = huh.NewSelect[iconOption]().
		Title("Select your favorite icon theme (also based on your terminal)").
		Options(opts...).Value(&m.selected)
	m.form = huh.NewForm(huh.NewGroup(m.logo, m.icons, m.picker)).WithTheme(huh.ThemeFunc(wizardTheme))

	return m
}

// Run preserves the standalone wizard entry point.
func Run() (*config.Config, error) {
	model := New()
	model.form.SubmitCmd = tea.Quit
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return nil, err
	}
	cfg, done := model.Result()
	if !done {
		return nil, huh.ErrUserAborted
	}
	return cfg, nil
}

func wizardTheme(isDark bool) *huh.Styles {
	// Huh v2.0.3 supplies LightDark's two palettes in reverse order.
	// Invert only this theme input; terminal detection and the logo keep isDark.
	s := huh.ThemeCharm(!isDark)
	s.FieldSeparator = lipgloss.NewStyle().SetString("\n")
	s.Focused.NoteTitle = lipgloss.NewStyle()
	s.Blurred.NoteTitle = lipgloss.NewStyle()
	s.Focused.Card = s.Focused.Base.Padding(0, 0)
	s.Blurred.Card = s.Blurred.Base.Padding(0, 0)
	return s
}
