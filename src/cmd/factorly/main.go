// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"encoding/json"
	"time"

	"github.com/factorly-dev/factorly/internal"
	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/configyaml"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/oauth"
	"github.com/factorly-dev/factorly/internal/openapi"
	"github.com/factorly-dev/factorly/internal/output"
	"github.com/factorly-dev/factorly/internal/projectpath"
	"github.com/factorly-dev/factorly/internal/provider"
	codeprov "github.com/factorly-dev/factorly/internal/provider/code"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
	"github.com/factorly-dev/factorly/internal/store"
	"github.com/factorly-dev/factorly/internal/update"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/factorly-dev/factorly/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configPath string
var workspaceName string
var configDir string
var verbose bool
var wrapMode bool
var serveMode string // "stdio" or "http" — controls built-in tool registration

// sharedLogger is the process-wide logger instance, set during bootstrap.
// Used by logVaultOp to avoid creating a second logger (which would break hash chains).
var sharedLogger logger.Logger

func vlog(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[factorly] "+format+"\n", args...)
	}
}

func main() {
	// Check for updates in background (at most once per day).
	// Skip for "version" command — it does its own synchronous check.
	var updateCh chan update.Result
	if len(os.Args) < 2 || os.Args[1] != "version" {
		updateCh = make(chan update.Result, 1)
		go func() {
			updateCh <- update.Check()
		}()
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}

	// Print update message after command completes (non-blocking)
	if updateCh != nil {
		select {
		case result := <-updateCh:
			if result.Message != "" {
				fmt.Fprintf(os.Stderr, "\n%s\n", result.Message)
			}
		default:
		}
	}
}

var rootCmd = &cobra.Command{
	Use:               "factorly",
	Short:             "One endpoint. All your tools.",
	Long:              "Factorly wraps your existing agent tools — MCP servers, REST APIs, CLIs — into a single endpoint.",
	SilenceUsage:      true,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

var utilsCmd = &cobra.Command{
	Use:   "utils",
	Short: "Utility commands",
}

var autocompleteCmd = &cobra.Command{
	Use:   "autocomplete [bash|zsh|fish|powershell]",
	Short: "Generate shell autocompletion script",
	Long: `Generate an autocompletion script for the specified shell.

Bash:
  source <(factorly utils autocomplete bash)

Zsh:
  factorly utils autocomplete zsh > "${fpath[1]}/_factorly"

Fish:
  factorly utils autocomplete fish | source

PowerShell:
  factorly utils autocomplete powershell | Out-String | Invoke-Expression`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

var versionCheck bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		var result update.Result
		if versionCheck {
			result = update.CheckNow()
		} else {
			result = update.Check()
		}
		if result.UpToDate {
			fmt.Printf("factorly %s (latest)\n", internal.Version)
		} else if result.Message != "" {
			fmt.Printf("factorly %s\n", internal.Version)
			fmt.Fprintf(os.Stderr, "\n%s\n", result.Message)
		} else {
			fmt.Printf("factorly %s\n", internal.Version)
		}
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionCheck, "check", false, "force version check (bypass cache)")
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage and list configured tools",
	RunE:  runToolsList, // default: list tools when no subcommand given
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured tools",
	RunE:  runToolsList,
}

var toolsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print a tool's (or workflow's) YAML definition to stdout",
	Args:  requireArgs(1, "factorly tools show <name>"),
	RunE:  runToolsShow,
}

func runToolsShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	tc, ok := cfg.Tools[name]
	if !ok {
		return fmt.Errorf("tool %q is not configured", name)
	}
	out, err := configyaml.RenderTool(name, tc)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

func runToolsList(cmd *cobra.Command, args []string) error {
	cfg, reg, err := loadConfig()
	if err != nil {
		return err
	}

	// Bootstrap providers to discover MCP sub-tools.
	// If bootstrap fails (e.g., vault locked), show config-level tools without MCP discovery.
	if hasMCPTools(cfg) {
		if _, err := bootstrapProviders(cfg, reg); err != nil {
			vlog("MCP discovery skipped: %v", err)
		}
	}

	// Build shadow policy to filter denied tools from listing
	rules := buildShadowRules(cfg)
	mergeDisabledToolsFromEnv(rules)
	var shadowPolicy *shadow.Policy
	if len(rules) > 0 {
		shadowPolicy = shadow.New(rules, nil, shadow.ProjectRateStorePath(configPath))
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tDESCRIPTION\tPARAMETERS")
	for _, t := range reg.ListVisible() {
		if shadowPolicy != nil && shadowPolicy.IsDenied(t.Name) {
			continue
		}
		params := make([]string, len(t.Parameters))
		for i, p := range t.Parameters {
			params[i] = p.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Type, t.Description, strings.Join(params, ", "))
	}
	return w.Flush()
}

var callCmd = &cobra.Command{
	Use:                "call <tool> [--param value ...]",
	Short:              "Call a tool",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Manually extract global flags that cobra can't parse due to DisableFlagParsing
		args = extractGlobalFlags(args)

		// Handle --help / -h manually since DisableFlagParsing hides it from cobra
		for _, a := range args {
			if a == "--help" || a == "-h" {
				return cmd.Help()
			}
		}

		if err := checkCommandAllowed("call"); err != nil {
			return err
		}

		if len(args) == 0 {
			return fmt.Errorf("usage: factorly call <tool> [--param value ...]")
		}
		toolName := args[0]
		params := parseToolArgs(args[1:])

		vlog("calling tool: %s", toolName)
		vlog("  params: %v", redactSensitiveParams(params))

		// Load config and validate tool exists before opening vault
		cfg, reg, err := loadConfig()
		if err != nil {
			return err
		}
		// Skip early validation for MCP tools — sub-tools are discovered
		// during bootstrapProviders, not loadConfig
		if tool, err := reg.Get(toolName); err != nil && !hasMCPTools(cfg) {
			return err
		} else if tool != nil {
			if err := tool.ValidateParams(params); err != nil {
				return err
			}
		}

		p, err := bootstrapProviders(cfg, reg)
		if err != nil {
			return err
		}

		// Resolve {{prefix:KEY}} refs in parameter values. Uses the
		// resolver populated by bootstrapProviders so {{store:KEY}},
		// {{env:VAR}}, {{expr:...}} all work in command-line param
		// values, not just in tool YAML defaults.
		//
		// Vault refs need extra handling: when no {{vault:...}} ref
		// appears in the config, initResolver intentionally skips
		// opening the vault (no password prompt for vault-free
		// projects). But the user might still pass a vault ref on the
		// command line. So check for that case and lazily open vault
		// here, registering it on the cached resolver.
		resolver := getCachedResolver()
		if resolver != nil {
			needsVault := false
			for _, v := range params {
				if vault.HasVaultRefs(v) {
					needsVault = true
					break
				}
			}
			if needsVault {
				backend, err := getCachedVault()
				if err != nil {
					return fmt.Errorf("resolving vault refs in params: %w", err)
				}
				resolver.Register("vault", backend)
			}
			for k, pv := range params {
				params[k] = resolveVaultRef(resolver, pv)
			}
		}

		result, err := p.Execute(toolName, params, "cli")
		if err != nil {
			return err
		}

		vlog("  status: %s (exit code: %d, duration: %s)", map[bool]string{true: "error", false: "success"}[result.IsError()], result.ExitCode, result.Duration)

		if result.Output != "" {
			fmt.Print(result.Output)
		}
		if result.IsError() {
			if result.Error != "" {
				fmt.Fprintln(os.Stderr, result.Error)
			}
			os.Exit(result.ExitCode)
		}
		return nil
	},
}

