package ui

import (
	"fmt"
	"math"
	"strings"
)

func (p *pageState) renderChannelControls(c *pageCanvas, x, width int, focused bool) {
	h := len(c.rows)
	positions := [5]int{3, 4, 5, 6, 7}
	switch {
	case h >= 22:
		positions = [5]int{5, 9, 11, 14, 18}
	case h >= 18:
		positions = [5]int{4, 7, 9, 11, 14}
	case h >= 14:
		positions = [5]int{4, 6, 7, 9, 11}
	}
	pan := "Center"
	if p.effects.Pan < 0 {
		pan = fmt.Sprintf("L %.0f%%", -p.effects.Pan*100)
	} else if p.effects.Pan > 0 {
		pan = fmt.Sprintf("R %.0f%%", p.effects.Pan*100)
	}
	toggle := func(on bool) string {
		if on {
			return "[ On ]"
		}
		return "[ Off ]"
	}
	values := [5]string{pan, toggle(p.effects.Mono), toggle(p.effects.Swap), effectDB(p.effects.PreampDB) + " dB", toggle(p.effects.Bypass)}
	for i, y := range positions {
		active := focused && p.effect == i+5
		marker := "  "
		if active {
			marker = "› "
		}
		c.pair(y, x, width, marker+effectLabels[i+5], values[i])
		if active {
			c.highlight(y, x, width)
		}
	}
	if h >= 18 {
		p.renderEffectRail(c, positions[0]+1, x+2, width-2, p.effects.Pan, -1, 1, p.effect == 5 && focused, "L", "R")
	}
	if h >= 22 {
		p.renderEffectRail(c, positions[3]+1, x+2, width-2, p.effects.PreampDB, -12, 0, p.effect == 8 && focused, "-12", "0")
	}
}
func (p *pageState) renderEffectRail(c *pageCanvas, y, x, width int, value, low, high float64, active bool, left, right string) {
	railX := x + len(left) + 1
	railWidth := width - len(left) - len(right) - 2
	if railWidth < 3 {
		return
	}
	c.dim(y, x, left)
	c.dim(y, railX, strings.Repeat("─", railWidth))
	c.dim(y, railX+railWidth+1, right)
	positionFor := func(v float64) int {
		return railX + int(math.Round((v-low)/(high-low)*float64(railWidth-1)))
	}
	if low < 0 && high > 0 {
		c.dim(y, positionFor(0), "┊")
	}
	position := positionFor(value)
	if active && !p.effects.Bypass {
		c.label(y, position, "●")
	} else {
		c.dim(y, position, "●")
	}
}
