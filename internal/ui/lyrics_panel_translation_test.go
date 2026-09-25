package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// panelTranslationModel is panelPartsModel with the parts declared as
// translations, the shape LRC's same-timestamp merge and TTML's x-translation
// produce.
func panelTranslationModel(t *testing.T, parts []string) *Model {
	t.Helper()
	m := panelPartsModel(t, 0, parts)
	m.Audio.Lyrics.TranslationsInParts = true
	return m
}

// A translated row is subordinate to the line being sung: it keeps the accent's
// hue but steps halfway toward the grey the unplayed text uses, and it carries no
// bold. The sung row itself is untouched.
func TestPanelWindow_TranslationRowIsQuieter(t *testing.T) {
	m := panelTranslationModel(t, []string{"The rain I hear falls", "我听见雨滴落在青青草地"})
	plan := m.layoutPlan()
	rows := panelWindow(m, plan)
	const anchor = 0 // one line, so the group hugs the top: see panelPartsModel's file

	sung, translated := rows[anchor], rows[anchor+1]
	if len(sung.spans) != 1 || len(translated.spans) != 1 {
		t.Fatalf("span counts = %d/%d, want 1 whole-line span each", len(sung.spans), len(translated.spans))
	}
	if got, want := panelRowText(translated), "我听见雨滴落在青青草地"; got != want {
		t.Fatalf("row %d = %q, want %q", anchor+1, got, want)
	}
	if got, want := sung.spans[0].Style.Render("x"), panelCurrentStyle(m).Render("x"); got != want {
		t.Errorf("sung row style changed: rendered %q, want %q", got, want)
	}
	// Halfway from the emphasis colour to lyricGrey. A cover-less track takes the
	// ANSI 141 fallback (#af87ff), so the step lands on #9f89c4.
	if got, want := translated.spans[0].Style.GetForeground(), lipgloss.Color("#9f89c4"); got != want {
		t.Errorf("translated row foreground = %v, want %v", got, want)
	}
	if translated.spans[0].Style.GetBold() {
		t.Error("translated row is bold, want regular weight")
	}
	panelRowWidths(t, rows, plan.PanelInnerW)
}
