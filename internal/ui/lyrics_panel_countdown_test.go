package ui

import (
	"testing"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
)

// This file covers the panel's waiting state: a gap long enough to be worth
// announcing shows the same three-dot countdown as the one-line footer, on its
// own row directly under the line the panel is holding.

// panelGapModel builds a panel sitting in a gap: line "first" fills 0..end, line
// "next" starts at nextStart, and elapsed is between the two. contextLines caps
// the window the way the config knob does (0 = no cap).
func panelGapModel(t *testing.T, contextLines int, end, nextStart, elapsed time.Duration) *Model {
	t.Helper()
	m := panelModel(t, contextLines)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, End: end, Text: "first"},
		{Time: nextStart, Text: "next"},
		{Time: nextStart + 5*time.Second, Text: "later"},
	}}
	m.Audio.Elapsed = elapsed
	m.Audio.UpdateLyricIndex()
	return m
}

// wantDots is the countdown the views show for a line secs away, in the model's
// icon theme.
func wantDots(m *Model, secs float64) string {
	return buildLyricCountdown(secs, m.Icons.LyricFilled, m.Icons.LyricEmpty)
}

// A long wait puts the countdown on its own row directly under the held line, and
// the line being waited for stays below it.
func TestPanelWindow_LongGapShowsCountdownDots(t *testing.T) {
	m := panelGapModel(t, 2, 5*time.Second, 12*time.Second, 10*time.Second)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH)

	if got := panelRowText(rows[anchor]); got != "first" {
		t.Errorf("row %d = %q, want the held line", anchor, got)
	}
	if got, want := panelRowText(rows[anchor+1]), wantDots(m, 2); got != want {
		t.Errorf("row %d = %q, want the countdown %q on its own row", anchor+1, got, want)
	}
	if got := panelRowText(rows[anchor+2]); got != "next" {
		t.Errorf("row %d = %q, want the upcoming line", anchor+2, got)
	}
	equalRows(t, panelTexts(t, m), []string{"first", wantDots(m, 2), "next", "later"})
	panelRowWidths(t, rows, plan.PanelInnerW)
}

// Nothing is being sung in a gap, so the held line is not drawn as current: it
// has ended. The countdown is the only accent left, which is what says "waiting".
func TestPanelWindow_GapDimsTheHeldLine(t *testing.T) {
	m := panelGapModel(t, 2, 5*time.Second, 12*time.Second, 10*time.Second)
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH)

	held := rows[anchor]
	if got := panelRowText(held); got != "first" {
		t.Fatalf("row %d = %q, want the held line", anchor, got)
	}
	if len(held.spans) != 1 {
		t.Fatalf("held row has %d spans, want 1", len(held.spans))
	}
	if got, dim, lit := held.spans[0].Style.Render("x"), panelContextStyle.Render("x"), panelCurrentStyle(m).Render("x"); got != dim || got == lit {
		t.Errorf("held line style = %q, want the context style %q (current is %q)", got, dim, lit)
	}

	dots := rows[anchor+1]
	if len(dots.spans) != 1 {
		t.Fatalf("countdown row has %d spans, want 1", len(dots.spans))
	}
	if got, want := dots.spans[0].Style.Render("x"), panelCurrentStyle(m).Render("x"); got != want {
		t.Errorf("countdown style = %q, want the current style %q", got, want)
	}
}

// The countdown is live, not a static marker: it follows the seconds left, using
// the same ladder as the footer (see buildLyricCountdown).
func TestPanelWindow_CountdownDotsCountDown(t *testing.T) {
	cases := []struct {
		name    string
		elapsed time.Duration
	}{
		{"long wait", 8 * time.Second},
		{"under three seconds", 9*time.Second + 500*time.Millisecond},
		{"under two seconds", 10*time.Second + 500*time.Millisecond},
		{"under one second", 11*time.Second + 500*time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := panelGapModel(t, 2, 5*time.Second, 12*time.Second, tc.elapsed)
			want := wantDots(m, (12*time.Second - tc.elapsed).Seconds())
			if got := panelTexts(t, m); len(got) < 2 || got[1] != want {
				t.Fatalf("rows = %q, want the countdown %q second", got, want)
			}
		})
	}
}

// The threshold matches the footer's: a gap of exactly five seconds stays quiet,
// anything longer counts down.
func TestPanelWindow_CountdownStartsAfterFiveSeconds(t *testing.T) {
	atThreshold := panelGapModel(t, 2, 5*time.Second, 10*time.Second, 7*time.Second)
	if got := panelTexts(t, atThreshold); len(got) < 3 || got[1] != "next" {
		t.Errorf("rows = %q, want no countdown at exactly the threshold", got)
	}

	pastThreshold := panelGapModel(t, 2, 5*time.Second, 10*time.Second+time.Millisecond, 7*time.Second)
	want := wantDots(pastThreshold, 3.001)
	if got := panelTexts(t, pastThreshold); len(got) < 3 || got[1] != want {
		t.Errorf("rows = %q, want the countdown %q past the threshold", got, want)
	}
}

