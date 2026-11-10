package ui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type browserEntry struct {
	name, path string
	directory  bool
}

// The browser belongs to one page and every read carries its navigation
// generation. Workers read immutable inputs and never mutate the model.
type fileBrowser struct {
	viewport               listViewport
	purpose                string
	field, cursor          int
	generation             uint64
	directory, target, err string
	entries                []browserEntry
	loading                bool
}
type fileBrowserMsg struct {
	browser    *fileBrowser
	generation uint64
	target     string
	directory  bool
	entries    []browserEntry
	err        error
}

func (m *Model) openFileBrowser(purpose string, field int, start string) tea.Cmd {
	if m.Anim != nil {
		m.Anim.Close()
	}
	if start == "" {
		if purpose == "soundfont" && m.pages.draft.SoundfontPath != "" {
			start = filepath.Dir(m.pages.draft.SoundfontPath)
		}
		if purpose == "audio" && m.Audio.Player != nil && !remoteAudioLocation(m.Audio.Player.Path()) {
			start = filepath.Dir(m.Audio.Player.Path())
		}
		if start == "" {
			start, _ = os.UserHomeDir()
		}
		if start == "" {
			start = "."
		}
	}
	m.pages.browser = &fileBrowser{purpose: purpose, field: field, entries: []browserEntry{}}
	m.pages.editor = nil
	m.UI.Mode = ModeNormal
	m.UI.Focus = FocusContent
	return m.fileBrowserLocation(start)
}

func (m *Model) fileBrowserLocation(target string) tea.Cmd {
	b := m.pages.browser
	if b == nil {
		return nil
	}
	target = expandTilde(target)
	if !remoteAudioLocation(target) && !filepath.IsAbs(target) && b.directory != "" {
		target = filepath.Join(b.directory, target)
	}
	b.generation++
	generation, purpose := b.generation, b.purpose
	b.target = target
	b.loading = true
	b.err = ""
	return func() tea.Msg {
		result := fileBrowserMsg{browser: b, generation: generation, target: target, entries: []browserEntry{}}
		if remoteAudioLocation(target) {
			if purpose != "audio" || !validRemoteAudioLocation(target) {
				result.err = fmt.Errorf("enter an HTTP(S) audio URL with a host")
			}
			return result
		}
		absolute, err := filepath.Abs(target)
		if err != nil {
			result.err = err
			return result
		}
		result.target = absolute
		// #nosec G703 -- target is an explicitly selected local path.
		info, err := os.Stat(absolute)
		if err != nil {
			result.err = err
			return result
		}
		if !info.IsDir() {
			if purpose == "directory" {
				result.err = fmt.Errorf("choose a directory")
				return result
			}
			if !info.Mode().IsRegular() {
				result.err = fmt.Errorf("choose a regular file or directory")
				return result
			}
			if purpose == "soundfont" && !strings.EqualFold(filepath.Ext(absolute), ".sf2") {
				result.err = fmt.Errorf("choose a .sf2 SoundFont file")
			}
			if purpose == "soundfont" && result.err == nil {
				_, result.err = parseSetting(pageSetting{key: "soundfont_path"}, absolute)
			}
			return result
		}
		result.directory = true
		result.entries, result.err = readBrowserDirectory(absolute, purpose)
		return result
	}
}

func readBrowserDirectory(path, purpose string) ([]browserEntry, error) {
	// #nosec G703 -- directory was chosen by the user, listing is read-only.
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]browserEntry, 0, len(entries))
	for _, entry := range entries {
		directory := entry.IsDir()
		full := filepath.Join(path, entry.Name())
		regular := entry.Type().IsRegular()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			directory, regular = info.IsDir(), info.Mode().IsRegular()
		}
		allowed := playableExt(filepath.Ext(entry.Name()))
		if purpose == "directory" {
			allowed = false
		}
		if purpose == "soundfont" {
			allowed = strings.EqualFold(filepath.Ext(entry.Name()), ".sf2")
		}
		if !directory && (!regular || !allowed) {
			continue
		}
		out = append(out, browserEntry{name: entry.Name(), path: full, directory: directory})
	}
	slices.SortFunc(out, func(a, b browserEntry) int {
		if a.directory != b.directory {
			if a.directory {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.name, b.name)
	})
	return out, nil
}

func (m *Model) applyFileBrowser(msg fileBrowserMsg) tea.Cmd {
	b := m.pages.browser
	if b == nil || b != msg.browser || b.generation != msg.generation || m.exiting {
		return nil
	}
	b.loading = false
	if msg.err != nil {
		b.err = browserErrorText(msg.err)
		return nil
	}
	if msg.directory {
		b.directory = msg.target
		b.entries = msg.entries
		b.cursor = 0
		if m.pages.editor != nil && m.pages.editor.purpose == "browser_location" {
			m.pages.editor = nil
		}
		return nil
	}
	if b.purpose == "directory" && !msg.directory {
		if err := m.pages.addLibraryRoot(b.field, msg.target); err != nil {
			b.err = err.Error()
			return nil
		}
		m.pages.browser = nil
		m.pages.editor = nil
		return nil
	}
	if b.purpose == "soundfont" {
		if err := m.pages.applyDraftValue(b.field, msg.target); err != nil {
			b.err = err.Error()
			return nil
		}
		m.pages.browser = nil
		m.pages.editor = nil
		return nil
	}
	m.pages.browser = nil
	m.pages.editor = nil
	_, cmd := handleLoadTrack(m, LoadTrackMsg{Path: msg.target})
	return cmd
}

func (m *Model) updateFileBrowser(msg tea.KeyPressMsg) tea.Cmd {
	b := m.pages.browser
	switch msg.String() {
	case "esc", "ctrl+g":
		m.pages.browser = nil
	case "up", "k":
		b.cursor = max(0, b.cursor-1)
	case "down", "j":
		b.cursor = max(0, min(len(b.entries)-1, b.cursor+1))
	case "home", "g":
		b.cursor = 0
	case "end", "G":
		b.cursor = max(0, len(b.entries)-1)
	case "pgup":
		b.cursor = max(0, b.cursor-5)
	case "pgdown":
		b.cursor = max(0, min(len(b.entries)-1, b.cursor+5))
	case "backspace", "left", "u":
		if b.directory != "" {
			return m.fileBrowserLocation(filepath.Dir(b.directory))
		}
	case "ctrl+l":
		b.generation++
		b.loading = false
		value := b.directory
		if value == "" {
			value = b.target
		}
		return m.pages.openEditor("Go to path or audio URL", "browser_location", value, -1)
	case "r":
		value := b.directory
		if value == "" {
			value = b.target
		}
		return m.fileBrowserLocation(value)
	case "s":
		if b.purpose == "directory" && b.directory != "" {
			return m.selectLibraryDirectory()
		}
	case "enter", "right":
		if b.loading || len(b.entries) == 0 {
			return nil
		}
		entry := b.entries[min(b.cursor, len(b.entries)-1)]
		if msg.String() == "right" && !entry.directory {
			return nil
		}
		return m.fileBrowserLocation(entry.path)
	}
	return nil
}

func remoteAudioLocation(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}
func validRemoteAudioLocation(path string) bool {
	u, err := url.Parse(path)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}
func validOpenLocation(path string) bool {
	if remoteAudioLocation(path) {
		return validRemoteAudioLocation(path)
	}
	return isValidAudioPath(path)
}

func browserErrorText(err error) string {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return "Cannot open path: " + pathError.Err.Error()
	}
	return err.Error()
}
