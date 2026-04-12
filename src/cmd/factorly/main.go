package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"encoding/json"
	"time"

	"github.com/factorly-dev/factorly-cli/internal"
	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/logger"
	"github.com/factorly-dev/factorly-cli/internal/oauth"
	"github.com/factorly-dev/factorly-cli/internal/openapi"
	"github.com/factorly-dev/factorly-cli/internal/provider"
	"github.com/factorly-dev/factorly-cli/internal/proxy"
	"github.com/factorly-dev/factorly-cli/internal/registry"
	"github.com/factorly-dev/factorly-cli/internal/shadow"
	"github.com/factorly-dev/factorly-cli/internal/vault"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configPath string
var configDir string
var verbose bool
var wrapMode bool

func vlog(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[factorly] "+format+"\n", args...)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:          "factorly",
	Short:        "One endpoint. All your tools.",
	Long:         "Factorly wraps your existing agent tools — MCP servers, REST APIs, CLIs — into a single endpoint.",
	SilenceUsage: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("factorly %s\n", internal.Version)
	},
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

func runToolsList(cmd *cobra.Command, args []string) error {
	cfg, reg, err := loadConfig()
	if err != nil {
		return err
	}

	// Bootstrap providers to discover MCP sub-tools
	if hasMCPTools(cfg) {
		if _, err := bootstrapProviders(cfg, reg); err != nil {
			return err
		}
	}

	// Build shadow policy to filter denied tools from listing
	rules := buildShadowRules(cfg)
	mergeDisabledToolsFromEnv(rules)
	var shadowPolicy *shadow.Policy
	if len(rules) > 0 {
		shadowPolicy = shadow.New(rules, nil, "")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tDESCRIPTION\tPARAMETERS")
	for _, t := range reg.List() {
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
		if _, err := reg.Get(toolName); err != nil && !hasMCPTools(cfg) {
			return err
		}

		p, err := bootstrapProviders(cfg, reg)
		if err != nil {
			return err
		}

		// Resolve {{vault:KEY}} refs in parameter values
		for _, v := range params {
			if vault.HasVaultRefs(v) {
				backend, err := openVault()
				if err != nil {
					return fmt.Errorf("resolving vault refs in params: %w", err)
				}
				defer backend.Close()
				resolver := vault.NewResolver()
				resolver.Register("vault", backend)
				for k, pv := range params {
					params[k] = resolveVaultRef(resolver, pv)
				}
				break
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

		// Tools directory
		useToolsDir := prompt(scanner, "Use a tools directory for modular configs? (y/n)", "n")
		toolsDir := ""
		if strings.HasPrefix(strings.ToLower(useToolsDir), "y") {
			toolsDir = prompt(scanner, "Tools directory path", "./tools")
			// Resolve tools dir relative to the config file location
			absToolsDir := toolsDir
			if !filepath.IsAbs(absToolsDir) {
				absToolsDir = filepath.Join(filepath.Dir(outPath), absToolsDir)
			}
			if err := os.MkdirAll(absToolsDir, 0o755); err != nil {
				return fmt.Errorf("creating tools directory: %w", err)
			}
		}

		// Example tool
		addExample := prompt(scanner, "Add an example CLI tool? (y/n)", "y")

		// Build config
		type yamlConfig struct {
			ToolsDir string                       `yaml:"tools_dir,omitempty"`
			Tools    map[string]config.ToolConfig `yaml:"tools"`
		}
		cfg := yamlConfig{
			ToolsDir: toolsDir,
			Tools:    make(map[string]config.ToolConfig),
		}

		if strings.HasPrefix(strings.ToLower(addExample), "y") {
			cfg.Tools["web.fetch"] = config.ToolConfig{
				Type:        "cli",
				Description: "Fetch a webpage",
				Command:     "curl",
				Args:        []string{"-s", "{url}"},
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
				} else if toolsDir != "" {
					// Write to tools dir (resolved relative to config)
					specName := filepath.Base(specPath)
					specName = strings.TrimSuffix(specName, filepath.Ext(specName))
					resolvedToolsDir := toolsDir
					if !filepath.IsAbs(resolvedToolsDir) {
						resolvedToolsDir = filepath.Join(filepath.Dir(outPath), resolvedToolsDir)
					}
					outFile := filepath.Join(resolvedToolsDir, specName+".yaml")
					data, err := yaml.Marshal(tools)
					if err == nil {
						if err := os.WriteFile(outFile, data, 0o644); err != nil {
							fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", outFile, err)
						} else {
							fmt.Fprintf(os.Stderr, "Wrote %d tools to %s\n", len(tools), outFile)
						}
					}
				} else {
					// Merge into main config
					for name, tool := range tools {
						cfg.Tools[name] = tool
					}
					fmt.Fprintf(os.Stderr, "Added %d tools from spec\n", len(tools))
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
		fmt.Println("Run 'factorly tools' to see your configured tools.")
		return nil
	},
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "print detailed progress to stderr")

	initCmd.Flags().StringVarP(&initOut, "out", "o", "", "output file path (default: .factorly/factorly.yaml)")

	importOpenAPICmd.Flags().StringVarP(&importOpenAPIOut, "out", "o", "", "output file path (default: stdout)")
	importOpenAPICmd.Flags().StringVarP(&importOpenAPIPrefix, "prefix", "p", "", "tool name prefix (default: from spec title)")
	importCmd.AddCommand(importOpenAPICmd, templatesCmd)

	toolsCmd.AddCommand(toolsListCmd, addCmd, removeCmd, importCmd, recordCmd)
	rootCmd.AddCommand(versionCmd, toolsCmd, callCmd, initCmd, syncCmd, vaultCmd, authCmd, statusCmd, serveCmd, wrapCmd, logsCmd)
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
			vlog("found config: %s", cfgPath)
		} else {
			vlog("using config: %s", cfgPath)
		}
		cfg, err = config.Load(cfgPath)
	}
	if err != nil {
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
			}
		}
		reg.Register(&registry.Tool{
			Name:        name,
			Type:        toolCfg.Type,
			Description: toolCfg.Description,
			Parameters:  params,
			ProviderKey: toolCfg.Type,
			MaxOutput:   toolCfg.MaxOutput,
			Compress:    toolCfg.Compress,
		})
	}

	return cfg, reg, nil
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

	providers := make(map[string]provider.Provider)
	cliTools := make(map[string]provider.CLIToolDef)
	restTools := make(map[string]provider.RESTToolDef)
	mcpServers := make(map[string]provider.MCPServerDef)
	hasOAuth := false

	for name, toolCfg := range cfg.Tools {
		switch toolCfg.Type {
		case "cli":
			def := provider.CLIToolDef{
				Command:     toolCfg.Command,
				Args:        toolCfg.Args,
				Stdin:       toolCfg.Stdin,
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
			restDef := provider.RESTToolDef{
				Method:  toolCfg.Method,
				BaseURL: resolveVaultRef(resolver, toolCfg.BaseURL),
				Path:    toolCfg.Path,
				Headers: resolveVaultMap(resolver, toolCfg.Headers),
			}
			if toolCfg.Auth != nil {
				authDef := &provider.AuthDef{
					Type:   toolCfg.Auth.Type,
					Token:  resolveVaultRef(resolver, toolCfg.Auth.Token),
					Header: toolCfg.Auth.Header,
					Value:  resolveVaultRef(resolver, toolCfg.Auth.Value),
				}
				if toolCfg.Auth.Type == "oauth" {
					oauthCfg := cfg.ResolveOAuthProvider(toolCfg.Auth)
					authDef.OAuthProvider = &oauth.ProviderConfig{
						ClientID:     resolveVaultRef(resolver, oauthCfg.ClientID),
						ClientSecret: resolveVaultRef(resolver, oauthCfg.ClientSecret),
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
				})
			}
			restTools[name] = restDef
			vlog("  registered rest tool: %s", name)
		case "mcp":
			mcpServers[name] = provider.MCPServerDef{
				Command:   toolCfg.Command,
				Args:      toolCfg.Args,
				Env:       resolveVaultMap(resolver, toolCfg.Env),
				EnvStrict: toolCfg.EnvIsolation == "strict",
				URL:       resolveVaultRef(resolver, toolCfg.URL),
			}
			vlog("  registered mcp server: %s", name)
		}
	}

	// Validate no unresolved vault refs remain in provider configs
	if err := validateNoVaultRefs(restTools); err != nil {
		return nil, err
	}

	if len(cliTools) > 0 {
		vlog("initialized cli provider (%d tools)", len(cliTools))
		providers["cli"] = provider.NewCLI(cliTools)
	}
	if len(restTools) > 0 {
		var tokenStore provider.TokenStore
		if hasOAuth && resolver != nil {
			if backend := resolver.Backend("vault"); backend != nil {
				tokenStore = &vaultTokenStore{backend: backend}
			}
		}
		restProvider := provider.NewREST(restTools, tokenStore)
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
		log, err := logger.NewJSONL("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to open log: %v\n", err)
			logIface = logger.NopLogger{}
		} else {
			logIface = log
		}
	}

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
		policy := shadow.New(shadowRules, cf, "")
		proxyOpts = append(proxyOpts, proxy.WithShadow(policy))
		vlog("shadow policy active (%d rules)", len(shadowRules))
	}

	p := proxy.New(reg, providers, logIface, proxyOpts...)
	return p, nil
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

