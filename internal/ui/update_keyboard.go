package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// fullwidthRune maps fullwidth Unicode characters/CJK keyboard to their ASCII equivalents.
func fullwidthRune(r rune) rune {
	switch r {
	case '：', '；':
		return ':'
	case '［', '【', '「':
		return '['
	case '］', '】', '」':
		return ']'
	case '／':
		return '/'
	case '－':
		return '-'
	case '＋':
		return '+'
	case '＝':
		return '='
	case '＞', '》':
		return '>'
	case '＜', '《':
		return '<'
	case '？':
		return '?'
	case '＇', '‘', '’':
		return '\''
	case '＂', '“', '”':
		return '"'
	case '＾':
		return '^'
	case '～':
		return '~'
	case '＿':
		return '_'
	case '＠':
		return '@'
	case '＃':
		return '#'
	case '％':
		return '%'
	case '＆':
		return '&'
	case '＊':
		return '*'
	case '（':
		return '('
	case '）':
		return ')'
	}
	return r
}

// normalizedKey wraps a key string so that key.Matches sees a normalized version
// with fullwidth characters mapped to ASCII.
type normalizedKey struct{ raw string }

func (k normalizedKey) String() string {
	return strings.Map(fullwidthRune, k.raw)
}

// normMatch is like key.Matches but normalizes fullwidth characters first.
func normMatch(msg tea.KeyPressMsg, b ...key.Binding) bool {
	return key.Matches(normalizedKey{msg.String()}, b...)
}

func handleKeyPress(m *Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.UI.Mode == ModeCommand {
		return handleCommandModeKeyPress(m, msg)
	}
	return handleNormalModeKeyPress(m, msg)
}
