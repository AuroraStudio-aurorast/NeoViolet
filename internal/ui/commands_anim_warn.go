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

// renderAnimWarning draws the photosensitivity gate in place of the animation.
//
// The gate is this program's own rather than the library's. The library's would
// claim the space bar, which is already play/pause here, and it cannot know the
// styles or the words the rest of this interface uses. The animation is loaded
// but deliberately never started, so nothing flashes before somebody chooses to
// see it.
func renderAnimWarning(m *Model, plan layoutPlan) string {
	readings := m.Anim.Warning()
	palette := newAnimWarningPalette(m.Accent)
	lines := warnDetail(readings, palette)
	width := warningBlockWidth(plan.ContentInnerW)
	if len(wrapWarnLines(lines, width)) > plan.ContentInnerH {
		lines = warnSummary(readings, palette)
	}
	if len(wrapWarnLines(lines, width)) > plan.ContentInnerH {
		lines = warnCompact(readings, palette)
	}
	return layoutWarnLines(lines, plan.ContentInnerW, plan.ContentInnerH)
}

// renderAnimPowerWarning draws the battery gate. Like the photosensitivity gate
// it is drawn by the host, and unlike it nothing has been loaded at all: this
// reading comes from the machine rather than from the file.
func renderAnimPowerWarning(m *Model, plan layoutPlan) string {
	reading := m.Anim.Reading()
	palette := newAnimWarningPalette(m.Accent)
	lines := powerWarnDetail(reading, m.warnBelow(), palette)
	if len(wrapWarnLines(lines, warningBlockWidth(plan.ContentInnerW))) > plan.ContentInnerH {
		lines = powerWarnCompact(reading, m.warnBelow(), palette)
	}
	return layoutWarnLines(lines, plan.ContentInnerW, plan.ContentInnerH)
}

// powerWarnDetail is the battery gate when there is room to say what it costs.
//
// It is written for whoever is playing music, so it names no setting and says
// nothing about how the program works: the charge, what an animation costs, and
// the way out.
func powerWarnDetail(reading power.Status, warnBelow int, palette animWarningPalette) []warnLine {
	return []warnLine{
		{Text: "Low battery", Style: palette.caution},
		{},
		{Text: palette.battery(reading.Percent, warnBelow, false)},
		{},
		{Text: "Playing the animation may drain the battery faster.", Style: palette.body},
		{Text: "Only the animation is waiting. Music is unaffected.", Style: palette.muted},
		{},
		{Text: palette.keys(false)},
	}
}

