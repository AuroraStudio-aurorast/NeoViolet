package fetch

import (
	"errors"
	"strings"
	"testing"
)

func validTrack() Track {
	return Track{
		ID:           1,
		TrackName:    "Shelter",
		ArtistName:   "Porter Robinson & Madeon",
		AlbumName:    "Shelter",
		Duration:     219,
		SyncedLyrics: "[00:01.00]line one\n[00:02.00]line two",
	}
}

func TestValidateTrackOK(t *testing.T) {
	if err := ValidateTrack(validTrack(), ValidateOpts{ReqDuration: 219}); err != nil {
		t.Fatalf("ValidateTrack() error: %v", err)
	}
}

func TestValidateTrackRequiredFields(t *testing.T) {
	tr := validTrack()
	for _, mutate := range []func(*Track){
		func(t *Track) { t.ID = 0 },
		func(t *Track) { t.TrackName = "" },
		func(t *Track) { t.ArtistName = "" },
	} {
		tr := tr
		mutate(&tr)
		if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
			t.Errorf("ValidateTrack() error = %v, want ErrInvalidPayload", err)
		}
	}
}

func TestValidateTrackDurationRange(t *testing.T) {
	tr := validTrack()
	tr.Duration = 0
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("duration 0: error = %v, want ErrInvalidPayload", err)
	}
	tr.Duration = 4000
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("duration 4000: error = %v, want ErrInvalidPayload", err)
	}
}

func TestValidateTrackDurationDrift(t *testing.T) {
	tr := validTrack()
	// strict: 10s drift is rejected
	if err := ValidateTrack(tr, ValidateOpts{ReqDuration: 209}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("strict drift: error = %v, want ErrInvalidPayload", err)
	}
	// basic: drift is ignored
	if err := ValidateTrack(tr, ValidateOpts{Basic: true, ReqDuration: 209}); err != nil {
		t.Errorf("basic drift: error = %v, want nil", err)
	}
}

func TestValidateTrackEmptyLyrics(t *testing.T) {
	tr := validTrack()
	tr.SyncedLyrics = ""
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("empty lyrics: error = %v, want ErrInvalidPayload", err)
	}
}

func TestValidateTrackGarbageLyrics(t *testing.T) {
	tr := validTrack()
	tr.SyncedLyrics = "this is not lrc\njust some text\n"
	// strict: must parse to >=1 valid line
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("strict garbage: error = %v, want ErrInvalidPayload", err)
	}
	// basic: non-empty is enough
	if err := ValidateTrack(tr, ValidateOpts{Basic: true}); err != nil {
		t.Errorf("basic garbage: error = %v, want nil", err)
	}
}

func TestValidateTrackLineLimit(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < maxLyricLines+1; i++ {
		sb.WriteString("[00:01.00]line\n")
	}
	tr := validTrack()
	tr.SyncedLyrics = sb.String()
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("line limit: error = %v, want ErrInvalidPayload", err)
	}
}

func TestValidateTrackLongLine(t *testing.T) {
	tr := validTrack()
	tr.SyncedLyrics = "[00:01.00]" + strings.Repeat("x", maxLyricLineLen+1)
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("long line: error = %v, want ErrInvalidPayload", err)
	}
}

func TestValidateTrackLastTimestamp(t *testing.T) {
	tr := validTrack()
	tr.SyncedLyrics = "[00:01.00]line\n[00:59.00]late"
	// duration 219 + 10s slack = 229s; 59s is fine
	if err := ValidateTrack(tr, ValidateOpts{}); err != nil {
		t.Fatalf("in-range last timestamp: error = %v, want nil", err)
	}
	tr.SyncedLyrics = "[00:01.00]line\n[04:00.00]way late"
	if err := ValidateTrack(tr, ValidateOpts{}); !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("late timestamp: error = %v, want ErrInvalidPayload", err)
	}
}

func TestCleanControl(t *testing.T) {
	in := "a\x00b\x01c\nd\te"
	got := cleanControl(in)
	want := "abc\nd\te"
	if got != want {
		t.Errorf("cleanControl() = %q, want %q", got, want)
	}
}

func TestIsInstrumental(t *testing.T) {
	tr := validTrack()
	tr.Instrumental = true
	tr.SyncedLyrics = ""
	if !IsInstrumental(tr) {
		t.Error("IsInstrumental() = false, want true for instrumental with no lyrics")
	}
	tr.SyncedLyrics = "[00:01.00]lyrics"
	if IsInstrumental(tr) {
		t.Error("IsInstrumental() = true, want false when lyrics present")
	}
}
