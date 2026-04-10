package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Hint controls which compression filters to apply.
type Hint string

const (
	HintJSON Hint = "json" // Compact JSON objects/arrays
	HintLogs Hint = "logs" // Deduplicate repeated lines
	HintAll  Hint = "all"  // Apply all filters
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

var blankLinePattern = regexp.MustCompile(`\n{3,}`)

// Compress applies output compression filters to reduce token usage.
// Always-on: ANSI stripping, whitespace normalization.
// Optional (via hints): JSON compaction, log deduplication.
func Compress(s string, hints ...Hint) string {
	if s == "" {
		return s
	}

	hintSet := make(map[Hint]bool, len(hints))
	for _, h := range hints {
		hintSet[h] = true
	}
	all := hintSet[HintAll]

	// 1. ANSI strip (always on)
	s = stripANSI(s)

	// 2. Whitespace normalize (always on)
	s = normalizeWhitespace(s)

	// 3. JSON compact (when json hint)
	if all || hintSet[HintJSON] {
		s = compactJSON(s)
	}

	// 4. Log dedup (when logs hint)
	if all || hintSet[HintLogs] {
		s = deduplicateLines(s)
	}

	return s
}

// Process applies compression then truncation in one call.
// This is the main entry point used by the proxy.
func Process(s string, maxBytes int, hints ...Hint) string {
	s = Compress(s, hints...)
	return Truncate(s, maxBytes)
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func normalizeWhitespace(s string) string {
	return blankLinePattern.ReplaceAllString(s, "\n\n")
}

func compactJSON(s string) string {
	trimmed := strings.TrimSpace(s)
	if !json.Valid([]byte(trimmed)) {
		return s
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return s
	}
	// Preserve leading/trailing whitespace from original if the trimmed
	// version was valid JSON.
	prefix := s[:len(s)-len(strings.TrimLeft(s, " \t\n\r"))]
	suffix := s[len(strings.TrimRight(s, " \t\n\r")):]
	return prefix + buf.String() + suffix
}

func deduplicateLines(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}

	var result []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		count := 1
		for i+count < len(lines) && lines[i+count] == line {
			count++
		}
		if count > 1 {
			result = append(result, fmt.Sprintf("%s [repeated %d times]", line, count))
		} else {
			result = append(result, line)
		}
		i += count
	}

	return strings.Join(result, "\n")
}
