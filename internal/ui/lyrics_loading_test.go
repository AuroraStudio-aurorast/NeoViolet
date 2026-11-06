package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFetchingLyricsAnimatesWithoutOtherLoads(t *testing.T) {
	m := setupModel()
	m.Config.TickRate = 30
	m.Audio.Player = &mockPlayer{}
	m.LyricsFetching = true
	plan := layoutPlan{PanelInnerW: 27, PanelInnerH: 10, PanelWidth: 30, ContentHeight: 10}
	firstPanel := renderLyricsPanel(m, plan)
	firstLine, ok := renderOneLineLyrics(m, 66)
	if !ok {
		t.Fatal("fetching footer missing")
	}
	for range 10 {
		handleTick(m)
	}
	laterLine, _ := renderOneLineLyrics(m, 66)
	laterPanel := renderLyricsPanel(m, plan)
	for _, pair := range [][2]string{{firstLine, laterLine}, {firstPanel, laterPanel}} {
		if pair[0] == pair[1] {
			t.Error("lyrics fetch stayed static without audio or animation loading")
		}
		if ansi.Strip(pair[0]) != ansi.Strip(pair[1]) {
			t.Error("fetch sweep moved its label")
		}
	}
	if strings.TrimSpace(ansi.Strip(firstLine)) != "Fetching lyrics" {
		t.Fatalf("footer = %q", ansi.Strip(firstLine))
	}
	if lipgloss.Width(firstLine) != 66 {
		t.Error("fetching changed footer width")
	}
	if lipgloss.Width(firstPanel) != 30 || lipgloss.Height(firstPanel) != 10 {
		t.Error("fetching changed panel geometry")
	}
	plain := ansi.Strip(firstLine)
	left := len(plain) - len(strings.TrimLeft(plain, " "))
	right := len(plain) - len(strings.TrimRight(plain, " "))
	if left-right > 1 || right-left > 1 {
		t.Error("fetch label not centered")
	}
	m.LyricsFetching = false
	if _, ok := renderOneLineLyrics(m, 66); ok {
		t.Error("fetching footer remained after completion")
	}
	if strings.Contains(ansi.Strip(renderLyricsPanel(m, plan)), "Fetching lyrics") {
		t.Error("fetching panel remained after completion")
	}
}

func TestFetchingLyricsKeepsExistingLyricsVisible(t *testing.T) {
	m := lyricFooterModel()
	m.Audio.Player = &mockPlayer{}
	plan := m.layoutPlan()
	plan.PanelInnerW, plan.PanelInnerH = 27, 10
	plan.PanelWidth, plan.ContentHeight = 30, 10
	line, _ := renderOneLineLyrics(m, 66)
	panel := renderLyricsPanel(m, plan)
	m.LyricsFetching = true
	fetchedLine, _ := renderOneLineLyrics(m, 66)
	if fetchedLine != line || renderLyricsPanel(m, plan) != panel {
		t.Error("background fetch replaced existing lyrics")
	}
}
