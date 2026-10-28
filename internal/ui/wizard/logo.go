package wizard

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// Each character is a pixel: dot is transparent, t/s are text/shadow,
// and W/g/p/P are the fixed white, gray, and purple icon colors.
var logoPixels = [...]string{
	"..tttttt..tttttt....ttttt.....ppp.....",
	".tssssssstsssssss..tssssss...pWWWPp...",
	"tss....ssss....sss.ts...sss..pWWWWgP..",
	"ts......ss......ss.t.....ss..pWWWWgPP.",
	"ts......tsstttttts.......ts..pWWWgPWgP",
	"ts......tssssssss........ts..pgggPWWgP",
	"ts......ts...............ts..pPPPWWgP.",
	"ts......tss....ttttt....tss..pWWWggP..",
	"ts.......ssstttss.ssstttss...pgggPP...",
	"ts........ssssss...ssssss.....PPP.....",
	"......................................",
	"tt......tt.tt.........tt...........tt.",
	"ts......ts.ts.........ts...........ts.",
	"ts......ts............ts...........ts.",
	"ts......ts.ts..ttttt..ts...ttttt..tsss",
	"tss....tss.ts.tssssss.ts..tssssss.tsss",
	".ss....ts..ts.ts...ss.ts..ts...ss..ss.",
	".ts....ts..ts.ts...ts.ts..tssttss..ts.",
	".tss..tss..ts.ts...ts.ts..ts.......ts.",
	"..ssttss...ts.tssttss.tss.tsstttt..tss",
	"...ssss....ts..sssss...sss.sssss....ss",
}

// renderLogo packs two pixel rows into each terminal row. Default background
// resets preserve transparency, including the bottom half of the final row.
func renderLogo(isDark bool) string {
	palette := map[byte]string{
		'W': "255;255;255",
		'g': "217;217;217",
		'p': "157;50;194",
		'P': "146;6;194",
		't': "24;24;24",
		's': "0;0;0",
	}
	if isDark {
		palette['t'] = "255;255;255"
		palette['s'] = "217;217;217"
	}

	var out strings.Builder
	for y := 0; y < len(logoPixels); y += 2 {
		out.WriteString("\x1b[0m")
		var previousForeground, previousBackground string
		for x := range len(logoPixels[0]) {
			top := palette[logoPixels[y][x]]
			var bottom string
			if y+1 < len(logoPixels) {
				bottom = palette[logoPixels[y+1][x]]
			}

			glyph, foreground, background := '▀', top, bottom
			if top == "" {
				foreground, background = bottom, ""
				glyph = '▄'
				if bottom == "" {
					glyph = ' '
				}
			}
			if foreground != previousForeground || background != previousBackground {
				fg, bg := "39", "49"
				if foreground != "" {
					fg = "38;2;" + foreground
				}
				if background != "" {
					bg = "48;2;" + background
				}
				out.WriteString("\x1b[" + fg + ";" + bg + "m")
				previousForeground, previousBackground = foreground, background
			}
			out.WriteRune(glyph)
		}
		out.WriteString("\x1b[0m\n")
	}
	return out.String()
}

// logoNote keeps background queries inside Bubble Tea's input loop so terminal
// replies cannot consume wizard keystrokes or block startup on unsupported terminals.
type logoNote struct {
	*huh.Note
	requestedBackground bool
}

func newLogoNote() *logoNote {
	return &logoNote{Note: huh.NewNote().Title(logoTitle(true))}
}

func logoTitle(isDark bool) string {
	return renderLogo(isDark) + "\nWelcome to NeoViolet — a terminal music player."
}

func (n *logoNote) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if background, ok := msg.(tea.BackgroundColorMsg); ok {
		n.Title(logoTitle(background.IsDark()))
	}
	_, cmd := n.Note.Update(msg)
	// Huh groups update skipped notes but do not call their Init methods.
	if !n.requestedBackground {
		n.requestedBackground = true
		cmd = tea.Batch(cmd, tea.RequestBackgroundColor)
	}
	return n, cmd
}
