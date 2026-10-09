package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
)

// contentBoxAllocationBudget is what wrapping a frame was measured at, with room
// for the box growing. Its point is that putting the frame itself back into the
// style's measuring path -- thousands of allocations for a dense frame -- has to
// argue its case here rather than pass unnoticed.
const contentBoxAllocationBudget = 400

// denseFrame builds a frame the shape a real animation hands over: every cell
// carries its own two colours, which is what makes it both large and expensive
// for anything that has to walk it.
func denseFrame(columns, lines int) string {
	var b strings.Builder
	for y := 0; y < lines; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < columns; x++ {
			fmt.Fprintf(&b, "\x1b[38;5;%dm\x1b[48;5;%dm▀", (x*7)%256, (y*11)%256)
		}
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// boxPlan is a real plan for a terminal showing both the content area and the
// lyric panel, so the box under test has the geometry the layout gives it.
func boxPlan() layoutPlan {
	return computeLayout(100, 30, input(config.PanelModeOn, true, false))
}

func styledBox(style lipgloss.Style, block string, plan layoutPlan) string {
	return style.Width(plan.ContentWidth).Height(plan.ContentHeight).Render(block)
}

func TestContentBox_SpendsLittleOnAFrameItAlreadyFits(t *testing.T) {
	plan := boxPlan()
	block := denseFrame(plan.ContentInnerW, plan.ContentInnerH)

	got := int(testing.AllocsPerRun(20, func() { contentBox(contentStyle, block, plan) }))
	if got > contentBoxAllocationBudget {
		t.Fatalf("drawing a frame cost %d allocations, want no more than %d", got, contentBoxAllocationBudget)
	}
	t.Logf("%d allocations for %d bytes of frame", got, len(block))
}

func TestContentBox_DrawsTheSameBoxTheStyleWouldHave(t *testing.T) {
	plan := boxPlan()
	block := denseFrame(plan.ContentInnerW, plan.ContentInnerH)

	for _, focused := range []bool{false, true} {
		style := contentStyle
		if focused {
			style = style.BorderForeground(lipgloss.Color("15"))
		}
		want := styledBox(style, block, plan)
		got := contentBox(style, block, plan)

		// The bytes differ by the colour the pieces carry in and the reset that
		// follows it -- inert sequences, and a pen left in a known state for the
		// frame. What the terminal is asked to show is the same either way.
		if ansi.Strip(got) != ansi.Strip(want) {
			t.Fatalf("focused=%v: the box differs from the one the style draws\n got:\n%s\nwant:\n%s", focused, ansi.Strip(got), ansi.Strip(want))
		}
	}
}

func TestContentBox_KeepsTheFramesOwnBytes(t *testing.T) {
	plan := boxPlan()
	block := denseFrame(plan.ContentInnerW, plan.ContentInnerH)
	got := contentBox(contentStyle, block, plan)

	for i, line := range strings.Split(block, "\n") {
		if !strings.Contains(got, line) {
			t.Fatalf("line %d of the frame did not survive drawing: %q", i, line)
		}
	}
}

// An animation that has nothing to show still owns the box: the frame is blank,
// and the box is still the size the layout gave it.
func TestContentBox_BlanksAFrameWithNothingInIt(t *testing.T) {
	plan := boxPlan()

	got := contentBox(contentStyle, "", plan)
	if want := ansi.Strip(styledBox(contentStyle, "", plan)); ansi.Strip(got) != want {
		t.Fatalf("a blank frame drew\n%s\nwant\n%s", ansi.Strip(got), want)
	}
	if lines := strings.Count(got, "\n") + 1; lines != plan.ContentHeight {
		t.Fatalf("a blank frame drew %d lines, want %d", lines, plan.ContentHeight)
	}
}

// The border is the one part of the box that carries a colour of its own, and a
// box whose border lost it would pass a comparison of its glyphs.
func TestContentBox_KeepsTheBorderColour(t *testing.T) {
	plan := boxPlan()
	block := denseFrame(plan.ContentInnerW, plan.ContentInnerH)

	for _, focused := range []bool{false, true} {
		style := contentStyle
		if focused {
			style = style.BorderForeground(lipgloss.Color("15"))
		}

		// The ink is whatever the style drew with, taken from the style's own
		// render rather than named here, so the test cannot disagree with it.
		ink := borderInk(styledBox(style, block, plan))
		if ink == "" {
			t.Fatalf("focused=%v: the style drew no colour on its border", focused)
		}
		if got := contentBox(style, block, plan); !strings.Contains(got, ink) {
			t.Fatalf("focused=%v: the border was not drawn in %q", focused, ink)
		}
	}
}

// borderInk is the escape sequence the box draws its left border with.
func borderInk(box string) string {
	glyph := strings.Index(box, "│")
	if glyph <= 0 {
		return ""
	}
	start := strings.LastIndex(box[:glyph], "\x1b[")
	if start < 0 {
		return ""
	}
	return box[start : glyph+len("│")]
}

// The command takes no arguments: ":anim on" is a typo, and answering it by
// toggling the animation would be the wrong thing to do about a typo.
func TestAnimCommand_TakesNoArguments(t *testing.T) {
	m := animModel(t, animPlainFixture, 100, 30)
	m.Anim.Close()

	runAnim(m, invocation{Parts: []string{"anim", "on"}})

	if m.Error.Message == "" {
		t.Error("an argument was ignored")
	}
	if m.animVisible() {
		t.Error("an argument toggled the animation anyway")
	}
}