var initOut string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new factorly.yaml config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		outPath := initOut
		if outPath == "" {
			outPath = filepath.Join(".factorly", "factorly.yaml")
		}
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("%s already exists", outPath)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		scanner := bufio.NewScanner(os.Stdin)

		fmt.Println("Setting up a new Factorly config.")
		fmt.Println()

		// Standard setup: tools live in .factorly/tools/ next to the
		// config. The loader auto-discovers them, so no tools_dir line
		// is needed in YAML.
		toolsDir := filepath.Join(filepath.Dir(outPath), "tools")
		if err := os.MkdirAll(toolsDir, 0o755); err != nil {
			return fmt.Errorf("creating tools directory: %w", err)
		}

		// Example tool
		addExample := prompt(scanner, "Add an example CLI tool? (y/n)", "y")

		// Build config
		type yamlConfig struct {
			Tools map[string]config.ToolConfig `yaml:"tools"`
		}
		cfg := yamlConfig{
			Tools: make(map[string]config.ToolConfig),
		}

		if strings.HasPrefix(strings.ToLower(addExample), "y") {
			cfg.Tools["web.fetch"] = config.ToolConfig{
				Type:        "cli",
				Description: "Fetch a webpage",
				Command:     "curl",
				Args:        []string{"-s", "{{url}}"},
			}
		}

		// Always create a "default" workspace. It auto-loads whenever
		// no --workspace flag is set, so {{env:NAME}} references in
		// factorly.yaml can be overlaid from one place. Users add
		// staging/prod as siblings when they need them.
		wsDir := filepath.Join(filepath.Dir(outPath), "workspaces")
		if err := os.MkdirAll(wsDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create workspaces dir: %v\n", err)
		} else {
			defaultPath := filepath.Join(wsDir, "default.yaml")
			defaultBody := `description: Default workspace (auto-loaded when no --workspace is set)
vars:
  # Add overrides like API_BASE: http://localhost:3000 here, then
  # reference them from factorly.yaml via {{env:API_BASE}}.
  # Add sibling files (staging.yaml, prod.yaml) and switch with
  # 'factorly call ... --workspace staging'.
`
			if err := os.WriteFile(defaultPath, []byte(defaultBody), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", defaultPath, err)
			}
		}

		// OpenAPI import
		addOpenAPI := prompt(scanner, "Import tools from an OpenAPI spec? (y/n)", "n")
		if strings.HasPrefix(strings.ToLower(addOpenAPI), "y") {
			specPath := prompt(scanner, "OpenAPI spec path or URL", "")
			if specPath != "" {
				prefix := prompt(scanner, "Tool name prefix (leave empty for auto)", "")
				tools, err := openapi.Generate(specPath, openapi.GenerateOpts{Prefix: prefix})
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to import OpenAPI spec: %v\n", err)
				} else {
					specName := filepath.Base(specPath)
					specName = strings.TrimSuffix(specName, filepath.Ext(specName))
					outFile := filepath.Join(toolsDir, specName+".yaml")
					data, err := yaml.Marshal(tools)
					if err == nil {
						if err := os.WriteFile(outFile, data, 0o644); err != nil {
							fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", outFile, err)
						} else {
							fmt.Fprintf(os.Stderr, "Wrote %d tools to %s\n", len(tools), outFile)
						}
					}
				}
			}
		}

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}

		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}

		fmt.Printf("\nCreated %s\n", outPath)

		// Offer to ignore runtime state in an existing .gitignore. We only
		// prompt when a .gitignore is already present — we're not the
		// gitignore manager and creating one on the user's behalf would be
		// presumptuous.
		maybeOfferGitignore(scanner, filepath.Dir(outPath))

		fmt.Println("\nTip: install a bundled blueprint (gmail, linear, github, ...) with:")
		fmt.Println("  factorly blueprint install <name>")
		fmt.Println("Or browse the catalog at /blueprints/browse when running 'factorly ui'.")

		// Offer to sync with AI clients
		doSync := prompt(scanner, "Connect to your AI agent now? (y/n)", "y")
		if strings.HasPrefix(strings.ToLower(doSync), "y") {
			fmt.Println()
			if err := runSync(cmd, nil); err != nil {
				fmt.Fprintf(os.Stderr, "warning: sync failed: %v\n", err)
				fmt.Println("You can sync later with: factorly sync")
			}
		} else {
			fmt.Println("\nRun 'factorly sync' when you're ready to connect to your agent.")
		}

		return nil
	},
}

// maybeOfferGitignore looks for a .gitignore at the project root and,
// if found and missing the relevant entries, asks the user whether
// to append them. configDir is the directory containing the config
// file we just wrote (either "." or ".factorly").
func maybeOfferGitignore(scanner *bufio.Scanner, configDir string) {
	// Repo root is one level above .factorly/, or the current directory
	// when the config sits at the top level.
	repoRoot := "."
	if filepath.Base(configDir) == ".factorly" {
		repoRoot = filepath.Dir(configDir)
		if repoRoot == "" {
			repoRoot = "."
		}
	}
	giPath := filepath.Join(repoRoot, ".gitignore")

	existing, err := os.ReadFile(giPath)
	if err != nil {
		// No .gitignore — skip silently. We're not the gitignore manager.
		return
	}
	body := string(existing)
	entries := []string{
		".factorly/audit.jsonl",
		".factorly/ratelimit.json",
		".factorly/runs/",
	}
	needed := []string{}
	for _, e := range entries {
		if !gitignoreContains(body, e) {
			needed = append(needed, e)
		}
	}
	if len(needed) == 0 {
		return
	}

	answer := prompt(scanner, "Append runtime state files to .gitignore? (audit.jsonl, ratelimit.json, runs/) (y/n)", "y")
	if !strings.HasPrefix(strings.ToLower(answer), "y") {
		return
	}

	var b strings.Builder
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") && len(body) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("\n# Factorly runtime state — see https://factorly.dev\n")
	for _, e := range needed {
		b.WriteString(e)
		b.WriteString("\n")
	}
	if err := os.WriteFile(giPath, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update .gitignore: %v\n", err)
		return
	}
	fmt.Printf("Updated %s\n", giPath)
}

