// MPRIS pure logic. This file is intentionally NOT linux-gated: the functions
// here only build dbus.Variant values and validate arguments, so they compile
// and are unit-tested on every platform; the linux-only D-Bus wiring lives in
// controller_linux.go.

package mediactl

import (
	"time"

	"github.com/godbus/dbus/v5"
)

// playbackStatus maps the TUI playback state to the MPRIS Playback_Status
// enum: no track loaded is "Stopped", otherwise Playing/Paused.
func playbackStatus(playing, hasTrack bool) string {
	switch {
	case !hasTrack:
		return "Stopped"
	case playing:
		return "Playing"
	default:
		return "Paused"
	}
}

// rootProps returns the org.mpris.MediaPlayer2 (Root) interface properties.
// CanQuit is false because this player exposes no quit path; CanSetFullscreen
// and Fullscreen are false since there is no windowing support; OpenUri is
// unsupported, so SupportedUriSchemes is empty.
// validSetPosition reports whether an MPRIS SetPosition call should be acted
// on per spec: the track ID must match the currently-playing track, and the
// position must lie in [0, track length] (an unknown length allows any
// non-negative position).
func validSetPosition(trackID, curID dbus.ObjectPath, pos int64, dur time.Duration) bool {
	if trackID != curID {
		return false
	}
	if pos < 0 || (dur > 0 && pos > int64(dur/time.Microsecond)) {
		return false
	}
	return true
}

func rootProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"CanQuit":             dbus.MakeVariant(false),
		"CanRaise":            dbus.MakeVariant(false),
		"CanSetFullscreen":    dbus.MakeVariant(false),
		"Fullscreen":          dbus.MakeVariant(false),
		"HasTrackList":        dbus.MakeVariant(false),
		"Identity":            dbus.MakeVariant("NeoViolet"),
		"DesktopEntry":        dbus.MakeVariant("neoviolet"),
		"SupportedUriSchemes": dbus.MakeVariant([]string{}),
		"SupportedMimeTypes":  dbus.MakeVariant([]string{}),
	}
}

func playerProps(s PlayState, trackID string) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(playbackStatus(s.Playing, s.HasTrack)),
		"LoopStatus":     dbus.MakeVariant("None"),
		"Rate":           dbus.MakeVariant(1.0),
		"Shuffle":        dbus.MakeVariant(false),
		"Metadata":       dbus.MakeVariant(buildMetadata(s, trackID)),
		"Volume":         dbus.MakeVariant(s.Volume),
		"Position":       dbus.MakeVariant(int64(s.Position / time.Microsecond)),
		"MinimumRate":    dbus.MakeVariant(1.0),
		"MaximumRate":    dbus.MakeVariant(1.0),
		"CanGoNext":      dbus.MakeVariant(true),
		"CanGoPrevious":  dbus.MakeVariant(true),
		"CanPlay":        dbus.MakeVariant(true),
		"CanPause":       dbus.MakeVariant(true),
		"CanSeek":        dbus.MakeVariant(true),
		"CanControl":     dbus.MakeVariant(true),
	}
}

func buildMetadata(s PlayState, trackID string) map[string]dbus.Variant {
	m := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath(trackID)),
		"xesam:title":   dbus.MakeVariant(s.Title),
	}
	if s.Artist != "" {
		m["xesam:artist"] = dbus.MakeVariant([]string{s.Artist})
	}
	if s.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(s.Album)
	}
	if s.Duration > 0 {
		m["mpris:length"] = dbus.MakeVariant(int64(s.Duration / time.Microsecond))
	}
	return m
}
