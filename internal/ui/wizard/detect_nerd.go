// Package wizard provides the first-run terminal capability detection flow.
package wizard

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// nerdFontRe matches font names containing Nerd Font indicators.
// Matches "Nerd Font" (flexible spacing), "NerdFont" (concatenated),
// or standalone "NF" (word boundary on both sides, avoids INFO/CONF).
var nerdFontRe = regexp.MustCompile(`(?i)(?:Nerd\s*Font|NerdFont|\bNF\b)`)

// MatchNerdFont returns true if the name contains a Nerd Font indicator.
func MatchNerdFont(name string) bool {
	return nerdFontRe.MatchString(name)
}

// fastfetchBin caches the absolute path to the fastfetch binary.
var fastfetchBin string

// detectNerdFont checks multiple sources in order (fastest first):
//  1. NEOVIOLET_FONT env var (set by neoviolet-gui)
//  2. fastfetch --json --structure TerminalFont
//  3. System font index (fc-list / system_profiler / registry)
//  4. OSC 50 terminal escape query (xterm fallback)
func detectNerdFont() bool {
	// 1 — Env var from GUI (most reliable, zero cost)
	if font := os.Getenv("NEOVIOLET_FONT"); font != "" {
		return nerdFontRe.MatchString(font)
	}

	// 2 — fastfetch
	if font := queryFastfetch(); font != "" && nerdFontRe.MatchString(font) {
		return true
	}

	// 3 — System font index
	if hasSystemNerdFont() {
		return true
	}

	// 4 — OSC 50 terminal query
	if name := queryOSC50Font(); name != "" && nerdFontRe.MatchString(name) {
		return true
	}

	return false
}

// ── fastfetch ──────────────────────────────────────────────────────

// fastfetchPaths are common fastfetch installation paths per platform.
var fastfetchPaths = func() []string {
	p := []string{"/opt/homebrew/bin/fastfetch", "/usr/local/bin/fastfetch", "/usr/bin/fastfetch"}
	if h := os.Getenv("HOME"); h != "" {
		p = append(p, h+"/.local/bin/fastfetch", h+"/.cargo/bin/fastfetch")
	}
	return p
}()

func lookupFastfetch() string {
	if fastfetchBin != "" {
		return fastfetchBin
	}
	if p, err := exec.LookPath("fastfetch"); err == nil {
		fastfetchBin = p
		return p
	}
	for _, p := range fastfetchPaths {
		if _, err := os.Stat(p); err == nil {
			fastfetchBin = p
			return p
		}
	}
	return ""
}

type fastfetchResult struct {
	Result *struct {
		Font *struct {
			Name string `json:"name"`
		} `json:"font"`
	} `json:"result"`
}

func queryFastfetch() string {
	bin := lookupFastfetch()
	if bin == "" {
		return ""
	}
	// #nosec G204 -- bin is resolved from a fixed lookup list (PATH and known
	// install locations); args are a fixed flag set.
	out, err := exec.Command(bin, "--json", "--structure", "TerminalFont").Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	var results []fastfetchResult
	if err := json.Unmarshal(out, &results); err != nil || len(results) == 0 || results[0].Result == nil || results[0].Result.Font == nil {
		return ""
	}
	return results[0].Result.Font.Name
}

// ── System font index ──────────────────────────────────────────────

func hasSystemNerdFont() bool {
	switch runtime.GOOS {
	case "darwin":
		return hasNerdFontDarwin()
	case "linux":
		return hasNerdFontLinux()
	case "windows":
		return hasNerdFontWindows()
	}
	return false
}

// hasNerdFontDarwin checks macOS: fc-list (fast) → system_profiler (slow fallback).
func hasNerdFontDarwin() bool {
	if nerdFontCountFcList() > 0 {
		return true
	}
	type r struct{ found bool }
	ch := make(chan r, 1)
	go func() { ch <- r{nerdFontCountSystemProfiler() > 0} }()
	select {
	case v := <-ch:
		return v.found
	case <-time.After(5 * time.Second):
		return false
	}
}

func hasNerdFontLinux() bool { return nerdFontCountFcList() > 0 }

func hasNerdFontWindows() bool {
	cmds := []string{
		`Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts' | Select-String -Pattern Nerd`,
		`Get-ChildItem "$env:LOCALAPPDATA\Microsoft\Windows\Fonts" -Name | Select-String -Pattern Nerd`,
	}
	for _, c := range cmds {
		// #nosec G204 -- powershell is a fixed binary; the command is a fixed
		// registry/font query from a compile-time constant list.
		out, err := exec.Command("powershell", "-NoProfile", "-Command", c).Output()
		if err == nil && len(out) > 0 {
			return true
		}
	}
	return false
}

func nerdFontCountFcList() int {
	out, err := exec.Command("fc-list").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		// fc-list: path: Family1,Family2,...:style=...
		// We check each comma-separated family name.
		if colon := strings.Index(line, ":"); colon >= 0 {
			rest := line[colon+1:]
			if s2 := strings.Index(rest, ":"); s2 >= 0 {
				for _, fam := range strings.Split(rest[:s2], ",") {
					if nerdFontRe.MatchString(strings.TrimSpace(fam)) {
						n++
						break
					}
				}
			}
		}
	}
	return n
}

func nerdFontCountSystemProfiler() int {
	out, err := exec.Command("system_profiler", "SPFontsDataType").Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Family:") {
			name := strings.TrimSpace(line[7:])
			if nerdFontRe.MatchString(name) {
				n++
			}
		}
	}
	return n
}

// ── OSC 50 terminal query ──────────────────────────────────────────

func queryOSC50Font() string {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return ""
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer func() { _ = term.Restore(fd, old) }()

	_, _ = os.Stdout.WriteString("\x1b]50;?\x07")

	ch := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			ch <- ""
			return
		}
		ch <- extractOSC50Font(buf[:n])
	}()

	select {
	case name := <-ch:
		return name
	case <-time.After(100 * time.Millisecond):
		return ""
	}
}

func extractOSC50Font(data []byte) string {
	re := regexp.MustCompile(`\x1b\]50;(.+?)(?:\x07|\x1b\\)`)
	m := re.FindStringSubmatch(string(data))
	if len(m) < 2 {
		return ""
	}
	name := strings.TrimSpace(m[1])
	if strings.HasPrefix(name, "-") {
		parts := strings.Split(name, "-")
		if len(parts) > 2 {
			name = strings.ReplaceAll(parts[2], "_", " ")
		}
	}
	if idx := strings.IndexAny(name, ":"); idx >= 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}
