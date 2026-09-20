// Package ui provides the TUI interface for NeoViolet media player
package ui

import (
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/accent"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

// Mode represents the current input mode
type Mode int

// Input modes.
const (
	ModeNormal Mode = iota
	ModeCommand
)

// Focus represents the currently focused UI element
type Focus int

// Focus targets.
const (
	FocusTabBar Focus = iota
	FocusContent
	FocusFooter
)

type (
	// TickMsg signals a periodic UI tick.
	TickMsg struct{}

	// PlaybackUpdateMsg carries playback position updates from the audio loop.
	PlaybackUpdateMsg struct {
		Progress float64
		Elapsed  time.Duration
	}

	// ErrorMsg reports an error to the UI with an auto-dismiss timer.
	ErrorMsg struct {
		Message    string
		Timer      int
		Generation int // 0 for non-load errors (always shown); >0 checked against Model.loadGeneration
	}

	// AudioLoadedMsg signals that a track finished loading.
	AudioLoadedMsg struct {
		Player     audio.AudioPlayer
		Path       string
		Generation int // matches Model.loadGeneration; stale messages are ignored
	}

	// VolumeMsg carries a volume level and change delta.
	VolumeMsg struct {
		Level float64
		Delta float64
	}

	// SeekMsg requests a seek to a position (absolute or relative).
	SeekMsg struct {
		Position time.Duration
		Relative bool
	}

	// AccentApplyMsg carries a newly extracted accent color.
	AccentApplyMsg struct {
		Accent *accent.Accent
	}

	// FetchLyricsResultMsg carries the outcome of an async online lyric fetch.
	FetchLyricsResultMsg struct {
		Data *lyrics.Data
		Err  error
		Sig  string // normalized track signature; stale results are dropped
	}

	// MediaCtlMsg carries an OS media-control command.
	MediaCtlMsg struct {
		Command mediactl.Command
	}

	// MediaCtlReadyMsg is sent once after the Bubble Tea program starts
	// to lazily initialize the OS media controller. Deferring init avoids
	// AppKit-related issues when the first-run wizard runs before the main
	// program on macOS.
	MediaCtlReadyMsg struct {
		Controller mediactl.Controller
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

// AudioState holds the audio playback state and lyric display state.
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
	Lyrics            *lyrics.Data
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

	// LastSentLyricElapsed and LastLyricPush bound the progress stream that keeps
	// the overlay's word highlight moving: the elapsed value and wall-clock time
	// of the last send.
	LastSentLyricElapsed time.Duration
	LastLyricPush        time.Time

	// LyricNextIndex is the index of the upcoming lyric line when no active lines exist.
	// -1 means no upcoming lyric (past end or no lyrics loaded).
	// >=0 indicates a gap — the view shows countdown dots until this line begins.
	LyricNextIndex int

	// LyricGapDuration is the total duration of the current gap (end-of-previous
	// line to start-of-next line). Used to decide whether to show countdown dots
	// (gap >5s) or a simple placeholder (gap ≤5s).
	LyricGapDuration time.Duration
}

// State holds the tab, focus, and layout state for the interface.
type State struct {
	ActiveTab  int
	Tabs       []string
	Mode       Mode
	Focus      Focus
	SavedFocus Focus

	CommandNotice string

	Width    int
	Height   int
	tabWidth int
}

// ComponentState holds the Bubble Tea component models used by the view.
type ComponentState struct {
	ProgressBar  progress.Model
	VolumeBar    progress.Model
	Help         help.Model
	CommandInput textinput.Model
}

// MessageState shows a transient text message that auto-dismisses.
// Used for both error (red) and info (green) status updates.
type MessageState struct {
	Message string
	Timer   int
	Visible bool
}

// Set displays a message with a countdown timer.
func (e *MessageState) Set(msg string, timer int) {
	e.Message = msg
	e.Timer = timer
	e.Visible = true
}

// Tick decrements the message timer and dismisses the message when it expires.
func (e *MessageState) Tick() {
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
	UI             *State
	Components     *ComponentState
	Error          *MessageState
	Info           *MessageState
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

	// Completion state for the command line. completionIndex is -1 while
	// nothing is selected: the candidate list is shown passively, so <enter>
	// keeps executing exactly what was typed until <tab> picks an entry.
	completionCandidates []candidate
	completionIndex      int
	completionSeg        segment

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

	// fetchCache and fetchRateLimit back online lyric fetching. They are
	// session-scoped and shared between auto-fetch and :lrc switch online.
	fetchCache     *fetch.Cache
	fetchRateLimit *fetch.RateLimit

	// LyricsFetching is true while an online fetch for the current track is
	// in flight, so the footer can show a fetching indicator.
	LyricsFetching bool

	// fetchCmd holds the pending online lyric fetch command until the end of
	// handleAudioLoaded, where it is batched with other startup commands.
	fetchCmd tea.Cmd

	// panelMode is the runtime panel mode (auto|on|off). It starts from the
	// config default and is changed by ":lrc panel"; it is never persisted.
	panelMode string

	MediaCtl mediactl.Controller
}
