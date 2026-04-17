// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/spf13/cobra"
)

var (
	execMaxOutput    int
	execCompress     string
	execEnvIsolation string
	execInteractive  bool
	execTimeout      string
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- <command> [args...]",
	Short: "Run a command through Factorly's safety layer",
	Long: `Run a single shell command with output compression, truncation,
and audit logging. The zero-config equivalent of a CLI tool definition.

Supports {{vault:KEY}} and {{env:VAR}} references in arguments:
  factorly exec -- curl -H "Authorization: Bearer {{vault:GITHUB_TOKEN}}" https://api.github.com/user

Examples:
  factorly exec -- git status
  factorly exec --compress json -- npm test
  factorly exec --env-isolation strict -- ./deploy.sh
  factorly exec -i -- psql -h localhost mydb`,
	RunE: runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	if err := checkCommandAllowed("exec"); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: factorly exec -- <command> [args...]")
	}

	// Build synthetic tool config from command args
	toolCfg := config.ToolConfig{
		Type:    "cli",
		Command: args[0],
		Args:    args[1:],
	}

	// Apply flags
	if execInteractive {
		toolCfg.Interactive = true
	}
	switch execCompress {
	case "none":
		// no compression
	case "json":
		toolCfg.Compress = []string{"json"}
	case "logs":
		toolCfg.Compress = []string{"logs"}
	default:
		toolCfg.Compress = []string{"all"}
	}
	toolCfg.MaxOutput = execMaxOutput
	if execTimeout != "" {
		toolCfg.Timeout = execTimeout
	}
	if execEnvIsolation == "strict" {
		toolCfg.EnvIsolation = "strict"
	}

	toolName := "exec"

	// Build synthetic config
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			toolName: toolCfg,
		},
	}

	// Build registry
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        toolName,
		Type:        "cli",
		ProviderKey: "cli",
		MaxOutput:   toolCfg.MaxOutput,
		Compress:    toolCfg.Compress,
	})

	vlog("exec: %s", strings.Join(args, " "))

	// Resolve {{env:VAR}} and {{vault:KEY}} refs in args
	resolver := vault.NewResolver()
	resolver.Register("env", vault.EnvBackend{})

	// Check if vault refs are present — open vault lazily
	hasVaultRefs := false
	for _, a := range args {
		if vault.HasVaultRefs(a) {
			hasVaultRefs = true
			break
		}
	}
	if vault.HasVaultRefs(toolCfg.Command) {
		hasVaultRefs = true
	}
	if hasVaultRefs {
		backend, err := openVault()
		if err != nil {
			return fmt.Errorf("resolving vault refs: %w", err)
		}
		defer backend.Close()
		resolver.Register("vault", backend)
	}

	// Resolve all refs
	toolCfg.Command, _ = resolver.Resolve(toolCfg.Command)
	for i, a := range toolCfg.Args {
		toolCfg.Args[i], _ = resolver.Resolve(a)
	}
	cfg.Tools[toolName] = toolCfg

	// Bootstrap providers (builds CLIProvider with our synthetic tool)
	p, err := bootstrapProviders(cfg, reg)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// Execute through proxy (gets compression, truncation, logging for free)
	result, err := p.Execute(toolName, nil, "exec")
	if err != nil {
		return err
	}

	if result.Output != "" {
		fmt.Print(result.Output)
	}
	if result.IsError() {
		if result.Error != "" {
			fmt.Fprint(os.Stderr, result.Error)
		}
		os.Exit(result.ExitCode)
	}
	return nil
}

func init() {
	execCmd.Flags().IntVar(&execMaxOutput, "max-output", 50000, "max output bytes per call")
	execCmd.Flags().StringVar(&execCompress, "compress", "all", "compression mode: all, json, logs, none")
	execCmd.Flags().StringVar(&execEnvIsolation, "env-isolation", "", "environment isolation: strict (minimal env) or standard (default, inherit parent)")
	execCmd.Flags().BoolVarP(&execInteractive, "interactive", "i", false, "connect directly to terminal (skip compression, for TTY tools)")
	execCmd.Flags().StringVar(&execTimeout, "timeout", "", "execution timeout (e.g. 30s, 5m; default: 30s)")
}
