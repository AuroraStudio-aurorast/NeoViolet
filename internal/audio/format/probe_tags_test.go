package format

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func probeID3Tag(items [][2]string) []byte {
	var frames []byte
	for _, item := range items {
		frames = append(frames, item[0]...)
		frames = append(frames, probeBE32(uint32(len(item[1])+1))...) // #nosec G115 -- small test tags.
		frames = append(frames, 0, 0, 3)
		frames = append(frames, item[1]...)
	}
	n := len(frames)
	header := []byte{'I', 'D', '3', 3, 0, 0, byte((n >> 21) & 127), byte((n >> 14) & 127), byte((n >> 7) & 127), byte(n & 127)}
	return append(header, frames...)
}

func probeAPETag(items [][2]string) []byte {
	var body []byte
	for _, item := range items {
		body = append(body, probeLE32(uint32(len(item[1])))...) // #nosec G115 -- small test tags.
		body = append(body, probeLE32(0)...)
		body = append(body, item[0]...)
		body = append(body, 0)
		body = append(body, item[1]...)
	}
	footer := append([]byte("APETAGEX"), probeLE32(2000)...)
	footer = append(footer, probeLE32(uint32(len(body)+32))...) // #nosec G115 -- small test tags.
	footer = append(footer, probeLE32(uint32(len(items)))...)   // #nosec G115 -- small test tags.
	footer = append(footer, make([]byte, 12)...)
	return append(body, footer...)
}

func TestProbeTagPrecedenceMatchesPlayback(t *testing.T) {
	for _, leading := range []bool{false, true} {
		t.Run(map[bool]string{false: "id3v1", true: "id3v2"}[leading], func(t *testing.T) {
			var data []byte
			if leading {
				data = probeID3Tag([][2]string{{"TIT2", "New"}, {"TPE1", "New artist"}, {"TALB", "New album"}})
			}
			data = append(data, 0xff, 0xfb, 0x90, 0)
			data = append(data, probeAPETag([][2]string{{"Title", "Old"}, {"Artist", "Old artist"}, {"Album", "Old album"}})...)
			v1 := make([]byte, 128)
			copy(v1, "TAG")
			copy(v1[3:33], "Legacy")
			copy(v1[33:63], "Legacy artist")
			copy(v1[63:93], "Legacy album")
			data = append(data, v1...)
			path := filepath.Join(t.TempDir(), "conflicting.mp3")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := NewDecoder().Probe(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			want := Metadata{"Legacy", "Legacy artist", "Legacy album"}
			if leading {
				want = Metadata{"New", "New artist", "New album"}
			}
			if got.Metadata != want || NewMetadataReader().Read(path) != want {
				t.Fatalf("probe=%+v playback=%+v want=%+v", got.Metadata, NewMetadataReader().Read(path), want)
			}
		})
	}
}

func TestProbeTrailingTagsFillOnlyMissingFields(t *testing.T) {
	data := probeID3Tag([][2]string{{"TIT2", "New"}, {"TPE1", "New artist"}})
	data = append(data, 0xff, 0xfb, 0x90, 0)
	data = append(data, probeAPETag([][2]string{{"Title", "Old"}, {"Artist", "Old artist"}, {"Album", "Fallback album"}})...)
	got := probeTestFile(t, "missing.mp3", data)
	if got.Metadata != (Metadata{"New", "New artist", "Fallback album"}) {
		t.Fatalf("trailing tags replaced existing fields: %+v", got.Metadata)
	}
}
