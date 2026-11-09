package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
)

func scrollFixture(t *testing.T) *Model {
	t.Helper()
	m := wiredSettingsModel(t)
	m.UI.ActiveTab = 0
	m.UI.Width = 80
	m.UI.Height = 24
	m.Config.Library.Roots = []string{"/music"}
	tracks := make([]library.Track, 40)
	for i := range tracks {
		tracks[i] = library.Track{Path: fmt.Sprintf("/music/%02d.wav", i), Title: fmt.Sprintf("Track %02d", i), Format: "WAV"}
	}
	m.installLibrary(tracks)
	return m
}
func scrollKey(m *Model, code rune) { pageKey(m, code, "") }
func selectedCanvas(m *Model) *pageCanvas {
	plan := m.layoutPlan()
	c := newPageCanvas(plan.ContentInnerW, plan.ContentInnerH, m)
	switch m.UI.ActiveTab {
	case 0:
		m.renderHome(c, true)
	case 1:
		m.renderPlaylists(c, true)
	case 3:
		m.pages.renderSettings(c, true)
	}
	return c
}
func TestLibraryCursorMovesWithinWindowBeforeScrollingInBothDirections(t *testing.T) {
	m := scrollFixture(t)
	slots := max(1, (m.layoutPlan().ContentInnerH-5)/2)
	for range slots + 2 {
		scrollKey(m, tea.KeyDown)
	}
	c := selectedCanvas(m)
	before := c.selections[0].row
	scrollKey(m, tea.KeyUp)
	c = selectedCanvas(m)
	if c.selections[0].row != before-2 {
		t.Fatal("up scrolled content instead of moving cursor within viewport")
	}
	body := stripANSI(c.String())
	if !strings.Contains(body, "Track 03") {
		t.Fatal("reverse navigation moved window origin")
	}
	for range slots - 1 {
		scrollKey(m, tea.KeyUp)
	}
	c = selectedCanvas(m)
	if c.selections[0].row != 4 {
		t.Fatal("upper boundary did not scroll to keep selection visible")
	}
}
func TestQueueAndSavedWindowsKeepReverseCursorInsideViewport(t *testing.T) {
	for _, kind := range []string{"queue", "collections", "detail"} {
		t.Run(kind, func(t *testing.T) {
			m := scrollFixture(t)
			m.UI.ActiveTab = 1
			if kind == "queue" {
				for _, track := range m.pages.visible {
					m.appendQueue(track)
				}
			} else {
				m.pages.saved = true
				m.playlists.loaded = true
				for i := range 25 {
					m.playlists.lists = append(m.playlists.lists, playlist.List{ID: fmt.Sprint(i), Name: fmt.Sprintf("List %02d", i), Tracks: m.pages.visible})
				}
				m.pages.detail = kind == "detail"
			}
			for range 20 {
				scrollKey(m, tea.KeyDown)
			}
			c := selectedCanvas(m)
			before := c.selections[0].row
			scrollKey(m, tea.KeyUp)
			c = selectedCanvas(m)
			stride := 3
			if kind == "detail" {
				stride = 1
			}
			if c.selections[0].row != before-stride {
				t.Fatal("reverse navigation pinned selection to bottom")
			}
		})
	}
}
func TestSettingsWindowCountsGroupHeadingsAndDoesNotPinSelection(t *testing.T) {
	m := scrollFixture(t)
	m.UI.ActiveTab = 3
	for range 18 {
		scrollKey(m, tea.KeyDown)
	}
	c := selectedCanvas(m)
	before := c.selections[0].row
	scrollKey(m, tea.KeyUp)
	c = selectedCanvas(m)
	if c.selections[0].row != before-1 {
		t.Fatal("settings cursor did not move inside viewport")
	}
}
func TestBrowserAndFormatPickerKeepScrolledWindow(t *testing.T) {
	m := scrollFixture(t)
	b := &fileBrowser{purpose: "audio", directory: "/music"}
	for i := range 40 {
		b.entries = append(b.entries, browserEntry{name: fmt.Sprintf("File %02d.wav", i), path: fmt.Sprintf("/music/%02d.wav", i)})
	}
	m.pages.browser = b
	for range 20 {
		scrollKey(m, tea.KeyDown)
	}
	renderBrowser := func() *pageCanvas {
		p := m.layoutPlan()
		c := newPageCanvas(p.ContentInnerW, p.ContentInnerH, m)
		b.render(c)
		return c
	}
	before := renderBrowser().selections[0].row
	scrollKey(m, tea.KeyUp)
	if renderBrowser().selections[0].row != before-1 {
		t.Fatal("browser selection stuck at bottom")
	}
	m.pages.browser = nil
	m.pages.picker = "formats"
	m.pages.formats = make([]string, 30)
	m.pages.enabled = make([]bool, 30)
	for i := range m.pages.formats {
		m.pages.formats[i] = fmt.Sprintf("format%02d", i)
	}
	for range 20 {
		scrollKey(m, tea.KeyDown)
	}
	renderPicker := func() *pageCanvas {
		p := m.layoutPlan()
		c := newPageCanvas(p.ContentInnerW, p.ContentInnerH, m)
		m.pages.renderPicker(c)
		return c
	}
	before = renderPicker().selections[0].row
	scrollKey(m, tea.KeyUp)
	if renderPicker().selections[0].row != before-1 {
		t.Fatal("format selection stuck at bottom")
	}
}

