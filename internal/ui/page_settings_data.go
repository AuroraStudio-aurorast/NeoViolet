package ui

type pageSetting struct {
	group                       int
	key                         string
	label, value, initial, kind string
	savedValue                  string
	options                     []string
}

var pageSettingGroups = [...]string{"General", "Playback engines", "Lyrics", "Animation", "Appearance", "Library & Advanced"}

func samplePageSettings() []pageSetting {
	fields := []pageSetting{
		{group: 0, key: "icon_theme", label: "Icon theme", value: "fallback", options: []string{"fallback", "nerd", "emoji"}},
		{group: 0, key: "accent.auto_accent", label: "Auto accent", value: "On", options: []string{"On", "Off"}},
		{group: 0, key: "default_volume", label: "Default startup volume", value: "70%"},
		{group: 0, key: "volume_step", label: "Volume step", value: "10%"},
		{group: 0, key: "seek_step", label: "Seek step", value: "5 s"},
		{group: 0, key: "track_switch_mode", label: "Track switch", value: "on_success", options: []string{"on_success", "immediate"}},
		{group: 1, key: "soundfont_path", label: "MIDI SoundFont", value: "Choose .sf2 file…", kind: "file"},
		{group: 1, key: "tracker_backend", label: "Tracker backend", value: "auto", options: []string{"auto", "gotracker", "openmpt"}},
		{group: 2, key: "lyrics.enabled", label: "Show lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 2, key: "lyrics.panel.mode", label: "Panel mode", value: "auto", options: []string{"auto", "on", "off"}},
		{group: 2, key: "lyrics.panel.width", label: "Panel width", value: "auto"},
		{group: 2, key: "lyrics.panel.context_lines", label: "Context lines", value: "Fill available rows"},
		{group: 2, key: "lyrics.scroll_speed", label: "Scroll speed", value: "6 ticks"},
		{group: 2, key: "lyrics.format_priority", label: "Format priority", value: "9 formats · reorder…", kind: "formats"},
		{group: 2, key: "lyrics.fetch.enabled", label: "Online lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 2, key: "lyrics.fetch.base_url", label: "Service URL", value: "https://lrclib.net"},
		{group: 2, key: "lyrics.fetch.timeout", label: "Request timeout", value: "10 s"},
		{group: 2, key: "lyrics.fetch.security", label: "Security", value: "strict", options: []string{"strict", "basic"}},
		{group: 2, key: "lyrics.fetch.insecure_tls", label: "Allow insecure TLS", value: "Off", options: []string{"Off", "On"}},
		{group: 3, key: "anim.auto", label: "Auto-play sidecar", value: "Off", options: []string{"Off", "On"}},
		{group: 3, key: "anim.photosensitivity.mode", label: "Flash check", value: "either", options: []string{"either", "declared", "off"}},
		{group: 3, key: "anim.power.warn_below", label: "Low battery warning", value: "20%"},
		{group: 4, key: "progress_bar.fill", label: "Progress characters", value: "▮ ▯"},
		{group: 4, key: "progress_bar.scaled", label: "Scaled progress", value: "Off", options: []string{"Off", "On"}},
		{group: 4, key: "progress_bar.show_percentage", label: "Progress percentage", value: "Off", options: []string{"Off", "On"}},
		{group: 4, key: "volume_bar.fill", label: "Volume characters", value: "▰ ▱"},
		{group: 4, key: "volume_bar.width", label: "Volume width", value: "16 columns"},
		{group: 4, key: "volume_bar.show_percentage", label: "Volume percentage", value: "On", options: []string{"Off", "On"}},
		{group: 4, key: "tick_rate", label: "UI tick rate", value: "30 / s"},
		{group: 5, key: "", label: "Music folders", value: "2 folders…", kind: "folders"},
		{group: 5, key: "command_history.max", label: "History limit", value: "50 commands"},
		{group: 5, key: "error.duration", label: "Message duration", value: "90 ticks"},
	}
	for i := range fields {
		fields[i].initial = fields[i].value
	}
	return fields
}
