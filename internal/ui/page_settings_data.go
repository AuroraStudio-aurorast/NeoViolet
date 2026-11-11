package ui

type pageSetting struct {
	group                       int
	key                         string
	label, value, initial, kind string
	savedValue                  string
	options                     []string
}

var pageSettingGroups = [...]string{"Library & Playback", "Lyrics", "Appearance", "Playback Engines", "Animation", "GUI", "Desktop Lyrics", "Advanced"}

func samplePageSettings() []pageSetting {
	fields := []pageSetting{
		{group: 0, key: "library.roots", label: "Music folders", value: "2 folders…", kind: "folders"},
		{group: 0, key: "default_volume", label: "Remembered volume", value: "70%"},
		{group: 0, key: "volume_step", label: "Volume step", value: "10%"},
		{group: 0, key: "seek_step", label: "Seek step", value: "5 s"},
		{group: 0, key: "track_switch_mode", label: "Track switch", value: "on_success", options: []string{"on_success", "immediate"}},
		{group: 0, key: "mediactl.enabled", label: "System integration (restart)", value: "On", options: []string{"On", "Off"}},
		{group: 1, key: "lyrics.enabled", label: "Show lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 1, key: "lyrics.fetch.enabled", label: "Online lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 1, key: "lyrics.panel.mode", label: "Panel mode", value: "auto", options: []string{"auto", "on", "off"}},
		{group: 1, key: "lyrics.format_priority", label: "Format priority", value: "9 formats · reorder…", kind: "formats"},
		{group: 2, key: "icon_theme", label: "Icon theme", value: "fallback", options: []string{"fallback", "nerd", "emoji"}},
		{group: 2, key: "accent.auto_accent", label: "Auto accent", value: "On", options: []string{"On", "Off"}},
		{group: 2, key: "progress_bar.show_percentage", label: "Progress percentage", value: "Off", options: []string{"Off", "On"}},
		{group: 2, key: "volume_bar.show_percentage", label: "Volume percentage", value: "On", options: []string{"Off", "On"}},
		{group: 3, key: "soundfont_path", label: "MIDI SoundFont", value: "Choose .sf2 file…", kind: "file"},
		{group: 3, key: "tracker_backend", label: "Tracker backend", value: "auto", options: []string{"auto", "gotracker", "openmpt"}},
		{group: 4, key: "anim.auto", label: "Auto-play sidecar", value: "Off", options: []string{"Off", "On"}},
		{group: 4, key: "anim.photosensitivity.mode", label: "Flash check", value: "either", options: []string{"either", "declared", "off"}},
		{group: 4, key: "anim.power.warn_below", label: "Low battery warning", value: "20%"},
		{group: 5, key: "gui.monospace_font", label: "Terminal font"},
		{group: 5, key: "gui.font_size", label: "Terminal font size"},
		{group: 5, key: "gui.opacity", label: "Terminal opacity"},
		{group: 5, key: "gui.zoom_via_scroll", label: "Scroll to zoom", options: []string{"Off", "On"}},
		{group: 5, key: "gui.menu_bar", label: "Window menu bar", options: []string{"Off", "On"}},
		{group: 5, key: "gui.minimize_button", label: "Minimize button", options: []string{"Off", "On"}},
		{group: 5, key: "gui.system_decorations", label: "System chrome (reopen GUI)", options: []string{"Off", "On"}},
		{group: 5, key: "gui.neoviolet_path", label: "Player path (next launch)", kind: "file"},
		{group: 6, key: "desktop_lyrics.enabled", label: "Desktop lyrics", options: []string{"On", "Off"}},
		{group: 6, key: "desktop_lyrics.font_size", label: "Desktop font size"},
		{group: 6, key: "desktop_lyrics.opacity", label: "Desktop opacity"},
		{group: 6, key: "desktop_lyrics.highlight", label: "Highlight color", options: []string{"violet", "white"}},
		{group: 6, key: "desktop_lyrics.additional_lines", label: "Translation / extra lines", options: []string{"On", "Off"}},
		{group: 6, key: "desktop_lyrics.next_line", label: "Preview next line", options: []string{"Off", "On"}},
		{group: 6, key: "desktop_lyrics.locked", label: "Lock desktop lyrics", options: []string{"Off", "On"}},
		{group: 7, key: "lyrics.panel.width", label: "Panel width", value: "auto"},
		{group: 7, key: "lyrics.panel.context_lines", label: "Context lines", value: "Fill available rows"},
		{group: 7, key: "lyrics.scroll_speed", label: "Scroll speed", value: "6 ticks"},
		{group: 7, key: "lyrics.fetch.base_url", label: "Service URL", value: "https://lrclib.net"},
		{group: 7, key: "lyrics.fetch.timeout", label: "Request timeout", value: "10 s"},
		{group: 7, key: "lyrics.fetch.security", label: "Security", value: "strict", options: []string{"strict", "basic"}},
		{group: 7, key: "lyrics.fetch.insecure_tls", label: "Allow insecure TLS", value: "Off", options: []string{"Off", "On"}},
		{group: 7, key: "progress_bar.fill", label: "Progress characters", value: "▮ ▯"},
		{group: 7, key: "progress_bar.scaled", label: "Scaled progress", value: "Off", options: []string{"Off", "On"}},
		{group: 7, key: "volume_bar.fill", label: "Volume characters", value: "▰ ▱"},
		{group: 7, key: "volume_bar.width", label: "Volume width", value: "16 columns"},
		{group: 7, key: "tick_rate", label: "UI tick rate", value: "30 / s"},
		{group: 7, key: "command_history.max", label: "History limit", value: "50 commands"},
		{group: 7, key: "error.duration", label: "Message duration", value: "90 ticks"},
	}
	for i := range fields {
		fields[i].initial = fields[i].value
	}
	return fields
}
