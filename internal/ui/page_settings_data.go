package ui

type pageSetting struct {
	group                       int
	label, value, initial, kind string
	options                     []string
}

var pageSettingGroups = [...]string{"General", "Playback engines", "Lyrics", "Animation", "Appearance", "Library & Advanced"}

func samplePageSettings() []pageSetting {
	fields := []pageSetting{
		{group: 0, label: "Icon theme", value: "fallback", options: []string{"fallback", "nerd", "emoji"}},
		{group: 0, label: "Auto accent", value: "On", options: []string{"On", "Off"}},
		{group: 0, label: "Default startup volume", value: "70%"},
		{group: 0, label: "Volume step", value: "10%"},
		{group: 0, label: "Seek step", value: "5 s"},
		{group: 0, label: "Track switch", value: "on_success", options: []string{"on_success", "immediate"}},
		{group: 1, label: "MIDI SoundFont", value: "Choose .sf2 file…", kind: "file"},
		{group: 1, label: "Tracker backend", value: "auto", options: []string{"auto", "gotracker", "openmpt"}},
		{group: 2, label: "Show lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 2, label: "Panel mode", value: "auto", options: []string{"auto", "on", "off"}},
		{group: 2, label: "Panel width", value: "auto"},
		{group: 2, label: "Context lines", value: "Fill available rows"},
		{group: 2, label: "Scroll speed", value: "6 ticks"},
		{group: 2, label: "Format priority", value: "9 formats · reorder…", kind: "formats"},
		{group: 2, label: "Online lyrics", value: "On", options: []string{"On", "Off"}},
		{group: 2, label: "Service URL", value: "https://lrclib.net"},
		{group: 2, label: "Request timeout", value: "10 s"},
		{group: 2, label: "Security", value: "strict", options: []string{"strict", "basic"}},
		{group: 2, label: "Allow insecure TLS", value: "Off", options: []string{"Off", "On"}},
		{group: 3, label: "Auto-play sidecar", value: "Off", options: []string{"Off", "On"}},
		{group: 3, label: "Flash check", value: "either", options: []string{"either", "declared", "off"}},
		{group: 3, label: "Low battery warning", value: "20%"},
		{group: 4, label: "Progress characters", value: "▮ ▯"},
		{group: 4, label: "Scaled progress", value: "Off", options: []string{"Off", "On"}},
		{group: 4, label: "Progress percentage", value: "Off", options: []string{"Off", "On"}},
		{group: 4, label: "Volume characters", value: "▰ ▱"},
		{group: 4, label: "Volume width", value: "16 columns"},
		{group: 4, label: "Volume percentage", value: "On", options: []string{"Off", "On"}},
		{group: 4, label: "UI tick rate", value: "30 / s"},
		{group: 5, label: "Music folders", value: "2 folders…", kind: "folders"},
		{group: 5, label: "History limit", value: "50 commands"},
		{group: 5, label: "Message duration", value: "90 ticks"},
	}
	for i := range fields {
		fields[i].initial = fields[i].value
	}
	return fields
}
