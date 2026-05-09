package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func loadAudio(filePath, sfPath string) tea.Msg {
	logger.Info("Loading audio file", "path", filePath)
	player := audio.NewPlayer()
	if sfPath != "" {
		player.SetSoundfontPath(sfPath)
	}
	if err := player.Open(filePath); err != nil {
		logger.Error("Failed to load audio", "path", filePath, "err", err)
		return ErrorMsg{Message: fmt.Sprintf("Failed to load audio: %v", err), Timer: 180}
	}
	return AudioLoadedMsg{Player: player, Path: filePath}
}

// Global key bindings
var keys = KeyMap{
	TabNext: key.NewBinding(
		key.WithKeys("]", "n"),
		key.WithHelp("]", "next tab"),
	),
	TabPrev: key.NewBinding(
		key.WithKeys("[", "p"),
		key.WithHelp("[", "prev tab"),
	),
	Play: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "play"),
	),
	Pause: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "pause"),
	),
	Next: key.NewBinding(
		key.WithKeys(">", "l"),
		key.WithHelp(">", "next track"),
	),
	Prev: key.NewBinding(
		key.WithKeys("<", "h"),
		key.WithHelp("<", "prev track"),
	),
	VolumeUp: key.NewBinding(
		key.WithKeys("+", "="),
		key.WithHelp("+/=", "volume up"),
	),
	VolumeDown: key.NewBinding(
		key.WithKeys("-"),
		key.WithHelp("-", "volume down"),
	),
	SeekForward: key.NewBinding(
		key.WithKeys("ctrl+f", "right"),
		key.WithHelp("ctrl+f", "seek forward"),
	),
	SeekBackward: key.NewBinding(
		key.WithKeys("ctrl+b", "left"),
		key.WithHelp("ctrl+b", "seek backward"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Command: key.NewBinding(
		key.WithKeys(":", "/"),
		key.WithHelp(":", "command mode"),
	),
	NormalMode: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "normal mode"),
	),
	EnterTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "focus tab"),
	),
	EnterFooter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "focus footer"),
	),
}

func NewModel(filePath string, cfg *config.Config) *Model {
	if cfg == nil {
		cfg = &config.Config{}
	}

	var activeIcons IconSet
	switch cfg.IconTheme {
	case "emoji":
		activeIcons = EmojiIcons
	case "fallback":
		activeIcons = FallbackIcons
	default:
		activeIcons = NerdIcons
	}

	pbOpts := []progress.Option{
		progress.WithColors(
			lipgloss.Color("#DA70D6"),
			lipgloss.Color("#8A2BE2"),
		),
		progress.WithFillCharacters([]rune(cfg.ProgressBar.Fill[0])[0], []rune(cfg.ProgressBar.Fill[1])[0]),
		progress.WithScaled(cfg.ProgressBar.Scaled),
	}
	if !cfg.ProgressBar.ShowPercentage {
		pbOpts = append(pbOpts, progress.WithoutPercentage())
	}
	pb := progress.New(pbOpts...)

	vb := progress.New(
		progress.WithColors(
			lipgloss.Color("#00FF00"),
			lipgloss.Color("#FF0000"),
		),
		progress.WithFillCharacters([]rune(cfg.VolumeBar.Fill[0])[0], []rune(cfg.VolumeBar.Fill[1])[0]),
		progress.WithScaled(false),
	)
	if !cfg.VolumeBar.ShowPercentage {
		vb = progress.New(
			progress.WithColors(
				lipgloss.Color("#00FF00"),
				lipgloss.Color("#FF0000"),
			),
			progress.WithFillCharacters([]rune(cfg.VolumeBar.Fill[0])[0], []rune(cfg.VolumeBar.Fill[1])[0]),
			progress.WithScaled(false),
			progress.WithoutPercentage(),
		)
	}
	vb.SetWidth(cfg.VolumeBar.Width)

	h := help.New()
	h.ShowAll = false

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.EchoMode = textinput.EchoNormal
	ti.CharLimit = 100

	m := &Model{
		Audio: &AudioState{
			Volume: cfg.DefaultVolume,
		},
		UI: &UIState{
			Mode:     ModeNormal,
			Focus:    FocusTabBar,
			Tabs:     []string{"Home", "Playlists", "Effects", "Settings"},
			Width:    80,
			Height:   24,
			tabWidth: 20,
		},
		Components: &ComponentState{
			ProgressBar:  pb,
			VolumeBar:    vb,
			Help:         h,
			CommandInput: ti,
		},
		Config:      cfg,
		Icons:       activeIcons,
		Error:       &ErrorState{},
		Loading:     filePath != "",
		pendingPath: filePath,
	}

	logger.Info("Model created", "iconTheme", cfg.IconTheme, "tickRate", cfg.TickRate)

	ti.SetWidth(m.UI.Width - 1)

	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	if blinkCmd, ok := textinput.Blink().(tea.Cmd); ok {
		cmds = append(cmds, blinkCmd)
	}

	if m.pendingPath != "" {
		path := m.pendingPath
		sfPath := m.Config.SoundfontPath
		m.pendingPath = ""
		cmds = append(cmds, func() tea.Msg {
			return loadAudio(path, sfPath)
		})
	}

	cmds = append(cmds, tea.Tick(time.Second/time.Duration(m.Config.TickRate), func(t time.Time) tea.Msg {
		return TickMsg{}
	}))

	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return updateDispatcher(m, msg)
}

func (m *Model) View() tea.View {
	return renderMainView(m)
}

// Helper methods
func (m *Model) togglePlayback() {
	m.Audio.TogglePlayback()
}

func (m *Model) cleanup() {
	logger.Debug("Cleanup: closing player")
	m.Audio.Close()
}

func (m *Model) adjustVolume(delta float64) {
	m.Audio.AdjustVolume(delta)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
}

func (m *Model) updatePlaybackState() tea.Cmd {
	m.Audio.UpdatePosition()
	m.Audio.UpdateLyricIndex()
	m.Audio.AdvanceLyricScroll(m.Config.Lyrics.ScrollSpeed, m.UI.Width-6)
	return m.Components.ProgressBar.SetPercent(m.Audio.Progress)
}

func loadAccentCmd(player audio.AudioPlayer) tea.Cmd {
	return func() tea.Msg {
		img := player.CoverImage()
		if img == nil {
			return AccentApplyMsg{Accent: nil}
		}
		a, err := accent.FromImage(img)
		if err != nil {
			logger.Debug("Failed to extract accent", "err", err)
			return AccentApplyMsg{Accent: nil}
		}
		return AccentApplyMsg{Accent: &a}
	}
}

func (m *Model) rebuildProgressBar() {
	cfg := m.Config
	progressA := lipgloss.Color("#DA70D6")
	progressB := lipgloss.Color("#8A2BE2")

	if m.Accent != nil {
		progressA = lipgloss.Color(m.Accent.HexProgressA())
		progressB = lipgloss.Color(m.Accent.HexProgressB())
	}

	pbOpts := []progress.Option{
		progress.WithColors(progressA, progressB),
		progress.WithFillCharacters([]rune(cfg.ProgressBar.Fill[0])[0], []rune(cfg.ProgressBar.Fill[1])[0]),
	}
	if cfg.ProgressBar.Scaled {
		pbOpts = append(pbOpts, progress.WithScaled(true))
	}
	if !cfg.ProgressBar.ShowPercentage {
		pbOpts = append(pbOpts, progress.WithoutPercentage())
	}

	m.Components.ProgressBar = progress.New(pbOpts...)
}
