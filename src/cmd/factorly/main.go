package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/factorly-dev/factorly/internal"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/spf13/cobra"
)

var configPath string

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
		toolName := args[0]
		params := parseToolArgs(args[1:])

		p, _, err := bootstrap()
		if err != nil {
			return err
		}

		result, err := p.Execute(toolName, params, "cli")
		if err != nil {
			return err
		}

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

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to factorly.yaml")
	rootCmd.AddCommand(versionCmd, toolsCmd, callCmd)
}

func bootstrap() (*proxy.Proxy, *registry.Registry, error) {
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}

	reg := registry.New()
	providers := make(map[string]provider.Provider)

	// Group CLI tools by provider key
	cliTools := make(map[string]provider.CLIToolDef)

	for name, toolCfg := range cfg.Tools {
		switch toolCfg.Type {
		case "cli":
			cliTools[name] = provider.CLIToolDef{
				Command: toolCfg.Command,
				Args:    toolCfg.Args,
				Env:     toolCfg.Env,
			}

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
				ProviderKey: "cli",
			})

		default:
			fmt.Fprintf(os.Stderr, "warning: tool %q has unsupported type %q (Day 1 supports cli only)\n", name, toolCfg.Type)
		}
	}

	if len(cliTools) > 0 {
		providers["cli"] = provider.NewCLI(cliTools)
	}

	log, err := logger.NewJSONL("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to open log: %v\n", err)
		log = nil
	}

	var logIface logger.Logger
	if log != nil {
		logIface = log
	} else {
		logIface = logger.NopLogger{}
	}

	p := proxy.New(reg, providers, logIface)
	return p, reg, nil
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
