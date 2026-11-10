package playlist

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
)

func TestStoreRoundTripPreservesOrderDuplicatesAndReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "playlists.json")
	track := library.Track{Path: filepath.Join(t.TempDir(), "space, 雨.wav"), Title: "雨", Duration: time.Minute}
	list, err := New("Evening, 雨", []library.Track{track, track, {Path: "https://example.test/a?q=x,y", Title: "Radio"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = Save(path, []List{list}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, []List{list}) {
		t.Fatalf("round trip: %#v %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private store: %v %v", info, err)
	}
	snapshot := Clone(got)
	snapshot[0].Tracks[0].Title = "changed"
	if got[0].Tracks[0].Title != "雨" {
		t.Fatal("clone shares tracks")
	}
}
func TestStoreMissingInvalidAndUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlists.json")
	if lists, err := Load(path); err != nil || len(lists) != 0 {
		t.Fatalf("missing: %v %v", lists, err)
	}
	for _, data := range []string{"{", "null", `{ "version": 999, "lists": [] }`, `{ "version": 1, "lists": [{"id":"x","name":"  "}] }`, `{ "version": 1, "lists": [{"id":"x","name":"A"},{"id":"x","name":"B"}] }`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
		// #nosec G304 -- path is scoped to this test temporary directory.
		got, _ := os.ReadFile(path)
		if string(got) != data {
			t.Fatal("load changed unreadable store")
		}
	}
}
func TestStoreRejectsTransientInputsAndControlNames(t *testing.T) {
	for _, name := range []string{"", " \t", "bad\x1b[31m", "new\nline"} {
		if _, err := New(name, nil); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	for _, ref := range []string{"", "-", "stdin", "ftp://example.test/music", filepath.Join(os.TempDir(), "neoviolet-stdin-123.wav"), filepath.Join(os.TempDir(), "neoviolet-123", "cache-123.tmp")} {
		if _, err := New("A", []library.Track{{Path: ref}}); err == nil {
			t.Fatalf("accepted ephemeral reference %q", ref)
		}
	}
	list, err := New(" A ", []library.Track{{Path: "relative.wav"}})
	if err != nil || list.Name != "A" || !filepath.IsAbs(list.Tracks[0].Path) {
		t.Fatalf("normalization: %#v %v", list, err)
	}
}
func TestStoreValidationAndReplaceFailureKeepPreviousData(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "playlists.json")
	list, _ := New("A", nil)
	if err := Save(path, []List{list}); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- path is scoped to this test temporary directory.
	before, _ := os.ReadFile(path)
	bad := Clone([]List{list})
	bad[0].Name = "\n"
	if err := Save(path, bad); err == nil {
		t.Fatal("invalid snapshot was saved")
	}
	// #nosec G304 -- path is scoped to this test temporary directory.
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("validation replaced previous data")
	}
	target := filepath.Join(root, "blocked")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Save(target, []List{list}); err == nil {
		t.Fatal("directory replacement succeeded")
	}
	files, _ := os.ReadDir(root)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".playlists-") {
			t.Fatal("temporary file leaked")
		}
	}
}

func TestStorePreservesSignedURLWithUppercaseScheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlists.json")
	url := "HTTPS://audio.example.test/music.mp3?token=secret"
	list, err := New("Online", []library.Track{{Path: url}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, []List{list}); err != nil {
		t.Fatal(err)
	}
	lists, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := lists[0].Tracks[0].Path; got != url {
		t.Fatalf("stored source = %q, want exact original URL", got)
	}
}
