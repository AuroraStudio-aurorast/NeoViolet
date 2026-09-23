package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

// lrcSubcommandSpec is one ":lrc <sub>" entry. The second segment of the
// completion list is built from this table, so a subcommand cannot be offered
// without a handler (or handled without being offered).
type lrcSubcommandSpec struct {
	Name string
	Desc string // one-line English description (completion list, right column)
	Run  func(m *Model, inv invocation) (tea.Model, tea.Cmd)
}

// lrcSubcommands lists every subcommand, in the order the completion list and
// the usage message present them.
var lrcSubcommands = []lrcSubcommandSpec{
	{Name: "on", Desc: "Enable lyrics", Run: runLrcOn},
	{Name: "off", Desc: "Disable lyrics", Run: runLrcOff},
	{Name: "switch", Desc: "Switch lyrics format", Run: runLrcSwitch},
	{Name: "refresh", Desc: "Clear cache and refetch", Run: runLrcRefresh},
	{Name: "agent", Desc: "Filter lyrics by agent", Run: runLrcAgent},
	{Name: "desktop", Desc: "Toggle desktop lyrics", Run: runLrcDesktop},
	{Name: "panel", Desc: "Toggle the lyrics panel", Run: runLrcPanel},
}

// lrcSubcommandLookup finds a subcommand by its exact name.
func lrcSubcommandLookup(name string) (lrcSubcommandSpec, bool) {
	for _, sub := range lrcSubcommands {
		if sub.Name == name {
			return sub, true
		}
	}
	return lrcSubcommandSpec{}, false
}

// executeLrcCommand implements ":lrc" and its aliases: with no subcommand it
// reports the current state, otherwise the subcommand table dispatches.
func executeLrcCommand(m *Model, parts []string) (tea.Model, tea.Cmd) {
	if len(parts) < 2 {
		if m.Audio.Lyrics != nil && m.Audio.ShowLyrics {
			m.Info.Set("Lyrics: enabled", m.Config.Error.Duration)
		} else {
			m.Info.Set("Lyrics: disabled", m.Config.Error.Duration)
		}
		return m, nil
	}
	sub, ok := lrcSubcommandLookup(parts[1])
	if !ok {
		m.Error.Set(fmt.Sprintf("Unknown lrc subcommand: %s (use on, off, switch, refresh, agent, desktop, or panel)", parts[1]), m.Config.Error.Duration)
		return m, nil
	}
	return sub.Run(m, invocation{Parts: parts, Name: "lrc", Rest: strings.Join(parts[1:], " ")})
}

// runLrcOn implements ":lrc on".
func runLrcOn(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	if m.Audio.Lyrics != nil {
		m.Audio.ShowLyrics = true
		return m, nil
	}
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}
	data, err := lyrics.FindAndParse(m.Audio.Player.Path(), m.Config.Lyrics.FormatPriority)
	if err != nil {
		m.Error.Set(fmt.Sprintf("Failed to load lyrics: %v", err), m.Config.Error.Duration)
		return m, nil
	}
	if data == nil {
		m.Error.Set("No lyrics found for current track", m.Config.Error.Duration)
		return m, nil
	}
	m.Audio.Lyrics = data
	m.Audio.LyricIndex = -1
	m.Audio.ShowLyrics = true
	return m, nil
}

// runLrcOff implements ":lrc off".
func runLrcOff(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	m.Audio.ShowLyrics = false
	return m, nil
}

// runLrcSwitch implements ":lrc switch <format>"; "online" fetches instead of
// reading a sidecar.
func runLrcSwitch(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) < 3 {
		m.Error.Set(fmt.Sprintf("Usage: lrc switch <format> (available: %s)", strings.Join(lyrics.AvailableParsers(), ", ")), m.Config.Error.Duration)
		return m, nil
	}
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}
	format := inv.Parts[2]
	if format == "online" {
		return m, m.fetchLyricsManual()
	}
	data, err := lyrics.FindAndParse(m.Audio.Player.Path(), []string{format})
	if err != nil {
		m.Error.Set(fmt.Sprintf("Failed to parse %s lyrics: %v", format, err), m.Config.Error.Duration)
		return m, nil
	}
	if data == nil {
		m.Error.Set(fmt.Sprintf("No lyrics found for format: %s (available: %s)", format, strings.Join(lyrics.AvailableParsers(), ", ")), m.Config.Error.Duration)
		return m, nil
	}
	m.Audio.Lyrics = data
	m.Audio.LyricIndex = -1
	m.Audio.ShowLyrics = true
	return m, nil
}

