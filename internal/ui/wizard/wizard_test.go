package wizard

import (
	"regexp"
	"strings"
	"testing"
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

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func visibleText(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestLogoGradient(t *testing.T) {
	out := logoGradient()
	if out != logoGradient() {
		t.Error("logoGradient is not deterministic across calls")
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("logoGradient should end with a newline")
	}

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != len(logoLines) {
		t.Fatalf("logoGradient produced %d lines, want %d", len(lines), len(logoLines))
	}
	// lipgloss drops colour when no terminal is detected, so compare the
	// visible text rather than the escape sequences.
	for i, want := range logoLines {
		if got := visibleText(lines[i]); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
}