// powerWarnCompact keeps the charge, cost and actions when the explanation and
// its spacing would not fit after wrapping.
func powerWarnCompact(reading power.Status, warnBelow int, palette animWarningPalette) []warnLine {
	return []warnLine{
		{Text: "Low battery", Style: palette.caution},
		{Text: palette.battery(reading.Percent, warnBelow, true)},
		{Text: palette.keys(false)},
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
func warnDetail(p anim.Photosensitivity, palette animWarningPalette) []warnLine {
	lines := []warnLine{
		{Text: "Photosensitivity warning", Style: palette.title},
		{},
		{Text: "Flashing may trigger seizures.", Style: palette.body},
	}
	for _, reason := range strings.Split(warnReason(p), "\n") {
		lines = append(lines, warnLine{Text: reason, Style: palette.body})
	}
	lines = append(lines, warnLine{})

	areaLimit := int(photosensitivity.DefaultAreaThreshold * 1000)
	if p.Analysed() {
		areaLimit = p.Assessment.AreaThresholdPermille
	}
	for _, row := range warnTable(p) {
		style := palette.body
		switch {
		case strings.HasPrefix(row, "  limit"), strings.Contains(row, "gen/s"):
			style = palette.muted
		case strings.HasPrefix(row, "  the file says"):
			row = palette.measurements(row, [3]bool{
				p.Declared.General > photosensitivity.FlashLimitPerSecond,
				p.Declared.Red > photosensitivity.FlashLimitPerSecond,
				p.Declared.HasArea && float64(p.Declared.Area) > float64(areaLimit),
			})
		case strings.HasPrefix(row, "  our analysis"):
			row = palette.measurements(row, [3]bool{
				p.Assessment.GeneralFlashesPerSecond > photosensitivity.FlashLimitPerSecond,
				p.Assessment.RedFlashesPerSecond > photosensitivity.FlashLimitPerSecond,
				p.Assessment.MaxFlashAreaPermille > areaLimit,
			})
		}
		lines = append(lines, warnLine{Text: row, Style: style})
	}

	lines = append(lines,
		warnLine{},
		warnLine{Text: "The animation is not playing yet.", Style: palette.body},
		warnLine{
			Text:  palette.keys(false),
			Style: palette.body,
		},
		warnLine{Text: warnProvenance(p), Style: palette.muted},
	)
	return lines
}

// warnSummary keeps the source of the verdict visible when the table will not
// fit. Narrow forced lyric panels may wrap both the source and the actions.
func warnSummary(p anim.Photosensitivity, palette animWarningPalette) []warnLine {
	reason := "File and analysis both failed."
	rate := warnCompactReason(p, palette)
	switch {
	case !p.Analysed():
		reason = "File check failed; our analysis could not run."
	case p.Declared.Verdict == player.VerdictPass:
		reason = "File declares a pass; our analysis failed."
	case p.Declared.Verdict == player.VerdictUnknown:
		reason = "File declares no check; our analysis failed."
	case p.Assessment.Verdict == photosensitivity.VerdictPass:
		reason = "File check failed; our analysis passed."
		rate = palette.body.Render(fmt.Sprintf("Our analysis: %d/s (limit %d/s).",
			p.Assessment.GeneralFlashesPerSecond, photosensitivity.FlashLimitPerSecond))
	}
	return []warnLine{
		{Text: "Photosensitivity warning", Style: palette.title},
		{},
		{Text: "Flashing may trigger seizures.", Style: palette.body},
		{Text: rate},
		{Text: reason, Style: palette.muted},
		{},
		{Text: "The animation is not playing yet.", Style: palette.body},
		{Text: palette.keys(false)},
	}
}

// warnCompact is the four-line fallback for a box too short for the summary's
// source explanation and spacing. It keeps the risk, waiting state and actions.
func warnCompact(p anim.Photosensitivity, palette animWarningPalette) []warnLine {
	return []warnLine{
		{Text: "Photosensitivity warning", Style: palette.title},
		{Text: warnCompactReason(p, palette)},
		{Text: "The animation is not playing yet.", Style: palette.body},
		{Text: palette.keys(true)},
	}
}

// warnReason names which reading raised the gate, in the words of whoever is
// deciding whether to keep watching. The two readings disagree in the cases that
// matter, so which one objected is worth saying.
func warnReason(p anim.Photosensitivity) string {
	switch {
	case !p.Analysed():
		return "This file failed its own photosensitivity check.\nOur analysis could not run on it."
	case p.Declared.Verdict == player.VerdictFail && p.Assessment.Verdict == photosensitivity.VerdictPass:
		return "This file failed its own photosensitivity check,\nbut our analysis passed."
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
func warnCompactReason(p anim.Photosensitivity, palette animWarningPalette) string {
	general := 0
	if p.Analysed() {
		general = p.Assessment.GeneralFlashesPerSecond
	} else {
		// #nosec G115 -- a flash rate from the file's own metadata, which the
		// container stores unsigned and no plausible file makes large.
		general = int(p.Declared.General)
	}
	style := palette.body
	if general > photosensitivity.FlashLimitPerSecond {
		style = palette.risk
	}
	return palette.body.Render("Up to ") + style.Render(fmt.Sprintf("%d", general)) +
		palette.body.Render(fmt.Sprintf(" flashes/s (limit %d/s).", photosensitivity.FlashLimitPerSecond))
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

// wrapWarnLines measures the same ANSI-aware wrapping used to draw the warning,
// so a narrow content area cannot turn a fitting layout into clipped actions.
func wrapWarnLines(lines []warnLine, width int) []warnLine {
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
	return wrapped
}

func warningBlockWidth(width int) int {
	return min(58, max(1, width-4))
}

// layoutWarnLines centers a left-aligned reading block and fills the content
// height exactly. Styles are applied after wrapping so they survive line breaks.
func layoutWarnLines(lines []warnLine, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	blockWidth := warningBlockWidth(width)
	wrapped := wrapWarnLines(lines, blockWidth)
	left := strings.Repeat(" ", (width-blockWidth)/2)

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
		out = append(out, left+ansi.Truncate(line.Style.Render(line.Text), blockWidth, ""))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}
