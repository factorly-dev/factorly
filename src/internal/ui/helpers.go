// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/factorly-dev/factorly/internal/output"
)

func dedup(s []string) []string {
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func splitArgs(s string) []string {
	var args []string
	var current []byte
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current = append(current, c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' {
			if len(current) > 0 {
				args = append(args, string(current))
				current = current[:0]
			}
		} else {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		args = append(args, string(current))
	}
	return args
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseFilterForm(r *http.Request) *output.FilterConfig {
	jsonPath := strings.TrimSpace(r.FormValue("filter_json_path"))
	keepLines := splitComma(r.FormValue("filter_keep_lines"))
	stripLines := splitComma(r.FormValue("filter_strip_lines"))
	headLines := 0
	tailLines := 0
	maxLines := 0
	if v := r.FormValue("filter_head_lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			headLines = n
		}
	}
	if v := r.FormValue("filter_tail_lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			tailLines = n
		}
	}
	if v := r.FormValue("filter_max_lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxLines = n
		}
	}

	var replaces []output.ReplaceConfig
	if raw := strings.TrimSpace(r.FormValue("filter_replace")); raw != "" {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "→", 2)
			if len(parts) == 2 {
				replaces = append(replaces, output.ReplaceConfig{
					Pattern:     strings.TrimSpace(parts[0]),
					Replacement: strings.TrimSpace(parts[1]),
				})
			}
		}
	}

	if jsonPath == "" && len(keepLines) == 0 && len(stripLines) == 0 &&
		headLines == 0 && tailLines == 0 && maxLines == 0 && len(replaces) == 0 {
		return nil
	}

	return &output.FilterConfig{
		JSONPath:   jsonPath,
		HeadLines:  headLines,
		TailLines:  tailLines,
		MaxLines:   maxLines,
		KeepLines:  keepLines,
		StripLines: stripLines,
		Replace:    replaces,
	}
}
