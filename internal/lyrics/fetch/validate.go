package fetch

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

const (
	maxLyricLines   = 2000
	maxLyricLineLen = 500
	maxDurationSec  = 3600.0
	durationTolSec  = 5.0
	// lastLineSlackSec is how far past the track duration the final timestamp
	// may sit before the lyrics are rejected.
	lastLineSlackSec = 10.0
)

// ErrInvalidPayload wraps all content validation failures.
var ErrInvalidPayload = errors.New("fetch: invalid payload")

// ValidateOpts carries the validation policy for a fetched track.
type ValidateOpts struct {
	Basic       bool    // basic tier skips most content checks
	ReqDuration float64 // requested duration in seconds; 0 disables drift check
}

// ValidateTrack checks an LRCLIB track response (layer 3).
func ValidateTrack(t Track, opts ValidateOpts) error {
	if t.ID <= 0 || t.TrackName == "" || t.ArtistName == "" {
		return fmt.Errorf("%w: missing id/trackName/artistName", ErrInvalidPayload)
	}
	if t.Duration < 1 || t.Duration > maxDurationSec {
		return fmt.Errorf("%w: duration %v out of range", ErrInvalidPayload, t.Duration)
	}
	if !opts.Basic && opts.ReqDuration > 0 && abs(t.Duration-opts.ReqDuration) > durationTolSec {
		return fmt.Errorf("%w: duration %v deviates from requested %v", ErrInvalidPayload, t.Duration, opts.ReqDuration)
	}

	clean := cleanControl(t.SyncedLyrics)
	if strings.TrimSpace(clean) == "" {
		return fmt.Errorf("%w: empty syncedLyrics", ErrInvalidPayload)
	}
	if opts.Basic {
		return nil
	}

	parsed, err := lyrics.ParseLRC(strings.NewReader(clean))
	if err != nil || len(parsed.Lines) == 0 {
		return fmt.Errorf("%w: syncedLyrics has no valid lyric lines", ErrInvalidPayload)
	}
	if len(parsed.Lines) > maxLyricLines {
		return fmt.Errorf("%w: too many lines (%d)", ErrInvalidPayload, len(parsed.Lines))
	}
	for _, ln := range parsed.Lines {
		if len([]rune(ln.Text)) > maxLyricLineLen {
			return fmt.Errorf("%w: line too long", ErrInvalidPayload)
		}
	}
	if last := parsed.Lines[len(parsed.Lines)-1]; last.Time > time.Duration((t.Duration+lastLineSlackSec)*float64(time.Second)) {
		return fmt.Errorf("%w: last timestamp %v beyond duration", ErrInvalidPayload, last.Time)
	}
	return nil
}

// IsInstrumental reports whether the track is marked instrumental with no lyrics.
func IsInstrumental(t Track) bool {
	return t.Instrumental && strings.TrimSpace(t.SyncedLyrics) == ""
}

// cleanControl strips NUL and other control characters except \n and \t.
func cleanControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
