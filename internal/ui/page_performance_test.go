package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/library"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/playlist"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func benchmarkPageModel(count int) *Model {
	cfg := config.DefaultConfig()
	cfg.IconTheme = "fallback"
	cfg.Library.Roots = []string{"/music"}
	cfg.Lyrics.Panel.Mode = config.PanelModeOff
	m := NewModel("", &cfg)
	m.UI.Width, m.UI.Height, m.UI.Focus = 120, 30, FocusContent
	tracks := make([]library.Track, count)
	for i := range tracks {
		tracks[i] = library.Track{
			Path: fmt.Sprintf("/music/%06d.flac", i), Title: fmt.Sprintf("Song %06d 雨の音", i),
			Artist: fmt.Sprintf("Artist %04d", i%1000), Album: fmt.Sprintf("Album %05d", i%10000),
			Format: "FLAC", Duration: time.Minute * 4,
		}
	}
	m.installLibrary(tracks)
	return m
}

func BenchmarkLibraryFilter(b *testing.B) {
	for _, count := range []int{1000, 10000, 100000} {
		for _, query := range []string{"", "song", "nomatch"} {
			b.Run(fmt.Sprintf("n=%d/query=%s", count, query), func(b *testing.B) {
				m := benchmarkPageModel(count)
				m.pages.filter = query
				b.ReportAllocs()
				for b.Loop() {
					m.refreshLibraryView()
				}
			})
		}
	}
}

func BenchmarkLibraryInstall(b *testing.B) {
	for _, count := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("n=%d", count), func(b *testing.B) {
			m := benchmarkPageModel(count)
			tracks := m.libraryTracks
			b.ReportAllocs()
			for b.Loop() {
				m.installLibrary(tracks)
			}
		})
	}
}

func BenchmarkTabFrame(b *testing.B) {
	for tab := range 4 {
		b.Run(fmt.Sprintf("tab=%d", tab), func(b *testing.B) {
			m := benchmarkPageModel(1000)
			m.UI.ActiveTab = tab
			if tab == 1 {
				for _, track := range m.libraryTracks {
					m.session.entries = append(m.session.entries, m.newQueueEntry(track))
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				m.View()
			}
		})
	}
}

func TestLibraryFilterDoesNotAllocatePerTrack(t *testing.T) {
	m := benchmarkPageModel(2000)
	m.pages.filter = "SONG"
	allocations := testing.AllocsPerRun(10, func() { m.refreshLibraryView() })
	if len(m.pages.visible) != 2000 {
		t.Fatal("normalized query lost tracks")
	}
	if allocations > 2 {
		t.Fatalf("filter allocated %.0f times; budget is query normalization only", allocations)
	}
}

func TestLibrarySearchUsesCurrentMetadataAndSelection(t *testing.T) {
	m := queueFixture(t)
	tracks := []library.Track{
		{Path: "/music/a.flac", Title: "ÉTÉ 雨", Artist: "ARTIST", Album: "ALBUM", Format: "FLAC"},
		{Path: "/music/b.wav", Title: "ÉTÉ 雨", Artist: "OTHER", Album: "ALBUM", Format: "WAV"},
	}
	m.installLibrary(tracks)
	m.pages.cursors[0] = 1
	for _, query := range []string{"été 雨", "ALBUM", "雨 other"} {
		m.pages.filter = query
		m.refreshLibraryView()
		if len(m.pages.visible) == 0 || m.pages.visible[m.pages.cursors[0]].Path != "/music/b.wav" {
			t.Fatalf("filter %q lost selected track", query)
		}
	}
	m.pages.filter = "artist"
	m.refreshLibraryView()
	if len(m.pages.visible) != 1 || m.pages.visible[0].Path != "/music/a.flac" {
		t.Fatal("artist query did not use current tags")
	}
	tracks = slices.Clone(tracks)
	tracks[0].Artist = "Renamed"
	m.installLibrary(tracks)
	if len(m.pages.visible) != 0 {
		t.Fatal("metadata replacement retained stale search text")
	}
	m.pages.filter = "renamed"
	m.refreshLibraryView()
	if len(m.pages.visible) != 1 || m.pages.visible[0].Path != "/music/a.flac" {
		t.Fatal("updated metadata was not searchable")
	}
	m.pages.filter = ""
	m.pages.scope = slices.Index(m.pages.scopes, "WAV")
	m.refreshLibraryView()
	if len(m.pages.visible) != 1 || m.pages.visible[0].Path != "/music/b.wav" {
		t.Fatal("format scope changed search identity")
	}
	m.installLibrary(nil)
	if len(m.pages.visible) != 0 {
		t.Fatal("cleared library retained searchable tracks")
	}
}

func TestSizedPageContentAvoidsRepeatedLayoutMeasurement(t *testing.T) {
	m := benchmarkPageModel(1000)
	m.UI.ActiveTab = 2
	plan := m.layoutPlan()
	// This includes drawing and boxing; the full View benchmark tracks outer chrome.
	allocations := testing.AllocsPerRun(10, func() { renderContent(m, plan) })
	if allocations > 3500 {
		t.Fatalf("sized content allocated %.0f times; budget is 3500", allocations)
	}
}

func TestSizedPageContentPreservesCellsAndStyles(t *testing.T) {
	for _, width := range []int{minWidth, 80, 120} {
		for _, surface := range []string{"home", "queue", "effects", "settings", "editor", "browser", "formats", "roots", "quit", "saved"} {
			m := benchmarkPageModel(2)
			m.UI.Width = width
			switch surface {
			case "queue":
				m.UI.ActiveTab = 1
				m.appendQueue(m.libraryTracks[0])
			case "effects":
				m.UI.ActiveTab = 2
			case "settings":
				m.UI.ActiveTab = 3
			case "editor":
				m.pages.openEditor("Filter tracks", "filter", "界e\u0301 雨", 0)
			case "browser":
				m.pages.browser = &fileBrowser{purpose: "audio", directory: "/music", entries: []browserEntry{{name: "界e\u0301.wav", path: "/music/界e\u0301.wav"}}}
			case "formats":
				m.pages.picker = "formats"
			case "roots":
				m.pages.rootEditor = true
			case "quit":
				m.pages.quitSettings = true
			case "saved":
				m.UI.ActiveTab = 1
				m.pages.saved, m.pages.detail, m.playlists.loaded = true, true, true
				m.playlists.lists = []playlist.List{{ID: "list", Name: "界e\u0301", Tracks: m.libraryTracks}}
			}
			plan := m.layoutPlan()
			body := renderTabPage(m, plan)
			want := contentStyle.Width(plan.ContentWidth).Height(plan.ContentHeight).Render(body)
			got := renderContent(m, plan)
			if stripANSI(got) != stripANSI(want) {
				t.Fatalf("width=%d surface=%s changed visible content", width, surface)
			}
			actualCells := uv.NewStyledString(got).Lines(ansi.GraphemeWidth)
			expectedCells := uv.NewStyledString(want).Lines(ansi.GraphemeWidth)
			for y, row := range actualCells {
				for x := range row {
					if !row[x].Equal(&expectedCells[y][x]) {
						t.Fatalf("width=%d surface=%s cell=%d,%d changed style", width, surface, x, y)
					}
				}
			}
			for _, line := range strings.Split(got, "\n") {
				if lipgloss.Width(line) != plan.ContentWidth {
					t.Fatal("sized page overflowed its content box")
				}
			}
		}
	}
}
