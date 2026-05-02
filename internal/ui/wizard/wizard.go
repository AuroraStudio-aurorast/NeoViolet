package wizard

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

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
	start, end := "#c77dff", "#5a00b3"
	n := len(logoLines)
	var out string
	for i, line := range logoLines {
		if line == "" {
			out += "\n"
			continue
		}
		t := float64(i) / float64(n-1)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(lerpHex(start, end, t))).Bold(true)
		out += style.Render(line) + "\n"
	}
	return out
}

func lerpHex(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	return fmt.Sprintf("#%02x%02x%02x",
		int(float64(ar)+float64(br-ar)*t+0.5),
		int(float64(ag)+float64(bg-ag)*t+0.5),
		int(float64(ab)+float64(bb-ab)*t+0.5),
	)
}

func parseHex(s string) (int, int, int) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	r, _ := strconv.ParseInt(s[0:2], 16, 0)
	g, _ := strconv.ParseInt(s[2:4], 16, 0)
	b, _ := strconv.ParseInt(s[4:6], 16, 0)
	return int(r), int(g), int(b)
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
