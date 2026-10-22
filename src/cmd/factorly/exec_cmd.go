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
	execEnvVars      []string
	execVaultKeys    []string
)

var execCmd = &cobra.Command{
	Use:   "exec [flags] -- <command> [args...]",
	Short: "Run a command through Factorly's safety layer",
	Long: `Run a single shell command with output compression, truncation,
and audit logging. The zero-config equivalent of a CLI tool definition.

Supports {{vault:KEY}} and {{env:VAR}} references in arguments:
  factorly exec -- curl -H "Authorization: Bearer {{vault:GITHUB_TOKEN}}" https://api.github.com/user

Export vault secrets into the command's environment with --vault, and
forward variables from your own environment with a bare --env NAME:
  factorly exec --vault GITHUB_TOKEN -- gh pr list
  factorly exec --vault GH_TOKEN=GITHUB_TOKEN -- ./script.sh
  factorly exec --vault 'STRIPE_*' -- npm run sync
  factorly exec --env-isolation strict --env DISPLAY --env SSH_AUTH_SOCK -- xsel --clipboard

Commands run with no timeout unless --timeout is given.

Examples:
  factorly exec -- git status
  factorly exec --compress json -- npm test
  factorly exec --timeout 10m -- make build
  factorly exec --env-isolation strict --vault GITHUB_TOKEN -- gh pr list
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
	if execEnvIsolation == "strict" {
		toolCfg.EnvIsolation = "strict"
	}

	// Timeout: exec is a foreground, user-driven command, so the 30s
	// default that protects agent tool calls does not apply. No flag
	// (or "0"/"none") means no deadline; a duration opts into one.
	if _, _, err := parseCLIToolTimeout(execTimeout); err != nil {
		return fmt.Errorf("invalid --timeout %q: %w", execTimeout, err)
	}
	if strings.TrimSpace(execTimeout) == "" {
		toolCfg.Timeout = "none"
	} else {
		toolCfg.Timeout = execTimeout
	}

	// Parse --env: KEY=VALUE pairs are set now; bare names are forwarded
	// from the environment once the resolver exists (after phase 1).
	explicitEnv, forwardNames, err := parseEnvFlags(execEnvVars)
	if err != nil {
		return err
	}
	toolCfg.Env = explicitEnv

	// --vault KEY / ENV=KEY / GLOB → env entries holding {{vault:KEY}}
	// refs, so they ride the same resolver path as --env values.
	// Explicit --env entries win on conflict.
	if len(execVaultKeys) > 0 {
		specs, err := parseVaultEnvSpecs(execVaultKeys)
		if err != nil {
			return err
		}
		warnf := func(format string, a ...any) {
			fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...)
		}
		vaultEnv, err := vaultEnvRefs(specs, listCachedVaultKeys, warnf)
		if err != nil {
			return err
		}
		toolCfg.Env = mergeVaultEnv(toolCfg.Env, vaultEnv)
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

	// Resolve refs through the shared resolver (env + workspace overlay,
	// store, external vault backends, project→global vault cascade) —
	// the same one `call` uses, so exec sees every backend the project
	// does. Done here rather than left to bootstrapProviders because
	// --env values must be visible to args as {{env:KEY}} (phase 2).
	resolver, err := initExecResolver(cfg)
	if err != nil {
		return err
	}

	// Phase 1: resolve env values first (they may reference parent env/vault)
	for k, v := range toolCfg.Env {
		resolved, err := resolveExecRef(resolver, v, "env "+k)
		if err != nil {
			return err
		}
		toolCfg.Env[k] = resolved
	}

	// Phase 1b: forward bare --env NAME entries. Resolved after phase 1
	// so a forwarded value is set verbatim (never re-templated), and
	// before phase 2 so args can reference it via {{env:NAME}}. A bare
	// --env wins over --vault for the same name, like an explicit one.
	toolCfg.Env = forwardEnv(toolCfg.Env, forwardNames, resolver, vlog)

	// Phase 2: rebuild env backend with resolved --env values as
	// overrides so args can reference them via {{env:KEY}}. --env wins
	// over workspace vars (more specific intent), workspace wins over
	// os env.
	workspaceVars := loadActiveWorkspaceVars(cfg)
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
	toolCfg.Command, err = resolveExecRef(resolver, toolCfg.Command, "command")
	if err != nil {
		return err
	}
	for i, a := range toolCfg.Args {
		toolCfg.Args[i], err = resolveExecRef(resolver, a, fmt.Sprintf("argument %d", i+1))
		if err != nil {
			return err
		}
	}
	// Env values are final; keep bootstrapProviders' own resolve pass
	// from templating them a second time.
	toolCfg.Env = escapeResolvedEnv(toolCfg.Env)
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
			if !strings.HasSuffix(result.Error, "\n") {
				fmt.Fprintln(os.Stderr)
			}
		}
		os.Exit(result.ExitCode)
	}
	return nil
}

// resolveExecRef resolves refs in s and fails when a secret-backend
// reference survives. Running the command with a literal "{{vault:KEY}}"
// in its environment or arguments is never what the user meant — a
// bogus credential reaching a real service is worse than stopping here.
// Failures on safe backends (env/store) are only warned about, matching
// how config-based tools behave.
func resolveExecRef(resolver *vault.Resolver, s, where string) (string, error) {
	if s == "" {
		return s, nil
	}
	resolved, rerr := resolver.Resolve(s)
	if backend, key, ok := vault.UnresolvedSecretRef(s, resolved); ok {
		if rerr != nil {
			return "", fmt.Errorf("%s: {{%s:%s}} could not be resolved: %w", where, backend, key, rerr)
		}
		return "", fmt.Errorf("%s: {{%s:%s}} could not be resolved (no %q backend available)", where, backend, key, backend)
	}
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", rerr)
	}
	return resolved, nil
}

// initExecResolver builds the shared resolver for exec's synthetic
// config. exec runs without loading the project (it must work with
// zero config), but when a project is present its vault_backends are
// borrowed so {{op:KEY}}-style external refs resolve exactly as they
// would for `call`. Workspace selection follows the same rules as
// loadConfig. A missing or broken project config is logged and ignored.
func initExecResolver(cfg *config.Config) (*vault.Resolver, error) {
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}
	resolveWorkspaceName(cfgPath)

	if project, err := config.Load(cfgPath, config.WithWorkspace(workspaceName)); err == nil {
		cfg.VaultBackends = project.VaultBackends
	} else {
		vlog("exec: project config not loaded (%v); external vault backends unavailable", err)
	}

	resolver, err := initResolver(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolving refs: %w", err)
	}
	return resolver, nil
}

func init() {
	execCmd.Flags().IntVar(&execMaxOutput, "max-output", 50000, "max output bytes per call")
	execCmd.Flags().StringVar(&execCompress, "compress", "all", "compression mode: all, json, logs, none")
	execCmd.Flags().StringVar(&execEnvIsolation, "env-isolation", "", "environment isolation: strict (minimal env) or standard (default, inherit parent)")
	execCmd.Flags().BoolVarP(&execInteractive, "interactive", "i", false, "connect directly to terminal (skip compression, for TTY tools)")
	execCmd.Flags().StringVar(&execTimeout, "timeout", "", "execution timeout (e.g. 30s, 5m; default: none)")
	execCmd.Flags().StringArrayVar(&execEnvVars, "env", nil, "set env var (KEY=VALUE, supports {{env:VAR}} and {{vault:KEY}}) or forward one (NAME); repeatable")
	execCmd.Flags().StringArrayVar(&execVaultKeys, "vault", nil, "export vault secret as env var (KEY, ENV=KEY, or glob like 'STRIPE_*'; '*' exports everything); repeatable")
}