// gitignoreContains checks if a non-empty, non-comment line in body
// equals the given pattern. Permissive — doesn't try to evaluate
// gitignore syntax (negations, globs); a literal match is enough to
// avoid duplicate entries.
func gitignoreContains(body, pattern string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == pattern {
			return true
		}
	}
	return false
}

func prompt(scanner *bufio.Scanner, label string, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import tool definitions from external sources",
}

var importOpenAPIOut string
var importOpenAPIPrefix string

var importOpenAPICmd = &cobra.Command{
	Use:   "openapi <spec-path>",
	Short: "Generate tool definitions from an OpenAPI spec",
	Args:  requireArgs(1, "factorly tools import openapi <spec-path>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		tools, err := openapi.Generate(args[0], openapi.GenerateOpts{
			Prefix: importOpenAPIPrefix,
		})
		if err != nil {
			return err
		}

		data, err := yaml.Marshal(tools)
		if err != nil {
			return fmt.Errorf("marshaling output: %w", err)
		}

		if importOpenAPIOut != "" {
			if err := os.WriteFile(importOpenAPIOut, data, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", importOpenAPIOut, err)
			}
			fmt.Fprintf(os.Stderr, "Wrote %d tools to %s\n", len(tools), importOpenAPIOut)
			return nil
		}

		fmt.Print(string(data))
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to factorly.yaml")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "path to directory of tool definition files")
	rootCmd.PersistentFlags().StringVarP(&workspaceName, "workspace", "w", "", "named workspace overlay (defaults to FACTORLY_WORKSPACE)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print detailed progress to stderr")

	initCmd.Flags().StringVarP(&initOut, "out", "o", "", "output file path (default: .factorly/factorly.yaml)")

	importOpenAPICmd.Flags().StringVarP(&importOpenAPIOut, "out", "o", "", "output file path (default: stdout)")
	importOpenAPICmd.Flags().StringVarP(&importOpenAPIPrefix, "prefix", "p", "", "tool name prefix (default: from spec title)")
	importCmd.AddCommand(importOpenAPICmd)

	toolsCmd.AddCommand(toolsListCmd, toolsShowCmd, addCmd, removeCmd, importCmd, recordCmd, statusCmd, toolsPromoteCmd)
	utilsCmd.AddCommand(autocompleteCmd)
	rootCmd.AddCommand(versionCmd, toolsCmd, callCmd, initCmd, syncCmd, vaultCmd, storeCmd, workspacesCmd, authCmd, serveCmd, wrapCmd, execCmd, logsCmd, utilsCmd, uiCmd, blueprintCmd)
}

// loadConfig loads config and builds a registry. Does not open the vault
// or create providers — use for read-only commands like `tools`.
func loadConfig() (*config.Config, *registry.Registry, error) {
	config.Verbose = nil
	if verbose {
		config.Verbose = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[factorly] "+format+"\n", args...)
		}
	}

	var cfg *config.Config
	var err error

	if configDir != "" {
		vlog("loading config from directory: %s", configDir)
		cfg, err = config.LoadDir(configDir)
	} else {
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = config.FindConfig()
			configPath = cfgPath // persist for UI/other commands that need the write path
			vlog("found config: %s", cfgPath)
		} else {
			vlog("using config: %s", cfgPath)
		}
		// --workspace wins; FACTORLY_WORKSPACE is the fallback so users
		// can set it once per shell. If neither is set and a "default"
		// workspace exists, auto-select it — that's the Postman-style
		// "always have an environment active" default. Persist back
		// into workspaceName so downstream code (audit log, vault) sees
		// the resolved value.
		if workspaceName == "" {
			workspaceName = os.Getenv("FACTORLY_WORKSPACE")
		}
		if workspaceName == "" && workspace.Exists(cfgPath, "default") {
			workspaceName = "default"
			vlog("auto-selected default workspace")
		}
		if workspaceName != "" {
			vlog("using workspace: %s", workspaceName)
		}
		cfg, err = config.Load(cfgPath, config.WithWorkspace(workspaceName))
	}
	if err != nil {
		return nil, nil, err
	}

	// Register built-in tools (after user tools, so built-ins overwrite on conflict)
	mode := serveMode
	if mode == "" {
		mode = "stdio"
	}
	builtins.Register(cfg, builtins.Options{Mode: mode})

	// Now that builtins are merged into cfg.Tools, run cross-reference validation
	// (workflow step targets, pack requires) against the full tool set.
	if err := config.ValidateReferences(cfg, nil); err != nil {
		return nil, nil, err
	}

	vlog("loaded %d tools", len(cfg.Tools))

	reg := registry.New()
	for name, toolCfg := range cfg.Tools {
		params := make([]registry.Parameter, len(toolCfg.Parameters))
		for i, p := range toolCfg.Parameters {
			params[i] = registry.Parameter{
				Name:        p.Name,
				Description: p.Description,
				Required:    p.Required,
				Type:        p.Type,
				Default:     p.Default,
				Min:         p.Min,
				Max:         p.Max,
				MinLength:   p.MinLength,
				MaxLength:   p.MaxLength,
				Pattern:     p.Pattern,
				Enum:        p.Enum,
			}
		}
		tool := &registry.Tool{
			Name:        name,
			Type:        toolCfg.Type,
			Description: toolCfg.Description,
			Hidden:      toolCfg.Hidden,
			Parameters:  params,
			ProviderKey: toolCfg.Type,
			MaxOutput:   toolCfg.MaxOutput,
			Compress:    toolCfg.Compress,
			Filter:      output.CompileFilter(toolCfg.Filter),
		}
		// Pass allow overrides from shadow config to registry for built-in guards
		if toolCfg.Shadow != nil {
			var overrides []string
			overrides = append(overrides, toolCfg.Shadow.AllowPatterns...)
			overrides = append(overrides, toolCfg.Shadow.AllowPaths...)
			overrides = append(overrides, toolCfg.Shadow.AllowURLs...)
			if len(overrides) > 0 {
				tool.AllowOverrides = overrides
			}
		}
		reg.Register(tool)
	}

	return cfg, reg, nil
}

