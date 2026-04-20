// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter defines a per-tool output filtering pipeline.
// Regexes are pre-compiled at load time.
type Filter struct {
	MatchOutput []MatchRule
	StripLines  []*regexp.Regexp
	KeepLines   []*regexp.Regexp
	Replace     []ReplaceRule
	HeadLines   int
	TailLines   int
	MaxLines    int
}

// MatchRule short-circuits the entire output if Pattern matches.
type MatchRule struct {
	Pattern *regexp.Regexp
	Message string
	Unless  *regexp.Regexp // nil if not set
}

// ReplaceRule applies a regex substitution to each line.
type ReplaceRule struct {
	Pattern     *regexp.Regexp
	Replacement string
}

// Apply runs the filter pipeline on the input string.
// Returns the filtered output. Nil filter is a no-op.
func (f *Filter) Apply(s string) string {
	if f == nil {
		return s
	}

	// Stage a: match_output short-circuit
	for _, m := range f.MatchOutput {
		if m.Pattern.MatchString(s) {
			if m.Unless != nil && m.Unless.MatchString(s) {
				continue // unless guard triggered, skip this rule
			}
			return m.Message
		}
	}

	lines := strings.Split(s, "\n")

	// Stage b: strip_lines / keep_lines (mutually exclusive)
	if len(f.KeepLines) > 0 {
		lines = keepMatching(lines, f.KeepLines)
	} else if len(f.StripLines) > 0 {
		lines = stripMatching(lines, f.StripLines)
	}

	// Stage c: replace
	for _, r := range f.Replace {
		for i, line := range lines {
			lines[i] = r.Pattern.ReplaceAllString(line, r.Replacement)
		}
	}

	// Stage d: head/tail with omission marker
	if f.HeadLines > 0 || f.TailLines > 0 {
		lines = headTail(lines, f.HeadLines, f.TailLines)
	}

	// Stage e: max_lines
	if f.MaxLines > 0 && len(lines) > f.MaxLines {
		lines = lines[:f.MaxLines]
		lines = append(lines, fmt.Sprintf("... truncated to %d lines", f.MaxLines))
	}

	return strings.Join(lines, "\n")
}

func stripMatching(lines []string, patterns []*regexp.Regexp) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if matchesAny(line, patterns) {
			continue
		}
		result = append(result, line)
	}
	return result
}

func keepMatching(lines []string, patterns []*regexp.Regexp) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if matchesAny(line, patterns) {
			result = append(result, line)
		}
	}
	return result
}

func matchesAny(s string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

func headTail(lines []string, head, tail int) []string {
	total := len(lines)
	if head <= 0 {
		head = 0
	}
	if tail <= 0 {
		tail = 0
	}
	if head+tail >= total {
		return lines // nothing to omit
	}

	result := make([]string, 0, head+tail+1)
	result = append(result, lines[:head]...)
	omitted := total - head - tail
	result = append(result, fmt.Sprintf("... %d lines omitted ...", omitted))
	result = append(result, lines[total-tail:]...)
	return result
}
