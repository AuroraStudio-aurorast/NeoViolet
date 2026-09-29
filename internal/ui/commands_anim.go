package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"
)

// warnDetailRows is the box height at which the photosensitivity gate can show
// what it measured rather than only what it concluded. Below this it says the
// worst number and offers the same choice. TestWarnPanel_FitsItsBox checks the
// tall layout against this figure, so the two cannot drift apart.
const warnDetailRows = 14

// syncAnim keeps the animation aligned with the audio and sized for its box.
//
// It runs on the tick rather than from a resize or panel hook because the box
// depends on the terminal, the panel mode and the lyric row all at once, and the
// audio position moves underneath it. One idempotent call covers every one of
// those without any of them having to know an animation exists.
func (m *Model) syncAnim() tea.Cmd {
	if m.Anim == nil {
		return nil
	}
	plan := m.layoutPlan()
	return m.Anim.Sync(m.Audio.Elapsed, m.Audio.IsPlaying, plan.ContentInnerW, plan.ContentInnerH)
}

// runAnim implements ":anim" and its alias: it turns the animation for the
// current track on or off.
func runAnim(m *Model, _ invocation) (tea.Model, tea.Cmd) {
	if m.Anim == nil || m.Anim.Loading {
		return m, nil
	}
	if m.animVisible() {
		m.Anim.Close()
		return m, nil
	}
	if m.Audio.Player == nil {
		m.Error.Set("No audio loaded", m.Config.Error.Duration)
		return m, nil
	}

	plan := m.layoutPlan()
	m.animRequested = true
	return m, m.Anim.LoadFor(m.Audio.Player.Path(), plan.ContentInnerW, plan.ContentInnerH)
}

// handleAnimLoaded installs a sidecar that finished loading, or reports why
// there was none.
func handleAnimLoaded(m *Model, msg anim.LoadedMsg) (tea.Model, tea.Cmd) {
	requested := m.animRequested
	m.animRequested = false

	cmd := m.Anim.Apply(msg)

	switch {
	case errors.Is(m.Anim.Err, anim.ErrNoSidecar):
		// Most tracks have no animation, which is an answer rather than a
		// failure -- unless somebody asked for one by name.
		if requested {
			m.Error.Set("No animation for this track", m.Config.Error.Duration)
		} else {
			m.Info.Set("No animation for this track", m.Config.Error.Duration)
		}
	case m.Anim.Err != nil:
		m.Error.Set(fmt.Sprintf("Animation failed to load: %v", m.Anim.Err), m.Config.Error.Duration)
	}

	return m, cmd
}

// animVisible reports whether the animation is showing. A Model built without
// NewModel carries no animation state and reads as hidden.
func (m *Model) animVisible() bool {
	return m.Anim != nil && m.Anim.Visible
}

// animBody returns what the animation wants to draw in the content box, and
// whether it wants to draw anything at all.
//
// The box belongs to the active tab, and the animation borrows it: an .nvaa file
// is a sidecar of the track, like its lyrics, so it takes the content area
// rather than a page of its own.
func animBody(m *Model, plan layoutPlan) (string, bool) {
	if m.Anim == nil {
		return "", false
	}

	switch {
	case m.Anim.Loading:
		return renderAnimLoading(m, plan), true
	case m.Anim.Gated():
		return renderAnimWarning(m, plan), true
	case m.Anim.Visible:
		if !m.Anim.Fits(plan.ContentInnerW, plan.ContentInnerH) {
			// The box changed and the player has not been resized yet. One
			// blank frame is better than a frame that overflows the border.
			return "", true
		}
		return m.Anim.View(), true
	}
	return "", false
}

// renderAnimLoading fills the box while a sidecar is being read. The vectors in
// testdata load instantly; a full-length animation does not, and an empty box
// would read as a failure.
func renderAnimLoading(m *Model, plan layoutPlan) string {
	idx := m.loadingTick / 6 % len(loadingFrames)
	return layoutWarnLines([]warnLine{{
		Text:  loadingFrames[idx] + " Loading animation...",
		Style: infoStyle,
	}}, plan.ContentInnerW, plan.ContentInnerH)
}

