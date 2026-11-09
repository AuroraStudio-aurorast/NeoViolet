package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

var saveMu sync.Mutex

// Clone returns a draft that shares no mutable preferences with its source.
func (c Config) Clone() Config {
	c.Lyrics.FormatPriority = slices.Clone(c.Lyrics.FormatPriority)
	c.ProgressBar.Fill = slices.Clone(c.ProgressBar.Fill)
	c.VolumeBar.Fill = slices.Clone(c.VolumeBar.Fill)
	if c.Accent.AutoAccent != nil {
		enabled := *c.Accent.AutoAccent
		c.Accent.AutoAccent = &enabled
	}
	return c
}

// Save serializes writers and replaces the configuration only after a complete
// same-directory temporary file has been written and closed successfully.
func (c *Config) Save() error {
	saveMu.Lock()
	defer saveMu.Unlock()
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// #nosec G301 -- user config directories are intentionally world-readable.
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return fmt.Errorf("create config temporary file: %w", err)
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync config: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	// #nosec G703 -- both paths are resolved application config paths.
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
