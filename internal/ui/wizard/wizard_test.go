package wizard

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestIconOptionConfigValue(t *testing.T) {
	cases := []struct {
		name string
		in   iconOption
		want string
	}{
		{"nerd", IconNerd, "nerd"},
		{"emoji", IconEmoji, "emoji"},
		{"fallback", IconFallback, "fallback"},
		{"out of range falls back to nerd", iconOption(99), "nerd"},
	}
	for _, tc := range cases {
		if got := tc.in.ConfigValue(); got != tc.want {
			t.Errorf("iconOption(%d).ConfigValue() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Two options sharing a config value would be indistinguishable once written
// to config.json, and the wizard would silently lose a choice.
func TestIconOptionConfigValuesAreDistinct(t *testing.T) {
	seen := make(map[string]iconOption, 3)
	for _, opt := range []iconOption{IconNerd, IconEmoji, IconFallback} {
		v := opt.ConfigValue()
		if prev, dup := seen[v]; dup {
			t.Errorf("iconOption(%d) and iconOption(%d) both map to %q", prev, opt, v)
		}
		seen[v] = opt
	}
}

func TestWizardOptionContrast(t *testing.T) {
	for _, tc := range []struct {
		name string
		dark bool
	}{
		{name: "dark", dark: true},
		{name: "light", dark: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			styles := wizardTheme(tc.dark)
			for _, style := range []lipgloss.Style{
				styles.Focused.Option,
				styles.Blurred.Option,
				styles.Focused.UnselectedOption,
				styles.Blurred.UnselectedOption,
			} {
				gray := color.GrayModel.Convert(style.GetForeground()).(color.Gray)
				if tc.dark && gray.Y < 180 {
					t.Errorf("dark background needs light option text; got brightness %d", gray.Y)
				}
				if !tc.dark && gray.Y > 75 {
					t.Errorf("light background needs dark option text; got brightness %d", gray.Y)
				}
			}
		})
	}
}