// renderAnimWarning draws the photosensitivity gate in place of the animation.
//
// The gate is this program's own rather than the library's. The library's would
// claim the space bar, which is already play/pause here, and it cannot know the
// styles or the words the rest of this interface uses. The animation is loaded
// but deliberately never started, so nothing flashes before somebody chooses to
// see it.
func renderAnimWarning(m *Model, plan layoutPlan) string {
	readings := m.Anim.Warning()
	if plan.ContentInnerH >= warnDetailRows {
		return layoutWarnLines(warnDetail(readings), plan.ContentInnerW, plan.ContentInnerH)
	}
	return layoutWarnLines(warnCompact(readings), plan.ContentInnerW, plan.ContentInnerH)
}

// warnLine is one line of the gate before it is wrapped into the box.
type warnLine struct {
	Text  string
	Style lipgloss.Style
}

// warnDetail is the gate when there is room for the numbers.
//
// Both readings are listed because they can disagree, and the limit row is what
// makes the others mean anything: a flash rate without the threshold it broke is
// not something a viewer can weigh.
func warnDetail(p anim.Photosensitivity) []warnLine {
	lines := []warnLine{
		{Text: "PHOTOSENSITIVITY WARNING", Style: warnStyle},
		{},
	}
	for _, reason := range strings.Split(warnReason(p), "\n") {
		lines = append(lines, warnLine{Text: reason})
	}
	lines = append(lines, warnLine{})

	for _, row := range warnTable(p) {
		style := lipgloss.NewStyle()
		if strings.HasPrefix(row, "  limit") {
			style = panelContextStyle
		}
		lines = append(lines, warnLine{Text: row, Style: style})
	}

	lines = append(lines,
		warnLine{},
		warnLine{Text: "Playback is paused until you choose."},
		warnLine{},
		warnLine{
			Text:  inputStyle.Render("[enter]") + "  play anyway      " + inputStyle.Render("[esc]") + "  cancel",
			Style: lipgloss.NewStyle(),
		},
		warnLine{Text: warnProvenance(p), Style: panelContextStyle},
	)
	return lines
}

// warnCompact is the gate for a box with no room for the table. Below the detail
// threshold the smallest usable terminal leaves four rows, and the reason, the
// worst number and the way out all have to fit in them.
func warnCompact(p anim.Photosensitivity) []warnLine {
	return []warnLine{
		{Text: "PHOTOSENSITIVITY WARNING", Style: warnStyle},
		{Text: warnCompactReason(p)},
		{Text: "Playback is paused until you choose."},
		{Text: inputStyle.Render("[enter]") + " play anyway   " + inputStyle.Render("[esc]") + " cancel"},
	}
}

// warnReason names which reading raised the gate, in the words of whoever is
// deciding whether to keep watching. The two readings disagree in the cases that
// matter, so which one objected is worth saying.
func warnReason(p anim.Photosensitivity) string {
	switch {
	case !p.Analysed():
		return "This file failed its own photosensitivity check.\nOur analysis could not run on it."
	case p.Declared.Verdict == player.VerdictFail:
		return "This file failed its own photosensitivity check,\nand our analysis agrees."
	case p.Declared.Verdict == player.VerdictPass:
		return "This file declares a pass, but our analysis found\nflashing above the thresholds."
	default:
		return "This file declares no check of its own, and our\nanalysis failed it."
	}
}

// warnCompactReason is the one-line version: the worst flash rate against the
// limit it broke.
func warnCompactReason(p anim.Photosensitivity) string {
	general := 0
	if p.Analysed() {
		general = p.Assessment.GeneralFlashesPerSecond
	} else {
		// #nosec G115 -- a flash rate from the file's own metadata, which the
		// container stores unsigned and no plausible file makes large.
		general = int(p.Declared.General)
	}
	return fmt.Sprintf("Up to %d flashes/s (limit %d/s).", general, photosensitivity.FlashLimitPerSecond)
}

