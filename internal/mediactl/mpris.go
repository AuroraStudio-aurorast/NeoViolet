package mediactl

import (
	"time"

	"github.com/godbus/dbus/v5"
)

func playbackStatus(playing bool) string {
	if playing {
		return "Playing"
	}
	return "Paused"
}

func playerProps(s PlayState, trackID string) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"PlaybackStatus": dbus.MakeVariant(playbackStatus(s.Playing)),
		"LoopStatus":     dbus.MakeVariant("None"),
		"Rate":           dbus.MakeVariant(1.0),
		"Shuffle":        dbus.MakeVariant(false),
		"Metadata":       dbus.MakeVariant(buildMetadata(s, trackID)),
		"Volume":         dbus.MakeVariant(1.0),
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
