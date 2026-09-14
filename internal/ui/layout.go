package ui

import "github.com/AuroraStudio-aurorast/neoviolet/internal/config"

// Layout contract for the main view. Every region size in a frame is derived
// here and nowhere else: renderContent, renderFooter and renderLyricsPanel all
// receive a layoutPlan instead of recomputing widths from m.UI.Width.
//
// All sizes are terminal cells (columns / rows).

const (
	minWidth  = 68 // smallest usable terminal width
	minHeight = 17 // smallest usable terminal height

	tabsHeight = 3 // tab bar including its border
	helpHeight = 1 // help / status line

	// footerBaseRows is the footer without a lyric row:
	// rounded border (2) + song (1) + progress (1) + volume (1).
	footerBaseRows = 5
	// footerLyricRows is the row the one_line lyric renderer adds.
	footerLyricRows = 1

	// Panel box chrome: rounded border plus one column of padding per side.
	panelBorderW  = 2
	panelBorderH  = 2
	panelPaddingH = 1

	// oneLineWidthInset is the footer chrome the one_line lyric row leaves
	// free: border (2) + padding (2) + 2 spare columns.
	oneLineWidthInset = 6

	// autoPanelRatioNum/Den is the share of the terminal an auto-width panel
	// asks for: 3/10 = 30%.
	autoPanelRatioNum = 3
	autoPanelRatioDen = 10
)

// lyricDisplayMode selects which lyric surface is rendered in a frame.
type lyricDisplayMode int

const (
	lyricModeOneLine lyricDisplayMode = iota // footer single row (existing behaviour)
	lyricModePanel                           // right-hand panel
)

// layoutInput is the runtime state computeLayout depends on.
type layoutInput struct {
	PanelMode      string // auto | on | off
	PanelWidth     int    // configured width including borders; ignored when PanelWidthAuto
	PanelWidthAuto bool   // derive the width from the terminal instead
	ShowLyrics     bool   // AudioState.ShowLyrics
	OneLineVisible bool   // the footer needs its lyric row
}

// layoutPlan is the single source of truth for region sizes in one frame.
type layoutPlan struct {
	Width  int
	Height int

	PanelShown  bool
	PanelWidth  int // including borders; 0 when hidden
	PanelInnerW int // writable columns; 0 when hidden
	PanelInnerH int // writable rows; 0 when hidden

	ContentWidth  int
	ContentHeight int
	FooterRows    int

	LyricMode         lyricDisplayMode
	OneLineLyricWidth int // marquee width; 0 while the panel is shown
}

// computeLayout derives every region size for a w x h terminal.
//
// Invariants (asserted by layout_test.go):
//
//	ContentWidth + PanelWidth == Width
//	tabsHeight + ContentHeight + FooterRows + helpHeight == Height
func computeLayout(w, h int, in layoutInput) layoutPlan {
	if w <= 0 || h <= 0 {
		return layoutPlan{}
	}

	mode := in.PanelMode
	if mode != config.PanelModeOn && mode != config.PanelModeOff {
		mode = config.PanelModeAuto // "" and unknown values behave as auto
	}
	panelWidth, fits := panelWidthFor(w, in)
	if !fits && mode == config.PanelModeOn {
		// A forced panel still appears on a narrow terminal: the content area
		// gives way down to the smallest usable box instead of losing the panel.
		panelWidth, fits = config.MinPanelWidth, true
	}

	p := layoutPlan{Width: w, Height: h}

	// :lrc off hides the whole panel so the content area gets the columns back;
	// a track without lyrics keeps the panel and shows a placeholder instead.
	if in.ShowLyrics && fits && panelShown(mode, w, panelWidth) {
		p.PanelShown = true
		p.PanelWidth = effectivePanelWidth(mode, w, panelWidth)
		p.PanelInnerW = p.PanelWidth - panelBorderW - 2*panelPaddingH
		p.PanelInnerH = 0 // set below, once the content height is known
		p.LyricMode = lyricModePanel
	} else {
		p.OneLineLyricWidth = w - oneLineWidthInset
	}

	p.FooterRows = footerBaseRows
	if p.LyricMode == lyricModeOneLine && in.OneLineVisible {
		p.FooterRows += footerLyricRows
	}

	p.ContentWidth = w - p.PanelWidth
	p.ContentHeight = h - tabsHeight - p.FooterRows - helpHeight
	if p.PanelShown {
		p.PanelInnerH = p.ContentHeight - panelBorderH
	}
	return p
}

// panelWidthFor resolves the panel's column count for a w-wide terminal.
//
// auto asks for autoPanelRatio of the terminal, floored at MinPanelWidth and
// capped by both MaxPanelWidth and whatever is left once the content area keeps
// its minimum width. fit=false means no auto panel is usable at that width. A
// configured width is returned as given, defensively clamped.
func panelWidthFor(w int, in layoutInput) (width int, fit bool) {
	if !in.PanelWidthAuto {
		return clampPanelWidth(in.PanelWidth), true
	}

	maxW := config.MaxPanelWidth
	if room := w - minWidth; room < maxW {
		maxW = room
	}
	if maxW < config.MinPanelWidth {
		return 0, false
	}

	// autoPanelRatio of the terminal, kept inside the configured bounds and then
	// capped by whatever the content area can spare (maxW >= MinPanelWidth here,
	// so the two never fight).
	width = clampPanelWidth(w * autoPanelRatioNum / autoPanelRatioDen)
	if width > maxW {
		width = maxW
	}
	return width, true
}

// clampPanelWidth keeps a width inside the documented bounds. Normalize already
// clamps config values, so this only guards programmatic callers.
func clampPanelWidth(width int) int {
	if width < config.MinPanelWidth {
		return config.MinPanelWidth
	}
	if width > config.MaxPanelWidth {
		return config.MaxPanelWidth
	}
	return width
}

// panelShown reports whether the panel fits at terminal width w:
//
//	off  -> never
//	on   -> any usable width (the panel may squeeze the content area)
//	auto -> only when the content area keeps its minimum width
func panelShown(mode string, w, panelWidth int) bool {
	switch mode {
	case config.PanelModeOff:
		return false
	case config.PanelModeOn:
		return w >= minWidth
	default:
		return w >= minWidth+panelWidth
	}
}

// effectivePanelWidth shrinks an explicitly forced panel so the content area
// never drops below minWidth. mode=auto always has room by construction.
func effectivePanelWidth(mode string, w, panelWidth int) int {
	if mode == config.PanelModeOn && w < minWidth+panelWidth {
		if shrunk := w - minWidth; shrunk > config.MinPanelWidth {
			return shrunk
		}
		return config.MinPanelWidth
	}
	return panelWidth
}

// layoutPlan derives this model's layout for the current terminal size. It is
// the only place that maps Model state onto layoutInput.
func (m *Model) layoutPlan() layoutPlan {
	return computeLayout(m.UI.Width, m.UI.Height, layoutInput{
		PanelMode:      m.panelMode,
		PanelWidth:     int(m.Config.Lyrics.Panel.Width),
		PanelWidthAuto: m.Config.Lyrics.Panel.Width == config.PanelWidthAuto,
		ShowLyrics:     m.Audio.ShowLyrics,
		OneLineVisible: m.oneLineVisible(),
	})
}

// oneLineVisible mirrors the condition that used to decide whether the footer
// keeps its lyric row. It matches the old renderFooter logic for all eight
// combinations of (Lyrics != nil, ShowLyrics, LyricsFetching).
func (m *Model) oneLineVisible() bool {
	return (m.Audio.Lyrics != nil && m.Audio.ShowLyrics) || m.LyricsFetching
}
