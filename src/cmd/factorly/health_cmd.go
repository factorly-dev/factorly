package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/oauth"
	"github.com/factorly-dev/factorly-cli/internal/provider"
	"github.com/factorly-dev/factorly-cli/internal/vault"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
)

type healthResult struct {
	Name    string
	Type    string // "cli", "rest", "mcp"
	OK      bool
	Message string
	Details []string
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check that all configured tools are reachable",
	RunE:  runHealth,
}

func runHealth(cmd *cobra.Command, args []string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	// Open vault if needed (for OAuth token checks and vault-ref'd URLs)
	var resolver *vault.Resolver
	resolver, _ = initResolver(cfg)

	var results []healthResult

	// Collect and sort tool names for deterministic output
	var cliNames, restNames, mcpNames []string
	for name, toolCfg := range cfg.Tools {
		switch toolCfg.Type {
		case "cli":
			cliNames = append(cliNames, name)
		case "rest":
			restNames = append(restNames, name)
		case "mcp":
			mcpNames = append(mcpNames, name)
		}
	}
	sort.Strings(cliNames)
	sort.Strings(restNames)
	sort.Strings(mcpNames)

	for _, name := range cliNames {
		results = append(results, checkCLI(name, cfg.Tools[name]))
	}
	for _, name := range restNames {
		results = append(results, checkREST(name, cfg.Tools[name], resolver))
	}
	for _, name := range mcpNames {
		results = append(results, checkMCP(name, cfg.Tools[name], resolver))
	}

	printHealthResults(results)

	issues := 0
	for _, r := range results {
		if !r.OK {
			issues++
		}
	}
	if issues > 0 {
		os.Exit(1)
	}
	return nil
}

func checkCLI(name string, cfg config.ToolConfig) healthResult {
	path, err := exec.LookPath(cfg.Command)
	if err != nil {
		return healthResult{
			Name: name, Type: "cli", OK: false,
			Message: fmt.Sprintf("command %q not found in PATH", cfg.Command),
		}
	}
	return healthResult{
		Name: name, Type: "cli", OK: true,
		Message: fmt.Sprintf("%s found at %s", cfg.Command, path),
	}
}

func checkREST(name string, cfg config.ToolConfig, resolver *vault.Resolver) healthResult {
	baseURL := cfg.BaseURL
	if resolver != nil {
		baseURL = resolveVaultRef(resolver, baseURL)
	}

	r := healthResult{Name: name, Type: "rest"}

	// Check URL reachability
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Head(baseURL)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		r.OK = false
		r.Message = fmt.Sprintf("%s unreachable: %v", baseURL, cleanHTTPError(err))
	} else {
		resp.Body.Close()
		r.OK = true
		r.Message = fmt.Sprintf("%s reachable (%d, %s)", baseURL, resp.StatusCode, elapsed)
	}

	// Check auth
	if cfg.Auth != nil {
		switch cfg.Auth.Type {
		case "oauth":
			tokenKey := config.OAuthTokenKey(cfg.Auth)
			if tokenKey != "" {
				r.Details = append(r.Details, checkOAuthToken(tokenKey, resolver))
			}
		case "bearer":
			token := cfg.Auth.Token
			if resolver != nil {
				token = resolveVaultRef(resolver, token)
			}
			if token == "" || vault.HasVaultRefs(token) {
				r.Details = append(r.Details, "bearer: ✗ token not set")
			} else {
				r.Details = append(r.Details, "bearer: ✓ token configured")
			}
		case "header":
			value := cfg.Auth.Value
			if resolver != nil {
				value = resolveVaultRef(resolver, value)
			}
			if value == "" || vault.HasVaultRefs(value) {
				r.Details = append(r.Details, fmt.Sprintf("header %s: ✗ value not set", cfg.Auth.Header))
			} else {
				r.Details = append(r.Details, fmt.Sprintf("header %s: ✓ configured", cfg.Auth.Header))
			}
		}
	}

	return r
}