func TestListViewportRetainsWindowUntilCursorCrossesBoundary(t *testing.T) {
	var window listViewport
	for _, step := range []struct{ cursor, count, capacity, want int }{
		{0, 20, 5, 0}, {4, 20, 5, 0}, {5, 20, 5, 1}, {6, 20, 5, 2},
		{5, 20, 5, 2}, {2, 20, 5, 2}, {1, 20, 5, 1},
		{19, 20, 5, 15}, {18, 20, 5, 15}, {0, 20, 5, 0},
		{19, 20, 5, 15}, {19, 20, 10, 10}, {19, 20, 3, 17},
		{2, 3, 3, 0}, {0, 0, 3, 0},
	} {
		window.reveal(step.cursor, step.count, step.capacity)
		if window.top != step.want {
			t.Fatalf("cursor/count/capacity=%d/%d/%d got top %d want %d", step.cursor, step.count, step.capacity, window.top, step.want)
		}
	}
}
func TestLibraryWindowSurvivesTabSwitchAndResize(t *testing.T) {
	m := scrollFixture(t)
	for range 12 {
		scrollKey(m, tea.KeyDown)
	}
	scrollKey(m, tea.KeyUp)
	before := selectedCanvas(m).selections[0].row
	pageKey(m, ']', "]")
	pageKey(m, '[', "[")
	if selectedCanvas(m).selections[0].row != before {
		t.Fatal("tab switch reset viewport")
	}
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 17})
	c := selectedCanvas(m)
	if c.selections[0].row >= m.layoutPlan().ContentInnerH-1 {
		t.Fatal("shrink hid cursor")
	}
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 32})
	if selectedCanvas(m).selections[0].row >= m.layoutPlan().ContentInnerH-1 {
		t.Fatal("grow hid cursor")
	}
	start := m.pages.viewports.home.top
	scrollKey(m, tea.KeyUp)
	if m.pages.viewports.home.top != start {
		t.Fatal("grow anchored cursor to bottom")
	}
}
func TestRootEditorUsesIndependentBoundaryScrolling(t *testing.T) {
	m := scrollFixture(t)
	m.pages.rootEditor = true
	for i := range 30 {
		m.pages.draft.Library.Roots = append(m.pages.draft.Library.Roots, fmt.Sprintf("/music/root%02d", i))
	}
	render := func() *pageCanvas {
		p := m.layoutPlan()
		c := newPageCanvas(p.ContentInnerW, p.ContentInnerH, m)
		m.pages.renderLibraryRoots(c)
		return c
	}
	for range 20 {
		scrollKey(m, tea.KeyDown)
	}
	before := render().selections[0].row
	scrollKey(m, tea.KeyUp)
	if render().selections[0].row != before-1 {
		t.Fatal("root cursor stuck at bottom")
	}
}
func TestSavedDetailWindowResetsForAnotherCollection(t *testing.T) {
	m := scrollFixture(t)
	m.UI.ActiveTab = 1
	m.pages.saved = true
	m.pages.detail = true
	m.playlists.loaded = true
	m.playlists.lists = []playlist.List{{ID: "first", Name: "First", Tracks: m.pages.visible}, {ID: "second", Name: "Second", Tracks: m.pages.visible}}
	for range 20 {
		scrollKey(m, tea.KeyDown)
	}
	scrollKey(m, tea.KeyEscape)
	scrollKey(m, tea.KeyDown)
	scrollKey(m, tea.KeyEnter)
	c := selectedCanvas(m)
	if c.selections[0].row != 6 || m.pages.savedTrack != 0 {
		t.Fatal("another collection inherited scrolled detail viewport")
	}
}
func TestViewportRenderingDoesNotMutateOffsets(t *testing.T) {
	m := scrollFixture(t)
	for range 15 {
		scrollKey(m, tea.KeyDown)
	}
	before := m.pages.viewports
	selectedCanvas(m)
	selectedCanvas(m)
	if m.pages.viewports != before {
		t.Fatal("view changed scrolling state")
	}
}
