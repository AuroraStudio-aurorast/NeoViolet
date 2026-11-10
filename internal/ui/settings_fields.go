package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/charmbracelet/x/ansi"
)

// Paths follow Config's JSON contract; the draft remains typed Config. Conversion
// happens only while opening/editing fields, never per audio tick or render.

func configValues(cfg config.Config) map[string]any {
	data, _ := json.Marshal(cfg)
	values := map[string]any{}
	_ = json.Unmarshal(data, &values)
	return values
}
func settingValue(values map[string]any, path string) any {
	parts := strings.Split(path, ".")
	node := values
	for _, key := range parts[:len(parts)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			return nil
		}
		node = next
	}
	return node[parts[len(parts)-1]]
}
func setSettingValue(values map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	node := values
	for _, key := range parts[:len(parts)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			return
		}
		node = next
	}
	node[parts[len(parts)-1]] = value
}
func settingsFromConfig(cfg config.Config) []pageSetting {
	fields := samplePageSettings()
	values := configValues(cfg)
	defaults := configValues(config.DefaultConfig())
	for i := range fields {
		if fields[i].key == "" {
			fields[i].value = "Not available yet"
			fields[i].initial = fields[i].value
			continue
		}
		fields[i].value = formatSettingValue(fields[i].key, settingValue(values, fields[i].key))
		if fields[i].key == "library.roots" && cfg.Library.Roots == nil {
			fields[i].value = "0 folders"
		}
		fields[i].savedValue = fields[i].value
		fields[i].initial = formatSettingValue(fields[i].key, settingValue(defaults, fields[i].key))
	}
	return fields
}
func formatSettingValue(key string, value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "On"
		}
		return "Off"
	case float64:
		if key == "default_volume" || key == "volume_step" {
			return strconv.FormatFloat(v*100, 'f', -1, 64) + "%"
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		if v == "" {
			return ""
		}
		return v
	case []any:
		if key == "library.roots" {
			return fmt.Sprintf("%d folders", len(v))
		}
		if key == "lyrics.format_priority" {
			return fmt.Sprintf("%d formats · reorder…", len(v))
		}
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return "On" // nil AutoAccent means enabled.
	}
}

func (p *pageState) setDraftField(index int, text string) error {
	field := p.settings[index]
	if field.key == "" {
		return fmt.Errorf("music folders will be connected with the library")
	}
	value, err := parseSetting(field, text)
	if err != nil {
		return fmt.Errorf("%s: %w", field.label, err)
	}
	return p.applyDraftValue(index, value)
}

