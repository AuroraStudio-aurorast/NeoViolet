// Package config loads and saves the NeoViolet user configuration and
// exposes helper paths for the config and cache directories.
package config

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// useXDG controls whether config is stored at the XDG standard path
// (~/.config/neoviolet/config.json) instead of next to the executable.
// Must be set via SetXDGConfig before Load/Save/Exists.
var useXDG atomic.Bool

// SetXDGConfig controls whether the XDG standard config path is used.
// Must be called before Load, Save, or Exists.
func SetXDGConfig(enabled bool) { useXDG.Store(enabled) }

// LyricsConfig holds the UI lyrics display and fetch settings.
type LyricsConfig struct {
	Enabled        bool              `json:"enabled"`
	ScrollSpeed    int               `json:"scroll_speed"`
	FormatPriority []string          `json:"format_priority"`
	Fetch          LyricsFetchConfig `json:"fetch"`
	Panel          LyricsPanelConfig `json:"panel"`
}

// LyricsPanelConfig controls the right-hand lyrics panel (the "panel" display
// mode). Width is the total box width including its border and padding.
type LyricsPanelConfig struct {
	Mode  string `json:"mode"`  // PanelModeAuto | PanelModeOn | PanelModeOff
	Width int    `json:"width"` // MinPanelWidth..MaxPanelWidth
	// ContextLines caps how many lyric lines the panel shows on each side of
	// the current one. 0 fills every row the box has with lyrics; 1..MaxPanelContextLines
	// gives a deliberately compact window.
	ContextLines int `json:"context_lines"` // 0 = fill, else 1..MaxPanelContextLines
}

// Panel display modes. PanelModeAuto shows the panel only when the content
// area can keep its minimum width.
const (
	PanelModeAuto = "auto"
	PanelModeOn   = "on"
	PanelModeOff  = "off"

	DefaultPanelWidth = 32 // inner width 28 columns
	MinPanelWidth     = 24 // inner width 20 columns
	MaxPanelWidth     = 48

	// DefaultPanelContextLines is 0, i.e. no cap: the panel fills its box with
	// as many lyric lines as fit.
	DefaultPanelContextLines = 0
	MaxPanelContextLines     = 10
)

// LyricsFetchConfig controls online lyrics fetching (LRCLIB-compatible API).
type LyricsFetchConfig struct {
	Enabled     bool   `json:"enabled"`      // online fetch master switch (required for auto-fetch)
	BaseURL     string `json:"base_url"`     // API root; empty = DefaultBaseURL
	Timeout     int    `json:"timeout"`      // per-request timeout in seconds
	Security    string `json:"security"`     // "strict" | "basic"
	InsecureTLS bool   `json:"insecure_tls"` // skip TLS certificate verification
}

// Default values for online lyrics fetching.
const (
	DefaultBaseURL      = "https://lrclib.net"
	DefaultFetchTimeout = 10
	DefaultSecurity     = "strict"
)

// ProgressBarConfig controls the progress bar rendering.
type ProgressBarConfig struct {
	Fill           []string `json:"fill"`
	Scaled         bool     `json:"scaled"`
	ShowPercentage bool     `json:"show_percentage"`
}

// VolumeBarConfig controls the volume bar rendering.
type VolumeBarConfig struct {
	Width          int      `json:"width"`
	ShowPercentage bool     `json:"show_percentage"`
	Fill           []string `json:"fill"`
}

// CommandHistoryConfig limits the command history size.
type CommandHistoryConfig struct {
	Max int `json:"max"`
}

// ErrorConfig controls error message display duration.
type ErrorConfig struct {
	Duration int `json:"duration"`
}

// AccentConfig controls terminal accent color auto-detection.
type AccentConfig struct {
	AutoAccent *bool `json:"auto_accent"`
}

// IsEnabled reports whether accent auto-detection is enabled.
func (a AccentConfig) IsEnabled() bool {
	if a.AutoAccent == nil {
		return true
	}
	return *a.AutoAccent
}

// Config is the root user configuration, persisted as JSON.
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

// Normalize validates and repairs fields in place, returning true when any
// value changed (so the caller can decide to re-save).
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

	normalizeLyricsPanel(&c.Lyrics.Panel)

	f := &c.Lyrics.Fetch

	// base_url: accept only http/https with a non-empty host; strip trailing slash.
	// Invalid values fall back to empty (meaning DefaultBaseURL) so a bad config
	// never blocks the app.
	if f.BaseURL != "" {
		u, err := url.Parse(f.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			logger.Warn("invalid lyrics fetch base_url, falling back to default", "url", f.BaseURL)
			f.BaseURL = ""
		} else {
			f.BaseURL = strings.TrimRight(f.BaseURL, "/")
		}
	}

	// timeout: <=0 falls back to default.
	if f.Timeout <= 0 {
		f.Timeout = DefaultFetchTimeout
	}

	// security: only "strict"/"basic" are accepted.
	if f.Security != "strict" && f.Security != "basic" {
		f.Security = DefaultSecurity
	}

	if f.InsecureTLS {
		logger.Warn("lyrics fetch: TLS certificate verification disabled")
	}

	// Plain http (non-localhost) is not recommended.
	if u, err := url.Parse(f.BaseURL); err == nil && u.Scheme == "http" && !isLocalHost(u.Hostname()) {
		logger.Warn("lyrics fetch over plain http (not recommended)", "url", f.BaseURL)
	}

	// A private/local base_url may be an accidental SSRF target; warn but do not
	// block, since self-hosted instances may legitimately live on a LAN.
	if u, err := url.Parse(f.BaseURL); err == nil && isPrivateHost(u.Hostname()) {
		logger.Warn("lyrics fetch base_url points to private/local address", "url", f.BaseURL)
	}

	volumeChanged := c.DefaultVolume != orig.DefaultVolume
	panelChanged := c.Lyrics.Panel != orig.Lyrics.Panel
	fetchChanged := f.Enabled != orig.Lyrics.Fetch.Enabled ||
		f.BaseURL != orig.Lyrics.Fetch.BaseURL ||
		f.Timeout != orig.Lyrics.Fetch.Timeout ||
		f.Security != orig.Lyrics.Fetch.Security ||
		f.InsecureTLS != orig.Lyrics.Fetch.InsecureTLS
	return volumeChanged || fetchChanged || panelChanged
}

