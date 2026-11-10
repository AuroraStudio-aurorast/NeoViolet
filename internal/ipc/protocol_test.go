package ipc

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

func TestCrossLanguageProtocolFixturePreservesZerosAndPaths(t *testing.T) {
	data, err := os.ReadFile("testdata/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	fixtureHello := Hello()
	fixtureHello.LibopenmptVersion = "0.8.9" // Fixed transport sample, independent of the host library.
	messages := map[string]Message{
		"hello":        fixtureHello,
		"preferences":  {Type: "preferences", Revision: 2, Preferences: &Preferences{GUI: config.GUIConfig{MonospaceFont: "Test Mono", FontSize: 20, Opacity: .8, MinimizeButton: true}, DesktopLyrics: config.DesktopLyricsConfig{FontSize: 28, Opacity: .6, Highlight: "white", AdditionalLines: true}}},
		"phase":        {Type: "phase", Phase: "wizard"},
		"pick_file":    {Type: "pick_file", Purpose: "soundfont", RequestID: "soundfont-7"},
		"client_hello": {Type: "client_hello", Client: "neoviolet-gui", Protocol: 1, Capabilities: []string{"file_picker", "audio_file_picker", "settings_preferences"}},
		"file_picked":  {Type: "file_picked", Purpose: "soundfont", RequestID: "soundfont-7", Status: "cancelled"},
		"result":       {Type: "result", RequestID: "mute", Status: "succeeded"},
		"state":        {Type: "state", State: &PlaybackState{Revision: 7, TrackID: 3, Path: "/music/a \"song\" 雪.flac", Title: "雪", HasTrack: true, DesktopLyrics: true, Duration: 120, ConfigDir: "/settings/neoviolet"}},
	}
	for name, message := range messages {
		got, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var actual, want any
		if err := json.Unmarshal(got, &actual); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(fixture[name], &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, want) {
			t.Errorf("%s: got %s, want %s", name, got, fixture[name])
		}
	}
	var requests []Message
	if err := json.Unmarshal(fixture["requests"], &requests); err != nil {
		t.Fatal(err)
	}
	if requests[0].Volume == nil || *requests[0].Volume != 0 || requests[1].Position == nil || *requests[1].Position != 0 {
		t.Fatal("explicit zero was lost")
	}
	if requests[2].Path != "/music/a \"song\" 雪.flac" {
		t.Fatal("quoted Unicode path changed")
	}
	if requests[4].Section != "desktop_lyrics" || requests[5].Key != "gui.font_size" || string(requests[5].Value) != "20" {
		t.Fatal("preference requests lost")
	}
	if requests[3].Type != "open_settings" || requests[3].RequestID != "settings" {
		t.Fatal("settings navigation request was lost")
	}
}

func TestHandshakeDescribesTheRunningPlayer(t *testing.T) {
	original := version.Version
	version.Version = "v9.8.7-test"
	t.Cleanup(func() { version.Version = original })
	data, err := json.Marshal(Hello())
	if err != nil {
		t.Fatal(err)
	}
	var hello map[string]any
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatal(err)
	}
	if hello["version"] != "v9.8.7-test" {
		t.Fatalf("handshake version: %s", data)
	}
	capabilities := hello["capabilities"].([]any)
	found := false
	for _, capability := range capabilities {
		if capability == "request_quit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("quit policy capability missing: %s", data)
	}
}

func TestHandshakeReportsTheResolvedOpenmptVersion(t *testing.T) {
	expected := synth.OpenmptVersion()
	data, err := json.Marshal(Hello())
	if err != nil {
		t.Fatal(err)
	}
	var hello map[string]any
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatal(err)
	}
	if expected == "" {
		if _, present := hello["libopenmpt_version"]; present {
			t.Fatalf("unavailable optional library reported: %s", data)
		}
	} else if hello["libopenmpt_version"] != expected {
		t.Fatalf("loaded libopenmpt version missing from handshake: %s (want %s)", data, expected)
	}
}

func TestOptionalLibraryVersionSurvivesWireRoundTrip(t *testing.T) {
	var message Message
	if err := json.Unmarshal([]byte(`{"type":"hello","version":"dev","libopenmpt_version":"0.8.9"}`), &message); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip["libopenmpt_version"] != "0.8.9" {
		t.Fatalf("runtime library version lost: %s", raw)
	}
}
