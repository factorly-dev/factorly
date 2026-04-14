package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/parsing/curl"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var recordFlags struct {
	curl    string
	name    string
	noVault bool
	dryRun  bool
}

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Create a tool from a curl command",
	Long: `Parse a curl command and generate a Factorly REST tool definition.

Pipe a curl command:
  echo 'curl -H "Authorization: Bearer tok" https://api.example.com/data' | factorly tools record

Or pass inline:
  factorly tools record --curl 'curl -X POST https://api.stripe.com/v1/charges -d amount=2000'

Use --dry-run to preview without writing.`,
	RunE: runRecord,
}

func runRecord(cmd *cobra.Command, args []string) error {
	// 1. Get curl input
	input, err := getCurlInput()
	if err != nil {
		return err
	}

	// 2. Parse
	parsed, err := curl.Parse(input)
	if err != nil {
		return fmt.Errorf("parsing curl: %w", err)
	}

	// 3. Convert to tool config
	tool, auth := curl.ToToolConfig(parsed)

	// 4. Determine name
	toolName := recordFlags.name
	if toolName == "" {
		toolName = curl.DeriveToolName(parsed.URL, parsed.Method)
	}
	if isInteractive() && recordFlags.name == "" {
		scanner := bufio.NewScanner(os.Stdin)
		override := prompt(scanner, fmt.Sprintf("Tool name [%s]", toolName), toolName)
		if override != "" {
			toolName = override
		}
	}

	// 5. Handle auth — offer vault storage
	if auth != nil && !recordFlags.noVault && isInteractive() {
		scanner := bufio.NewScanner(os.Stdin)
		authDesc := fmt.Sprintf("%s auth detected", auth.Type)
		if auth.HeaderName != "" {
			authDesc = fmt.Sprintf("%s header (%s) detected", auth.HeaderName, auth.Type)
		}
		fmt.Fprintf(os.Stderr, "  %s\n", authDesc)

		storeInVault := prompt(scanner, fmt.Sprintf("Store credential in vault as %s? (y/n)", auth.VaultKey), "y")
		if strings.HasPrefix(strings.ToLower(storeInVault), "y") {
			vaultKey := prompt(scanner, "Vault key", auth.VaultKey)
			backend, err := openVault()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open vault: %v\n", err)
			} else {
				if err := backend.Set(vaultKey, auth.RawValue); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not store in vault: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "  Stored %s in vault\n", vaultKey)
					// Update tool auth to use vault ref
					vaultRef := "{{vault:" + vaultKey + "}}"
					if tool.Auth != nil {
						switch tool.Auth.Type {
						case "bearer":
							tool.Auth.Token = vaultRef
						case "basic":
							tool.Auth.Token = vaultRef
						case "header":
							tool.Auth.Value = vaultRef
						}
					}
				}
				backend.Close()
			}
		}
	}

	// 6. Dry-run: print YAML and exit
	if recordFlags.dryRun {
		data, err := yaml.Marshal(map[string]config.ToolConfig{toolName: tool})
		if err != nil {
			return fmt.Errorf("marshaling: %w", err)
		}
		fmt.Print(string(data))
		return nil
	}

	// 7. Load config and write
	cfg, _, _ := loadConfig()
	if cfg == nil {
		cfg = &config.Config{Tools: make(map[string]config.ToolConfig)}
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]config.ToolConfig)
	}
	// Get tools_dir from raw config
	if cfg.ToolsDir == "" {
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = config.FindConfig()
		}
		if data, err := os.ReadFile(cfgPath); err == nil {
			var raw struct {
				ToolsDir string `yaml:"tools_dir"`
			}
			if err := yaml.Unmarshal(data, &raw); err == nil && raw.ToolsDir != "" {
				cfg.ToolsDir = raw.ToolsDir
			}
		}
	}

	if _, exists := cfg.Tools[toolName]; exists {
		return fmt.Errorf("tool %q already exists in config", toolName)
	}

	outPath, err := writeNewTool(toolName, tool, cfg)
	if err != nil {
		return err
	}

	// 8. Summary
	fmt.Fprintf(os.Stderr, "\n  Created %s\n", toolName)
	fmt.Fprintf(os.Stderr, "    type: %s\n", tool.Type)
	fmt.Fprintf(os.Stderr, "    method: %s\n", tool.Method)
	fmt.Fprintf(os.Stderr, "    base_url: %s\n", tool.BaseURL)
	fmt.Fprintf(os.Stderr, "    path: %s\n", tool.Path)
	if tool.Auth != nil {
		fmt.Fprintf(os.Stderr, "    auth: %s\n", tool.Auth.Type)
	}
	if len(tool.Parameters) > 0 {
		paramNames := make([]string, len(tool.Parameters))
		for i, p := range tool.Parameters {
			paramNames[i] = p.Name + " (" + p.In + ")"
		}
		fmt.Fprintf(os.Stderr, "    params: %s\n", strings.Join(paramNames, ", "))
	}
	fmt.Fprintf(os.Stderr, "\n  Added %s to %s\n", toolName, outPath)

	// Usage hint
	if len(tool.Parameters) > 0 {
		var paramHints []string
		for _, p := range tool.Parameters {
			paramHints = append(paramHints, "--"+p.Name+" <value>")
		}
		fmt.Fprintf(os.Stderr, "  Usage: factorly call %s %s\n", toolName, strings.Join(paramHints, " "))
	}

	return nil
}

func getCurlInput() (string, error) {
	// --curl flag
	if recordFlags.curl != "" {
		return recordFlags.curl, nil
	}

	// Piped stdin
	if !isInteractive() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		input := strings.TrimSpace(string(data))
		if input == "" {
			return "", fmt.Errorf("no curl command received on stdin")
		}
		return input, nil
	}

	// Interactive: prompt for paste
	fmt.Fprintln(os.Stderr, "Paste your curl command (end with Enter on a line without \\):")
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if !strings.HasSuffix(strings.TrimSpace(line), "\\") {
			break
		}
	}
	input := strings.Join(lines, "\n")
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("no curl command entered")
	}
	return input, nil
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func init() {
	recordCmd.Flags().StringVar(&recordFlags.curl, "curl", "", "curl command string")
	recordCmd.Flags().StringVar(&recordFlags.name, "name", "", "override auto-derived tool name")
	recordCmd.Flags().BoolVar(&recordFlags.noVault, "no-vault", false, "skip vault storage prompt")
	recordCmd.Flags().BoolVar(&recordFlags.dryRun, "dry-run", false, "print YAML to stdout without writing")
}
