// MPRIS pure-logic tests. NOT linux-gated: these exercise playbackStatus,
// playerProps, buildMetadata, rootProps and validSetPosition, which compile
// on every platform (the D-Bus wiring itself is tested under a linux tag).

package mediactl

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestPlaybackStatus(t *testing.T) {
	tests := []struct {
		playing  bool
		hasTrack bool
		want     string
	}{
		{true, true, "Playing"},
		{false, true, "Paused"},
		{false, false, "Stopped"},
		{true, false, "Stopped"}, // HasTrack dominates: no track → Stopped
	}
	for _, tt := range tests {
		got := playbackStatus(tt.playing, tt.hasTrack)
		if got != tt.want {
			t.Errorf("playbackStatus(%v, %v) = %q, want %q", tt.playing, tt.hasTrack, got, tt.want)
		}
	}
}

func TestBuildMetadata(t *testing.T) {
	s := PlayState{
		Title:    "Test Song",
		Artist:   "Test Artist",
		Album:    "Test Album",
		Duration: 3 * time.Minute,
	}
	m := buildMetadata(s, "/track/1")

	if m["xesam:title"].Value() != "Test Song" {
		t.Errorf("title = %v, want Test Song", m["xesam:title"].Value())
	}
	artists, ok := m["xesam:artist"].Value().([]string)
	if !ok || len(artists) != 1 || artists[0] != "Test Artist" {
		t.Errorf("artist = %v, want [Test Artist]", m["xesam:artist"].Value())
	}
	if m["xesam:album"].Value() != "Test Album" {
		t.Errorf("album = %v, want Test Album", m["xesam:album"].Value())
	}
	length, ok := m["mpris:length"].Value().(int64)
	if !ok || length != int64(3*time.Minute/time.Microsecond) {
		t.Errorf("length = %v, want %d", length, int64(3*time.Minute/time.Microsecond))
	}
	if m["mpris:trackid"].Value().(dbus.ObjectPath) != "/track/1" {
		t.Errorf("trackid = %v", m["mpris:trackid"].Value())
	}
}

func TestBuildMetadataEmptyArtist(t *testing.T) {
	s := PlayState{Title: "No Artist"}
	m := buildMetadata(s, "/track/2")

	if _, ok := m["xesam:artist"]; ok {
		t.Error("xesam:artist should not be set when Artist is empty")
	}
	if _, ok := m["xesam:album"]; ok {
		t.Error("xesam:album should not be set when Album is empty")
	}
	if _, ok := m["mpris:length"]; ok {
		t.Error("mpris:length should not be set when Duration is 0")
	}
}

func TestPlayerProps(t *testing.T) {
	s := PlayState{Title: "T", Playing: true, HasTrack: true, Volume: 0.5}
	p := playerProps(s, "/track/3")

	if p["PlaybackStatus"].Value() != "Playing" {
		t.Errorf("PlaybackStatus = %v, want Playing", p["PlaybackStatus"].Value())
	}
	if p["CanGoNext"].Value() != true {
		t.Error("CanGoNext should be true")
	}
	if p["CanControl"].Value() != true {
		t.Error("CanControl should be true")
	}
	if p["Volume"].Value() != 0.5 {
		t.Errorf("Volume = %v, want 0.5", p["Volume"].Value())
	}
}

func TestPlayerPropsVolumeZero(t *testing.T) {
	// A muted track must report Volume 0, not fall back to 1.0.
	s := PlayState{Title: "T", HasTrack: true, Volume: 0}
	p := playerProps(s, "/track/4")
	if p["Volume"].Value() != 0.0 {
		t.Errorf("Volume = %v, want 0.0", p["Volume"].Value())
	}
}

func TestRootProps(t *testing.T) {
	p := rootProps()

	if p["Identity"].Value() != "NeoViolet" {
		t.Errorf("Identity = %v, want NeoViolet", p["Identity"].Value())
	}
	// Honest capability flags: no quit path, no fullscreen, no OpenUri.
	if p["CanQuit"].Value() != false {
		t.Error("CanQuit should be false (no quit path)")
	}
	if p["CanSetFullscreen"].Value() != false || p["Fullscreen"].Value() != false {
		t.Error("Fullscreen flags should be false")
	}
	if p["CanRaise"].Value() != false {
		t.Error("CanRaise should be false")
	}
	if p["HasTrackList"].Value() != false {
		t.Error("HasTrackList should be false")
	}
	if p["DesktopEntry"].Value() != "neoviolet" {
		t.Errorf("DesktopEntry = %v, want neoviolet", p["DesktopEntry"].Value())
	}
	schemes, ok := p["SupportedUriSchemes"].Value().([]string)
	if !ok || len(schemes) != 0 {
		t.Errorf("SupportedUriSchemes = %v, want empty", p["SupportedUriSchemes"].Value())
	}
	// All 9 required Root properties must be present.
	for _, prop := range []string{"CanQuit", "CanRaise", "CanSetFullscreen", "Fullscreen",
		"HasTrackList", "Identity", "DesktopEntry", "SupportedUriSchemes", "SupportedMimeTypes"} {
		if _, ok := p[prop]; !ok {
			t.Errorf("rootProps missing required property %s", prop)
		}
	}
}

func TestValidSetPosition(t *testing.T) {
	cur := dbus.ObjectPath("/neoviolet/track/1")
	dur := 2 * time.Minute

	cases := []struct {
		name    string
		trackID dbus.ObjectPath
		pos     int64
		dur     time.Duration
		want    bool
	}{
		{"valid in range", cur, 30_000_000, dur, true},
		{"exactly at length", cur, int64(2 * time.Minute / time.Microsecond), dur, true},
		{"stale track id", dbus.ObjectPath("/neoviolet/track/0"), 30_000_000, dur, false},
		{"negative position", cur, -1, dur, false},
		{"beyond length", cur, int64(3 * time.Minute / time.Microsecond), dur, false},
		{"unknown length, any non-negative", cur, 1_000_000_000, 0, true},
	}
	for _, tc := range cases {
		if got := validSetPosition(tc.trackID, cur, tc.pos, tc.dur); got != tc.want {
			t.Errorf("%s: validSetPosition(%q, %q, %d, %v) = %v, want %v",
				tc.name, tc.trackID, cur, tc.pos, tc.dur, got, tc.want)
		}
	}
}
