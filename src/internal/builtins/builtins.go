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
		// If the user already declared this builtin in their config
		// with a Shadow block, preserve their shadow over the builtin's
		// default. Lets users tighten or extend a builtin's oversight
		// (e.g., shadow.confirm: true on factorly.fetch, shadow.max_calls
		// on factorly.code) without losing the builtin's intrinsic
		// config (Type, Parameters, Description, MaxOutput, Compress).
		if existing, ok := cfg.Tools[name]; ok && existing.Shadow != nil {
			tc.Shadow = mergeShadow(tc.Shadow, existing.Shadow)
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

	// factorly.code runs an agent-supplied Go script through the same
	// yaegi sandbox used by `type: code` tools. Scripts can call other
	// registered tools via factorly.Call. Inner calls go through the
	// proxy's full shadow/vault/audit machinery — the script itself
	// can only reach what's already exposed as a tool.
	register("factorly.code", config.ToolConfig{
		Type:        "builtin",
		Description: "Run a Go script that can call other factorly tools (overseen, logged)",
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "code", Type: "text", Description: "Go source declaring `func Run(params map[string]string) (any, error)`", Required: true},
			{Name: "params", Type: "json", Description: "JSON object of params to pass to Run"},
		},
	})
}

// mergeShadow combines the builtin's default Shadow with a user-supplied
// one. User fields win when explicitly set; otherwise builtin defaults
// fill in. Used so a user can write `factorly.code: { shadow: { max_calls:
// 200 } }` and get the builtin's other shadow defaults preserved.
//
// Nil safety: if either side is nil, the other is returned (or a clone
// thereof). If both are nil, returns nil.
func mergeShadow(base, user *config.ShadowConfig) *config.ShadowConfig {
	if user == nil {
		return base
	}
	if base == nil {
		return user
	}
	merged := *base // shallow copy of the builtin default
	if len(user.Deny) > 0 {
		merged.Deny = user.Deny
	}
	if user.Confirm != nil {
		merged.Confirm = user.Confirm
	}
	if user.RateLimit != "" {
		merged.RateLimit = user.RateLimit
	}
	if user.MaxCalls != 0 {
		merged.MaxCalls = user.MaxCalls
	}
	if len(user.LogParams) > 0 {
		merged.LogParams = user.LogParams
	}
	if len(user.AllowPatterns) > 0 {
		merged.AllowPatterns = user.AllowPatterns
	}
	if len(user.AllowPaths) > 0 {
		merged.AllowPaths = user.AllowPaths
	}
	if len(user.AllowURLs) > 0 {
		merged.AllowURLs = user.AllowURLs
	}
	return &merged
}