// normalizeLyricsPanel clamps the panel settings into their documented ranges.
// Unknown modes fall back to auto so a hand-edited config never selects an
// undefined layout.
func normalizeLyricsPanel(p *LyricsPanelConfig) {
	switch p.Mode {
	case PanelModeAuto, PanelModeOn, PanelModeOff:
	default:
		p.Mode = PanelModeAuto
	}
	p.Width = clampInt(p.Width, MinPanelWidth, MaxPanelWidth)
	p.ContextLines = clampInt(p.ContextLines, 0, MaxPanelContextLines)
}

// clampInt constrains v to [lo, hi]. Values at or below lo take lo because an
// absent JSON key decodes to zero, which is never a usable size here.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// isLocalHost reports whether host is localhost/127.x/::1 (plain http allowed).
func isLocalHost(host string) bool {
	return host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}

// isPrivateHost reports whether host is a private/loopback/link-local IP or a
// local-domain suffix. Used for warnings only; requests are never blocked.
func isPrivateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suf := range []string{".local", ".internal", ".lan", ".home", ".localhost"} {
		if strings.HasSuffix(host, suf) {
			return true
		}
	}
	return host == "localhost"
}

// DefaultConfig returns the built-in default configuration.
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
			FormatPriority: []string{"embedded", "lrc", "ttml", "qrc", "yrc", "eslrc", "lys", "online"},
			Fetch: LyricsFetchConfig{
				Enabled:  true,
				Timeout:  DefaultFetchTimeout,
				Security: DefaultSecurity,
			},
			Panel: LyricsPanelConfig{
				Mode:         PanelModeAuto,
				Width:        DefaultPanelWidth,
				ContextLines: DefaultPanelContextLines,
			},
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
			AutoAccent: func() *bool { v := true; return &v }(),
		},
	}
}

// Exists reports whether a config.json already exists on disk.
func Exists() bool {
	path, err := configPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Dir returns the directory holding config.json.
func Dir() (string, error) {
	if useXDG.Load() {
		xdgHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgHome == "" || !filepath.IsAbs(xdgHome) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("get home dir for XDG config: %w", err)
			}
			xdgHome = filepath.Join(home, ".config")
		}
		return filepath.Join(xdgHome, "neoviolet"), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("get executable path: %w", err)
	}
	return filepath.Dir(exe), nil
}

func configPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// CacheDir returns the directory for cached data (e.g. fetched lyrics).
// XDG mode: $XDG_CACHE_HOME/neoviolet/lyrics, falling back to ~/.cache/neoviolet/lyrics.
// Non-XDG:  <Dir()>/caches/lyrics.
func CacheDir() (string, error) {
	if useXDG.Load() {
		xdgCache := os.Getenv("XDG_CACHE_HOME")
		if xdgCache == "" || !filepath.IsAbs(xdgCache) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("get home dir for XDG cache: %w", err)
			}
			xdgCache = filepath.Join(home, ".cache")
		}
		return filepath.Join(xdgCache, "neoviolet", "lyrics"), nil
	}

	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "caches", "lyrics"), nil
}

// Load reads the user configuration, creating it with defaults on first run.
func Load() (*Config, error) {
	cfg := DefaultConfig()
	path, err := configPath()
	if err != nil {
		return &cfg, nil
	}
	// #nosec G304 -- path is the user's own config.json under the config dir.
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

	// Validate SoundfontPath if configured.
	if cfg.SoundfontPath != "" {
		if _, err := os.Stat(cfg.SoundfontPath); err != nil {
			logger.Warn("SoundfontPath not found", "path", cfg.SoundfontPath, "err", err)
		}
	}

	if cfg.Normalize() {
		logger.Info("Config auto-repaired")
		if saveErr := cfg.Save(); saveErr != nil {
			logger.Warn("Failed to save auto-repaired config", "err", saveErr)
		}
	}
	logger.Info("Config loaded", "path", path, "iconTheme", cfg.IconTheme)
	return &cfg, nil
}

// Save writes the configuration to disk (creating parent dirs as needed).
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	// #nosec G301 -- config dir is intentionally world-readable (0755).
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
