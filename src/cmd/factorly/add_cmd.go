package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var addFlags struct {
	name        string
	toolType    string
	description string
	command     string
	args        string
	stdin       string
	baseURL     string
	method      string
	path        string
	authType    string
	authToken   string
	url         string // MCP HTTP
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new tool to the config",
	Long: `Interactively create a new tool definition and add it to your config.

Use flags for non-interactive mode:
  factorly tools add --name web.fetch --type cli --command curl --args '-s,{{url}}'
  factorly tools add --name slack --type mcp --command npx --args '@modelcontextprotocol/server-slack'`,
	RunE: runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	// Load existing config to check for duplicates and find write location.
	// Errors are OK here — config might not exist yet or have no tools.
	cfg, _, _ := loadConfig()
	if cfg == nil {
		cfg = &config.Config{Tools: make(map[string]config.ToolConfig)}
	}
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]config.ToolConfig)
	}

	// Try to get ToolsDir from raw config even if validation failed
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

	var tool config.ToolConfig
	var toolName string
	var err error

	if addFlags.name != "" && addFlags.toolType != "" {
		// Non-interactive mode
		toolName = addFlags.name
		tool, err = buildToolFromFlags()
		if err != nil {
			return err
		}
	} else {
		// Interactive mode
		scanner := bufio.NewScanner(os.Stdin)
		toolName, tool, err = promptForTool(scanner)
		if err != nil {
			return err
		}
	}

	// Check for duplicates
	if cfg != nil {
		if _, exists := cfg.Tools[toolName]; exists {
			return fmt.Errorf("tool %q already exists in config", toolName)
		}
	}

	// Determine where to write
	outPath, err := writeNewTool(toolName, tool, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Added %s to %s\n", toolName, outPath)

	// Optional health check
	if addFlags.name == "" {
		// Only in interactive mode
		scanner := bufio.NewScanner(os.Stdin)
		runCheck := prompt(scanner, "Run health check? (y/n)", "y")
		if strings.HasPrefix(strings.ToLower(runCheck), "y") {
			r := runToolHealthCheck(toolName, tool)
			icon := "✓"
			if !r.OK {
				icon = "✗"
			}
			fmt.Fprintf(os.Stderr, "  %s %s  %s\n", icon, r.Name, r.Message)
		}
	}

	return nil
}

func buildToolFromFlags() (config.ToolConfig, error) {
	tool := config.ToolConfig{
		Type:        addFlags.toolType,
		Description: addFlags.description,
	}

	switch addFlags.toolType {
	case "cli":
		if addFlags.command == "" {
			return tool, fmt.Errorf("--command is required for cli tools")
		}
		tool.Command = addFlags.command
		if addFlags.args != "" {
			tool.Args = splitArgs(addFlags.args)
		}
		if addFlags.stdin != "" {
			tool.Stdin = addFlags.stdin
		}
	case "rest":
		if addFlags.baseURL == "" {
			return tool, fmt.Errorf("--base-url is required for rest tools")
		}
		if addFlags.method == "" {
			addFlags.method = "GET"
		}
		tool.BaseURL = addFlags.baseURL
		tool.Method = strings.ToUpper(addFlags.method)
		tool.Path = addFlags.path
		if addFlags.authType != "" && addFlags.authType != "none" {
			tool.Auth = &config.AuthConfig{
				Type:  addFlags.authType,
				Token: addFlags.authToken,
			}
		}
	case "mcp":
		if addFlags.command == "" && addFlags.url == "" {
			return tool, fmt.Errorf("--command (stdio) or --url (http) is required for mcp tools")
		}
		tool.Command = addFlags.command
		if addFlags.args != "" {
			tool.Args = splitArgs(addFlags.args)
		}
		tool.URL = addFlags.url
	default:
		return tool, fmt.Errorf("unknown tool type %q (use cli, rest, or mcp)", addFlags.toolType)
	}

	return tool, nil
}

