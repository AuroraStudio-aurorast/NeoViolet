package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/anim"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/power"
)

const warnDetailRows = 14

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

// renderAnimPowerWarning draws the battery gate. Like the photosensitivity gate
// it is drawn by the host, and unlike it nothing has been loaded at all: this
// reading comes from the machine rather than from the file.
func renderAnimPowerWarning(m *Model, plan layoutPlan) string {
	reading := m.Anim.Reading()
	if plan.ContentInnerH >= warnDetailRows {
		return layoutWarnLines(powerWarnDetail(reading, m.warnBelow()), plan.ContentInnerW, plan.ContentInnerH)
	}
	return layoutWarnLines(powerWarnCompact(reading, m.warnBelow()), plan.ContentInnerW, plan.ContentInnerH)
}

// powerWarnDetail is the battery gate when there is room to say what it costs.
//
// It is written for whoever is playing music, so it names no setting and says
// nothing about how the program works: the charge, what an animation costs, and
// the way out.
func powerWarnDetail(reading power.Status, warnBelow int) []warnLine {
	return []warnLine{
		{Text: "LOW BATTERY WARNING", Style: warnStyle},
		{},
		{Text: fmt.Sprintf("Battery at %d%%, below %d%%.", reading.Percent, warnBelow)},
		{},
		{Text: "Playing the animation may drain the battery faster."},
		{},
		{Text: inputStyle.Render("[enter]") + "  play anyway      " + inputStyle.Render("[esc]") + "  cancel"},
	}
}

// powerWarnCompact is the battery gate for the smallest usable box: the charge,
// what it costs, and the way out, in three rows.
func powerWarnCompact(reading power.Status, warnBelow int) []warnLine {
	return []warnLine{
		{Text: "LOW BATTERY WARNING", Style: warnStyle},
		{Text: fmt.Sprintf("Battery at %d%%, below %d%%; the animation drains it faster.", reading.Percent, warnBelow)},
		{Text: inputStyle.Render("[enter]") + " play anyway   " + inputStyle.Render("[esc]") + " cancel"},
	}
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
		warnLine{Text: "The animation is not playing yet."},
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
		{Text: "The animation is not playing yet."},
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
