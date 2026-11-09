package ui

import "charm.land/bubbles/v2/key"

func pageHelpBindings(m *Model) []key.Binding {
	binding := func(k, description string) key.Binding {
		return key.NewBinding(key.WithKeys(k), key.WithHelp(k, description))
	}
	if m.pages == nil || m.animVisible() {
		return nil
	}
	if m.pages.editor != nil {
		return []key.Binding{binding("enter", "confirm"), binding("esc", "cancel")}
	}
	if m.pages.picker != "" {
		return []key.Binding{binding("↑/↓", "move"), binding("enter", "choose"), binding("esc", "back")}
	}
	bindings := []key.Binding{binding("↑/↓", "move")}
	switch m.UI.ActiveTab {
	case 0:
		bindings = append(bindings, binding("f", "filter"), binding("←/→", "format"))
	case 1:
		if m.pages.saved {
			bindings = append(bindings, binding("P/a", "play/append"), binding("c/e", "create/rename"), binding("r", "refresh"))
		} else {
			bindings = append(bindings, binding("s", "save queue"))
		}
		bindings = append(bindings, binding("v", "queue/saved"))
	case 2:
		bindings = []key.Binding{binding("←/→", "select"), binding("+/−", "sample")}
	case 3:
		bindings = append(bindings, binding("enter", "edit"), binding("1-6", "group"))
	}
	return bindings
}
