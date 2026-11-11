package config

// Track switch modes choose when a replacement request unloads the current song.
const (
	TrackSwitchOnSuccess = "on_success"
	TrackSwitchImmediate = "immediate"
)

// Playback modes choose the next entry after a track finishes.
const (
	PlaybackOrder     = "order"
	PlaybackRepeatOne = "repeat_one"
	PlaybackShuffle   = "shuffle"
)
