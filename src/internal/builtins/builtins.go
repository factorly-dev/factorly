// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package builtins

import (
	"github.com/factorly-dev/factorly/internal/config"
)

// Options controls which built-in tools are registered.
type Options struct {
	Mode string // "stdio" or "http" — local tools only register in stdio mode
}

// Register adds built-in tools to the config. Built-ins execute in-process
// via the builtin provider — no subprocess needed for most operations.
func Register(cfg *config.Config, opts Options) {
	if cfg.DisableBuiltins {
		return
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]config.ToolConfig)
	}

	disabled := make(map[string]bool, len(cfg.DisabledBuiltins))
	for _, name := range cfg.DisabledBuiltins {
		disabled[name] = true
	}

	register := func(name string, tc config.ToolConfig) {
		if disabled[name] {
			return
		}
		cfg.Tools[name] = tc
	}

	// Universal (all modes) — runs server-side where credentials live
	register("factorly.fetch", config.ToolConfig{
		Type:        "builtin",
		Description: "Fetch a URL (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "url", Description: "URL to fetch", Required: true},
		},
	})

	// Local only (stdio mode) — runs on the agent's machine
	if opts.Mode == "http" {
		return
	}

	register("factorly.shell", config.ToolConfig{
		Type:        "builtin",
		Description: "Run a shell command (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "command", Description: "Shell command to execute", Required: true},
		},
	})

	register("factorly.read_file", config.ToolConfig{
		Type:        "builtin",
		Description: "Read a local file (overseen, logged, compressed)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to read", Required: true},
		},
	})

	register("factorly.write_file", config.ToolConfig{
		Type:        "builtin",
		Description: "Write content to a local file (overseen, logged, confirmable)",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to write", Required: true},
			{Name: "content", Description: "Content to write", Required: true},
		},
	})

	register("factorly.clipboard", config.ToolConfig{
		Type:        "builtin",
		Description: "Copy text to the system clipboard (overseen, logged, confirmable)",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "text", Type: "text", Description: "Text to copy to clipboard", Required: true},
		},
	})
}
