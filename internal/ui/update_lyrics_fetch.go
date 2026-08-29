package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ipc"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
)

// maybeFetchLyrics starts an async online lyric fetch when the track has no
// local lyrics and all auto-fetch preconditions hold. Returns nil otherwise.
func (m *Model) maybeFetchLyrics(path string) tea.Cmd {
	fetchCfg := m.Config.Lyrics.Fetch
	if !fetchCfg.Enabled || !hasOnlineEntry(m.Config.Lyrics.FormatPriority) {
		return nil
	}
	title := m.Audio.CurrentSong
	artist := m.Audio.Artist
	if title == "" || artist == "" || artist == "Unknown Artist" {
		return nil
	}
	if audio.IsSyntheticFormat(filepath.Ext(path)) {
		return nil
	}
	baseURL := effectiveBaseURL(fetchCfg.BaseURL)
	if m.fetchRateLimit.Blocked(baseURL) {
		logger.Debug("lyrics fetch skipped: provider cooling down", "base_url", baseURL)
		return nil
	}
	sig := m.currentSig()
	if _, _, ok := m.fetchCache.Lookup(sig); ok {
		return nil // found / negative / cooling / pending: nothing to do
	}
	m.fetchCache.Store(sig, fetch.CachePending, nil)
	m.LyricsFetching = true

	meta := fetch.TrackMeta{Title: title, Artist: artist, Album: m.Audio.Album, Duration: m.Audio.Duration.Seconds()}
	return m.buildFetchCmd(meta, sig, baseURL)
}

// buildFetchCmd returns a tea.Cmd that runs FetchLyrics and reports the result
// via FetchLyricsResultMsg. Shared by auto-fetch and :lrc switch online.
func (m *Model) buildFetchCmd(meta fetch.TrackMeta, sig, baseURL string) tea.Cmd {
	fetchCfg := m.Config.Lyrics.Fetch
	opts := fetch.FetchOpts{
		BaseURL:     baseURL,
		Timeout:     time.Duration(fetchCfg.Timeout) * time.Second,
		InsecureTLS: fetchCfg.InsecureTLS,
		Basic:       fetchCfg.Security == "basic",
		Cache:       m.fetchCache,
		RateLimit:   m.fetchRateLimit,
	}
	return func() tea.Msg {
		data, err := fetch.FetchLyrics(context.Background(), meta, opts)
		return FetchLyricsResultMsg{Data: data, Err: err, Sig: sig}
	}
}

// handleFetchLyricsResult mounts fetched lyrics, guarding against stale
// results from a superseded track switch.
func handleFetchLyricsResult(m *Model, msg FetchLyricsResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		// Transient failures must not poison the cache or the pending marker;
		// negative results were already cached inside FetchLyrics.
		if !errors.Is(msg.Err, fetch.ErrNotFound) && !errors.Is(msg.Err, fetch.ErrNoMatch) {
			m.fetchCache.Clear(msg.Sig)
		}
		if msg.Sig != m.currentSig() {
			return m, nil
		}
		switch {
		case errors.Is(msg.Err, fetch.ErrOffline):
			logger.Debug("lyrics fetch offline, skipped")
		case errors.Is(msg.Err, fetch.ErrRateLimited):
			logger.Warn("lyrics fetch rate limited")
		default:
			logger.Debug("lyrics fetch failed", "err", msg.Err)
		}
		m.LyricsFetching = false
		return m, nil
	}
	if msg.Sig != m.currentSig() {
		return m, nil // stale success: cache was written, display is dropped
	}
	m.LyricsFetching = false
	if msg.Data != nil {
		m.Audio.Lyrics = msg.Data
		m.Audio.LyricIndex = -1
		m.Audio.ShowLyrics = true
	}
	return m, nil
}

// currentSig returns the normalized signature of the current track.
func (m *Model) currentSig() string {
	return fetch.Sign(m.Audio.CurrentSong, m.Audio.Artist, m.Audio.Album, m.Audio.Duration.Seconds())
}

