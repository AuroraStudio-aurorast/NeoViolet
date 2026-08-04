package fetch

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	bracketRe = regexp.MustCompile(`\([^()]*\)|\[[^\]]*\]`)
	featRe    = regexp.MustCompile(`(?i)\b\s*(feat\.?|ft\.?|featuring)\b.*$`)
	artistSep = regexp.MustCompile(`(?i)\s*(,|;|、|/|&|\x00|\bfeat\.?\b|\bft\.?\b|\bfeaturing\b|\bwith\b)\s*`)
	punctChars = "`~!@#$%^&*()_|+=?;:.,<>{}[]\\/"
)

// Normalize canonicalizes a string for comparison: strips bracket blocks and
// "feat." suffixes, folds case and diacritics, maps punctuation to spaces,
// removes apostrophes, and collapses whitespace.
func Normalize(s string) string {
	s = bracketRe.ReplaceAllString(s, " ")
	s = featRe.ReplaceAllString(s, "")
	s = norm.NFKD.String(s) // decompose; also maps full-width to half-width

	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue // drop combining marks (diacritics)
		}
		b.WriteRune(r)
	}
	s = strings.ToLower(b.String())

	var c strings.Builder
	for _, r := range s {
		switch {
		case strings.ContainsRune(punctChars, r):
			c.WriteRune(' ')
		case r == '\'' || r == '\u2019':
			// drop apostrophes entirely: don't -> dont
		default:
			c.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(c.String()), " ")
}

// SplitArtists splits a multi-artist string on separators, normalizing each
// part, and returns unique non-empty entries. "and" is intentionally kept
// (e.g. "Simon and Garfunkel" stays whole).
func SplitArtists(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range artistSep.Split(s, -1) {
		n := Normalize(part)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// MatchTrack reports whether candidate title matches the requested title
// after normalization.
func MatchTrack(reqTitle, candTitle string) bool {
	return Normalize(reqTitle) == Normalize(candTitle)
}

// MatchArtist matches artist sets: strict requires every requested artist to
// appear in the candidate set; relaxed requires any overlap.
func MatchArtist(reqArtists, candArtists string, strict bool) bool {
	req := SplitArtists(reqArtists)
	cand := SplitArtists(candArtists)
	if len(req) == 0 || len(cand) == 0 {
		return false
	}
	if strict {
		for _, r := range req {
			found := false
			for _, c := range cand {
				if r == c {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	for _, r := range req {
		for _, c := range cand {
			if r == c {
				return true
			}
		}
	}
	return false
}

// Sign builds the normalized cache key for a track: title|artist|album|seconds.
func Sign(title, artist, album string, duration float64) string {
	return strings.Join([]string{
		Normalize(title),
		Normalize(artist),
		Normalize(album),
		strconv.Itoa(int(math.Round(duration))),
	}, "|")
}
