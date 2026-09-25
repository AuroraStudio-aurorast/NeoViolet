package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// loadingFrames is the animated spinner shown during track loading.
var loadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var appLayoutStyle = lipgloss.NewStyle()

func renderMainView(m *Model) tea.View {
	if m.Loading && !m.switchingTrack {
		// Full-screen loading: only for initial startup load.
		// Runtime track switches keep the main UI visible.
		idx := m.loadingTick / 6 % len(loadingFrames)
		return tea.NewView(loadingStyle.Render(loadingFrames[idx] + " Loading..."))
	}

	if m.UI.Width < minWidth || m.UI.Height < minHeight {
		msg := fmt.Sprintf("Terminal too small (%dx%d), resize to at least %dx%d",
			m.UI.Width, m.UI.Height, minWidth, minHeight)
		return tea.NewView(warnStyle.Width(m.UI.Width).Height(m.UI.Height).Align(lipgloss.Center, lipgloss.Center).Render(msg))
	}

	plan := m.layoutPlan()

	header := renderTabs(m)
	content := renderContent(m, plan)
	if plan.PanelShown {
		content = lipgloss.JoinHorizontal(lipgloss.Top, content, renderLyricsPanel(m, plan))
	}
	footer := renderFooter(m, plan)
	help := renderHelp(m)

	layout := appLayoutStyle.
		Width(m.UI.Width).
		Height(m.UI.Height)

	frame := layout.Render(header + "\n" + content + "\n" + footer + "\n" + help)
	// The list overlays the finished frame, because its row is measured from the
	// top of the frame rather than from the top of the content block.
	if rows := completionRows(m, plan); rows > 0 {
		frame = overlayCompletion(m, plan, frame, rows)
	}

	view := tea.NewView(frame)
	view.AltScreen = true
	view.AltScreen = true

	if m.Audio.CurrentSong != "" {
		song := m.Audio.CurrentSong
		if m.Audio.Artist != "" && m.Audio.Artist != "Unknown Artist" {
			view.WindowTitle = "NeoViolet | " + song + " - " + m.Audio.Artist
		} else {
			view.WindowTitle = "NeoViolet | " + song
		}
	} else {
		view.WindowTitle = "NeoViolet"
	}

	return view
}

