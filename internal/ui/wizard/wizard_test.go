package wizard

import (
	"testing"
)

func TestMatchNerdFont(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"nerd font family", "JetBrainsMono Nerd Font", true},
		{"nerd font family lowercase", "JetBrainsMono nerd font", true},
		{"concatenated nerd font", "JetBrainsMono NerdFont", true},
		{"family with nerd suffix", "Hack Nerd Font Mono", true},
		{"standalone NF", "CaskaydiaCove NF", true},
		{"plain family", "JetBrains Mono", false},
		{"NF inside word", "CaskaydiaCove NNF", false},
		{"nf inside word lowercase", "information", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		if got := MatchNerdFont(tc.in); got != tc.want {
			t.Errorf("MatchNerdFont(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestExtractOSC50Font(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"BEL terminated", []byte("\x1b]50;JetBrainsMono Nerd Font\x07"), "JetBrainsMono Nerd Font"},
		{"ST terminated", []byte("\x1b]50;Monospace\x1b\\"), "Monospace"},
		{"surrounding whitespace trimmed", []byte("\x1b]50; JetBrains Mono \x07"), "JetBrains Mono"},
		{"xterm leading-dash format", []byte("\x1b]50;-*-fixed-medium-r-*-*-18-*\x07"), "fixed"},
		{"xterm format underscores become spaces", []byte("\x1b]50;-*-DejaVu_Sans_Mono-regular-*\x07"), "DejaVu Sans Mono"},
		{"fontconfig style cut at colon", []byte("\x1b]50;JetBrains Mono:style=Regular\x07"), "JetBrains Mono"},
		{"no osc50 sequence", []byte("hello world"), ""},
		{"empty response", []byte("\x1b]50;\x07"), ""},
	}
	for _, tc := range cases {
		if got := extractOSC50Font(tc.in); got != tc.want {
			t.Errorf("extractOSC50Font(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
