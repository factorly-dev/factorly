// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/factorly-dev/factorly/internal/help"
	"github.com/spf13/cobra"
)

var (
	helpTopic    string
	helpToolName string
)

// explainCmd emits the same self-describing onboarding text the
// `factorly.help` MCP builtin returns. Default output is a tight
// overview + behaviors block ready to paste into a Claude / Cursor /
// Codex system prompt. Use --topic for deeper sections, --tool for
// per-tool docs.
//
// The intent is: install factorly, run `factorly explain`, paste the
// output into your agent's system prompt. The agent then also has
// `factorly.help` available at runtime so it can self-discover as it
// goes. Same corpus, two surfaces.
//
// Named "explain" rather than "help" so it doesn't shadow cobra's
// auto-generated `factorly help <subcommand>` (which is genuinely
// useful for the CLI). What this prints is a primer for the agent,
// not a help page for the CLI user.
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Print a factorly primer for your agent (or yourself)",
	Long: "Emits a markdown block describing factorly: credential model, oversight,\n" +
		"workflows, blueprints, and a snapshot of what's installed. Paste it into\n" +
		"your agent's system prompt, or read it yourself to get oriented.\n\n" +
		"The same content is available at runtime to any connected agent via the\n" +
		"`factorly.help` MCP tool — so an agent can self-discover what's available\n" +
		"without prompt engineering.",
	Example: "  # Print the default overview (paste into your agent's system prompt)\n" +
		"  factorly explain\n\n" +
		"  # Specific topic\n" +
		"  factorly explain --topic vault\n" +
		"  factorly explain --topic shadow\n" +
		"  factorly explain --topic workflows\n\n" +
		"  # Detailed docs for one tool\n" +
		"  factorly explain --tool github.list_repos",
	RunE: runExplain,
}

func runExplain(cmd *cobra.Command, args []string) error {
	in := help.Inputs{CfgPath: resolveCfgPath()}
	if cfg, _, err := loadConfig(); err == nil {
		in.Config = cfg
	} else if !errors.Is(err, os.ErrNotExist) {
		// Config exists but failed to load — surface the error so the
		// user knows their help output isn't reflecting reality. A
		// fresh install with no config falls into the os.ErrNotExist
		// branch and renders the generic view, which is correct.
		fmt.Fprintf(os.Stderr, "warning: config load failed (%v) — rendering generic help\n\n", err)
	}

	var out string
	switch {
	case helpToolName != "":
		out = help.RenderTool(helpToolName, in.Config)
		if out == "" {
			return fmt.Errorf("tool %q not found in your config — run `factorly tools` to see what's available", helpToolName)
		}
	default:
		out = help.Render(help.Topic(helpTopic), in)
	}
	fmt.Println(out)
	return nil
}

func init() {
	explainCmd.Flags().StringVar(&helpTopic, "topic", "",
		"specific topic to render (vault, shadow, workflows, blueprints, tools, what-is)")
	explainCmd.Flags().StringVar(&helpToolName, "tool", "",
		"render detailed docs for one tool by name")
}