func renderTabs(m *Model) string {
	tabIcons := []string{m.Icons.Home, m.Icons.Playlist, m.Icons.Effects, m.Icons.Settings}
	var tabs []string

	for i, name := range m.UI.Tabs {
		prefix := tabIcons[i]
		tabContent := " " + prefix
		if prefix != "" {
			tabContent += " "
		}
		tabContent += name + " "
		if i == m.UI.ActiveTab {
			accented := activeTabStyle.BorderForeground(lipgloss.Color(accentOrDefault(m.Accent, "57")))
			tabs = append(tabs, accented.Width(m.UI.tabWidth).Render(tabContent))
		} else {
			s := tabStyle
			if m.UI.Focus == FocusTabBar {
				s = s.BorderForeground(lipgloss.Color("15"))
			}
			tabs = append(tabs, s.Width(m.UI.tabWidth).Render(tabContent))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func renderContent(m *Model, plan layoutPlan) string {
	descriptions := map[int]string{
		0: "Browse your music library and manage the queue",
		1: "View and manage your playlists",
		2: "Adjust audio effects and enhancements",
		3: "Configure application settings",
	}

	content := fmt.Sprintf("[ %s ]\n\n%s",
		m.UI.Tabs[m.UI.ActiveTab],
		descriptions[m.UI.ActiveTab],
	)

	s := contentStyle
	if m.UI.Focus == FocusContent {
		s = s.BorderForeground(lipgloss.Color("15"))
	}

	return s.
		Width(plan.ContentWidth).
		Height(plan.ContentHeight).
		Render(content)
}

func renderFooter(m *Model, plan layoutPlan) string {
	icon := m.Icons.Play
	if m.Audio.IsPlaying {
		icon = m.Icons.Pause
	}

	var songLine string
	if m.Audio.Player != nil {
		artist := m.Audio.Artist
		if artist == "" || artist == "Unknown Artist" {
			songLine = m.Audio.CurrentSong
		} else {
			songLine = fmt.Sprintf("%s - %s", m.Audio.CurrentSong, artist)
		}
	} else {
		songLine = fmt.Sprintf("%s  No audio loaded", m.Icons.Music)
	}
	songLine = truncateLine(songLine, plan.Width-4)

	timeDisplay := formatDuration(m.Audio.Elapsed) + " / " + formatDuration(m.Audio.Duration)

	// Set progress bar width based on available space
	timeWidth := lipgloss.Width(timeDisplay)
	pbWidth := plan.Width - 4 - timeWidth - 1
	if pbWidth < 10 {
		pbWidth = 10
	}
	m.Components.ProgressBar.SetWidth(pbWidth)
	progressBar := m.Components.ProgressBar.ViewAs(m.Audio.Progress)

	// Combine progress bar and time display on one line
	progressLine := lipgloss.JoinHorizontal(lipgloss.Center,
		progressBar,
		" ",
		footerTextStyle.Render(timeDisplay),
	)

	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	prevText := whiteStyle.Render(m.Icons.Prev)
	playPauseText := whiteStyle.Render(icon)
	nextText := whiteStyle.Render(m.Icons.Next)
	statusLine := lipgloss.JoinHorizontal(lipgloss.Left,
		prevText,
		" ",
		playPauseText,
		" ",
		nextText,
	)
	volumeLabel := footerTextStyle.Render(m.Icons.Volume + " ")
	volumeBar := m.Components.VolumeBar.ViewAs(m.Audio.Volume)
	volumeSection := lipgloss.JoinHorizontal(lipgloss.Left,
		volumeLabel,
		volumeBar,
	)

	// Combine play/pause icon (left) and volume section (right-aligned) on one line
	availableWidth := plan.Width - 4
	playWidth := lipgloss.Width(statusLine)
	volumeWidth := lipgloss.Width(volumeSection)
	spaceCount := availableWidth - playWidth - volumeWidth
	if spaceCount < 1 {
		spaceCount = 1
	}
	volumeLine := lipgloss.JoinHorizontal(lipgloss.Left,
		statusLine,
		strings.Repeat(" ", spaceCount),
		volumeSection,
	)

	// Lyric rendering (one_line mode): at most one row, marquee-scrolled. When
	// the panel is shown the footer has no lyric row at all.
	var lyricRows []string
	if plan.LyricMode == lyricModeOneLine {
		if text, ok := renderOneLineLyrics(m, plan.OneLineLyricWidth); ok {
			lyricRows = append(lyricRows, text)
		}
	}

	// Combine all lines vertically
	elements := []string{songLine, progressLine, volumeLine}
	elements = append(elements, lyricRows...)
	content := lipgloss.JoinVertical(lipgloss.Top, elements...)

	s := footerStyle
	if m.UI.Focus == FocusFooter {
		s = s.BorderForeground(lipgloss.Color("15"))
	}

	return s.Width(plan.Width).Render(content)
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func truncateLine(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	width := 0
	for i, r := range runes {
		cw := lipgloss.Width(string(r))
		if width+cw > maxWidth-1 {
			return string(runes[:i]) + "…"
		}
		width += cw
	}
	return s
}

// maxCompletionRows caps the candidate overlay height. It stays at or below
// footerBaseRows on purpose: the list replaces the footer band, so it never
// asks for a row the frame does not have.
const maxCompletionRows = 5

// minCompactWidth is the narrowest row that still gets a middle ellipsis.
const minCompactWidth = 12

// markerWidth is the width of the selection marker ("  " or "▸ "): every row
// carries one, so the value and the description share the remaining cells.
const markerWidth = 2

// completionRows is the number of overlay rows this frame: candidates capped by
// maxCompletionRows and by the footer band the list is drawn in.
func completionRows(m *Model, plan layoutPlan) int {
	if m.UI.Mode != ModeCommand {
		return 0
	}
	rows := len(m.completionCandidates)
	if rows > maxCompletionRows {
		rows = maxCompletionRows
	}
	if rows > plan.FooterRows {
		rows = plan.FooterRows
	}
	return rows
}

// completionTop is the frame row the candidate list starts on: the bottom rows
// of the footer band, directly above the command line.
//
// The rows are what make the position safe. maxCompletionRows <= footerBaseRows
// keeps the list inside the footer band on every terminal, so the content box
// and the lyrics panel are never covered — only the footer is, and only while a
// list is up. The list spans the frame's columns: appLayoutStyle adds neither
// border nor padding, so every column is usable and the offsets are absolute.
func completionTop(plan layoutPlan, rows int) int {
	return plan.Height - helpHeight - rows
}

// completionWindowStart is the first visible candidate: the window centres on
// the selection and clamps to the list.
func completionWindowStart(m *Model, rows int) int {
	total := len(m.completionCandidates)
	if rows <= 0 || total <= rows {
		return 0
	}
	if m.completionIndex < 0 {
		return 0
	}
	start := m.completionIndex - rows/2
	if start < 0 {
		start = 0
	}
	if limit := total - rows; start > limit {
		start = limit
	}
	return start
}

// renderCompletion renders the candidate block: one row per candidate, up to
// completionRows rows, each exactly width cells wide.
func renderCompletion(m *Model, plan layoutPlan) string {
	rows := completionRows(m, plan)
	if rows == 0 {
		return ""
	}
	width := plan.Width // the list spans the frame
	start := completionWindowStart(m, rows)

	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		index := start + i
		lines = append(lines, completionRow(m.completionCandidates[index], index == m.completionIndex, width))
	}
	return strings.Join(lines, "\n")
}

// overlayCompletion draws the candidate list over the footer band, at the row
// completionTop returns.
//
// Only the footer rows go through the compositor: compositing re-renders a line
// and re-encodes its SGR sequences, and the header, the content box, the lyrics
// panel and the command line all have to come through byte for byte what they
// were. The list never leaves the footer band, so nothing outside it has to be
// touched. The stack is sized to the band and the list sits inside it, so the
// frame keeps its size and the overlay consumes no rows.
//
// The layers go through a compositor rather than a plain canvas: a layer's Draw
// paints at the area it is handed, so only the compositor applies the offsets
// set with Y and X.
func overlayCompletion(m *Model, plan layoutPlan, frame string, rows int) string {
	top := completionTop(plan, rows)

	bandTop := plan.Height - helpHeight - plan.FooterRows
	lines := strings.Split(frame, "\n")
	band := strings.Join(lines[bandTop:bandTop+plan.FooterRows], "\n")

	stack := lipgloss.NewCompositor(
		lipgloss.NewLayer(band),
		lipgloss.NewLayer(renderCompletion(m, plan)).
			Y(top-bandTop).
			Z(1),
	)
	// The band is replaced in place: the rows above it (header, content, panel)
	// and below it (the command line) never reach the compositor.
	composited := strings.Split(stack.Render(), "\n")
	tail := lines[bandTop+plan.FooterRows:]
	return strings.Join(append(append(lines[:bandTop:bandTop], composited...), tail...), "\n")
}

// completionRow renders one row, padded to avail. Three tiers:
// value plus description, value only, then a compacted value. The marker is
// part of the row, so the tier checks budget for it: every row comes out
// exactly avail wide, which is what keeps the composited list the same size as
// the frame underneath it.
func completionRow(cand candidate, selected bool, avail int) string {
	value, desc := cand.Value, cand.Desc
	budget := avail - markerWidth
	if w := lipgloss.Width(value); w > budget {
		value = compactValue(cand, budget)
		desc = ""
	} else if desc != "" && w+2+lipgloss.Width(desc) > budget {
		desc = ""
	}

	marker := "  "
	rowStyle, descStyle := completionRowStyle, completionDescStyle
	if selected {
		marker = "▸ "
		rowStyle, descStyle = completionSelectedStyle, completionSelectedDescStyle
	}

	head := marker + value
	if desc == "" {
		pad := avail - lipgloss.Width(head)
		return rowStyle.Render(head + strings.Repeat(" ", max(pad, 0)))
	}
	// The description only survives tier 1, so head+2+desc fits and pad >= 0.
	pad := avail - lipgloss.Width(head) - lipgloss.Width(desc) - 2
	return rowStyle.Render(head+"  ") + descStyle.Render(desc+strings.Repeat(" ", max(pad, 0)))
}

// compactValue shortens an over-wide candidate: paths lose their left-hand
// directories (the file name is what tells them apart), everything else is
// truncated the way the rest of the UI truncates.
func compactValue(cand candidate, avail int) string {
	if !cand.Path {
		return truncateLine(cand.Value, avail)
	}
	return compactPath(cand.Value, avail)
}

// compactPath keeps the file name and as many trailing directories as fit,
// replacing the head with an ellipsis: "…/VeryLong/name.mp3". The split follows
// the separators the path is written with, so a Windows path is compacted on its
// own separators instead of being read as one unbreakable name.
func compactPath(path string, avail int) string {
	if lipgloss.Width(path) <= avail {
		return path
	}
	if avail < minCompactWidth {
		return truncateLine(path, avail)
	}
	sep := pathSep(path)
	segs := strings.Split(path, sep)
	if len(segs) > 0 && segs[0] == "" {
		segs = segs[1:] // absolute path: drop the leading empty segment
	}
	base := segs[len(segs)-1]
	if lipgloss.Width(base)+2 > avail {
		return truncateLine(path, avail) // pathological file name
	}

	best := "…" + sep + base
	for k := 2; k <= len(segs); k++ {
		cand := "…" + sep + strings.Join(segs[len(segs)-k:], sep)
		if lipgloss.Width(cand) > avail {
			break
		}
		best = cand
	}
	return best
}

func renderHelp(m *Model) string {
	if m.UI.Mode == ModeCommand {
		return renderCommandLine(m)
	}

	if m.switchingTrack {
		idx := m.loadingTick / 6 % len(loadingFrames)
		return infoStyle.Width(m.UI.Width).Render(" " + loadingFrames[idx] + " Loading...")
	}

	if m.Info.Message != "" && m.Info.Visible {
		return infoStyle.Width(m.UI.Width).Render(" " + m.Info.Message)
	}

	if m.Error.Message != "" && m.Error.Visible {
		return errorStyle.Width(m.UI.Width).Render(" " + m.Error.Message)
	}

	return m.Components.Help.View(keys)
}