var cachedResolver *vault.Resolver

// getCachedResolver returns the resolver from the last bootstrapProviders call.
func getCachedResolver() *vault.Resolver {
	return cachedResolver
}

// bootstrapProviders opens the vault if needed, creates providers, and
// wires everything into a proxy. Takes config and registry from loadConfig().
// confirmFn is used for shadow confirm prompts — nil uses the default CLI prompt.
func bootstrapProviders(cfg *config.Config, reg *registry.Registry, confirmFn ...shadow.ConfirmFunc) (*proxy.Proxy, error) {
	// Open vault resolver only when executing tools
	resolver, err := initResolver(cfg)
	if err != nil {
		return nil, err
	}
	cachedResolver = resolver

	// Register expr as a resolver backend so {{expr:now()}} etc. work everywhere
	if resolver != nil {
		resolver.RegisterFunc("expr", func(content string) (string, error) {
			return provider.EvalExpr(content, nil), nil
		})
	}

	providers := make(map[string]provider.Provider)
	cliTools := make(map[string]provider.CLIToolDef)
	restTools := make(map[string]provider.RESTToolDef)
	mcpServers := make(map[string]provider.MCPServerDef)
	workflowDefs := make(map[string][]provider.WorkflowStep)
	codeTools := make(map[string]config.ToolConfig)
	hasOAuth := false

	for name, toolCfg := range cfg.Tools {
		var vaultKeys []string // collect vault keys accessed for this tool

		switch toolCfg.Type {
		case "cli":
			// Resolve backend refs in args (e.g., {{vault:KEY}}, {{op:KEY}})
			resolvedArgs := make([]string, len(toolCfg.Args))
			for i, arg := range toolCfg.Args {
				resolvedArgs[i] = resolveVaultRefTracked(resolver, arg, &vaultKeys)
			}
			def := provider.CLIToolDef{
				Command:     resolveVaultRefTracked(resolver, toolCfg.Command, &vaultKeys),
				Args:        resolvedArgs,
				Stdin:       resolveVaultRefTracked(resolver, toolCfg.Stdin, &vaultKeys),
				Interactive: toolCfg.Interactive,
				Env:         resolveVaultMap(resolver, toolCfg.Env),
				EnvStrict:   toolCfg.EnvIsolation == "strict",
			}
			if toolCfg.Timeout != "" {
				if d, err := time.ParseDuration(toolCfg.Timeout); err == nil {
					def.Timeout = d
				} else {
					vlog("warning: invalid timeout %q for tool %s: %v", toolCfg.Timeout, name, err)
				}
			}
			cliTools[name] = def
			vlog("  registered cli tool: %s", name)
		case "rest":
			var vaultRefs []string
			collectRef := func(original string) {
				if vault.HasVaultRefs(original) {
					vaultRefs = append(vaultRefs, original)
				}
			}
			collectRef(toolCfg.BaseURL)
			collectRef(toolCfg.Path)
			if toolCfg.Auth != nil {
				collectRef(toolCfg.Auth.Token)
				collectRef(toolCfg.Auth.Value)
			}
			restDef := provider.RESTToolDef{
				Method:   toolCfg.Method,
				BaseURL:  resolveVaultRefTracked(resolver, toolCfg.BaseURL, &vaultKeys),
				Path:     resolveVaultRefTracked(resolver, toolCfg.Path, &vaultKeys),
				Body:     toolCfg.Body,
				BodyType: toolCfg.BodyType,
				Headers:  resolveVaultMap(resolver, toolCfg.Headers),
			}
			if toolCfg.Auth != nil {
				authDef := &provider.AuthDef{
					Type:   toolCfg.Auth.Type,
					Token:  resolveVaultRefTracked(resolver, toolCfg.Auth.Token, &vaultKeys),
					Header: toolCfg.Auth.Header,
					Value:  resolveVaultRefTracked(resolver, toolCfg.Auth.Value, &vaultKeys),
				}
				if toolCfg.Auth.Type == "oauth" {
					oauthCfg := cfg.ResolveOAuthProvider(toolCfg.Auth)
					collectRef(oauthCfg.ClientID)
					collectRef(oauthCfg.ClientSecret)
					authDef.OAuthProvider = &oauth.ProviderConfig{
						ClientID:     resolveVaultRefTracked(resolver, oauthCfg.ClientID, &vaultKeys),
						ClientSecret: resolveVaultRefTracked(resolver, oauthCfg.ClientSecret, &vaultKeys),
						AuthURL:      oauthCfg.AuthURL,
						TokenURL:     oauthCfg.TokenURL,
						Scopes:       oauthCfg.Scopes,
					}
					authDef.TokenKey = config.OAuthTokenKey(toolCfg.Auth)
					hasOAuth = true
				}
				restDef.Auth = authDef
			}
			for _, p := range toolCfg.Parameters {
				restDef.Params = append(restDef.Params, provider.RESTParamDef{
					Name:     p.Name,
					In:       p.In,
					Required: p.Required,
					Type:     p.Type,
				})
			}
			if toolCfg.Timeout != "" {
				if d, err := time.ParseDuration(toolCfg.Timeout); err == nil {
					restDef.Timeout = d
				} else {
					vlog("warning: invalid timeout %q for rest tool %s: %v", toolCfg.Timeout, name, err)
				}
			}
			if len(vaultRefs) > 0 && resolver != nil {
				refs := vaultRefs // capture for closure
				res := resolver   // capture for closure
				restDef.RedactSecrets = func(s string) string {
					return res.Redact(s, refs)
				}
			}
			restTools[name] = restDef
			vlog("  registered rest tool: %s", name)
		case "mcp":
			def := provider.MCPServerDef{
				Command:   toolCfg.Command,
				Args:      toolCfg.Args,
				Env:       resolveVaultMap(resolver, toolCfg.Env),
				EnvStrict: toolCfg.EnvIsolation == "strict",
				URL:       resolveVaultRef(resolver, toolCfg.URL),
			}
			if toolCfg.Timeout != "" {
				if d, err := time.ParseDuration(toolCfg.Timeout); err == nil {
					def.Timeout = d
				} else {
					vlog("warning: invalid timeout %q for mcp server %s: %v", toolCfg.Timeout, name, err)
				}
			}
			mcpServers[name] = def
			vlog("  registered mcp server: %s", name)
		case "builtin":
			vlog("  registered builtin tool: %s", name)
		case "workflow":
			steps := make([]provider.WorkflowStep, len(toolCfg.Steps))
			for i, s := range toolCfg.Steps {
				ws := provider.WorkflowStep{
					Tool:    s.Tool,
					Params:  s.Params,
					Store:   s.Store,
					If:      s.If,
					Require: s.Require,
				}
				for _, sc := range s.Switch {
					ws.Switch = append(ws.Switch, provider.WorkflowSwitchCase{
						Condition: sc.Condition,
						Tool:      sc.Tool,
						Params:    sc.Params,
						Store:     sc.Store,
					})
				}
				steps[i] = ws
			}
			workflowDefs[name] = steps
			vlog("  registered workflow: %s (%d steps)", name, len(steps))
		case "code":
			codeTools[name] = toolCfg
			vlog("  registered code tool: %s", name)
		}

		// Store vault keys on registry tool for audit logging
		if len(vaultKeys) > 0 {
			if tool, err := reg.Get(name); err == nil {
				tool.VaultKeys = dedup(vaultKeys)
			}
		}
	}

	// Validate no unresolved vault refs remain in provider configs
	if err := validateNoVaultRefs(restTools); err != nil {
		return nil, err
	}

	// Built-in provider (in-process, no subprocess)
	// Scope file operations to the project directory (config file's parent)
	projectDir := ""
	if configPath != "" {
		projectDir = filepath.Dir(configPath)
	}
	providers["builtin"] = provider.NewBuiltinProvider(serveMode, projectDir)
	vlog("initialized builtin provider (root: %s)", projectDir)

	if len(cliTools) > 0 {
		vlog("initialized cli provider (%d tools)", len(cliTools))
		providers["cli"] = provider.NewCLI(cliTools)
	}
	if len(restTools) > 0 {
		var tokenStore provider.TokenStore
		if hasOAuth && resolver != nil {
			if backend := resolver.Backend("vault"); backend != nil {
				tokenStore = newVaultTokenStore(backend)
			}
		}
		restProvider := provider.NewREST(restTools, tokenStore)
		restProvider.Verbose = verbose
		if err := restProvider.Setup(); err != nil {
			return nil, fmt.Errorf("rest provider setup: %w", err)
		}
		vlog("initialized rest provider (%d tools)", len(restTools))
		providers["rest"] = restProvider
	}
	if len(mcpServers) > 0 {
		var mcpProvider *provider.MCPProvider
		if wrapMode {
			mcpProvider = provider.NewMCPNoNamespace(mcpServers)
		} else {
			mcpProvider = provider.NewMCP(mcpServers)
		}
		if err := mcpProvider.Setup(); err != nil {
			return nil, fmt.Errorf("mcp provider setup: %w", err)
		}
		discovered, err := mcpProvider.DiscoverTools()
		if err != nil {
			_ = mcpProvider.Teardown()
			return nil, fmt.Errorf("mcp provider discovery: %w", err)
		}
		vlog("initialized mcp provider (%d servers, %d tools)", len(mcpServers), len(discovered))
		providers["mcp"] = mcpProvider

		// Register discovered tools in the registry
		for _, dt := range discovered {
			params := make([]registry.Parameter, len(dt.Parameters))
			for i, dp := range dt.Parameters {
				params[i] = registry.Parameter{
					Name:        dp.Name,
					Description: dp.Description,
					Required:    dp.Required,
				}
			}
			// Inherit output settings from the parent MCP server config
			parentCfg := cfg.Tools[dt.ServerKey]
			reg.Register(&registry.Tool{
				Name:        dt.Name,
				Type:        "mcp",
				Description: dt.Description,
				Parameters:  params,
				ProviderKey: "mcp",
				MaxOutput:   parentCfg.MaxOutput,
				Compress:    parentCfg.Compress,
			})
			vlog("    discovered: %s (%d params)", dt.Name, len(dt.Parameters))
		}
	}

	var logIface logger.Logger
	if os.Getenv("FACTORLY_NO_LOG") != "" {
		vlog("logging disabled (FACTORLY_NO_LOG set)")
		logIface = logger.NopLogger{}
	} else {
		// FACTORLY_LOG_PATH wins. Otherwise the log lives next to the
		// active config: project configs get .factorly/audit.jsonl in
		// their directory; the global config falls back to
		// ~/.config/factorly/audit.jsonl.
		logPath := os.Getenv("FACTORLY_LOG_PATH")
		if logPath == "" {
			logPath = logger.ProjectLogPath(configPath)
		}
		log, err := logger.NewJSONL(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to open log: %v\n", err)
			logIface = logger.NopLogger{}
		} else {
			logIface = log
		}
	}
	sharedLogger = logger.WithWorkspace(logIface, workspaceName)

	// Build shadow policy from config
	var proxyOpts []proxy.Option
	shadowRules := buildShadowRules(cfg)
	mergeDisabledToolsFromEnv(shadowRules)
	if len(shadowRules) > 0 {
		// Use provided confirm function, or default to CLI stdin prompt
		var cf shadow.ConfirmFunc
		if len(confirmFn) > 0 && confirmFn[0] != nil {
			cf = confirmFn[0]
		} else {
			cf = func(ctx context.Context, toolName string, params map[string]string) bool {
				fmt.Fprintf(os.Stderr, "⚠ Tool %q requires confirmation. Proceed? (y/n): ", toolName)
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					return strings.HasPrefix(strings.ToLower(scanner.Text()), "y")
				}
				return false
			}
		}
		policy := shadow.New(shadowRules, cf, shadow.ProjectRateStorePath(configPath))
		proxyOpts = append(proxyOpts, proxy.WithShadow(policy))
		vlog("shadow policy active (%d rules)", len(shadowRules))
	}

	if resolver != nil {
		proxyOpts = append(proxyOpts, proxy.WithResolver(resolver))
	}

	p := proxy.New(reg, providers, sharedLogger, proxyOpts...)

	// Wire workflow provider after proxy creation (needs proxy reference for step execution)
	if len(workflowDefs) > 0 {
		wp := provider.NewWorkflowProvider(p, verbose)
		wp.SetRunsDir(projectpath.Resolve(configPath, "runs", ""))
		for name, steps := range workflowDefs {
			wp.RegisterWorkflow(name, steps)
		}
		p.RegisterProvider("workflow", wp)
		vlog("initialized workflow provider (%d workflows)", len(workflowDefs))
	}

	// Wire code provider after proxy creation. Built in two cases:
	//   1. User has at least one type:code tool in config (V1 path).
	//   2. The factorly.code builtin is registered (the agent-authored
	//      script path — V2). In stdio mode it's always registered.
	//
	// Bad scripts (compile error, missing Run, denied import) log a
	// warning and are skipped so a single broken tool doesn't fail the
	// whole config load.
	_, hasCodeBuiltin := cfg.Tools["factorly.code"]
	if len(codeTools) > 0 || hasCodeBuiltin {
		cp := codeprov.NewProvider(p, verbose)
		registered := 0
		for name, tc := range codeTools {
			maxCalls := 0
			if tc.Shadow != nil {
				maxCalls = tc.Shadow.MaxCalls
			}
			if err := cp.RegisterCode(name, tc.Code, maxCalls); err != nil {
				vlog("warning: code tool %q failed to register: %v", name, err)
				continue
			}
			registered++
		}
		p.RegisterProvider("code", cp)
		vlog("initialized code provider (%d/%d code tools compiled)", registered, len(codeTools))

		// V2: register the factorly.code builtin handler now that the
		// code provider exists. Handler unpacks `code` + `params` and
		// delegates to cp.Run with the user-configured (or default)
		// max_calls budget read from the builtin's shadow config.
		if hasCodeBuiltin {
			if bp, ok := providers["builtin"].(*provider.BuiltinProvider); ok {
				builtinCfg := cfg.Tools["factorly.code"]
				bp.RegisterHandler("factorly.code", makeFactorlyCodeHandler(cp, builtinCfg))
				vlog("initialized factorly.code builtin handler")
			}
		}
	}

	// Register factorly.store.* builtin handlers. These give the agent
	// the four operations the CLI exposes — save, search, list, delete —
	// against the same workspace-scoped bbolt store. Handlers go through
	// getActiveStore(), so workspace cascade and audit logging are
	// identical to the CLI path.
	if bp, ok := providers["builtin"].(*provider.BuiltinProvider); ok {
		if _, has := cfg.Tools["factorly.store.save"]; has {
			bp.RegisterHandler("factorly.store.save", makeStoreSaveHandler())
		}
		if _, has := cfg.Tools["factorly.store.search"]; has {
			bp.RegisterHandler("factorly.store.search", makeStoreSearchHandler())
		}
		if _, has := cfg.Tools["factorly.store.list"]; has {
			bp.RegisterHandler("factorly.store.list", makeStoreListHandler())
		}
		if _, has := cfg.Tools["factorly.store.delete"]; has {
			bp.RegisterHandler("factorly.store.delete", makeStoreDeleteHandler())
		}
		vlog("initialized factorly.store.* builtin handlers")
	}

	return p, nil
}

