// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
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
	Pipe        *PipeCommand // CLI pipe (executed directly)
	PipeFn      PipeFunc     // injected pipe (for REST/MCP, set by proxy)
}

// PipeCommand defines a CLI command to pipe output through.
type PipeCommand struct {
	Command string
	Args    []string
	Timeout time.Duration
}

// PipeFunc is a function that pipes output through an external tool.
// Used for REST/MCP pipes where the proxy injects the execution logic.
type PipeFunc func(input string) (string, error)

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

	result := strings.Join(lines, "\n")

	// Stage f: pipe through external tool
	if f.PipeFn != nil {
		if piped, err := f.PipeFn(result); err == nil {
			result = piped
		} else {
			fmt.Fprintf(os.Stderr, "warning: filter pipe failed: %v\n", err)
		}
	} else if f.Pipe != nil {
		if piped, err := runPipe(f.Pipe, result); err == nil {
			result = piped
		} else {
			fmt.Fprintf(os.Stderr, "warning: filter pipe failed: %v\n", err)
		}
	}

	return result
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

func runPipe(pipe *PipeCommand, input string) (string, error) {
	timeout := pipe.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pipe.Command, pipe.Args...)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
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