// The wait before the first line is a gap too, so a long intro counts down and
// the first line stays unhighlighted until it starts.
func TestPanelWindow_CountdownAtSongStart(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 8 * time.Second, Text: "first"},
		{Time: 13 * time.Second, Text: "second"},
	}}
	m.Audio.Elapsed = 0
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH)
	if got, want := panelRowText(rows[anchor]), "first"; got != want {
		t.Errorf("row %d = %q, want %q", anchor, got, want)
	}
	if got, want := panelRowText(rows[anchor+1]), wantDots(m, 8); got != want {
		t.Errorf("row %d = %q, want the countdown %q", anchor+1, got, want)
	}
	if got := panelRowText(rows[anchor+2]); got != "second" {
		t.Errorf("row %d = %q, want the upcoming line", anchor+2, got)
	}
}

// The countdown belongs to the wait: while a line is being sung the panel shows
// none. Nothing is waiting then — UpdateLyricIndex clears the gap state whenever a
// line is active — so the panel is back to the current line and the one after it,
// with the line that already ended still visible above.
func TestPanelWindow_NoCountdownWhileALineIsActive(t *testing.T) {
	m := panelGapModel(t, 2, 5*time.Second, 12*time.Second, 10*time.Second)
	dots := wantDots(m, 2)
	if got := panelTexts(t, m); len(got) < 2 || got[1] != dots {
		t.Fatalf("rows = %q, want the countdown %q in the gap", got, dots)
	}

	m.Audio.Elapsed = 12 * time.Second // "next" starts
	m.Audio.UpdateLyricIndex()
	// Premise: the gap state is cleared, which is what makes a countdown
	// impossible here rather than merely hidden.
	if m.Audio.LyricGapDuration != 0 || m.Audio.LyricNextIndex != -1 {
		t.Fatalf("gap state = %v / %d, want it cleared while a line is active",
			m.Audio.LyricGapDuration, m.Audio.LyricNextIndex)
	}
	equalRows(t, panelTexts(t, m), []string{"first", "next", "later"})
}

// The countdown follows the whole group: a held line that wraps to two part rows
// gets its countdown under both of them.
func TestPanelWindow_CountdownFollowsAMultiPartLine(t *testing.T) {
	m := panelModel(t, 2)
	m.Audio.Lyrics = &lyrics.Data{Lines: []lyrics.LyricLine{
		{Time: 0, End: 5 * time.Second, Text: "Hello world | 你好世界", Parts: []string{"Hello world", "你好世界"}},
		{Time: 12 * time.Second, Text: "next"},
		{Time: 17 * time.Second, Text: "later"},
	}}
	m.Audio.Elapsed = 10 * time.Second
	m.Audio.UpdateLyricIndex()

	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	anchor := panelAnchor(plan.PanelInnerH)
	equalRows(t, panelTexts(t, m), []string{"Hello world", "你好世界", wantDots(m, 2), "next", "later"})
	if got, want := panelRowText(rows[anchor+2]), wantDots(m, 2); got != want {
		t.Errorf("row %d = %q, want the countdown %q under the whole group", anchor+2, got, want)
	}
}

// The shared helper is the single source of truth for both views: no lyrics, no
// upcoming line, and a gap at the threshold all stay quiet.
func TestLyricCountdownDots(t *testing.T) {
	m := panelGapModel(t, 2, 5*time.Second, 12*time.Second, 10*time.Second)
	if dots, ok := lyricCountdownDots(m); !ok || dots != wantDots(m, 2) {
		t.Errorf("dots = %q ok = %v, want %q true", dots, ok, wantDots(m, 2))
	}

	m.Audio.Lyrics = nil
	if dots, ok := lyricCountdownDots(m); ok || dots != "" {
		t.Errorf("dots = %q ok = %v, want none without lyrics", dots, ok)
	}

	m = panelGapModel(t, 2, 5*time.Second, 12*time.Second, 10*time.Second)
	m.Audio.LyricNextIndex = len(m.Audio.Lyrics.Lines)
	if dots, ok := lyricCountdownDots(m); ok || dots != "" {
		t.Errorf("dots = %q ok = %v, want none past the last line", dots, ok)
	}

	m = panelGapModel(t, 2, 5*time.Second, 10*time.Second, 7*time.Second)
	if dots, ok := lyricCountdownDots(m); ok || dots != "" {
		t.Errorf("dots = %q ok = %v, want none at the threshold", dots, ok)
	}
}