// makeStoreSaveHandler returns a builtin handler that writes a
// key/value to the active workspace store. Mirrors `factorly store
// set` end-to-end including audit logging.
//
// Per-op open via withActiveStore so the bbolt file lock is released
// as soon as the write completes — concurrent factorly processes
// (CLI from another terminal, factorly ui) aren't blocked.
func makeStoreSaveHandler() provider.BuiltinHandler {
	return func(ctx context.Context, params map[string]string) (*provider.Result, error) {
		key := params["key"]
		if key == "" {
			return &provider.Result{Error: "key is required", ExitCode: 1}, nil
		}
		value := params["value"]
		ttl, hasTTL, ttlErr := parseStoreTTL(params["ttl"])
		if ttlErr != nil {
			return &provider.Result{Error: ttlErr.Error(), ExitCode: 1}, nil
		}
		var resultErr string
		err := withActiveStore(func(backend store.Backend) error {
			if hasTTL {
				lb, ok := backend.(*store.LocalBackend)
				if !ok {
					resultErr = "TTL not supported by backend"
					return nil
				}
				if err := lb.SetWithTTL(key, value, ttl); err != nil {
					logStoreOp("save", key, "error")
					resultErr = err.Error()
					return nil
				}
			} else if err := backend.Set(key, value); err != nil {
				logStoreOp("save", key, "error")
				resultErr = err.Error()
				return nil
			}
			logStoreOp("save", key, "success")
			return nil
		})
		if err != nil {
			return &provider.Result{Error: err.Error(), ExitCode: 1}, nil
		}
		if resultErr != "" {
			return &provider.Result{Error: resultErr, ExitCode: 1}, nil
		}
		return &provider.Result{Output: "saved " + key}, nil
	}
}

