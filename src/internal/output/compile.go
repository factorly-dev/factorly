// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package output

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/ohler55/ojg/jp"
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
	JSONPath    string              `yaml:"json_path,omitempty"`
	Pipe        *PipeConfig         `yaml:"pipe,omitempty"`
}

// PipeConfig is the YAML representation of a pipe tool.
// For CLI pipes, command/args are used directly.
// For REST/MCP pipes, the proxy resolves execution at runtime.
type PipeConfig struct {
	Type    string   `yaml:"type,omitempty"`    // "cli" (default), "rest", "mcp"
	Command string   `yaml:"command,omitempty"` // CLI: executable
	Args    []string `yaml:"args,omitempty"`    // CLI: arguments
	Timeout string   `yaml:"timeout,omitempty"` // e.g. "5s", "10s"

	// REST fields
	BaseURL string            `yaml:"base_url,omitempty"`
	Method  string            `yaml:"method,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`

	// MCP fields
	Tool string `yaml:"tool,omitempty"` // MCP tool name to call
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

	if cfg.JSONPath != "" {
		expr, err := jp.ParseString(cfg.JSONPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid json_path %q: %v\n", cfg.JSONPath, err)
		} else {
			f.JSONPath = expr
		}
	}

	if cfg.Pipe != nil {
		pipeType := cfg.Pipe.Type
		if pipeType == "" && cfg.Pipe.Command != "" {
			pipeType = "cli"
		}
		if pipeType == "cli" && cfg.Pipe.Command != "" {
			pipe := &PipeCommand{
				Command: cfg.Pipe.Command,
				Args:    cfg.Pipe.Args,
			}
			if cfg.Pipe.Timeout != "" {
				if d, err := time.ParseDuration(cfg.Pipe.Timeout); err == nil {
					pipe.Timeout = d
				} else {
					fmt.Fprintf(os.Stderr, "warning: invalid pipe timeout %q: %v\n", cfg.Pipe.Timeout, err)
				}
			}
			f.Pipe = pipe
		}
		// REST/MCP pipes: the proxy sets f.PipeFn at runtime after reading PipeConfig
		// from the tool's FilterConfig. Nothing to compile here.
	}

	// Return nil if nothing was configured (avoids empty filter overhead)
	hasPipe := f.Pipe != nil || (cfg.Pipe != nil && (cfg.Pipe.Type == "rest" || cfg.Pipe.Type == "mcp"))
	if len(f.MatchOutput) == 0 && len(f.StripLines) == 0 && len(f.KeepLines) == 0 &&
		len(f.Replace) == 0 && f.HeadLines == 0 && f.TailLines == 0 && f.MaxLines == 0 &&
		len(f.JSONPath) == 0 && !hasPipe {
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
