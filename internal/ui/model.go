package ui

import (
	"fmt"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"neoviolet/internal/audio"
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
		key.WithKeys(":"),
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

func NewModel(filePath string) *Model {
	pb := progress.New(
		progress.WithColors(
			lipgloss.Color("#DA70D6"),
			lipgloss.Color("#8A2BE2"),
		),
		progress.WithScaled(true),
		progress.WithoutPercentage(),
	)
	pb.SetWidth(60)

	vb := progress.New(
		progress.WithColors(
			lipgloss.Color("#00FF00"),
			lipgloss.Color("#FF0000"),
		),
		progress.WithFillCharacters('▰', '▱'),
		progress.WithScaled(false),
	)
	vb.SetWidth(16)

	h := help.New()
	h.ShowAll = false

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.EchoMode = textinput.EchoNormal
	ti.CharLimit = 100

	m := &Model{
		Audio: &AudioState{
			Volume: 1.0,
		},
		UI: &UIState{
			Mode:     ModeNormal,
			Focus:    FocusTabBar,
			Tabs:     []string{"Library", "Queue", "Settings"},
			Width:    80, // Default width
			Height:   24, // Default height
			tabWidth: 20, // Default tab width
		},
		Components: &ComponentState{
			ProgressBar:  pb,
			VolumeBar:    vb,
			Help:         h,
			CommandInput: ti,
		},
		Error: &ErrorState{},
	}

	// Initialize with audio file if provided
	if filePath != "" {
		player := audio.NewPlayer()
		if err := player.Open(filePath); err != nil {
			m.Error.Message = fmt.Sprintf("Failed to open audio file: %v", err)
			m.Error.Timer = 180
			m.Error.Visible = true
		} else {
			m.Audio.Player = player
			m.Audio.Duration = player.Duration()
			m.Components.ProgressBar.SetPercent(0)

			if player.Title() != "" {
				m.Audio.CurrentSong = player.Title()
			} else {
				m.Audio.CurrentSong = filepath.Base(filePath)
			}

			if player.Artist() != "" {
				m.Audio.Artist = player.Artist()
			} else {
				m.Audio.Artist = "Unknown Artist"
			}

			// Apply stored volume to the new player
			player.SetVolume(m.Audio.Volume)
		}
	}

	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	// Add blink command for text input
	if blinkCmd, ok := textinput.Blink().(tea.Cmd); ok {
		cmds = append(cmds, blinkCmd)
	}

	// Start playback if player exists
	if m.Audio.Player != nil {
		if err := m.Audio.Player.Play(); err != nil {
			m.Error.Message = fmt.Sprintf("Failed to start playback: %v", err)
			m.Error.Timer = 180
			m.Error.Visible = true
		}
	}

	cmds = append(cmds, tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
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
	if m.Audio.Player == nil {
		return
	}

	if m.Audio.Player.IsPlaying() {
		m.Audio.Player.Pause()
		m.Audio.IsPlaying = false
	} else {
		m.Audio.Player.Play()
		m.Audio.IsPlaying = true
	}
}

func (m *Model) cleanup() {
	if m.Audio.Player != nil {
		m.Audio.Player.Close()
		m.Audio.Player = nil
	}
}

func (m *Model) adjustVolume(delta float64) {
	// Update Model's volume regardless of Player existence
	m.Audio.Volume += delta
	if m.Audio.Volume > 1.0 {
		m.Audio.Volume = 1.0
	}
	if m.Audio.Volume < 0 {
		m.Audio.Volume = 0
	}

	// Apply volume to Player if it exists
	if m.Audio.Player != nil {
		m.Audio.Player.SetVolume(m.Audio.Volume)
	}

	// Update volume bar
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
}

func (m *Model) updatePlaybackState() {
	if m.Audio.Player != nil {
		pos := m.Audio.Player.Position()
		duration := m.Audio.Player.Duration()

		m.Audio.Elapsed = pos

		if duration > 0 && m.Audio.Duration != duration {
			m.Audio.Duration = duration
		}

		if m.Audio.Duration > 0 {
			m.Audio.Progress = float64(pos) / float64(m.Audio.Duration)
			if m.Audio.Progress > 1.0 {
				m.Audio.Progress = 1.0
			}
			if m.Audio.Progress < 0 {
				m.Audio.Progress = 0
			}
		} else {
			m.Audio.Progress = 0
		}

		m.Components.ProgressBar.SetPercent(m.Audio.Progress)
	}
}
