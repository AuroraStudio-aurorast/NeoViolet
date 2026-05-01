// Package ui provides the TUI interface for NeoViolet media player
package ui

import (
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	
	"neoviolet/internal/audio"
)

// Mode represents the current input mode
type Mode int

const (
	ModeNormal Mode = iota
	ModeCommand
)

// Focus represents the currently focused UI element
type Focus int

const (
	FocusTabBar Focus = iota
	FocusContent
	FocusFooter
)

// Custom message types for BubbleTea architecture
type (
	TickMsg struct{}
	
	PlaybackUpdateMsg struct {
		Progress float64
		Elapsed  time.Duration
	}
	
	ErrorMsg struct {
		Message string
		Timer   int
	}
	
	AudioLoadedMsg struct {
		Player audio.AudioPlayer
		Path   string
	}
	
	VolumeMsg struct {
		Level float64
		Delta float64
	}
	
	SeekMsg struct {
		Position time.Duration
		Relative bool
	}
)

// Re-export window size message for unified handling
type WindowSizeMsg = tea.WindowSizeMsg

// KeyMap defines all keyboard shortcuts
type KeyMap struct {
	TabNext    key.Binding
	TabPrev    key.Binding
	Play       key.Binding
	Pause      key.Binding
	Next       key.Binding
	Prev       key.Binding
	VolumeUp   key.Binding
	VolumeDown key.Binding
	SeekForward  key.Binding
	SeekBackward key.Binding
	Quit       key.Binding
	Command    key.Binding
	NormalMode key.Binding
	EnterTab   key.Binding
	EnterFooter key.Binding
}

// ShortHelp returns the key bindings shown in the short help
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Play,
		k.Pause,
		k.Quit,
		k.Command,
	}
}

// FullHelp returns the key bindings shown in the full help
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.TabNext, k.TabPrev, k.EnterTab, k.EnterFooter},
		{k.Play, k.Pause, k.Next, k.Prev},
		{k.VolumeUp, k.VolumeDown, k.SeekForward, k.SeekBackward},
		{k.Quit, k.Command, k.NormalMode},
	}
}

// Sub-structs for separated responsibilities
type AudioState struct {
	Player      audio.AudioPlayer
	CurrentSong string
	Artist      string
	Progress    float64
	Volume      float64
	Duration    time.Duration
	Elapsed     time.Duration
	IsPlaying   bool
}

type UIState struct {
	ActiveTab int
	Tabs      []string
	Mode      Mode
	Focus     Focus
	Width     int
	Height    int
	tabWidth  int
}

type ComponentState struct {
	ProgressBar  progress.Model
	VolumeBar    progress.Model
	Help         help.Model
	CommandInput textinput.Model
}

type ErrorState struct {
	Message string
	Timer   int
	Visible bool
}

func (e *ErrorState) Set(msg string, timer int) {
	e.Message = msg
	e.Timer = timer
	e.Visible = true
}

func (e *ErrorState) Tick() {
	if e.Visible && e.Timer > 0 {
		e.Timer--
		if e.Timer <= 0 {
			e.Visible = false
			e.Message = ""
		}
	}
}

// Model represents the main application state
type Model struct {
	Audio          *AudioState
	UI             *UIState
	Components     *ComponentState
	Error          *ErrorState
	QuitConfirm    bool
	ExitCode       int
	CommandHistory []string
	historyIndex   int
}