// warnTable is the header and one row per reading, then the limits. Area is
// measured in thousandths of the viewport, which is how the thresholds are
// defined, and shown as a percentage, which is how it is read.
func warnTable(p anim.Photosensitivity) []string {
	lines := []string{fmt.Sprintf("  %-13s %5s %5s %6s", "", "gen/s", "red/s", "area")}

	if p.Declared.Verdict != player.VerdictUnknown {
		lines = append(lines, declaredRow(p))
	}
	if p.Analysed() {
		lines = append(lines, warnRow("our analysis",
			p.Assessment.GeneralFlashesPerSecond,
			p.Assessment.RedFlashesPerSecond,
			p.Assessment.MaxFlashAreaPermille, true))
	}

	areaLimit := int(photosensitivity.DefaultAreaThreshold * 1000)
	if p.Analysed() {
		areaLimit = p.Assessment.AreaThresholdPermille
	}
	lines = append(lines, warnRow("limit",
		photosensitivity.FlashLimitPerSecond, photosensitivity.FlashLimitPerSecond, areaLimit, true))
	return lines
}

// declaredRow renders what the file claims about itself. Its fields are unsigned
// in the container format while the analysis measures signed, so the conversion
// is made once, here, where the source is known.
func declaredRow(p anim.Photosensitivity) string {
	// #nosec G115 -- flash rates from the file's own metadata: the container
	// stores them unsigned and the widest is the analysis viewport in cells.
	return warnRow("the file says",
		int(p.Declared.General), int(p.Declared.Red), int(p.Declared.Area), p.Declared.HasArea)
}

// warnRow renders one comparison row.
func warnRow(label string, general, red, areaPermille int, hasArea bool) string {
	area := "-"
	if hasArea {
		area = fmt.Sprintf("%d%%", areaPermille/10)
	}
	return fmt.Sprintf("  %-13s %5s %5s %6s",
		label, fmt.Sprintf("%d/s", general), fmt.Sprintf("%d/s", red), area)
}

// warnProvenance names the standard and the run behind the numbers, so a verdict
// can be checked rather than taken on faith. The analysis is an approximation,
// and saying so is the difference between a warning and a claim.
func warnProvenance(p anim.Photosensitivity) string {
	if !p.Analysed() {
		return photosensitivity.Standard
	}
	return fmt.Sprintf("%s  analysis %dx%d, %d frames",
		photosensitivity.Standard, p.Assessment.Columns, p.Assessment.Lines, p.Assessment.FramesAnalyzed)
}

// layoutWarnLines wraps lines into a box and fills it exactly.
//
// Both limits are hard. The content box is sized by the layout contract, so a
// line wider than the box or a block taller than it would push the border out
// and break the frame around it. Wrapping is done in plain text and the style is
// applied afterwards, which keeps a wrapped line's style on every row it
// occupies.
func layoutWarnLines(lines []warnLine, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	var wrapped []warnLine
	for _, line := range lines {
		if line.Text == "" {
			wrapped = append(wrapped, warnLine{})
			continue
		}
		for _, part := range strings.Split(lipgloss.Wrap(line.Text, width, ""), "\n") {
			wrapped = append(wrapped, warnLine{Text: part, Style: line.Style})
		}
	}

	// Anything that did not fit is dropped from the bottom, which is why the
	// least important line is written last.
	if len(wrapped) > height {
		wrapped = wrapped[:height]
	}

	out := make([]string, 0, height)
	for range (height - len(wrapped)) / 2 {
		out = append(out, "")
	}
	for _, line := range wrapped {
		out = append(out, ansi.Truncate(line.Style.Render(line.Text), width, ""))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}
