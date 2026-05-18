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
	"github.com/factorly-dev/factorly/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	execMaxOutput    int
	execCompress     string
	execEnvIsolation string
	execInteractive  bool
	execTimeout      string
	execEnvVars      []string
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

	// Parse --env KEY=VALUE pairs
	if len(execEnvVars) > 0 {
		toolCfg.Env = make(map[string]string)
		for _, e := range execEnvVars {
			k, v, ok := strings.Cut(e, "=")
			if !ok {
				return fmt.Errorf("invalid --env format %q (expected KEY=VALUE)", e)
			}
			toolCfg.Env[k] = v
		}
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

	// Check if vault refs are present anywhere — open vault lazily
	hasVaultRefs := vault.HasVaultRefs(toolCfg.Command)
	for _, a := range toolCfg.Args {
		if vault.HasVaultRefs(a) {
			hasVaultRefs = true
			break
		}
	}
	for _, v := range toolCfg.Env {
		if vault.HasVaultRefs(v) {
			hasVaultRefs = true
			break
		}
	}

	// Build resolver with env + optional vault backends. When --workspace
	// (or FACTORLY_WORKSPACE) is active, seed the env backend with the
	// workspace's vars so {{env:NAME}} references resolve to workspace
	// values before falling back to os.Getenv — same overlay semantics
	// as `factorly call` uses via config.WithWorkspace.
	resolver := vault.NewResolver()
	workspaceVars := loadExecWorkspaceVars()
	if len(workspaceVars) > 0 {
		resolver.Register("env", vault.EnvBackendWithOverrides{Overrides: workspaceVars})
	} else {
		resolver.Register("env", vault.EnvBackend{})
	}
	if hasVaultRefs {
		backend, err := getCachedLocalVault()
		if err != nil {
			return fmt.Errorf("resolving vault refs: %w", err)
		}
		resolver.Register("vault", backend)
	}

	// Phase 1: resolve env values first (they may reference parent env/vault)
	for k, v := range toolCfg.Env {
		toolCfg.Env[k], _ = resolver.Resolve(v)
	}

	// Phase 2: rebuild resolver with resolved --env values as overrides
	// so args can reference them via {{env:KEY}}. --env wins over
	// workspace vars (more specific intent), workspace wins over os env.
	if len(toolCfg.Env) > 0 || len(workspaceVars) > 0 {
		merged := make(map[string]string, len(workspaceVars)+len(toolCfg.Env))
		for k, v := range workspaceVars {
			merged[k] = v
		}
		for k, v := range toolCfg.Env {
			merged[k] = v
		}
		resolver.Register("env", vault.EnvBackendWithOverrides{Overrides: merged})
	}

	// Phase 3: resolve command and args
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

// loadExecWorkspaceVars returns the vars map for the active workspace
// (resolved from --workspace flag or FACTORLY_WORKSPACE env), or nil
// when no workspace is active or the project has none. exec uses its
// own resolver setup (not config.Load), so the workspace overlay has
// to be wired here explicitly. Errors are swallowed — exec should
// still run even if the workspace file is missing; missing var refs
// will surface as unresolved-placeholder errors downstream which is
// the right signal.
func loadExecWorkspaceVars() map[string]string {
	name := workspaceName
	if name == "" {
		name = os.Getenv("FACTORLY_WORKSPACE")
	}
	if name == "" {
		return nil
	}
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}
	ws, err := workspace.Load(cfgPath, name)
	if err != nil || ws == nil {
		return nil
	}
	return ws.Vars
}

func init() {
	execCmd.Flags().IntVar(&execMaxOutput, "max-output", 50000, "max output bytes per call")
	execCmd.Flags().StringVar(&execCompress, "compress", "all", "compression mode: all, json, logs, none")
	execCmd.Flags().StringVar(&execEnvIsolation, "env-isolation", "", "environment isolation: strict (minimal env) or standard (default, inherit parent)")
	execCmd.Flags().BoolVarP(&execInteractive, "interactive", "i", false, "connect directly to terminal (skip compression, for TTY tools)")
	execCmd.Flags().StringVar(&execTimeout, "timeout", "", "execution timeout (e.g. 30s, 5m; default: 30s)")
	execCmd.Flags().StringArrayVar(&execEnvVars, "env", nil, "set env var (KEY=VALUE, supports {{env:VAR}} and {{vault:KEY}}); repeatable")
}