// applyDraftValue accepts a value already validated in Update or a filesystem
// worker. Saving SoundFont settings independently revalidates the file.
func (p *pageState) applyDraftValue(index int, value any) error {
	field := p.settings[index]
	values := configValues(p.draft)
	setSettingValue(values, field.key, value)
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	var candidate config.Config
	if err = json.Unmarshal(data, &candidate); err != nil {
		return err
	}
	p.draft = candidate
	canonical := configValues(candidate)
	p.settings[index].value = formatSettingValue(field.key, settingValue(canonical, field.key))
	equal := reflect.DeepEqual(settingValue(canonical, field.key), settingValue(configValues(p.baseline), field.key))
	if field.key == "library.roots" {
		equal = slices.Equal(candidate.Library.Roots, p.baseline.Library.Roots)
	}
	if field.key == "accent.auto_accent" {
		equal = candidate.Accent.IsEnabled() == p.baseline.Accent.IsEnabled()
	}
	if equal {
		delete(p.changed, field.key)
	} else {
		p.changed[field.key] = true
	}
	p.dirty = len(p.changed) > 0
	return nil
}
func parseSetting(f pageSetting, text string) (any, error) {
	text = strings.TrimSpace(text)
	if len(f.options) > 0 {
		if !slices.Contains(f.options, text) {
			return nil, fmt.Errorf("choose %s", strings.Join(f.options, ", "))
		}
		if text == "On" || text == "Off" {
			return text == "On", nil
		}
		return text, nil
	}
	switch f.key {
	case "default_volume", "volume_step":
		percent := strings.HasSuffix(text, "%")
		n, err := strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
		if percent {
			n /= 100
		}
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1 || f.key == "volume_step" && n == 0 {
			return nil, fmt.Errorf("use 0–1 or a percentage; step must be positive")
		}
		if f.key == "default_volume" {
			n = math.Round(n*100) / 100
		}
		return n, nil
	case "lyrics.fetch.base_url":
		if text == "" {
			return "", nil
		}
		u, err := url.Parse(text)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("use an HTTP(S) URL or leave empty for default")
		}
		return strings.TrimRight(text, "/"), nil
	case "soundfont_path":
		if text == "" {
			return "", nil
		}
		text = expandTilde(text)
		if !strings.EqualFold(filepath.Ext(text), ".sf2") {
			return nil, fmt.Errorf("choose a .sf2 file")
		}
		// #nosec G304 -- this is the user-entered SoundFont path validated by the settings editor.
		file, err := os.Open(text)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("choose a regular file")
		}
		return filepath.Abs(expandTilde(text))
	case "progress_bar.fill", "volume_bar.fill":
		chars := strings.Fields(text)
		if len(chars) != 2 {
			return nil, fmt.Errorf("enter two single-cell characters separated by a space")
		}
		for _, s := range chars {
			if ansi.StringWidth(s) != 1 || utf8.RuneCountInString(s) != 1 {
				return nil, fmt.Errorf("each character must occupy one terminal cell")
			}
		}
		return chars, nil
	case "lyrics.panel.width":
		if text == "auto" {
			return "auto", nil
		}
	}
	low, high := 0, 10000
	switch f.key {
	case "tick_rate":
		low, high = 1, 120
	case "seek_step":
		low, high = 1, 3600
	case "lyrics.scroll_speed":
		low, high = 1, 3600
	case "lyrics.fetch.timeout":
		low, high = 1, 300
	case "lyrics.panel.width":
		low, high = config.MinPanelWidth, config.MaxPanelWidth
	case "lyrics.panel.context_lines":
		high = config.MaxPanelContextLines
	case "anim.power.warn_below":
		high = 100
	case "volume_bar.width":
		low, high = 1, 40
	case "command_history.max":
		low, high = 1, 10000
	case "error.duration":
		low, high = 1, 36000
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < low || n > high {
		return nil, fmt.Errorf("enter an integer from %d to %d", low, high)
	}
	return n, nil
}

func (p *pageState) setFormatDraft(order []string) {
	p.draft.Lyrics.FormatPriority = slices.Clone(order)
	for i := range p.settings {
		if p.settings[i].key == "lyrics.format_priority" {
			p.settings[i].value = fmt.Sprintf("%d formats · reorder…", len(order))
		}
	}
	if slices.Equal(order, p.baseFormats) {
		delete(p.changed, "lyrics.format_priority")
	} else {
		p.changed["lyrics.format_priority"] = true
	}
	p.dirty = len(p.changed) > 0
}
func (p *pageState) resetSetting(index int) error {
	if p.settings[index].key == "library.roots" {
		return p.applyDraftValue(index, []string{})
	}
	if p.settings[index].key == "lyrics.format_priority" {
		p.setFormatDraft(config.DefaultConfig().Lyrics.FormatPriority)
		formats := slices.Clone(p.draft.Lyrics.FormatPriority)
		for _, name := range p.formats {
			if !slices.Contains(formats, name) {
				formats = append(formats, name)
			}
		}
		p.formats = formats
		p.enabled = make([]bool, len(p.formats))
		for i, name := range p.formats {
			p.enabled[i] = slices.Contains(p.draft.Lyrics.FormatPriority, name)
		}
		return nil
	}
	return p.setDraftField(index, p.settings[index].initial)
}
