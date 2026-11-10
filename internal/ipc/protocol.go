// Package ipc provides bidirectional TCP localhost communication
// between the NeoViolet TUI and the neoviolet-gui wrapper.
//
// The TUI listens on 127.0.0.1:0 (random port) and writes the assigned
// address and a random secret token to a temp file. The GUI reads this
// file, connects via TCP, and sends the token as the first line for
// authentication. All subsequent messages are newline-delimited JSON objects.
// The server acknowledges successful authentication with {"type":"hello"}.
//
// GUI → TUI message types:
//
//	{"type": "client_hello", "client":"neoviolet-gui", "protocol":1, "capabilities":["file_picker","audio_file_picker","settings_preferences"]}
//	{"type": "file_picked", "purpose":"soundfont|audio|library_directory|player_binary", "request_id":"...", "status":"selected|cancelled|failed", "path":"..."}
//	{"type": "open", "path": "/path/to/file"}
//	{"type": "desktop_lyrics", "enable": true|false}
//	{"type": "play"} | {"type": "pause"} — idempotent playback control
//	{"type": "seek", "position": 12.3} — absolute seconds, clamped to duration
//	{"type": "set_volume", "volume": 0.5} — 0..1, including zero
//	{"type": "get_state"} — request a full snapshot
//	{"type": "open_picker", "purpose":"audio"} — ask Go to request the native audio chooser
//	{"type": "open_settings"} — focus the TUI Settings page, retaining drafts
//	{"type": "play_pause"}                  — toggle play/pause
//	{"type": "request_quit"}                — ask Go whether confirmation is needed
//	{"type": "quit"}                        — confirmed GUI close, clean up and exit
//
// TUI → GUI message types:
//
//	{"type": "phase", "phase":"starting|wizard|initial_loading|ready"}
//	{"type": "pick_file", "purpose":"soundfont|audio|library_directory|player_binary", "request_id":"..."}
//	{"type": "hello"}                       — authentication succeeded
//	{"type": "desktop_lyrics", "enable": true|false} — synchronize overlay visibility
//	{"type": "quit", "dialog": true}     — loaded track or load in progress; show confirmation
//	{"type": "quit", "dialog": false}    — no track, or :wq; begin graceful shutdown
//	{"type": "lyrics", "lines": [...], "elapsed": 12.3, "title": "...", "artist": "..."}
//
// Requests may include a string request_id. The server sends result messages
// with that ID and status succeeded, failed (with error), or cancelled. Opening
// a track first yields accepted, then one terminal result when it starts,
// fails, or is superseded by another load. Results never migrate to a new session.
// Commands without request_id remain compatible and receive no result.
//
// hello includes protocol:1, version (the raw TUI build version), and capabilities
// ["results","state","playback","request_quit","startup","file_picker","audio_file_picker","settings_navigation","settings_preferences","settings_file_picker"].
// Product versions never gate
// protocol compatibility. Optional libopenmpt_version identifies the runtime-loaded
// library and is omitted when the library is unavailable; it is not the build lock version.
// request_quit leaves the TUI running until the GUI
// sends quit after deciding whether to show the requested confirmation.
// GUI capability negotiation follows token authentication. The startup host sends
// phase on connection and on transitions; only ready enables application actions.
// Initial load failure also reaches ready, allowing recovery; runtime replacements
// do not re-enter initial_loading. A pick_file request and its file_picked reply
// are scoped to one connection and request_id. Startup soundfont selection is
// wizard-only, with .sf2 validation in Go; settings_file_picker adds its runtime
// counterpart below. Purpose audio is ready-only, requires the GUI
// audio_file_picker capability, and uses Go audio-path validation and loading.
// Cancellation retains the current SoundFont or track.
// state messages carry a state object: track_id (accepted load generation),
// revision (increasing within one authenticated connection), path/title/artist/
// album, has_track/playing/loading/desktop_lyrics, duration/position in seconds,
// volume in 0..1, and config_dir (the player's actual settings directory).
// A snapshot is sent after authentication and on changes regardless of lyrics.
// Progress is limited to 4 Hz; discrete changes are pushed immediately.
// Unknown message types fail explicitly when request_id is present.
// settings_navigation advertises open_settings. Navigation fails explicitly
// while an editor, picker, confirmation or save owns the input; it never
// discards that interaction. It does not change playback or save the draft.
// settings_preferences adds optional open_settings section (gui|desktop_lyrics),
// and preferences snapshots: {type:"preferences",revision:1,preferences:{gui:{...},desktop_lyrics:{...}}}.
// Revisions increase within a connection. A client advertising settings_preferences
// receives a full snapshot on negotiation/get_state and after successful saves,
// never on playback ticks. GUI bootstrap reads these same sections from config.json;
// only Go writes preferences. GUI writes geometry separately in gui-state.json.
// set_preference accepts key gui.font_size, desktop_lyrics.enabled or
// desktop_lyrics.locked plus a JSON value. It persists asynchronously and sends
// a terminal result only after saving; errors leave live preferences unchanged.
// An active save/editor or conflicting dirty field rejects the request.
// Window chrome and player path apply on the next GUI/player launch.
// settings_file_picker extends pick_file/file_picked with ready-only purposes
// soundfont, library_directory and player_binary. The startup wizard retains
// its own SoundFont flow; replies are routed to the owner of their request ID.
// Selected settings paths are validated by Go and update only the current draft.
// Every runtime picker reply receives a terminal result. Audio success acknowledges load
// dispatch; settings success acknowledges the validated draft update, not saving.
// Cancellation receives result(cancelled). A capable GUI keeps terminal input
// suspended until this acknowledgment, since PTY and TCP delivery are independent.
// Cancellation preserves it; a failed native chooser falls back to the TUI browser.
// GUI open_picker {purpose:"audio"} asks Go to initiate the same ready-only audio
// chooser used by Home and :open. Its result acknowledges dispatch, not playback.
// GUI advertises settings_file_picker only when it can host these chooser kinds.
// A dialog and asynchronous selection validation belong to one authenticated
// connection; disconnection, supersession and shutdown invalidate late replies.
//
// The lyrics payload describes each line as Prefix followed by its display
// sub-lines (Parts when it has any, otherwise Text). Words carries the timings
// of the first sub-line when the format has them, so the overlay can highlight
// each fragment as it is sung instead of re-deriving the split.
// Lines includes neighboring cues during timed gaps; an empty list means no
// lyric data is available, rather than merely that no cue is active right now.
package ipc

