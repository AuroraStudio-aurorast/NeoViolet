package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type LyricsConfig struct {
	Enabled     bool `json:"enabled"`
	ScrollSpeed int  `json:"scroll_speed"`
}

type ProgressBarConfig struct {
	Fill            []string `json:"fill"`
	Scaled          bool     `json:"scaled"`
	ShowPercentage  bool     `json:"show_percentage"`
}

type VolumeBarConfig struct {
	Width           int      `json:"width"`
	ShowPercentage  bool     `json:"show_percentage"`
	Fill            []string `json:"fill"`
}

type CommandHistoryConfig struct {
	Max int `json:"max"`
}

type ErrorConfig struct {
	Duration int `json:"duration"`
}

type Config struct {
	IconTheme     string              `json:"icon_theme"`
	DefaultVolume float64             `json:"default_volume"`
	VolumeStep    float64             `json:"volume_step"`
	SeekStep      int                 `json:"seek_step"`
	TickRate      int                 `json:"tick_rate"`
	PlaybackDevice string             `json:"playback_device"`
	SampleRate    int                 `json:"sample_rate"`
	BufferSize    int                 `json:"buffer_size"`
	Crossfade     int                 `json:"crossfade"`
	ReplayGain    bool                `json:"replay_gain"`
	SoundfontPath string              `json:"soundfont_path"`
	Lyrics        LyricsConfig        `json:"lyrics"`
	ProgressBar   ProgressBarConfig    `json:"progress_bar"`
	VolumeBar     VolumeBarConfig     `json:"volume_bar"`
	CommandHistory CommandHistoryConfig `json:"command_history"`
	Error         ErrorConfig         `json:"error"`
}

func defaultConfig() Config {
	return Config{
		IconTheme:      "nerd",
		DefaultVolume:  1.0,
		VolumeStep:     0.1,
		SeekStep:       5,
		TickRate:       30,
		PlaybackDevice: "default",
		SampleRate:     44100,
		BufferSize:     100,
		Crossfade:      0,
		ReplayGain:     false,
		SoundfontPath:  "",
		Lyrics: LyricsConfig{
			Enabled:     true,
			ScrollSpeed: 6,
		},
		VolumeBar: VolumeBarConfig{
			Width:          16,
			ShowPercentage: true,
			Fill:           []string{"▰", "▱"},
		},
		ProgressBar: ProgressBarConfig{
			Scaled:          true,
			ShowPercentage:  false,
			Fill:            []string{"▮", "▯"},
		},
		CommandHistory: CommandHistoryConfig{
			Max: 50,
		},
		Error: ErrorConfig{
			Duration: 90,
		},
	}
}

func configPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	cfg := defaultConfig()
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
			return &cfg, nil
		}
		return &cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &cfg, fmt.Errorf("parse config: %w", err)
	}
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
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
