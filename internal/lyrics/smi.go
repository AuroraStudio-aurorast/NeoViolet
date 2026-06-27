package lyrics

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	smiSyncRe = regexp.MustCompile(`(?i)<SYNC\s+Start\s*=\s*(\d+)`)
	smiClassRe = regexp.MustCompile(`(?i)<P\s+Class\s*=\s*([^>\s]+)`)
	// Strip HTML tags except we need to track content between them
	htmlTagRe = regexp.MustCompile(`<[^>]*>`)
	// Match SMI metadata tags in HEAD
	smiTitleRe  = regexp.MustCompile(`(?i)<TITLE>([^<]*)</TITLE>`)
	smiBodyRe   = regexp.MustCompile(`(?is)<BODY[^>]*>(.*?)</BODY>`)
)

// agentClassMapping maps common SMI language class IDs to agent IDs.
// These are conventional class names used in SAMI files (KRCC=Korean,
// ENCC=English, JPCC=Japanese, etc.).
var agentClassCounter = 0

func init() {
	RegisterParser("smi", &smiParser{})
	agentClassCounter = 0
}

type smiParser struct{}

func (p *smiParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".smi", ".sami")
}

func (p *smiParser) Parse(r io.Reader, sourcePath string) (*LyricsData, error) {
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, fmt.Errorf("read smi: %w", err)
	}

	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", "\n")

	lyrics := &LyricsData{Path: sourcePath}

	// Extract metadata from HEAD
	if titleMatch := smiTitleRe.FindStringSubmatch(content); len(titleMatch) >= 2 {
		lyrics.Title = strings.TrimSpace(titleMatch[1])
	}

	// Extract BODY section
	bodyMatch := smiBodyRe.FindStringSubmatch(content)
	if bodyMatch == nil {
		return nil, fmt.Errorf("no <BODY> section found in smi")
	}
	bodyContent := bodyMatch[1]

	// Find all sync points with their content
	type syncEntry struct {
		startMs int64
		classes []classText // text per class at this sync point
	}

	syncPoints := smiSyncRe.FindAllStringSubmatchIndex(bodyContent, -1)
	if len(syncPoints) == 0 {
		return nil, fmt.Errorf("no <SYNC> elements found in smi")
	}

	var entries []syncEntry
	agentClasses := make(map[string]string) // class -> agent ID
	agentClassCounter = 0

	for i, sp := range syncPoints {
		startStr := bodyContent[sp[2]:sp[3]]
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			continue
		}

		// Content from this sync point to the next (or end of body)
		endPos := len(bodyContent)
		if i+1 < len(syncPoints) {
			endPos = syncPoints[i+1][0]
		}
		section := bodyContent[sp[1]:endPos]

		// Find all <P Class=xxx> text fragments within this section
		classMatches := smiClassRe.FindAllStringSubmatchIndex(section, -1)

		var classes []classText
		for j, cm := range classMatches {
			className := section[cm[2]:cm[3]]

			// Text starts after the closing > of the <P ...> tag.
			// The regex doesn't include > in the match, so scan forward.
			textStart := cm[1]
			if gtIdx := strings.IndexByte(section[cm[1]:], '>'); gtIdx >= 0 {
				textStart = cm[1] + gtIdx + 1
			}
			textEnd := len(section)
			if j+1 < len(classMatches) {
				textEnd = classMatches[j+1][0]
			} else {
				// End at next <SYNC> or end of section
				if nextSync := strings.Index(section[cm[1]:], "<SYNC"); nextSync >= 0 {
					textEnd = cm[1] + nextSync
				}
			}

			rawText := section[textStart:textEnd]
			cleanText := cleanSMIText(rawText)
			if cleanText == "" {
				continue
			}

			// Assign agent ID for this class if not already mapped
			if _, exists := agentClasses[className]; !exists {
				agentClassCounter++
				agentClasses[className] = fmt.Sprintf("v%d", agentClassCounter)
			}

			classes = append(classes, classText{
				Class: className,
				Text:  cleanText,
			})
		}

		if len(classes) == 0 {
			continue
		}

		entries = append(entries, syncEntry{
			startMs: startMs,
			classes: classes,
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid smi entries found")
	}

	// Build agent display names
	agents := make(map[string]string)
	for class, agentID := range agentClasses {
		agents[agentID] = class
	}
	lyrics.Agents = agents

	// Build LyricLine entries
	var lines []LyricLine
	for i, entry := range entries {
		start := time.Duration(entry.startMs) * time.Millisecond

		// End time is the next sync point's start (or 0 for last = unbounded)
		var end time.Duration
		if i+1 < len(entries) {
			end = time.Duration(entries[i+1].startMs) * time.Millisecond
		}

		for _, ct := range entry.classes {
			agentID := agentClasses[ct.Class]

			lines = append(lines, LyricLine{
				Time:  start,
				End:   end,
				Text:  ct.Text,
				Agent: agentID,
			})
		}
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no valid smi lines found")
	}

	sortLyricLines(lines)

	lyrics.Lines = lines
	return lyrics, nil
}

type classText struct {
	Class string
	Text  string
}

// cleanSMIText strips HTML tags, decodes common HTML entities, and trims whitespace.
func cleanSMIText(raw string) string {
	// Remove HTML tags
	cleaned := htmlTagRe.ReplaceAllString(raw, "")
	// Decode common HTML entities
	cleaned = decodeHTMLEntities(cleaned)
	// Normalize whitespace
	cleaned = strings.TrimSpace(cleaned)
	// Collapse multiple spaces/newlines into single space
	spaceRe := regexp.MustCompile(`\s+`)
	cleaned = spaceRe.ReplaceAllString(cleaned, " ")
	return cleaned
}

// decodeHTMLEntities decodes common HTML entities in text.
func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&#39;", "'")
	return s
}