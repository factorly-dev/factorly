package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/naming"
	"github.com/factorly-dev/factorly-cli/internal/registry"
	factorlyServer "github.com/factorly-dev/factorly-cli/internal/server"
	"github.com/spf13/cobra"
)

var (
	wrapURL          string
	wrapRateLimit    string
	wrapMaxOutput    int
	wrapCompress     string
	wrapServeHTTP    string
	wrapHTTPToken    string
	wrapEnvIsolation string
	wrapTimeout      string
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
	if err := checkCommandAllowed("wrap"); err != nil {
		return err
	}
	// Determine target: HTTP URL or stdio command
	var serverName string
	var toolCfg config.ToolConfig

	if wrapURL != "" {
		serverName = naming.DeriveNameFromURL(wrapURL)
		toolCfg = config.ToolConfig{
			Type: "mcp",
			URL:  wrapURL,
		}
	} else if len(args) > 0 {
		serverName = naming.DeriveNameFromCommand(args)
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

	// Apply timeout
	if wrapTimeout != "" {
		toolCfg.Timeout = wrapTimeout
	}

	// Apply environment isolation
	if wrapEnvIsolation == "strict" {
		toolCfg.EnvIsolation = "strict"
	}

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

func init() {
	wrapCmd.Flags().StringVar(&wrapURL, "url", "", "HTTP MCP server URL to wrap")
	wrapCmd.Flags().StringVar(&wrapRateLimit, "rate-limit", "", "rate limit (e.g. 100/hour)")
	wrapCmd.Flags().IntVar(&wrapMaxOutput, "max-output", 50000, "max output bytes per tool call")
	wrapCmd.Flags().StringVar(&wrapCompress, "compress", "all", "compression mode: all, json, logs, none")
	wrapCmd.Flags().StringVar(&wrapServeHTTP, "http", "", "start HTTP transport on this address (e.g. :3000) instead of stdio")
	wrapCmd.Flags().StringVar(&wrapHTTPToken, "http-token", "", "require Bearer token authentication for HTTP transport")
	wrapCmd.Flags().StringVar(&wrapEnvIsolation, "env-isolation", "", "environment isolation: strict (minimal env) or standard (default, inherit parent)")
	wrapCmd.Flags().StringVar(&wrapTimeout, "timeout", "", "tool call timeout (e.g. 30s, 5m; default: 30s)")
}