// makeStoreSearchHandler returns a substring-match handler. Output
// is newline-separated keys for ergonomic shell-style consumption.
func makeStoreSearchHandler() provider.BuiltinHandler {
	return func(ctx context.Context, params map[string]string) (*provider.Result, error) {
		var keys []string
		err := withCascadeStore(func(backend store.Backend) error {
			var listErr error
			keys, listErr = backend.Search(params["query"])
			return listErr
		})
		if err != nil {
			return &provider.Result{Error: err.Error(), ExitCode: 1}, nil
		}
		return &provider.Result{Output: joinNewline(keys)}, nil
	}
}

// makeStoreListHandler returns every key in the active store.
func makeStoreListHandler() provider.BuiltinHandler {
	return func(ctx context.Context, params map[string]string) (*provider.Result, error) {
		var keys []string
		err := withCascadeStore(func(backend store.Backend) error {
			var listErr error
			keys, listErr = backend.List()
			return listErr
		})
		if err != nil {
			return &provider.Result{Error: err.Error(), ExitCode: 1}, nil
		}
		return &provider.Result{Output: joinNewline(keys)}, nil
	}
}

// makeStoreDeleteHandler removes a key. Idempotent — missing keys
// are not errors, mirroring the CLI behavior.
func makeStoreDeleteHandler() provider.BuiltinHandler {
	return func(ctx context.Context, params map[string]string) (*provider.Result, error) {
		key := params["key"]
		if key == "" {
			return &provider.Result{Error: "key is required", ExitCode: 1}, nil
		}
		var resultErr string
		err := withActiveStore(func(backend store.Backend) error {
			if err := backend.Delete(key); err != nil {
				logStoreOp("delete", key, "error")
				resultErr = err.Error()
				return nil
			}
			logStoreOp("delete", key, "success")
			return nil
		})
		if err != nil {
			return &provider.Result{Error: err.Error(), ExitCode: 1}, nil
		}
		if resultErr != "" {
			return &provider.Result{Error: resultErr, ExitCode: 1}, nil
		}
		return &provider.Result{Output: "deleted " + key}, nil
	}
}

// joinNewline glues a string slice with newlines for the agent-
// facing output of List/Search. Pulled inline so the four handlers
// share one formatting rule.
func joinNewline(s []string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, k := range s[1:] {
		out += "\n" + k
	}
	return out
}

