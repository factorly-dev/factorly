// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"fmt"
	"os"
	"regexp"
)

// FilterConfig is the raw YAML/config representation of a filter.
type FilterConfig struct {
	MatchOutput []MatchOutputConfig `yaml:"match_output,omitempty"`
	StripLines  []string            `yaml:"strip_lines,omitempty"`
	KeepLines   []string            `yaml:"keep_lines,omitempty"`
	Replace     []ReplaceConfig     `yaml:"replace,omitempty"`
	HeadLines   int                 `yaml:"head_lines,omitempty"`
	TailLines   int                 `yaml:"tail_lines,omitempty"`
	MaxLines    int                 `yaml:"max_lines,omitempty"`
}

// MatchOutputConfig is the YAML representation of a match_output rule.
type MatchOutputConfig struct {
	Pattern string `yaml:"pattern"`
	Message string `yaml:"message"`
	Unless  string `yaml:"unless,omitempty"`
}

// ReplaceConfig is the YAML representation of a replace rule.
type ReplaceConfig struct {
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
}

// CompileFilter converts a FilterConfig (strings) into a Filter (compiled regexes).
// Invalid regexes are logged as warnings and skipped.
func CompileFilter(cfg *FilterConfig) *Filter {
	if cfg == nil {
		return nil
	}

	f := &Filter{
		HeadLines: cfg.HeadLines,
		TailLines: cfg.TailLines,
		MaxLines:  cfg.MaxLines,
	}

	for _, m := range cfg.MatchOutput {
		p := compileRegex(m.Pattern)
		if p == nil {
			continue
		}
		rule := MatchRule{Pattern: p, Message: m.Message}
		if m.Unless != "" {
			rule.Unless = compileRegex(m.Unless)
		}
		f.MatchOutput = append(f.MatchOutput, rule)
	}

	for _, s := range cfg.StripLines {
		if p := compileRegex(s); p != nil {
			f.StripLines = append(f.StripLines, p)
		}
	}

	for _, s := range cfg.KeepLines {
		if p := compileRegex(s); p != nil {
			f.KeepLines = append(f.KeepLines, p)
		}
	}

	for _, r := range cfg.Replace {
		p := compileRegex(r.Pattern)
		if p == nil {
			continue
		}
		f.Replace = append(f.Replace, ReplaceRule{Pattern: p, Replacement: r.Replacement})
	}

	// Return nil if nothing was configured (avoids empty filter overhead)
	if len(f.MatchOutput) == 0 && len(f.StripLines) == 0 && len(f.KeepLines) == 0 &&
		len(f.Replace) == 0 && f.HeadLines == 0 && f.TailLines == 0 && f.MaxLines == 0 {
		return nil
	}

	return f
}

func compileRegex(pattern string) *regexp.Regexp {
	r, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid filter regex %q: %v\n", pattern, err)
		return nil
	}
	return r
}
