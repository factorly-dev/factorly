package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/registry"
	factorlyServer "github.com/factorly-dev/factorly-cli/internal/server"
	"github.com/spf13/cobra"
)

var (
	wrapURL       string
	wrapRateLimit string
	wrapMaxOutput int
	wrapCompress  string
	wrapServeHTTP string
	wrapHTTPToken string
)

var wrapCmd = &cobra.Command{
	Use:   "wrap [flags] -- <command> [args...]",
	Short: "Proxy any MCP server with zero config",
	Long: `Wrap an existing MCP server to instantly add audit logging,
loop detection, output compression, and rate limiting.

Stdio server:
  factorly wrap -- npx @modelcontextprotocol/server-github

HTTP server:
  factorly wrap --url http://localhost:3001/mcp

With options:
  factorly wrap --rate-limit 100/hour --max-output 50000 -- npx server-github`,
	RunE: runWrap,
}

func runWrap(cmd *cobra.Command, args []string) error {
	// Determine target: HTTP URL or stdio command
	var serverName string
	var toolCfg config.ToolConfig

	if wrapURL != "" {
		serverName = deriveNameFromURL(wrapURL)
		toolCfg = config.ToolConfig{
			Type: "mcp",
			URL:  wrapURL,
		}
	} else if len(args) > 0 {
		serverName = deriveNameFromCommand(args)
		toolCfg = config.ToolConfig{
			Type:    "mcp",
			Command: args[0],
			Args:    args[1:],
		}
	} else {
		return fmt.Errorf("usage: factorly wrap --url <http-url> or factorly wrap -- <command> [args...]")
	}

	// Apply output processing defaults
	switch wrapCompress {
	case "none":
		// no compression
	case "json":
		toolCfg.Compress = []string{"json"}
	case "logs":
		toolCfg.Compress = []string{"logs"}
	default:
		toolCfg.Compress = []string{"all"}
	}
	toolCfg.MaxOutput = wrapMaxOutput

	// Apply shadow governance
	if wrapRateLimit != "" {
		toolCfg.Shadow = &config.ShadowConfig{
			RateLimit: wrapRateLimit,
		}
	}

	// Build synthetic config
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			serverName: toolCfg,
		},
	}

	// Build empty registry (MCP tools discovered during bootstrap)
	reg := registry.New()
	reg.Register(&registry.Tool{
		Name:        serverName,
		Type:        "mcp",
		ProviderKey: "mcp",
		MaxOutput:   toolCfg.MaxOutput,
		Compress:    toolCfg.Compress,
	})

	// Enable no-namespace mode so tools keep their original names
	wrapMode = true

	vlog("wrapping MCP server: %s", serverName)
	if len(toolCfg.Compress) > 0 {
		vlog("  compression: %s", strings.Join(toolCfg.Compress, ", "))
	}
	if toolCfg.MaxOutput > 0 {
		vlog("  max output: %d bytes", toolCfg.MaxOutput)
	}

	p, err := bootstrapProviders(cfg, reg, mcpElicitConfirm)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	defer p.Teardown()

	if verbose {
		factorlyServer.Verbose = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "[factorly] "+format+"\n", args...)
		}
	}

	s := factorlyServer.New(reg, p)

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Resolve HTTP token for serve mode
	if wrapServeHTTP != "" {
		if wrapHTTPToken == "" {
			wrapHTTPToken = os.Getenv("FACTORLY_HTTP_TOKEN")
		}
		if wrapHTTPToken == "" {
			fmt.Fprintln(os.Stderr, "WARNING: HTTP server has no authentication. Use --http-token or FACTORLY_HTTP_TOKEN for production.")
		}
		httpToken = wrapHTTPToken
		return serveHTTP(ctx, s, wrapServeHTTP)
	}
	return serveStdio(ctx, s)
}

// deriveNameFromCommand extracts a human-friendly name from a command and its args.
// e.g., "npx @modelcontextprotocol/server-github" → "server-github"
// e.g., "uvx mcp-server-fetch" → "mcp-server-fetch"
// e.g., "python -m my_server" → "my_server"
func deriveNameFromCommand(args []string) string {
	// Look at the last meaningful argument
	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		// Skip flags
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Handle scoped npm packages: @org/name → name
		if strings.Contains(arg, "/") {
			parts := strings.Split(arg, "/")
			arg = parts[len(parts)-1]
		}
		// Clean up common prefixes/suffixes
		name := strings.TrimPrefix(arg, "mcp-server-")
		name = strings.TrimPrefix(name, "server-")
		name = strings.TrimSuffix(name, ".py")
		name = strings.TrimSuffix(name, ".js")
		if name != "" && name != "npx" && name != "uvx" && name != "node" && name != "python" && name != "python3" {
			return sanitizeName(name)
		}
	}
	return "wrapped"
}

// deriveNameFromURL extracts a name from an HTTP URL.
// e.g., "http://localhost:3001/mcp" → "localhost"
func deriveNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "wrapped"
	}
	host := u.Hostname()
	if host == "" {
		return "wrapped"
	}
	// Use path if it's meaningful
	if p := path.Base(u.Path); p != "" && p != "/" && p != "." {
		return sanitizeName(p)
	}
	return sanitizeName(host)
}

// sanitizeName cleans a string for use as a tool config key.
func sanitizeName(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ToLower(s)
	return s
}

func init() {
	wrapCmd.Flags().StringVar(&wrapURL, "url", "", "HTTP MCP server URL to wrap")
	wrapCmd.Flags().StringVar(&wrapRateLimit, "rate-limit", "", "rate limit (e.g. 100/hour)")
	wrapCmd.Flags().IntVar(&wrapMaxOutput, "max-output", 50000, "max output bytes per tool call")
	wrapCmd.Flags().StringVar(&wrapCompress, "compress", "all", "compression mode: all, json, logs, none")
	wrapCmd.Flags().StringVar(&wrapServeHTTP, "http", "", "start HTTP transport on this address (e.g. :3000) instead of stdio")
	wrapCmd.Flags().StringVar(&wrapHTTPToken, "http-token", "", "require Bearer token authentication for HTTP transport")
}
