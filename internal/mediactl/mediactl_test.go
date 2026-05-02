package mediactl

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestPlaybackStatus(t *testing.T) {
	tests := []struct {
		playing bool
		want    string
	}{
		{true, "Playing"},
		{false, "Paused"},
	}
	for _, tt := range tests {
		got := playbackStatus(tt.playing)
		if got != tt.want {
			t.Errorf("playbackStatus(%v) = %q, want %q", tt.playing, got, tt.want)
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
	s := PlayState{Title: "T", Playing: true}
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
}

func TestStubController(t *testing.T) {
	c, err := newController()
	if err != nil {
		t.Fatalf("newController() error: %v", err)
	}

	ch, err := c.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	c.Update(PlayState{Title: "test"})

	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after Close()")
	}
}

func TestControllerInterface(t *testing.T) {
	var _ Controller = (*stubController)(nil)
}