// extractGlobalFlags pulls out global flags (-v, --verbose, -c, --config)
// from args since DisableFlagParsing prevents cobra from handling them.
func extractGlobalFlags(args []string) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-v", "--verbose":
			verbose = true
		case "-c", "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case "--config-dir":
			if i+1 < len(args) {
				configDir = args[i+1]
				i++
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

// initResolver checks if any config values contain vault references
// and opens the vault if needed. Returns an error if vault refs exist
// but the vault cannot be opened — never silently degrades.
func initResolver(cfg *config.Config) (*vault.Resolver, error) {
	hasRefs := false
	for _, tool := range cfg.Tools {
		if tool.Auth != nil {
			if vault.HasVaultRefs(tool.Auth.Token) || vault.HasVaultRefs(tool.Auth.Value) {
				hasRefs = true
				break
			}
		}
		for _, v := range tool.Env {
			if vault.HasVaultRefs(v) {
				hasRefs = true
				break
			}
		}
		for _, v := range tool.Headers {
			if vault.HasVaultRefs(v) {
				hasRefs = true
				break
			}
		}
		if vault.HasVaultRefs(tool.BaseURL) {
			hasRefs = true
		}
		if hasRefs {
			break
		}
	}

	if !hasRefs {
		return nil, nil
	}

	vlog("vault references detected, opening vault")
	backend, err := openVault()
	if err != nil {
		return nil, fmt.Errorf("vault required but failed to open: %w", err)
	}

	resolver := vault.NewResolver()
	resolver.Register("vault", backend)
	vlog("vault opened successfully")
	return resolver, nil
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

// vaultTokenStore implements provider.TokenStore using the vault backend.
type vaultTokenStore struct {
	backend vault.Backend
}

func (s *vaultTokenStore) GetTokenBundle(key string) (*oauth.TokenBundle, error) {
	raw, err := s.backend.Get(key)
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
	data, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	return s.backend.Set(key, string(data))
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
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				params[key] = args[i+1]
				i++
			} else {
				params[key] = "true"
			}
		}
	}
	return params
}
