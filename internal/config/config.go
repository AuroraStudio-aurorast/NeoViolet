package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// useXDG controls whether config is stored at the XDG standard path
// (~/.config/neoviolet/config.json) instead of next to the executable.
// Must be set via SetXDGConfig before Load/Save/ConfigExists.
var useXDG bool

// SetXDGConfig controls whether the XDG standard config path is used.
// Must be called before Load, Save, or ConfigExists.
func SetXDGConfig(enabled bool) { useXDG = enabled }

type LyricsConfig struct {
	Enabled        bool     `json:"enabled"`
	ScrollSpeed    int      `json:"scroll_speed"`
	FormatPriority []string `json:"format_priority"`
}

type ProgressBarConfig struct {
	Fill           []string `json:"fill"`
	Scaled         bool     `json:"scaled"`
	ShowPercentage bool     `json:"show_percentage"`
}

type VolumeBarConfig struct {
	Width          int      `json:"width"`
	ShowPercentage bool     `json:"show_percentage"`
	Fill           []string `json:"fill"`
}

type CommandHistoryConfig struct {
	Max int `json:"max"`
}

type ErrorConfig struct {
	Duration int `json:"duration"`
}

type AccentConfig struct {
	AutoAccent *bool `json:"auto_accent"`
}

func (a AccentConfig) IsEnabled() bool {
	if a.AutoAccent == nil {
		return true
	}
	return *a.AutoAccent
}

type Config struct {
	IconTheme      string               `json:"icon_theme"`
	DefaultVolume  float64              `json:"default_volume"`
	VolumeStep     float64              `json:"volume_step"`
	SeekStep       int                  `json:"seek_step"`
	TickRate       int                  `json:"tick_rate"`
	SoundfontPath  string               `json:"soundfont_path"`
	TrackerBackend string               `json:"tracker_backend"`
	Lyrics         LyricsConfig         `json:"lyrics"`
	ProgressBar    ProgressBarConfig    `json:"progress_bar"`
	VolumeBar      VolumeBarConfig      `json:"volume_bar"`
	CommandHistory CommandHistoryConfig `json:"command_history"`
	Error          ErrorConfig          `json:"error"`
	Accent         AccentConfig         `json:"accent"`
}

func (c *Config) Normalize() bool {
	orig := *c
	if c.DefaultVolume < 0 || c.DefaultVolume > 1.0 {
		if c.DefaultVolume < 0 {
			c.DefaultVolume = 0
		} else {
			c.DefaultVolume = 1.0
		}
	}
	normalized := math.Round(c.DefaultVolume*100) / 100
	if normalized != c.DefaultVolume {
		c.DefaultVolume = normalized
	}
	return c.DefaultVolume != orig.DefaultVolume
}

func boolPtr(v bool) *bool { return &v }

func DefaultConfig() Config {
	return Config{
		IconTheme:      "nerd",
		DefaultVolume:  1.0,
		VolumeStep:     0.1,
		SeekStep:       5,
		TickRate:       30,
		SoundfontPath:  "",
		TrackerBackend: "auto",
		Lyrics: LyricsConfig{
			Enabled:        true,
			ScrollSpeed:    6,
			FormatPriority: []string{"embedded", "lrc", "ttml", "qrc", "yrc", "eslrc", "lys"},
		},
		VolumeBar: VolumeBarConfig{
			Width:          16,
			ShowPercentage: true,
			Fill:           []string{"▰", "▱"},
		},
		ProgressBar: ProgressBarConfig{
			Scaled:         true,
			ShowPercentage: false,
			Fill:           []string{"▮", "▯"},
		},
		CommandHistory: CommandHistoryConfig{
			Max: 50,
		},
		Error: ErrorConfig{
			Duration: 90,
		},
		Accent: AccentConfig{
			AutoAccent: boolPtr(true),
		},
	}
}

func ConfigExists() bool {
	path, err := configPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func configPath() (string, error) {
	if useXDG {
		xdgHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgHome == "" || !filepath.IsAbs(xdgHome) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("get home dir for XDG config: %w", err)
			}
			xdgHome = filepath.Join(home, ".config")
		}
		return filepath.Join(xdgHome, "neoviolet", "config.json"), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	path, err := configPath()
	if err != nil {
		return &cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if saveErr := cfg.Save(); saveErr != nil {
				return &cfg, nil
			}
			logger.Info("Config created", "path", path)
			return &cfg, nil
		}
		return &cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &cfg, fmt.Errorf("parse config: %w", err)
	}

	switch cfg.TrackerBackend {
	case "auto", "openmpt", "gotracker":
	default:
		cfg.TrackerBackend = "auto"
	}

	if cfg.Normalize() {
		logger.Info("Volume auto-repaired", "default_volume", cfg.DefaultVolume)
		if saveErr := cfg.Save(); saveErr != nil {
			logger.Warn("Failed to save auto-repaired config", "err", saveErr)
		}
	}
	logger.Info("Config loaded", "path", path, "iconTheme", cfg.IconTheme)
	return &cfg, nil
}

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
