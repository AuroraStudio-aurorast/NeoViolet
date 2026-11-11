package config

import (
	"math"
	"runtime"
	"strings"
)

// GUIConfig contains user preferences; window geometry is GUI-local state.
type GUIConfig struct {
	MonospaceFont     string  `json:"monospace_font"`
	FontSize          int     `json:"font_size"`
	Opacity           float64 `json:"opacity"`
	NeovioletPath     string  `json:"neoviolet_path"`
	ZoomViaScroll     bool    `json:"zoom_via_scroll"`
	MenuBar           bool    `json:"menu_bar"`
	SystemDecorations bool    `json:"system_decorations"`
	MinimizeButton    bool    `json:"minimize_button"`
}

// DesktopLyricsConfig configures the overlay independently of TUI lyrics.
type DesktopLyricsConfig struct {
	Enabled         bool    `json:"enabled"`
	FontSize        int     `json:"font_size"`
	Opacity         float64 `json:"opacity"`
	Highlight       string  `json:"highlight"`
	AdditionalLines bool    `json:"additional_lines"`
	NextLine        bool    `json:"next_line"`
	Locked          bool    `json:"locked"`
}

func defaultGUI() GUIConfig {
	font := "monospace"
	switch runtime.GOOS {
	case "darwin":
		font = "Menlo"
	case "windows":
		font = "Consolas"
	}
	return GUIConfig{MonospaceFont: font, FontSize: 14, Opacity: 1, MinimizeButton: true}
}
func defaultDesktopLyrics() DesktopLyricsConfig {
	return DesktopLyricsConfig{Enabled: true, FontSize: 32, Opacity: 1, Highlight: "violet", AdditionalLines: true}
}
func (g *GUIConfig) normalize() {
	g.MonospaceFont = strings.TrimSpace(g.MonospaceFont)
	if g.MonospaceFont == "" {
		g.MonospaceFont = defaultGUI().MonospaceFont
	}
	g.FontSize = clampInt(g.FontSize, 8, 48)
	g.Opacity = clampOpacity(g.Opacity, .1)
	g.NeovioletPath = strings.TrimSpace(g.NeovioletPath)
}
func (d *DesktopLyricsConfig) normalize() {
	d.FontSize = clampInt(d.FontSize, 22, 48)
	d.Opacity = clampOpacity(d.Opacity, .4)
	if d.Highlight != "white" {
		d.Highlight = "violet"
	}
}
func clampOpacity(v, low float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	return math.Max(low, math.Min(v, 1))
}