// runLrcRefresh implements ":lrc refresh": bypass the disk cache (including
// negative "no lyrics" records) and refetch. The session cache is cleared
// inside fetchLyricsManual.
func runLrcRefresh(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}
	if err := fetch.RemoveCache(m.currentSig()); err != nil {
		m.Error.Set(fmt.Sprintf("Failed to clear lyrics cache: %v", err), m.Config.Error.Duration)
		return m, nil
	}
	m.Info.Set("Lyrics cache cleared, refetching", m.Config.Error.Duration)
	return m, m.fetchLyricsManual()
}

// runLrcAgent implements ":lrc agent [name|all]": with no argument it reports
// the current filter, with one it sets or clears it.
func runLrcAgent(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) < 3 {
		if m.Audio.Lyrics != nil && m.Audio.Lyrics.AgentFilter != "" {
			m.Info.Set(fmt.Sprintf("Lyrics agent filter: %s", m.Audio.Lyrics.AgentFilter), m.Config.Error.Duration)
		} else {
			m.Info.Set("Lyrics agent filter: all", m.Config.Error.Duration)
		}
		return m, nil
	}
	if m.Audio.Lyrics == nil {
		m.Error.Set("No lyrics loaded", m.Config.Error.Duration)
		return m, nil
	}
	filter := inv.Parts[2]
	switch filter {
	case "all", "":
		m.Audio.Lyrics.AgentFilter = ""
		m.Info.Set("Lyrics: showing all agents", m.Config.Error.Duration)
	default:
		m.Audio.Lyrics.AgentFilter = filter
		m.Info.Set(fmt.Sprintf("Lyrics: filtering agent %s", filter), m.Config.Error.Duration)
	}
	m.Audio.LyricScrollOffset = 0
	m.Audio.LyricIndex = -1
	m.Audio.ActiveLyricLines = nil
	return m, nil
}

// runLrcDesktop implements ":lrc desktop": toggle the desktop lyrics window and
// sync the change with the GUI if it is connected.
func runLrcDesktop(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	m.DesktopLyricsEnabled = !m.DesktopLyricsEnabled
	t := m.DesktopLyricsEnabled
	if t {
		m.Info.Set("Desktop lyrics: enabled", m.Config.Error.Duration)
	} else {
		m.Info.Set("Desktop lyrics: disabled", m.Config.Error.Duration)
	}
	if m.ipcServer != nil {
		_ = m.ipcServer.SendJSON(ipc.Message{
			Type:   "desktop_lyrics",
			Enable: &t,
		})
	}
	return m, nil
}

// runLrcPanel implements ":lrc panel [on|off|auto]". The change is
// session-scoped: the config file only supplies the default mode.
func runLrcPanel(m *Model, inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.Parts) >= 3 {
		switch mode := inv.Parts[2]; mode {
		case config.PanelModeOn, config.PanelModeOff, config.PanelModeAuto:
			m.panelMode = mode
		default:
			m.Error.Set(fmt.Sprintf("Unknown panel mode: %s (use on, off, or auto)", mode), m.Config.Error.Duration)
			return m, nil
		}
	}
	m.Info.Set(panelStatusText(m), m.Config.Error.Duration)
	return m, nil
}

// panelStatusText describes the effective panel state for the current size, so
// "nothing happened" is always explained.
func panelStatusText(m *Model) string {
	plan := m.layoutPlan()
	mode := m.panelMode
	if mode == "" {
		mode = config.PanelModeAuto
	}

	switch {
	case m.UI.Width < minWidth || m.UI.Height < minHeight:
		// renderMainView shows the resize warning instead of the frame, so the
		// panel cannot be visible whatever the mode says.
		return "Lyrics panel: hidden (terminal too small)"
	case plan.PanelShown:
		note := ""
		if m.Config.Lyrics.Panel.Width == config.PanelWidthAuto {
			note = ", auto width"
		}
		return fmt.Sprintf("Lyrics panel: %s (shown, %d cols%s)", mode, plan.PanelWidth, note)
	case mode == config.PanelModeOff:
		return "Lyrics panel: off (hidden)"
	case !m.Audio.ShowLyrics:
		return "Lyrics panel: hidden (lyrics are off)"
	default:
		// panelShown(on) is true for every usable width, so "not shown" here
		// implies auto: the content area would be squeezed below minWidth.
		// An auto width needs the smallest box, a configured one its own.
		width := m.Config.Lyrics.Panel.Width
		if width == config.PanelWidthAuto {
			width = config.MinPanelWidth
		}
		need := minWidth + int(width)
		return fmt.Sprintf("Lyrics panel: %s (hidden, needs %d cols, now %d)", mode, need, m.UI.Width)
	}
}
