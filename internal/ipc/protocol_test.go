package ipc

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
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
	messages := map[string]Message{
		"hello":  Hello(),
		"result": {Type: "result", RequestID: "mute", Status: "succeeded"},
		"state":  {Type: "state", State: &PlaybackState{Revision: 7, TrackID: 3, Path: "/music/a \"song\" 雪.flac", Title: "雪", HasTrack: true, DesktopLyrics: true, Duration: 120, ConfigDir: "/settings/neoviolet"}},
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
}
