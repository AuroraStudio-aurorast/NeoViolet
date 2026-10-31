package ui

import (
	"image"
	"os"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

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
	CycleFocus: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle focus"),
	),
}

// NewModel constructs the application model with the given config.
func NewModel(filePath string, cfg *config.Config, seekTo ...time.Duration) *Model {
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

	vbOpts := []progress.Option{
		progress.WithColors(
			lipgloss.Color("#00FF00"),
			lipgloss.Color("#FF0000"),
		),
		progress.WithFillCharacters([]rune(cfg.VolumeBar.Fill[0])[0], []rune(cfg.VolumeBar.Fill[1])[0]),
		progress.WithScaled(false),
	}
	if !cfg.VolumeBar.ShowPercentage {
		vbOpts = append(vbOpts, progress.WithoutPercentage())
	}
	vb := progress.New(vbOpts...)
	vb.SetWidth(cfg.VolumeBar.Width)

	h := help.New()
	h.ShowAll = false

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.EchoMode = textinput.EchoNormal
	ti.CharLimit = 256
	ti.ShowSuggestions = true
	// Free up/down for command history: textinput's defaults bind the
	// suggestion cursor to down/ctrl+n and up/ctrl+p, and "up"/"down" are
	// intercepted in command mode before they ever reach the input.
	ti.KeyMap.NextSuggestion = key.NewBinding(key.WithKeys("ctrl+n"))
	ti.KeyMap.PrevSuggestion = key.NewBinding(key.WithKeys("ctrl+p"))
	ti.SetSuggestions(nil)

	initialFocus := FocusTabBar
	if filePath != "" {
		initialFocus = FocusFooter
	}

	m := &Model{
		Audio: &AudioState{
			Volume:     cfg.DefaultVolume,
			ShowLyrics: true,
		},
		UI: &State{
			Mode:       ModeNormal,
			Focus:      initialFocus,
			SavedFocus: initialFocus,
			Tabs:       []string{"Home", "Playlists", "Effects", "Settings"},
			Width:      80,
			Height:     24,
			tabWidth:   20,
		},
		Components: &ComponentState{
			ProgressBar:  pb,
			VolumeBar:    vb,
			Help:         h,
			CommandInput: ti,
		},
		Config:      cfg,
		panelMode:   cfg.Lyrics.Panel.Mode,
		Icons:       activeIcons,
		Error:       &MessageState{},
		Info:        &MessageState{},
		Anim:        anim.New(),
		Loading:     filePath != "",
		pendingPath: filePath,
	}

	if len(seekTo) > 0 && seekTo[0] > 0 {
		m.pendingSeek = seekTo[0]
	}

	// Online lyric fetch state: session cache + persisted provider cooldown.
	m.fetchCache = fetch.NewCache()
	if dir, err := config.CacheDir(); err == nil {
		if rl, rlErr := fetch.LoadRateLimit(dir); rlErr == nil {
			m.fetchRateLimit = rl
		} else {
			logger.Warn("failed to load rate limit state", "err", rlErr)
		}
	}
	if m.fetchRateLimit == nil {
		m.fetchRateLimit = fetch.NewRateLimit()
	}

	// Initialize IPC server for bidirectional GUI communication.
	// Only start when running inside neoviolet-gui (not a standard terminal).
	// The GUI sets TERM_PROGRAM=neoviolet-gui as the terminal emulator name.
	if os.Getenv("TERM_PROGRAM") == "neoviolet-gui" {
		if srv, err := ipc.NewServer(); err != nil {
			logger.Warn("IPC server init failed", "err", err)
		} else {
			m.ipcServer = srv
		}
	} else {
		logger.Debug("Not running inside neoviolet-gui, IPC disabled")
	}

	// Load persisted command history
	loadHistory(m)

	logger.Info("Model created", "iconTheme", cfg.IconTheme, "tickRate", cfg.TickRate)

	syncCommandInputWidth(m)

	return m
}

// Init returns the initial commands for the Bubble Tea program.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	if blinkCmd, ok := textinput.Blink().(tea.Cmd); ok {
		cmds = append(cmds, blinkCmd)
	}

	if m.pendingPath != "" {
		path := m.pendingPath
		m.pendingPath = ""
		cmds = append(cmds, m.startAudioLoad(path))
	}

	// Accept GUI IPC connection in the background
	if m.ipcServer != nil {
		go func() {
			if err := m.ipcServer.Accept(); err != nil {
				logger.Warn("IPC accept failed", "err", err)
			}
		}()
	}

	cmds = append(cmds, tea.Tick(time.Second/time.Duration(m.Config.TickRate), func(_ time.Time) tea.Msg {
		return TickMsg{}
	}))

	return tea.Batch(cmds...)
}