// makeFactorlyCodeHandler returns a builtin handler that compiles +
// runs an agent-supplied Go script through the code provider. params
// arrives as map[string]string; the JSON inner "params" field is parsed
// to a map[string]string and forwarded as the script's Run() arg.
func makeFactorlyCodeHandler(cp *codeprov.Provider, builtinCfg config.ToolConfig) provider.BuiltinHandler {
	defaultMaxCalls := 0
	if builtinCfg.Shadow != nil {
		defaultMaxCalls = builtinCfg.Shadow.MaxCalls
	}
	return func(ctx context.Context, params map[string]string) (*provider.Result, error) {
		src := params["code"]
		if src == "" {
			return &provider.Result{Error: "code is required", ExitCode: 1}, nil
		}
		// Unpack params["params"] (JSON object) into the map the script's
		// Run() expects. Tolerate empty/missing → empty map.
		innerParams := map[string]string{}
		if raw := params["params"]; raw != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				return &provider.Result{Error: "params must be a JSON object: " + err.Error(), ExitCode: 1}, nil
			}
			for k, v := range parsed {
				innerParams[k] = fmt.Sprint(v)
			}
		}
		return cp.Run(ctx, src, innerParams, defaultMaxCalls)
	}
}

// checkCommandAllowed returns an error if the command is disabled in config.
func checkCommandAllowed(name string) error {
	if _, err := config.IsCommandDisabled(name); err != nil {
		return err
	}
	return nil
}

// requireArgs returns a cobra args validator with a helpful error message.
func requireArgs(n int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

// extractGlobalFlags pulls out global flags (-v/--verbose, -c/--config,
// --config-dir, -w/--workspace) from args since DisableFlagParsing
// prevents cobra from handling them. Supports both space-separated
// (--flag value) and equals (--flag=value) forms.
func extractGlobalFlags(args []string) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-v" || a == "--verbose":
			verbose = true
		case a == "-c" || a == "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--config="):
			configPath = strings.TrimPrefix(a, "--config=")
		case a == "--config-dir":
			if i+1 < len(args) {
				configDir = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--config-dir="):
			configDir = strings.TrimPrefix(a, "--config-dir=")
		case a == "-w" || a == "--workspace":
			if i+1 < len(args) {
				workspaceName = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--workspace="):
			workspaceName = strings.TrimPrefix(a, "--workspace=")
		default:
			remaining = append(remaining, a)
		}
	}
	return remaining
}

// initResolver builds the process-wide resolver attached to the proxy.
// It's used to resolve {{backend:KEY}} placeholders in tool *parameter
// values* at call time (vs. config values, which resolveBackendRefs
// handles at load time). Two backends are wired:
//
//   - "env" — always registered. Uses EnvBackendWithOverrides when a
//     workspace is active so {{env:NAME}} in a param resolves to the
//     workspace's vars before falling back to os.Getenv.
//   - "vault" + external backends — registered when the config contains
//     vault refs (so we don't prompt for a vault password on tools
//     that don't need one).
//
// Returns an error if vault refs exist but the vault cannot be opened —
// never silently degrades.
func initResolver(cfg *config.Config) (*vault.Resolver, error) {
	hasVaultRefs := false
	for _, tool := range cfg.Tools {
		if tool.Auth != nil {
			if vault.HasVaultRefs(tool.Auth.Token) || vault.HasVaultRefs(tool.Auth.Value) {
				hasVaultRefs = true
				break
			}
		}
		for _, v := range tool.Env {
			if vault.HasVaultRefs(v) {
				hasVaultRefs = true
				break
			}
		}
		for _, v := range tool.Headers {
			if vault.HasVaultRefs(v) {
				hasVaultRefs = true
				break
			}
		}
		if vault.HasVaultRefs(tool.BaseURL) {
			hasVaultRefs = true
		}
		for _, arg := range tool.Args {
			if vault.HasVaultRefs(arg) {
				hasVaultRefs = true
				break
			}
		}
		if vault.HasVaultRefs(tool.Stdin) {
			hasVaultRefs = true
		}
		if vault.HasVaultRefs(tool.Command) {
			hasVaultRefs = true
		}
		if hasVaultRefs {
			break
		}
	}

	resolver := vault.NewResolver()

	// Always register an env backend. Parameter values can reference
	// {{env:NAME}} at runtime — workspace vars, --env overrides, and
	// fall-through to os.Getenv all flow through here.
	if vars := loadActiveWorkspaceVars(cfg); len(vars) > 0 {
		resolver.Register("env", vault.EnvBackendWithOverrides{Overrides: vars})
	} else {
		resolver.Register("env", vault.EnvBackend{})
	}

	// Register the store backend for {{store:KEY}} reference syntax.
	// Per-substitution open via withCascadeStore so the bbolt file
	// lock is held only for the microseconds it takes to read the key
	// — concurrent factorly processes don't block on each other.
	// (Laziness is also automatic: no {{store:KEY}} ref means no open.)
	resolver.RegisterFunc("store", func(key string) (string, error) {
		var value string
		err := withCascadeStore(func(backend store.Backend) error {
			v, getErr := backend.Get(key)
			if getErr != nil {
				return getErr
			}
			value = v
			return nil
		})
		return value, err
	})
	vlog("registered store backend for {{store:KEY}} resolution")

	if !hasVaultRefs {
		// No vault refs in config — skip the password prompt, but still
		// return the resolver so the proxy can resolve env refs in params.
		return resolver, nil
	}

	vlog("vault references detected, opening vault")

	// Register external vault backends first (they don't need local vault)
	for name, backendCfg := range cfg.VaultBackends {
		resolver.Register(name, vault.NewExternalBackend(name, backendCfg))
		vlog("registered external vault backend: %s", name)
	}

	// Try to open vault (project vault, global vault, or fallback)
	backend, openErr := getCachedVault()
	if openErr != nil {
		if len(cfg.VaultBackends) > 0 {
			vlog("local vault failed to open: %v (external backends available)", openErr)
		} else {
			vlog("warning: vault references found but vault unavailable: %v — refs will not be resolved", openErr)
			return resolver, nil
		}
	} else {
		resolver.Register("vault", backend)
		vlog("vault opened successfully")
	}

	return resolver, nil
}

// loadActiveWorkspaceVars returns the vars map for the active workspace
// (resolved from --workspace / FACTORLY_WORKSPACE / default auto-load),
// or nil when none is active or its load fails. Errors are swallowed —
// initResolver still needs to return a usable resolver even if the
// workspace file is missing.
func loadActiveWorkspaceVars(_ *config.Config) map[string]string {
	if workspaceName == "" {
		return nil
	}
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}
	ws, err := workspace.Load(cfgPath, workspaceName)
	if err != nil || ws == nil {
		return nil
	}
	return ws.Vars
}

