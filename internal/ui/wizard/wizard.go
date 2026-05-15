package wizard

import (
	"os"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

type iconOption int

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

var logoLines = []string{
	"",
	"███╗   ██╗███████╗ ██████╗",
	"████╗  ██║██╔════╝██╔═══██╗",
	"██╔██╗ ██║█████╗  ██║   ██║",
	"██║╚██╗██║██╔══╝  ██║   ██║",
	"██║ ╚████║███████╗╚██████╔╝",
	"╚═╝  ╚═══╝╚══════╝ ╚═════╝",
	"",
	"██╗   ██╗██╗ ██████╗ ██╗     ███████╗████████╗",
	"██║   ██║██║██╔═══██╗██║     ██╔════╝╚══██╔══╝",
	"██║   ██║██║██║   ██║██║     █████╗     ██║",
	"╚██╗ ██╔╝██║██║   ██║██║     ██╔══╝     ██║",
	" ╚████╔╝ ██║╚██████╔╝███████╗███████╗   ██║",
	"  ╚═══╝  ╚═╝ ╚═════╝ ╚══════╝╚══════╝   ╚═╝",
}

func logoGradient() string {
	start, _ := colorful.Hex("#c77dff")
	end, _ := colorful.Hex("#5a00b3")
	n := len(logoLines)
	var out string
	for i, line := range logoLines {
		if line == "" {
			out += "\n"
			continue
		}
		t := float64(i) / float64(n-1)
		c := start.BlendHcl(end, t)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Bold(true)
		out += style.Render(line) + "\n"
	}
	return out
}

func Run() (*config.Config, error) {
	var selected iconOption
	var sfPath string

	home, _ := os.UserHomeDir()

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

	logo := logoGradient() + "\nWelcome to NeoViolet \u2014 a terminal music player."

	compact := func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.FieldSeparator = lipgloss.NewStyle().SetString("\n")
		s.Focused.NoteTitle = s.Focused.NoteTitle.MarginBottom(0)
		s.Focused.Card = s.Focused.Base.Padding(0, 0)
		s.Blurred.Card = s.Blurred.Base.Padding(0, 0)
		return s
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title(logo),
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
	).WithTheme(huh.ThemeFunc(compact))

	err := form.Run()
	if err != nil {
		return nil, err
	}

	cfg := config.DefaultConfig()
	cfg.IconTheme = selected.ConfigValue()
	cfg.SoundfontPath = sfPath
	return &cfg, nil
}