// Update handles a Bubble Tea message and returns the next model and command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return updateDispatcher(m, msg)
}

// View renders the current UI state.
//
// Every frame passes through here, so this is where the background goes on
// -- for the frames that have the screen to themselves: the loading screen and the
// too-small warning are drawn on the shell's own screen, and under the GUI the
// frame is drawn on the wrapper's canvas, so both keep the terminal's own colours.
// Painting either would outlast the app if it died before the renderer could
// undo the pin.
func (m *Model) View() tea.View {
	view := renderMainView(m)
	if view.AltScreen && !m.isGUI() {
		view.Content = paintBackground(view.Content, frameBackground(m))
		view.BackgroundColor = uiBackground
		view.ForegroundColor = uiForeground
	}
	return view
}

// Helper methods
func (m *Model) togglePlayback() {
	m.Audio.TogglePlayback()
}

func (m *Model) cleanup() {
	m.exiting = true
	m.pendingLoad.cancel()
	m.pendingLoad = nil
	logger.Debug("Cleanup: closing player")
	m.Audio.Close()
	if m.MediaCtl != nil {
		logger.Debug("Cleanup: closing media controller")
		_ = m.MediaCtl.Close()
	}
	if m.ipcServer != nil {
		logger.Debug("Cleanup: closing IPC server")
		m.ipcServer.Close()
	}
}

// isGUI returns true when the TUI is running inside the neoviolet-gui
// wrapper (detected via TERM_PROGRAM=neoviolet-gui). In GUI mode,
// accidental quit via Ctrl+C or double-tap q is disabled — the user
// must use :quit / :q to exit gracefully.
func (m *Model) isGUI() bool {
	return m.ipcServer != nil
}

// buildPlayState builds a mediactl.PlayState from the current audio state.
func (m *Model) buildPlayState() mediactl.PlayState {
	var cover image.Image
	if m.Audio.Player != nil {
		cover = m.Audio.Player.CoverImage()
	}
	return mediactl.PlayState{
		Title:    m.Audio.CurrentSong,
		Artist:   m.Audio.Artist,
		Album:    m.Audio.Album,
		Duration: m.Audio.Duration,
		Position: m.Audio.Elapsed,
		Playing:  m.Audio.IsPlaying,
		Volume:   m.Audio.Volume,
		HasTrack: m.Audio.Player != nil,
		Cover:    cover,
	}
}

func (m *Model) adjustVolume(delta float64) {
	m.Audio.AdjustVolume(delta)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
	m.saveVolumeConfig()
}

// saveVolumeConfig persists the current volume to config if it changed.
func (m *Model) saveVolumeConfig() {
	if m.Config.DefaultVolume == m.Audio.Volume {
		return
	}
	m.Config.DefaultVolume = m.Audio.Volume
	if err := m.Config.Save(); err != nil {
		logger.Warn("Failed to save volume config", "err", err)
	}
}

func (m *Model) updatePlaybackState() tea.Cmd {
	m.Audio.UpdatePosition()
	m.Audio.UpdateLyricIndex()
	// The marquee only exists in the one_line footer row; with the panel shown
	// there is nothing to scroll, so skip the bookkeeping entirely.
	if plan := m.layoutPlan(); plan.LyricMode == lyricModeOneLine {
		m.Audio.AdvanceLyricScroll(m.Config.Lyrics.ScrollSpeed, plan.OneLineLyricWidth)
	}
	return m.Components.ProgressBar.SetPercent(m.Audio.Progress)
}

func loadAccentCmd(player audio.AudioPlayer, generation int) tea.Cmd {
	return func() tea.Msg {
		img := player.CoverImage()
		if img == nil {
			return AccentApplyMsg{Generation: generation}
		}
		a, err := accent.FromImage(img)
		if err != nil {
			logger.Debug("Failed to extract accent", "err", err)
			return AccentApplyMsg{Generation: generation}
		}
		return AccentApplyMsg{Accent: &a, Generation: generation}
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

	pb := progress.New(pbOpts...)
	pb.SetPercent(m.Audio.Progress)
	m.Components.ProgressBar = pb
}