func checkOAuthToken(tokenKey string, resolver *vault.Resolver) string {
	if resolver == nil {
		return fmt.Sprintf("oauth: %s ✗ vault not available", tokenKey)
	}
	backend := resolver.Backend("vault")
	if backend == nil {
		return fmt.Sprintf("oauth: %s ✗ vault not available", tokenKey)
	}

	raw, err := backend.Get(tokenKey)
	if err != nil {
		return fmt.Sprintf("oauth: %s ✗ not authenticated (run: factorly auth login)", tokenKey)
	}

	var bundle oauth.TokenBundle
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		return fmt.Sprintf("oauth: %s ✗ invalid token data", tokenKey)
	}

	if bundle.IsExpired(0) {
		return fmt.Sprintf("oauth: %s ✗ expired (run: factorly auth login)", tokenKey)
	}

	if !bundle.Expiry.IsZero() {
		remaining := time.Until(bundle.Expiry).Round(time.Minute)
		return fmt.Sprintf("oauth: %s ✓ valid (expires in %s)", tokenKey, remaining)
	}
	return fmt.Sprintf("oauth: %s ✓ valid (no expiry)", tokenKey)
}

func checkMCP(name string, cfg config.ToolConfig, resolver *vault.Resolver) healthResult {
	r := healthResult{Name: name, Type: "mcp"}

	def := provider.MCPServerDef{
		Command: cfg.Command,
		Args:    cfg.Args,
		Env:     cfg.Env,
		URL:     cfg.URL,
	}
	if resolver != nil {
		def.URL = resolveVaultRef(resolver, def.URL)
	}

	// Try to connect (temporary — just for health check)
	c, err := provider.ConnectMCP(def)
	if err != nil {
		r.OK = false
		r.Message = fmt.Sprintf("failed to connect: %v", cleanMCPError(err))
		return r
	}
	defer c.Close()

	// Ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := c.Ping(ctx); err != nil {
		r.OK = false
		r.Message = fmt.Sprintf("connected but ping failed: %v", err)
		return r
	}
	pingTime := time.Since(start).Round(time.Millisecond)

	// List tools for count
	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	toolCount := 0
	if err == nil && tools != nil {
		toolCount = len(tools.Tools)
	}

	r.OK = true
	r.Message = fmt.Sprintf("connected, %d tools (ping: %s)", toolCount, pingTime)
	return r
}

func printHealthResults(results []healthResult) {
	currentType := ""
	healthy := 0
	issues := 0

	for _, r := range results {
		if r.Type != currentType {
			if currentType != "" {
				fmt.Println()
			}
			currentType = r.Type
			switch r.Type {
			case "cli":
				fmt.Println("  CLI Tools")
			case "rest":
				fmt.Println("  REST Tools")
			case "mcp":
				fmt.Println("  MCP Servers")
			}
		}

		icon := "✓"
		if !r.OK {
			icon = "✗"
			issues++
		} else {
			healthy++
		}

		fmt.Printf("  %s %-20s %s\n", icon, r.Name, r.Message)
		for _, detail := range r.Details {
			fmt.Printf("    ↳ %s\n", detail)
		}
	}

	fmt.Println()
	if issues == 0 {
		fmt.Printf("  %d healthy, 0 issues\n", healthy)
	} else {
		fmt.Printf("  %d healthy, %d issues\n", healthy, issues)
	}
}

// cleanHTTPError removes verbose URL/IP details from http errors for cleaner output.
func cleanHTTPError(err error) string {
	msg := err.Error()
	// Common patterns to simplify
	if len(msg) > 80 {
		// Trim to the core message
		if idx := len(msg) - 1; idx > 80 {
			msg = msg[:80] + "..."
		}
	}
	return msg
}

// cleanMCPError simplifies MCP connection errors for display.
func cleanMCPError(err error) string {
	return err.Error()
}

func init() {
	// healthCmd is registered in main.go init()
}
