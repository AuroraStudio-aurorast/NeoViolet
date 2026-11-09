package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneDoesNotShareMutablePreferences(t *testing.T) {
	original := DefaultConfig()
	draft := original.Clone()
	draft.Lyrics.FormatPriority[0] = "online"
	draft.ProgressBar.Fill[0] = "a"
	draft.VolumeBar.Fill[0] = "b"
	*draft.Accent.AutoAccent = false
	if original.Lyrics.FormatPriority[0] != "embedded" || original.ProgressBar.Fill[0] != "▮" || original.VolumeBar.Fill[0] != "▰" || !original.Accent.IsEnabled() {
		t.Fatal("draft shares applied settings")
	}
}
func TestSaveReplacesCompleteFileAndLeavesNoTemporaryFiles(t *testing.T) {
	SetXDGConfig(true)
	t.Cleanup(func() { SetXDGConfig(false) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := DefaultConfig()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg.SeekStep = 17
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil || loaded.SeekStep != 17 {
		t.Fatalf("replacement missing: %v", err)
	}
	dir, _ := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("temporary files left: %v", entries)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions: %v %v", info, err)
	}
}