func promptForTool(scanner *bufio.Scanner) (string, config.ToolConfig, error) {
	name := prompt(scanner, "Tool name", "")
	if name == "" {
		return "", config.ToolConfig{}, fmt.Errorf("tool name is required")
	}

	toolType := prompt(scanner, "Type (cli/rest/mcp)", "cli")
	description := prompt(scanner, "Description", "")

	tool := config.ToolConfig{
		Type:        toolType,
		Description: description,
	}

	switch toolType {
	case "cli":
		tool.Command = prompt(scanner, "Command", "")
		if tool.Command == "" {
			return "", tool, fmt.Errorf("command is required")
		}
		argsStr := prompt(scanner, "Args (comma-separated, use {{param}} for placeholders)", "")
		if argsStr != "" {
			tool.Args = splitArgs(argsStr)
		}
		stdinStr := prompt(scanner, "Stdin template (optional, use {{param}} for placeholders)", "")
		if stdinStr != "" {
			tool.Stdin = stdinStr
		}

	case "rest":
		tool.BaseURL = prompt(scanner, "Base URL", "")
		if tool.BaseURL == "" {
			return "", tool, fmt.Errorf("base_url is required")
		}
		tool.Method = strings.ToUpper(prompt(scanner, "Method", "GET"))
		tool.Path = prompt(scanner, "Path (e.g., /api/v1/items/{{id}})", "")

		authType := prompt(scanner, "Auth type (bearer/basic/header/oauth/none)", "none")
		if authType != "none" && authType != "" {
			tool.Auth = &config.AuthConfig{Type: authType}
			switch authType {
			case "bearer":
				tool.Auth.Token = prompt(scanner, "Token (or vault ref like {{vault:KEY}}})", "")
			case "basic":
				tool.Auth.Token = prompt(scanner, "Credentials (user:pass or vault ref)", "")
			case "header":
				tool.Auth.Header = prompt(scanner, "Header name", "X-Api-Key")
				tool.Auth.Value = prompt(scanner, "Header value (or vault ref)", "")
			case "oauth":
				tool.Auth.Provider = prompt(scanner, "OAuth provider name (from oauth_providers)", "")
				if tool.Auth.Provider == "" {
					tool.Auth.ClientID = prompt(scanner, "Client ID (or vault ref)", "")
					tool.Auth.AuthURL = prompt(scanner, "Auth URL", "")
					tool.Auth.TokenURL = prompt(scanner, "Token URL", "")
					tool.Auth.TokenKey = prompt(scanner, "Token key (vault key for token bundle)", "")
					scopesStr := prompt(scanner, "Scopes (comma-separated)", "")
					if scopesStr != "" {
						tool.Auth.Scopes = strings.Split(scopesStr, ",")
						for i := range tool.Auth.Scopes {
							tool.Auth.Scopes[i] = strings.TrimSpace(tool.Auth.Scopes[i])
						}
					}
				}
			}
		}

		// Parameters
		for {
			addParam := prompt(scanner, "Add parameter? (y/n)", "n")
			if !strings.HasPrefix(strings.ToLower(addParam), "y") {
				break
			}
			param := config.ParamConfig{}
			param.Name = prompt(scanner, "  Name", "")
			if param.Name == "" {
				break
			}
			param.In = prompt(scanner, "  In (query/path/header/body)", "query")
			reqStr := prompt(scanner, "  Required? (y/n)", "n")
			param.Required = strings.HasPrefix(strings.ToLower(reqStr), "y")
			tool.Parameters = append(tool.Parameters, param)
		}

	case "mcp":
		transport := prompt(scanner, "Transport (stdio/http)", "stdio")
		if transport == "http" {
			tool.URL = prompt(scanner, "Server URL", "")
			if tool.URL == "" {
				return "", tool, fmt.Errorf("url is required for HTTP MCP")
			}
		} else {
			tool.Command = prompt(scanner, "Command", "")
			if tool.Command == "" {
				return "", tool, fmt.Errorf("command is required")
			}
			argsStr := prompt(scanner, "Args (comma-separated)", "")
			if argsStr != "" {
				tool.Args = splitArgs(argsStr)
			}
		}

	default:
		return "", tool, fmt.Errorf("unknown type %q", toolType)
	}

	return name, tool, nil
}

