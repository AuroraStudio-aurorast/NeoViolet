package fetch

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		// case + whitespace
		{"The Quick  BROWN fox", "the quick brown fox"},
		// full-width to half-width (NFKD compat mapping)
		{"Ｆｅｅｌ", "feel"},
		// diacritics stripped
		{"Café", "cafe"},
		{"Für Elise", "fur elise"},
		{"Beyoncé", "beyonce"},
		// apostrophes dropped
		{"don't", "dont"},
		{"I’m", "im"},
		// punctuation to spaces
		{"A&W", "a w"},
		{"x*5", "x 5"},
		// bracket blocks stripped
		{"Song (Live)", "song"},
		{"Song [Remastered]", "song"},
		// feat. suffix stripped
		{"Song feat. Artist", "song"},
		{"Song featuring Artist", "song"},
		{"Song ft. Artist", "song"},
		// strip then collapse
		{"Song  (Live)  Mix", "song mix"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitArtists(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"Taylor Swift; Bon Iver", []string{"taylor swift", "bon iver"}},
		{"Taylor Swift, Bon Iver", []string{"taylor swift", "bon iver"}},
		{"Taylor Swift / Bon Iver", []string{"taylor swift", "bon iver"}},
		{"Taylor Swift feat. Bon Iver", []string{"taylor swift", "bon iver"}},
		{"Taylor Swift featuring Bon Iver", []string{"taylor swift", "bon iver"}},
		{"Taylor Swift ft. Bon Iver", []string{"taylor swift", "bon iver"}},
		// & is split (D13, real-data verified)
		{"Porter Robinson & Madeon", []string{"porter robinson", "madeon"}},
		// NUL separator seen in real LRCLIB data
		{"Porter Robinson\x00Madeon", []string{"porter robinson", "madeon"}},
		// and is kept whole
		{"Simon and Garfunkel", []string{"simon and garfunkel"}},
		// dupes removed
		{"A, A, B", []string{"a", "b"}},
		// single artist
		{"  Queen  ", []string{"queen"}},
		// empty
		{"", nil},
	}
	for _, tt := range tests {
		got := SplitArtists(tt.in)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitArtists(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestMatchTrack(t *testing.T) {
	if !MatchTrack("Shelter", "Shelter") {
		t.Error("MatchTrack identical = false")
	}
	if !MatchTrack("Shelter", "shelter (Live)") {
		t.Error("MatchTrack bracket variant = false")
	}
	if MatchTrack("Shelter", "Shelter Me") {
		t.Error("MatchTrack different title = true")
	}
}

func TestMatchArtistStrict(t *testing.T) {
	// requested subset of candidate
	if !MatchArtist("Porter Robinson", "Porter Robinson & Madeon", true) {
		t.Error("strict: subset should pass")
	}
	if !MatchArtist("Porter Robinson; Madeon", "Porter Robinson & Madeon", true) {
		t.Error("strict: two requested artists should pass")
	}
	if MatchArtist("Madeon; Daft Punk", "Porter Robinson & Madeon", true) {
		t.Error("strict: missing requested artist should fail")
	}
	if !MatchArtist("Porter Robinson / Madeon", "Porter Robinson & Madeon", true) {
		t.Error("strict: slash-separated request should match ampersand candidate")
	}
	if MatchArtist("", "Porter Robinson", true) {
		t.Error("strict: empty request should fail")
	}
}

func TestMatchArtistRelaxed(t *testing.T) {
	if !MatchArtist("Madeon", "Porter Robinson & Madeon", false) {
		t.Error("relaxed: overlap should pass")
	}
	if MatchArtist("Daft Punk", "Porter Robinson & Madeon", false) {
		t.Error("relaxed: no overlap should fail")
	}
}

func TestMatchArtistEndToEndShelter(t *testing.T) {
	req := "Porter Robinson/Madeon" // local tag style
	for _, cand := range []string{
		"Porter Robinson & Madeon",
		"Porter Robinson, Madeon",
		"Porter Robinson; Madeon",
		"Porter Robinson\x00Madeon",
	} {
		if !MatchArtist(req, cand, true) {
			t.Errorf("end-to-end: req %q vs cand %q should pass", req, cand)
		}
	}
	if MatchArtist(req, "Simon & Garfunkel", true) {
		t.Error("end-to-end: unrelated artist should fail")
	}
}

func TestSign(t *testing.T) {
	if Sign("Shelter", "Porter Robinson/Madeon", "Shelter", 219.01) !=
		Sign("shelter", "Porter Robinson & Madeon", "shelter", 219.4) {
		t.Error("Sign() should normalize equal tracks to the same key")
	}
	if Sign("A", "B", "C", 100) == Sign("A", "B", "C", 101) {
		t.Error("Sign() should differ when duration seconds differ")
	}
}
