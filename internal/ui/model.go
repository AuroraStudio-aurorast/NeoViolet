package ui

import (
	"fmt"
	"image"
	"io"
	"os"
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
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

func loadAudio(filePath, sfPath, trackerBackend string, generation int) tea.Msg {
	logger.Info("Loading audio file", "path", filePath)
	player := audio.NewPlayer()
	if sfPath != "" {
		player.SetSoundfontPath(sfPath)
	}
	player.SetTrackerBackend(trackerBackend)

	// Stdin mode: load audio data from pipe
	if filePath == "-" {
		return loadAudioFromStdin(player, generation)
	}

	if err := player.Open(filePath); err != nil {
		logger.Error("Failed to load audio", "path", filePath, "err", err)
		return ErrorMsg{Message: fmt.Sprintf("Failed to load audio: %v", err), Timer: 180, Generation: generation}
	}
	return AudioLoadedMsg{Player: player, Path: filePath, Generation: generation}
}

// loadAudioFromStdin reads audio data from os.Stdin and opens it via the player.
// os.Stdin is only read when it's a pipe (not a terminal); BubbleTea uses
// /dev/tty for terminal input so there is no conflict. Stdin is closed after
// reading to prevent accidental reuse.
func loadAudioFromStdin(player *audio.Player, generation int) tea.Msg {
	// Check if stdin is a pipe (not a terminal)
	info, err := os.Stdin.Stat()
	if err != nil {
		logger.Error("Failed to stat stdin", "err", err)
		return ErrorMsg{Message: "Failed to read stdin", Timer: 120, Generation: generation}
	}
	if info.Mode()&os.ModeNamedPipe == 0 && info.Mode()&os.ModeCharDevice != 0 {
		return ErrorMsg{
			Message: "stdin is a terminal; pipe audio data or provide a file path",
			Timer:   180,
			Generation: generation,
		}
	}

	// Limit stdin read to 500 MB to prevent memory exhaustion from
	// accidentally piped large files or /dev/zero.
	const maxStdinSize = 500 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinSize+1))
	os.Stdin.Close()
	if err != nil {
		logger.Error("Failed to read stdin", "err", err)
		return ErrorMsg{Message: fmt.Sprintf("Failed to read stdin: %v", err), Timer: 120, Generation: generation}
	}
	if len(data) > maxStdinSize {
		return ErrorMsg{
			Message:    "Stdin input exceeds 500 MB limit",
			Timer:      180,
			Generation: generation,
		}
	}

	if err := player.OpenReader("stdin", data); err != nil {
		logger.Error("Failed to load audio from stdin", "err", err)
		return ErrorMsg{Message: fmt.Sprintf("Failed to load from stdin: %v", err), Timer: 180, Generation: generation}
	}

	return AudioLoadedMsg{Player: player, Path: "stdin", Generation: generation}
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
	CycleFocus: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle focus"),
	),
}

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
	ti.CharLimit = 100

	initialFocus := FocusTabBar
	if filePath != "" {
		initialFocus = FocusFooter
	}

	m := &Model{
		Audio: &AudioState{
			Volume:     cfg.DefaultVolume,
			ShowLyrics: true,
		},
		UI: &UIState{
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
		Icons:       activeIcons,
		Error:       &MessageState{},
		Info:        &MessageState{},
		Loading:     filePath != "",
		pendingPath: filePath,
	}

	if len(seekTo) > 0 && seekTo[0] > 0 {
		m.pendingSeek = seekTo[0]
	}

	// Initialize OS media control layer (MPRIS on Linux, no-op elsewhere)
	var mediaCtlErr error
	m.MediaCtl, mediaCtlErr = mediactl.New()
	if mediaCtlErr != nil {
		logger.Warn("mediactl init failed", "err", mediaCtlErr)
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
		backend := m.Config.TrackerBackend
		m.pendingPath = ""
		gen := m.loadGeneration
		cmds = append(cmds, func() tea.Msg {
			// Start ConEmu progress bar (OSC 9;4) with indeterminate state
			fmt.Fprint(os.Stdout, "\033]9;4;3;0\a")
			return loadAudio(path, sfPath, backend, gen)
		})
	}

	// Accept GUI IPC connection in the background
	if m.ipcServer != nil {
		go func() {
			if err := m.ipcServer.Accept(); err != nil {
				logger.Warn("IPC accept failed", "err", err)
			}
		}()
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
	if m.MediaCtl != nil {
		logger.Debug("Cleanup: closing media controller")
		m.MediaCtl.Close()
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

	pb := progress.New(pbOpts...)
	pb.SetPercent(m.Audio.Progress)
	m.Components.ProgressBar = pb
}