import (
	"encoding/json"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

// Message is a JSON message exchanged between TUI and GUI over IPC.
type Message struct {
	Section           string          `json:"section,omitempty"`
	Key               string          `json:"key,omitempty"`
	Value             json.RawMessage `json:"value,omitempty"`
	Revision          uint64          `json:"revision,omitempty"`
	Preferences       *Preferences    `json:"preferences,omitempty"`
	Type              string          `json:"type"`
	Client            string          `json:"client,omitempty"`
	Phase             string          `json:"phase,omitempty"`
	Purpose           string          `json:"purpose,omitempty"`
	RequestID         string          `json:"request_id,omitempty"`
	Version           string          `json:"version,omitempty"`
	LibopenmptVersion string          `json:"libopenmpt_version,omitempty"`
	Protocol          int             `json:"protocol,omitempty"`
	Capabilities      []string        `json:"capabilities,omitempty"`
	Status            string          `json:"status,omitempty"`
	Error             string          `json:"error,omitempty"`
	Position          *float64        `json:"position,omitempty"`
	Volume            *float64        `json:"volume,omitempty"`
	State             *PlaybackState  `json:"state,omitempty"`
	Path              string          `json:"path,omitempty"`
	Dialog            *bool           `json:"dialog,omitempty"` // quit: true=show dialog, false=quit now

	// desktop_lyrics: enable/disable streaming
	Enable *bool `json:"enable,omitempty"`

	// lyrics: streaming payload from TUI to GUI
	Lines   []LyricLineJSON `json:"lines,omitempty"`
	Elapsed float64         `json:"elapsed,omitempty"` // seconds
	Title   string          `json:"title,omitempty"`
	Artist  string          `json:"artist,omitempty"`
}

// Preferences is the complete Go-owned GUI projection; it excludes geometry.
type Preferences struct {
	GUI           config.GUIConfig           `json:"gui"`
	DesktopLyrics config.DesktopLyricsConfig `json:"desktop_lyrics"`
}

// LyricLineJSON is a single lyric line serialized for IPC.
//
// A line renders as Prefix followed by its display sub-lines: Parts when it has
// any, otherwise Text alone.
type LyricLineJSON struct {
	Time      float64    `json:"time"`             // seconds
	End       float64    `json:"end"`              // seconds; 0 = unbounded (valid until the next line)
	Text      string     `json:"text"`             // the line's own text, without the agent label
	Parts     []string   `json:"parts,omitempty"`  // display sub-lines of a merged event; nil = plain line
	Words     []WordJSON `json:"words,omitempty"`  // word timings tiling the first sub-line; nil = none
	Prefix    string     `json:"prefix,omitempty"` // agent label ahead of the first sub-line; "" = none due
	Agent     string     `json:"agent"`            // agent ID, "" for no agent
	AgentName string     `json:"agent_name"`       // display name for agent, "" if n/a
}

// WordJSON is one timed fragment of a line's first display sub-line. The
// fragments concatenate back to that sub-line exactly, so a karaoke renderer can
// split it at any elapsed value without losing or reordering characters: a
// fragment starts at Time. End optionally bounds it before the next fragment;
// when absent, use the next fragment's Time or the line's End.
type WordJSON struct {
	Time float64 `json:"time"` // seconds
	End  float64 `json:"end,omitempty"`
	Text string  `json:"text"`
}

// PlaybackState is a read-only projection of the authoritative Go model.
// Zero values are serialized explicitly so pause, mute and rewind clear old UI.
type PlaybackState struct {
	Revision      uint64  `json:"revision"`
	TrackID       int     `json:"track_id"`
	Path          string  `json:"path"`
	Title         string  `json:"title"`
	Artist        string  `json:"artist"`
	Album         string  `json:"album"`
	HasTrack      bool    `json:"has_track"`
	Playing       bool    `json:"playing"`
	Loading       bool    `json:"loading"`
	DesktopLyrics bool    `json:"desktop_lyrics"`
	Duration      float64 `json:"duration"`
	Position      float64 `json:"position"`
	Volume        float64 `json:"volume"`
	ConfigDir     string  `json:"config_dir"`
}

// Hello identifies the running TUI and its independent wire capabilities.
func Hello() Message {
	return Message{Type: "hello", Protocol: 1, Version: version.Version, LibopenmptVersion: synth.OpenmptVersion(), Capabilities: []string{"results", "state", "playback", "request_quit", "startup", "file_picker", "audio_file_picker", "settings_navigation", "settings_preferences", "settings_file_picker"}}
}
