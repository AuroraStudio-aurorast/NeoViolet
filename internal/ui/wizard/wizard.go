package wizard

import (
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

// Run shows the first-run setup wizard and returns the resulting config.
func Run() (*config.Config, error) {
	var sfPath string

	home, _ := os.UserHomeDir()

	// Detect Nerd Font support: preselect Nerd Font if terminal has it,
	// otherwise fall back to Fallback for maximum compatibility.
	selected := IconFallback
	if detectNerdFont() {
		selected = IconNerd
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

	form := huh.NewForm(
		huh.NewGroup(
			newLogoNote(),
			huh.NewSelect[iconOption]().
				Title("Select your favorite icon theme (also based on your terminal)").
				Options(opts...).
				Value(&selected),
			huh.NewFilePicker().
				Title("Choose a SoundFont file (optional)").
				Description("Pick a .sf2 file for MIDI playback, or leave empty to skip.").
				AllowedTypes([]string{".sf2"}).
				CurrentDirectory(home).
				Height(8).
				Value(&sfPath),
		),
	).WithTheme(huh.ThemeFunc(wizardTheme))

	err := form.Run()
	if err != nil {
		return nil, err
	}

	cfg := config.DefaultConfig()
	cfg.IconTheme = selected.ConfigValue()
	cfg.SoundfontPath = sfPath
	return &cfg, nil
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
