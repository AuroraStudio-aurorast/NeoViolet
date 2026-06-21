// Package ui provides the TUI interface for NeoViolet media player
package ui

import (
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
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
		Message    string
		Timer      int
		Generation int // 0 for non-load errors (always shown); >0 checked against Model.loadGeneration
	}

	AudioLoadedMsg struct {
		Player     audio.AudioPlayer
		Path       string
		Generation int // matches Model.loadGeneration; stale messages are ignored
	}

	VolumeMsg struct {
		Level float64
		Delta float64
	}

	SeekMsg struct {
		Position time.Duration
		Relative bool
	}

	AccentApplyMsg struct {
		Accent *accent.Accent
	}

	MediaCtlMsg struct {
		Command mediactl.Command
	}

	// LoadTrackMsg requests loading a new audio track at runtime.
	// Sent by the stdin listener or :open command.
	LoadTrackMsg struct {
		Path string
	}
)

// KeyMap defines all keyboard shortcuts
type KeyMap struct {
	TabNext      key.Binding
	TabPrev      key.Binding
	Play         key.Binding
	Pause        key.Binding
	Next         key.Binding
	Prev         key.Binding
	VolumeUp     key.Binding
	VolumeDown   key.Binding
	SeekForward  key.Binding
	SeekBackward key.Binding
	Quit         key.Binding
	Command      key.Binding
	NormalMode   key.Binding
	CycleFocus   key.Binding
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
		{k.CycleFocus, k.TabNext, k.TabPrev},
		{k.Play, k.Pause, k.Next, k.Prev},
		{k.VolumeUp, k.VolumeDown, k.SeekForward, k.SeekBackward},
		{k.Quit, k.Command, k.NormalMode},
	}
}

// Sub-structs for separated responsibilities
type AudioState struct {
	Player            audio.AudioPlayer
	CurrentSong       string
	Artist            string
	Album             string
	Progress          float64
	Volume            float64
	Duration          time.Duration
	Elapsed           time.Duration
	IsPlaying         bool
	ShowLyrics        bool
	Lyrics            *lyrics.LyricsData
	LyricIndex        int
	LyricScrollOffset int
	LyricScrollTick   int
	LastLyricIndex    int

	// ActiveLyricLines holds the result of Lyrics.ActiveLines() for the current
	// elapsed time. Set during UpdateLyricIndex(). Used by the renderer.
	ActiveLyricLines []lyrics.LyricLine

	lastActiveSig string // signature for detecting active-line changes

	// LastSentLyricSig is the signature of the last lyrics payload sent
	// to the GUI via IPC. Used to avoid redundant sends (change-based push).
	LastSentLyricSig string

	// LyricNextIndex is the index of the upcoming lyric line when no active lines exist.
	// -1 means no upcoming lyric (past end or no lyrics loaded).
	// >=0 indicates a gap — the view shows countdown dots until this line begins.
	LyricNextIndex int

	// LyricGapDuration is the total duration of the current gap (end-of-previous
	// line to start-of-next line). Used to decide whether to show countdown dots
	// (gap >5s) or a simple placeholder (gap ≤5s).
	LyricGapDuration time.Duration
}

type UIState struct {
	ActiveTab  int
	Tabs       []string
	Mode       Mode
	Focus      Focus
	SavedFocus Focus
	Width      int
	Height     int
	tabWidth   int
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

// InfoState shows green status messages that auto-dismiss.
// Separate from ErrorState so status updates don't look like errors.
type InfoState struct {
	Message string
	Timer   int
	Visible bool
}

func (info *InfoState) Set(msg string, timer int) {
	info.Message = msg
	info.Timer = timer
	info.Visible = true
}

func (info *InfoState) Tick() {
	if info.Visible && info.Timer > 0 {
		info.Timer--
		if info.Timer <= 0 {
			info.Visible = false
			info.Message = ""
		}
	}
}

// Model represents the main application state
type Model struct {
	Audio          *AudioState
	UI             *UIState
	Components     *ComponentState
	Error          *ErrorState
	Info           *InfoState
	Config         *config.Config
	Icons          IconSet
	Accent         *accent.Accent
	QuitConfirm    bool
	ExitCode       int
	Loading        bool
	loadingTick    int
	pendingPath    string
	pendingSeek    time.Duration
	CommandHistory []string
	historyIndex   int

	// preferredLyricFormat is set when switching tracks to try the same
	// lyrics format that was active on the previous track before falling
	// back to the config priority order.
	preferredLyricFormat string

	// loadGeneration is incremented each time a new track load is initiated.
	// AudioLoadedMsg and ErrorMsg with a mismatched generation are ignored,
	// preventing stale load results from corrupting state on rapid switches.
	loadGeneration int

	// switchingTrack is true during a runtime track switch (stdin/:open).
	// When set, the view stays on the main UI and shows a loading indicator
	// in the help bar instead of switching to the full-screen loading view.
	switchingTrack bool

	// ipcServer handles bidirectional communication with the GUI wrapper
	// via Unix domain socket. nil when running standalone.
	ipcServer *ipc.Server

	// DesktopLyricsEnabled controls whether the TUI streams lyric data
	// to the GUI for the desktop lyrics overlay window.
	DesktopLyricsEnabled bool

	MediaCtl  mediactl.Controller
	mediaChan chan mediactl.Command
}
