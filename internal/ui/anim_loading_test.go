package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAnimLoadingCentersItsLabelAtEverySize(t *testing.T) {
	m := setupModel()
	for _, size := range [][2]int{{42, 10}, {66, 9}, {78, 17}, {118, 25}, {8, 3}, {0, 4}, {20, 0}} {
		plan := layoutPlan{ContentInnerW: size[0], ContentInnerH: size[1]}
		body := renderAnimLoading(m, plan)
		if size[0] == 0 || size[1] == 0 {
			if body != "" {
				t.Errorf("%v: empty region returned content", size)
			}
			continue
		}
		rows := strings.Split(ansi.Strip(body), "\n")
		if len(rows) != size[1] {
			t.Fatalf("%v: got %d rows", size, len(rows))
		}
		found := false
		for y, row := range rows {
			if lipgloss.Width(row) > size[0] {
				t.Fatalf("%v: overflowing row %q", size, row)
			}
			label := strings.TrimSpace(row)
			if label == "" {
				continue
			}
			found = true
			if size[0] >= 17 && label != "Loading animation" {
				t.Errorf("%v: label = %q", size, label)
			}
			left := len(row) - len(strings.TrimLeft(row, " "))
			right := size[0] - left - lipgloss.Width(label)
			if math.Abs(float64(left-right)) > 1 || math.Abs(float64(y-(size[1]-1-y))) > 1 {
				t.Errorf("%v: label not centered: left=%d right=%d row=%d", size, left, right, y)
			}
		}
		if !found {
			t.Errorf("%v: loading label missing", size)
		}
	}
}

func TestAnimLoadingSweepMovesRightWithoutMovingText(t *testing.T) {
	m := setupModel()
	plan := layoutPlan{ContentInnerW: 78, ContentInnerH: 17}
	var previousPlain string
	brightest := make([]int, 0, 2)
	for _, tick := range []int{10, 20} {
		m.loadingTick = tick
		body := renderAnimLoading(m, plan)
		plain, inks := warningForegrounds(t, body)
		if previousPlain != "" && plain != previousPlain {
			t.Fatal("sweep moved or changed its text")
		}
		previousPlain = plain
		best, at := -1.0, -1
		for i, ink := range inks {
			if ink == nil || plain[i] == ' ' || plain[i] == '\n' {
				continue
			}
			if light := testLuminance(color.RGBAModel.Convert(ink).(color.RGBA)); light > best {
				best, at = light, i
			}
		}
		brightest = append(brightest, at)
		for y, row := range frameBackgrounds(t, body) {
			for x, bg := range row {
				if bg != nil {
					t.Fatalf("loading cell %d,%d paints over the frame background", x, y)
				}
			}
		}
	}
	if brightest[0] < 0 || brightest[1] <= brightest[0] {
		t.Fatalf("highlight did not move right: %v", brightest)
	}
}

func TestAnimLoadingCyclesInOnePointFourSecondsAcrossTickRates(t *testing.T) {
	m := setupModel()
	plan := layoutPlan{ContentInnerW: 78, ContentInnerH: 17}
	for _, rate := range []int{10, 30, 60} {
		m.Config.TickRate = rate
		m.loadingTick = 0
		first := renderAnimLoading(m, plan)
		m.loadingTick = rate * 7 / 5
		if got := renderAnimLoading(m, plan); got != first {
			t.Errorf("%d Hz: sweep does not repeat after 1.4 seconds", rate)
		}
		m.loadingTick = rate / 2
		if got := renderAnimLoading(m, plan); got == first {
			t.Errorf("%d Hz: sweep stayed still", rate)
		}
	}
}