func writeNewTool(name string, tool config.ToolConfig, cfg *config.Config) (string, error) {
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}

	// Case 1: tools_dir is configured — write as standalone file
	if cfg != nil && cfg.ToolsDir != "" {
		dir := cfg.ToolsDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(cfgPath), dir)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating tools directory: %w", err)
		}
		outPath := filepath.Join(dir, name+".yaml")
		return outPath, writeToolFile(outPath, name, tool)
	}

	// Case 2: config is in .factorly/ — write as loose file
	if filepath.Base(filepath.Dir(cfgPath)) == ".factorly" {
		dir := filepath.Dir(cfgPath)
		outPath := filepath.Join(dir, name+".yaml")
		return outPath, writeToolFile(outPath, name, tool)
	}

	// Case 3: append to primary config file
	return cfgPath, appendToConfig(cfgPath, name, tool)
}

func writeToolFile(path, name string, tool config.ToolConfig) error {
	data, err := yaml.Marshal(map[string]config.ToolConfig{name: tool})
	if err != nil {
		return fmt.Errorf("marshaling tool: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func appendToConfig(cfgPath, name string, tool config.ToolConfig) error {
	// Read existing config
	var cfgMap map[string]any

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading config: %w", err)
		}
		cfgMap = map[string]any{"tools": map[string]any{}}
	} else {
		if err := yaml.Unmarshal(data, &cfgMap); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}

	// Get or create tools map
	toolsRaw, ok := cfgMap["tools"]
	if !ok || toolsRaw == nil {
		cfgMap["tools"] = map[string]any{}
		toolsRaw = cfgMap["tools"]
	}

	toolsMap, ok := toolsRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("config 'tools' field is not a map")
	}

	// Marshal the tool to a generic map to insert cleanly
	toolBytes, err := yaml.Marshal(tool)
	if err != nil {
		return fmt.Errorf("marshaling tool: %w", err)
	}
	var toolMap any
	if err := yaml.Unmarshal(toolBytes, &toolMap); err != nil {
		return fmt.Errorf("parsing marshaled tool: %w", err)
	}

	toolsMap[name] = toolMap

	outData, err := yaml.Marshal(cfgMap)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	return os.WriteFile(cfgPath, outData, 0o644)
}

func runToolHealthCheck(name string, tool config.ToolConfig) healthResult {
	switch tool.Type {
	case "cli":
		return checkCLI(name, tool)
	case "rest":
		return checkREST(name, tool, nil)
	case "mcp":
		return checkMCP(name, tool, nil)
	default:
		return healthResult{Name: name, OK: false, Message: "unknown type"}
	}
}

func splitArgs(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func init() {
	addCmd.Flags().StringVar(&addFlags.name, "name", "", "tool name")
	addCmd.Flags().StringVar(&addFlags.toolType, "type", "", "tool type (cli/rest/mcp)")
	addCmd.Flags().StringVar(&addFlags.description, "description", "", "tool description")
	addCmd.Flags().StringVar(&addFlags.command, "command", "", "command to run (cli/mcp)")
	addCmd.Flags().StringVar(&addFlags.args, "args", "", "comma-separated arguments (cli/mcp)")
	addCmd.Flags().StringVar(&addFlags.stdin, "stdin", "", "stdin template (cli)")
	addCmd.Flags().StringVar(&addFlags.baseURL, "base-url", "", "base URL (rest)")
	addCmd.Flags().StringVar(&addFlags.method, "method", "", "HTTP method (rest)")
	addCmd.Flags().StringVar(&addFlags.path, "path", "", "URL path (rest)")
	addCmd.Flags().StringVar(&addFlags.authType, "auth-type", "", "auth type (rest)")
	addCmd.Flags().StringVar(&addFlags.authToken, "auth-token", "", "auth token or vault ref (rest)")
	addCmd.Flags().StringVar(&addFlags.url, "url", "", "server URL (mcp http)")
}
