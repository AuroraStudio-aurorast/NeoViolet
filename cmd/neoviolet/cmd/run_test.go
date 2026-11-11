package cmd

import (
	"errors"
	"testing"
)

func TestRunWithOSMediaDisabled(t *testing.T) {
	want := errors.New("player error")
	calls := 0
	err := runWithOSMedia(false, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("disabled integration: err=%v, callback calls=%d", err, calls)
	}
}