func resolveVaultRef(resolver *vault.Resolver, s string) string {
	if resolver == nil || s == "" {
		return s
	}
	resolved, err := resolver.Resolve(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return s
	}
	return resolved
}

// resolveVaultRefTracked resolves a vault reference and appends accessed keys to the collector.
func resolveVaultRefTracked(resolver *vault.Resolver, s string, keys *[]string) string {
	if resolver == nil || s == "" {
		return s
	}
	resolved, accessed, err := resolver.ResolveTracked(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return s
	}
	*keys = append(*keys, accessed...)
	return resolved
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func resolveVaultMap(resolver *vault.Resolver, m map[string]string) map[string]string {
	if resolver == nil || len(m) == 0 {
		return m
	}
	resolved := make(map[string]string, len(m))
	for k, v := range m {
		resolved[k] = resolveVaultRef(resolver, v)
	}
	return resolved
}

// vaultTokenStore implements provider.TokenStore by resolving the
// active vault backend at call time. A static backend would pin OAuth
// reads/writes to whichever vault was open at provider construction —
// fine in CLI mode (workspace is fixed) but wrong in UI mode where the
// user can switch workspaces mid-session. The closure form lets the
// UI return its workspace-scoped chain on each call, so token
// refreshes land in vaults/<active>.enc rather than the project vault.
type vaultTokenStore struct {
	getBackend func() vault.Backend
}

// newVaultTokenStore wraps a single backend (CLI use case).
func newVaultTokenStore(b vault.Backend) *vaultTokenStore {
	return &vaultTokenStore{getBackend: func() vault.Backend { return b }}
}

// SetGetBackend lets the UI swap in a workspace-aware backend resolver
// after the proxy has been constructed. Without this, OAuth token
// refreshes triggered after a UI workspace switch would still write to
// the project vault (the one open when bootstrapProviders ran).
func (s *vaultTokenStore) SetGetBackend(fn func() vault.Backend) {
	s.getBackend = fn
}

func (s *vaultTokenStore) backend() (vault.Backend, error) {
	if s.getBackend == nil {
		return nil, fmt.Errorf("token store: no vault backend configured")
	}
	b := s.getBackend()
	if b == nil {
		return nil, fmt.Errorf("token store: vault backend unavailable")
	}
	return b, nil
}

func (s *vaultTokenStore) GetTokenBundle(key string) (*oauth.TokenBundle, error) {
	b, err := s.backend()
	if err != nil {
		return nil, err
	}
	raw, err := b.Get(key)
	if err != nil {
		return nil, err
	}
	var bundle oauth.TokenBundle
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		return nil, fmt.Errorf("parsing token bundle: %w", err)
	}
	return &bundle, nil
}

func (s *vaultTokenStore) SetTokenBundle(key string, bundle *oauth.TokenBundle) error {
	b, err := s.backend()
	if err != nil {
		return err
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	return b.Set(key, string(data))
}

func validateNoVaultRefs(restTools map[string]provider.RESTToolDef) error {
	for name, def := range restTools {
		if vault.HasVaultRefs(def.BaseURL) {
			return fmt.Errorf("unresolved vault reference in base_url for tool %q — check vault password and key", name)
		}
		if def.Auth != nil {
			if vault.HasVaultRefs(def.Auth.Token) {
				return fmt.Errorf("unresolved vault reference in auth token for tool %q — check vault password and key", name)
			}
			if vault.HasVaultRefs(def.Auth.Value) {
				return fmt.Errorf("unresolved vault reference in auth value for tool %q — check vault password and key", name)
			}
		}
		for k, v := range def.Headers {
			if vault.HasVaultRefs(v) {
				return fmt.Errorf("unresolved vault reference in header %q for tool %q — check vault password and key", k, name)
			}
		}
	}
	return nil
}

var sensitiveParamNames = []string{"token", "secret", "password", "key", "auth", "credential"}

func redactSensitiveParams(params map[string]string) map[string]string {
	redacted := make(map[string]string, len(params))
	for k, v := range params {
		lower := strings.ToLower(k)
		sensitive := false
		for _, s := range sensitiveParamNames {
			if strings.Contains(lower, s) {
				sensitive = true
				break
			}
		}
		if sensitive {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

func buildShadowRules(cfg *config.Config) map[string]*shadow.Rule {
	rules := make(map[string]*shadow.Rule)
	for name, toolCfg := range cfg.Tools {
		if toolCfg.Shadow == nil {
			continue
		}
		sc := toolCfg.Shadow
		confirmList, confirmAll := sc.ConfirmList()
		rule := &shadow.Rule{
			Deny:       sc.Deny,
			Confirm:    confirmList,
			ConfirmAll: confirmAll,
			LogParams:  sc.LogParams,
		}
		if sc.RateLimit != "" {
			rl, _ := shadow.ParseRateLimit(sc.RateLimit)
			rule.RateLimit = rl
		}
		rules[name] = rule
	}
	return rules
}

// mergeDisabledToolsFromEnv injects FACTORLY_DISABLED_TOOLS env var entries
// as deny rules into the shadow rules map.
func mergeDisabledToolsFromEnv(rules map[string]*shadow.Rule) {
	disabled := os.Getenv("FACTORLY_DISABLED_TOOLS")
	if disabled == "" {
		return
	}
	for _, toolName := range strings.Split(disabled, ",") {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		if existing, ok := rules[toolName]; ok {
			existing.Deny = append(existing.Deny, toolName)
		} else {
			rules[toolName] = &shadow.Rule{
				Deny: []string{toolName},
			}
		}
	}
	vlog("disabled tools from env: %s", disabled)
}

func hasMCPTools(cfg *config.Config) bool {
	for _, tool := range cfg.Tools {
		if tool.Type == "mcp" {
			return true
		}
	}
	return false
}

func parseToolArgs(args []string) map[string]string {
	params := make(map[string]string)
	stdinUsed := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				value := args[i+1]
				if value == "-" && !stdinUsed {
					// Read from stdin
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: reading stdin for --%s: %v\n", key, err)
					} else {
						value = strings.TrimRight(string(data), "\n")
					}
					stdinUsed = true
				} else if value == "@@" || strings.HasPrefix(value, "@@") {
					// Escaped @@ → literal @
					value = value[1:]
				} else if strings.HasPrefix(value, "@") {
					// Read from file (like curl's @filename)
					filePath := value[1:]
					data, err := os.ReadFile(filePath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "warning: reading file for --%s: %v\n", key, err)
					} else {
						value = strings.TrimRight(string(data), "\n")
					}
				}
				params[key] = value
				i++
			} else {
				params[key] = "true"
			}
		}
	}
	return params
}
