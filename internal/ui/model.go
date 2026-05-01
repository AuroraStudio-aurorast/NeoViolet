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

	"neoviolet/internal/audio"
)

func loadAudio(filePath string) tea.Msg {
	player := audio.NewPlayer()
	if err := player.Open(filePath); err != nil {
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

func NewModel(filePath string, fallbackIcon bool, emoji bool) *Model {
	switch {
	case emoji:
		Icons = EmojiIcons
	case fallbackIcon:
		Icons = FallbackIcons
	}

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
		Error:       &ErrorState{},
		Loading:     filePath != "",
		pendingPath: filePath,
	}

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
		m.pendingPath = ""
		cmds = append(cmds, func() tea.Msg {
			return loadAudio(path)
		})
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
	m.Audio.TogglePlayback()
}

func (m *Model) cleanup() {
	m.Audio.Close()
}

func (m *Model) adjustVolume(delta float64) {
	m.Audio.AdjustVolume(delta)
	m.Components.VolumeBar.SetPercent(m.Audio.Volume)
}

func (m *Model) updatePlaybackState() tea.Cmd {
	m.Audio.UpdatePosition()
	return m.Components.ProgressBar.SetPercent(m.Audio.Progress)
}
