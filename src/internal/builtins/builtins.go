// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package builtins

import (
	"os/exec"
	"runtime"

	"github.com/factorly-dev/factorly/internal/config"
)

// Options controls which built-in tools are registered.
type Options struct {
	Mode string // "stdio" or "http" — local tools only register in stdio mode
}

// Register adds built-in tools to the config. Built-ins are governed
// alternatives to unsafe agent tools: shell, read, write, fetch, clipboard.
// They use the existing CLI provider — no new provider needed.
func Register(cfg *config.Config, opts Options) {
	if cfg.DisableBuiltins {
		return
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]config.ToolConfig)
	}

	// Universal (all modes) — runs server-side where credentials live
	cfg.Tools["factorly.fetch"] = config.ToolConfig{
		Type:        "cli",
		Description: "Fetch a URL (governed, logged, compressed)",
		Command:     "curl",
		Args:        []string{"-sS", "{{url}}"},
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "url", Description: "URL to fetch", Required: true},
		},
	}

	// Local only (stdio mode) — runs on the agent's machine
	if opts.Mode == "http" {
		return
	}

	cfg.Tools["factorly.shell"] = config.ToolConfig{
		Type:        "cli",
		Description: "Run a shell command (governed, logged, compressed)",
		Command:     "sh",
		Args:        []string{"-c", "{{command}}"},
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "command", Description: "Shell command to execute", Required: true},
		},
	}

	cfg.Tools["factorly.read_file"] = config.ToolConfig{
		Type:        "cli",
		Description: "Read a local file (governed, logged, compressed)",
		Command:     "cat",
		Args:        []string{"{{path}}"},
		Compress:    []string{"all"},
		MaxOutput:   50000,
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to read", Required: true},
		},
	}

	cfg.Tools["factorly.write_file"] = config.ToolConfig{
		Type:        "cli",
		Description: "Write content to a local file (governed, logged, confirmable)",
		Command:     "tee",
		Args:        []string{"{{path}}"},
		Stdin:       "{{content}}",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "path", Description: "File path to write", Required: true},
			{Name: "content", Description: "Content to write", Required: true},
		},
	}

	cmd, args := clipboardCommand()
	cfg.Tools["factorly.clipboard"] = config.ToolConfig{
		Type:        "cli",
		Description: "Copy text to the system clipboard (governed, logged, confirmable)",
		Command:     cmd,
		Args:        args,
		Stdin:       "{{text}}",
		Shadow:      &config.ShadowConfig{Confirm: true},
		Parameters: []config.ParamConfig{
			{Name: "text", Description: "Text to copy to clipboard", Required: true},
		},
	}
}

// clipboardCommand returns the platform-specific clipboard command.
func clipboardCommand() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil
	case "windows":
		return "clip", nil
	default:
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}
		}
		return "xclip", []string{"-selection", "clipboard"}
	}
}
