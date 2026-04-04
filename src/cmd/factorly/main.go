package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/factorly-dev/factorly/internal"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/openapi"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configPath string
var configDir string
var verbose bool

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
	Short: "List all configured tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, reg, err := bootstrap()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tDESCRIPTION\tPARAMETERS")
		for _, t := range reg.List() {
			params := make([]string, len(t.Parameters))
			for i, p := range t.Parameters {
				params[i] = p.Name
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Type, t.Description, strings.Join(params, ", "))
		}
		return w.Flush()
	},
}

var callCmd = &cobra.Command{
	Use:                "call <tool> [--param value ...]",
	Short:              "Call a tool",
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Manually extract global flags that cobra can't parse due to DisableFlagParsing
		args = extractGlobalFlags(args)

		if len(args) == 0 {
			return fmt.Errorf("usage: factorly call <tool> [--param value ...]")
		}
		toolName := args[0]
		params := parseToolArgs(args[1:])

		vlog("calling tool: %s", toolName)
		vlog("  params: %v", params)

		p, _, err := bootstrap()
		if err != nil {
			return err
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
	Args:  cobra.ExactArgs(1),
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
	importCmd.AddCommand(importOpenAPICmd)

	rootCmd.AddCommand(versionCmd, toolsCmd, callCmd, importCmd, initCmd)
}

func bootstrap() (*proxy.Proxy, *registry.Registry, error) {
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
	providers := make(map[string]provider.Provider)

	cliTools := make(map[string]provider.CLIToolDef)
	restTools := make(map[string]provider.RESTToolDef)

	for name, toolCfg := range cfg.Tools {
		params := make([]registry.Parameter, len(toolCfg.Parameters))
		for i, p := range toolCfg.Parameters {
			params[i] = registry.Parameter{
				Name:        p.Name,
				Description: p.Description,
				Required:    p.Required,
			}
		}

		providerKey := toolCfg.Type

		switch toolCfg.Type {
		case "cli":
			cliTools[name] = provider.CLIToolDef{
				Command: toolCfg.Command,
				Args:    toolCfg.Args,
				Env:     toolCfg.Env,
			}
		case "rest":
			restDef := provider.RESTToolDef{
				Method:  toolCfg.Method,
				BaseURL: toolCfg.BaseURL,
				Path:    toolCfg.Path,
				Headers: toolCfg.Headers,
			}
			if toolCfg.Auth != nil {
				restDef.Auth = &provider.AuthDef{
					Type:   toolCfg.Auth.Type,
					Token:  toolCfg.Auth.Token,
					Header: toolCfg.Auth.Header,
					Value:  toolCfg.Auth.Value,
				}
			}
			for _, p := range toolCfg.Parameters {
				restDef.Params = append(restDef.Params, provider.RESTParamDef{
					Name:     p.Name,
					In:       p.In,
					Required: p.Required,
				})
			}
			restTools[name] = restDef
		}

		vlog("  registered tool: %s (type: %s)", name, toolCfg.Type)
		reg.Register(&registry.Tool{
			Name:        name,
			Type:        toolCfg.Type,
			Description: toolCfg.Description,
			Parameters:  params,
			ProviderKey: providerKey,
		})
	}

	if len(cliTools) > 0 {
		vlog("initialized cli provider (%d tools)", len(cliTools))
		providers["cli"] = provider.NewCLI(cliTools)
	}
	if len(restTools) > 0 {
		restProvider := provider.NewREST(restTools)
		if err := restProvider.Setup(); err != nil {
			return nil, nil, fmt.Errorf("rest provider setup: %w", err)
		}
		vlog("initialized rest provider (%d tools)", len(restTools))
		providers["rest"] = restProvider
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

	p := proxy.New(reg, providers, logIface)
	return p, reg, nil
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
