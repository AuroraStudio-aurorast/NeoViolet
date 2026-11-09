package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

func TestPageNavigationRetainsSelectionAndDoesNotEscapeFocus(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusTabBar
	pageKey(m, tea.KeyDown, "")
	if m.pages.cursors[0] != 0 {
		t.Fatal("unfocused page consumed navigation")
	}
	m.UI.Focus = FocusContent
	pageKey(m, tea.KeyDown, "")
	pageKey(m, tea.KeyDown, "")
	pageKey(m, ']', "]")
	pageKey(m, tea.KeyDown, "")
	pageKey(m, '[', "[")
	if m.pages.cursors[0] != 2 || m.pages.cursors[1] != 1 {
		t.Fatal("switching tabs reset page selection")
	}
	pageKey(m, tea.KeyRight, "")
	if m.pages.scope != 1 || len(m.pages.tracks()) != 4 {
		t.Fatal("format scope did not filter sample tracks")
	}
}

func TestSampleQueueOrderRemoveAndUndo(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	m.UI.ActiveTab = 1
	pageKey(m, tea.KeyDown, "")
	pageKey(m, 'J', "J")
	if m.pages.queue[2] != 1 {
		t.Fatal("reordering did not move the selected entry")
	}
	pageKey(m, 'd', "d")
	pageKey(m, 'u', "u")
	if len(m.pages.queue) != 6 || m.pages.queue[2] != 1 {
		t.Fatal("undo restored a different queue position")
	}
	for range 8 {
		pageKey(m, 'd', "d")
	}
	if got := stripANSI(renderContent(m, m.layoutPlan())); !strings.Contains(got, "queue is empty") {
		t.Fatal("empty queue has no recovery state")
	}
	pageKey(m, 'u', "u")
	if len(m.pages.queue) != 1 {
		t.Fatal("empty queue could not restore its last entry")
	}
}

func TestSampleEffectsAreBoundedAndResetLocally(t *testing.T) {
	m := setupModel()
	m.UI.Focus = FocusContent
	m.UI.ActiveTab = 2
	for range 30 {
		pageKey(m, '+', "+")
	}
	if m.pages.gains[0] != 12 {
		t.Fatal("sample EQ exceeded its displayed maximum")
	}
	for range 5 {
		pageKey(m, tea.KeyRight, "")
	}
	for range 30 {
		pageKey(m, '-', "-")
	}
	if m.pages.pan != -1 {
		t.Fatal("sample pan exceeded its channel range")
	}
	pageKey(m, 'r', "r")
	if m.pages.pan != 0 || m.Audio.Player != nil {
		t.Fatal("reset did not remain a presentation operation")
	}
}

func TestSampleFormatPickerOrdersAndCancelsWithoutSaving(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewModel("", &cfg)
	m.UI.Focus = FocusContent
	m.UI.ActiveTab = 3
	pageKey(m, '3', "3")
	for range 5 {
		pageKey(m, tea.KeyDown, "")
	}
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'J', "J")
	pageKey(m, tea.KeySpace, "")
	if m.pages.formats[1] != "embedded" || m.pages.enabled[1] {
		t.Fatal("priority ordering lost the selected format")
	}
	pageKey(m, tea.KeyEnter, "")
	if !strings.Contains(m.pages.settings[m.pages.cursors[3]].value, "8 formats") {
		t.Fatal("confirmation lost the enabled count")
	}
	if len(m.Config.Lyrics.FormatPriority) != 9 || m.Config.Lyrics.FormatPriority[0] != "embedded" {
		t.Fatal("sample picker changed real format preferences")
	}
}

func TestSampleFormatPickerCancelDiscardsChanges(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewModel("", &cfg)
	m.UI.Focus = FocusContent
	m.UI.ActiveTab = 3
	pageKey(m, '3', "3")
	for range 5 {
		pageKey(m, tea.KeyDown, "")
	}
	pageKey(m, tea.KeyEnter, "")
	pageKey(m, 'J', "J")
	pageKey(m, tea.KeySpace, "")
	pageKey(m, tea.KeyEscape, "")
	if m.pages.formats[0] != "embedded" || !m.pages.enabled[0] || m.pages.dirty {
		t.Fatal("cancel retained sample format edits")
	}
}