// hasOnlineEntry reports whether "online" is present in the priority list.
func hasOnlineEntry(priority []string) bool {
	for _, f := range priority {
		if f == "online" {
			return true
		}
	}
	return false
}

// effectiveBaseURL returns the configured provider URL or the default.
func effectiveBaseURL(cfgBase string) string {
	if cfgBase != "" {
		return cfgBase
	}
	return config.DefaultBaseURL
}

// lyricSig builds a compact signature for change detection across lyric pushes.
// When lines are empty/nil the signature is stable (no elapsed) to avoid spam.
func lyricSig(lines []ipc.LyricLineJSON, elapsed time.Duration, nextIdx int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d|", len(lines))
	for _, l := range lines {
		fmt.Fprintf(&sb, "%s|", l.Text)
	}
	fmt.Fprintf(&sb, "next=%d", nextIdx)
	if len(lines) > 0 {
		fmt.Fprintf(&sb, "|elapsed=%.1f", elapsed.Seconds())
	}
	return sb.String()
}

// Includes all agents (AgentFilter temporarily cleared), plus up to 2 previous
// and 2 next lines for context (so the GUI can show surrounding lyrics).
func buildLyricLinesJSON(data *lyrics.LyricsData, elapsed time.Duration) []ipc.LyricLineJSON {
	if data == nil || len(data.Lines) == 0 {
		return nil
	}

	// Clear AgentFilter to get all agents' lines.
	savedFilter := data.AgentFilter
	data.AgentFilter = ""
	defer func() { data.AgentFilter = savedFilter }()

	active := data.ActiveLines(elapsed)

	// Collect all relevant time values (distinct).
	seen := make(map[time.Duration]bool)
	var times []time.Duration
	addTime := func(t time.Duration) {
		if !seen[t] {
			seen[t] = true
			times = append(times, t)
		}
	}

	for _, line := range active {
		addTime(line.Time)
	}

	// Find context: up to 2 previous and 2 next time slots.
	// Walk backwards from the smallest active time.
	if len(times) > 0 {
		firstActive := times[0]
		for _, line := range data.Lines {
			if line.Time < firstActive {
				addTime(line.Time)
			}
		}
	}
	// Walk forwards from the largest active time.
	if len(times) > 0 {
		lastActive := times[len(times)-1]
		for _, line := range data.Lines {
			if line.Time > lastActive {
				addTime(line.Time)
			}
		}
	}

	// Sort the collected times.
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	// Keep: all active times, plus up to 2 before the first active, plus up to 2 after the last active.
	firstActiveIdx := -1
	lastActiveIdx := -1
	for i, t := range times {
		activeSet := make(map[time.Duration]bool)
		for _, a := range active {
			activeSet[a.Time] = true
		}
		if activeSet[t] {
			if firstActiveIdx < 0 {
				firstActiveIdx = i
			}
			lastActiveIdx = i
		}
	}

	keepStart := firstActiveIdx - 2
	if keepStart < 0 {
		keepStart = 0
	}
	keepEnd := lastActiveIdx + 2
	if keepEnd >= len(times) {
		keepEnd = len(times) - 1
	}

	keepSet := make(map[time.Duration]bool)
	for i := keepStart; i <= keepEnd; i++ {
		keepSet[times[i]] = true
	}

	// Build output: all lines whose Time is in keepSet.
	out := make([]ipc.LyricLineJSON, 0)
	for _, line := range data.Lines {
		if keepSet[line.Time] {
			displayText := data.LineDisplayText(line)
			agentName := ""
			if line.Agent != "" && data.Agents != nil {
				agentName = data.Agents[line.Agent]
			}
			out = append(out, ipc.LyricLineJSON{
				Time:      line.Time.Seconds(),
				End:       line.End.Seconds(),
				Text:      displayText,
				Agent:     line.Agent,
				AgentName: agentName,
			})
		}
	}
	return out
}
