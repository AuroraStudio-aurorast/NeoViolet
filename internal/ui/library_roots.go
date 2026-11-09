package ui

import (
	"fmt"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
)

func (p *pageState) renderLibraryRoots(c *pageCanvas) {
	c.label(0, 1, "Settings / Music folders")
	c.dim(1, 1, "Folder changes stay in the draft until s Save")
	c.rule(2, 1, c.width-2)
	roots := p.draft.Library.Roots
	available := max(1, len(c.rows)-5)
	start := max(0, p.rootCursor-available+1)
	for n, root := range roots[start:min(len(roots), start+available)] {
		marker := "  "
		if start+n == p.rootCursor {
			marker = "▸ "
		}
		c.text(4+n, 1, marker+compactPath(browserDisplayText(root), max(0, c.width-4)))
		if start+n == p.rootCursor {
			c.highlight(4+n, 1, c.width-2)
		}
	}
	if len(roots) == 0 {
		c.dim(4, 1, "No music folders · a Add a folder")
	}
	c.dim(len(c.rows)-1, 1, "a Add · d Remove reference · Esc Back")
}
func (m *Model) updateLibraryRoots(msg tea.KeyPressMsg) tea.Cmd {
	p := m.pages
	switch msg.String() {
	case "esc", "ctrl+g":
		p.rootEditor = false
	case "up", "k":
		p.rootCursor = max(0, p.rootCursor-1)
	case "down", "j":
		p.rootCursor = max(0, min(len(p.draft.Library.Roots)-1, p.rootCursor+1))
	case "a":
		return m.openFileBrowser("directory", p.cursors[3], "")
	case "d":
		roots := slices.Clone(p.draft.Library.Roots)
		if len(roots) > 0 {
			roots = slices.Delete(roots, p.rootCursor, p.rootCursor+1)
			_ = p.applyDraftValue(p.cursors[3], roots)
			p.rootCursor = max(0, min(p.rootCursor, len(roots)-1))
		}
	}
	return nil
}
func (p *pageState) addLibraryRoot(index int, path string) error {
	roots := slices.Clone(p.draft.Library.Roots)
	if !slices.Contains(roots, path) {
		roots = append(roots, path)
	}
	if err := p.applyDraftValue(index, roots); err != nil {
		return fmt.Errorf("music folder: %w", err)
	}
	p.rootCursor = max(0, len(roots)-1)
	return nil
}

func (m *Model) selectLibraryDirectory() tea.Cmd {
	b := m.pages.browser
	b.generation++
	generation := b.generation
	path := b.directory
	b.loading = true
	return func() tea.Msg {
		return fileBrowserMsg{browser: b, generation: generation, target: path, err: validateLibraryRoots([]string{path})}
	}
}

func validateLibraryRoots(roots []string) error {
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a music directory", root)
		}
	}
	return nil
}
