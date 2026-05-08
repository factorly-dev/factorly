// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/factorly-dev/factorly/internal/proxy"
	factorlyServer "github.com/factorly-dev/factorly/internal/server"
	"github.com/factorly-dev/factorly/internal/shadow"
	"github.com/factorly-dev/factorly/internal/ui"
	"github.com/factorly-dev/factorly/internal/vault"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var (
	uiHost     string
	uiPort     int
	uiMCP      bool
	uiMCPToken string
	uiNoLaunch bool
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the Factorly web UI",
	Long:  "Starts a localhost web server with a visual interface for configuring tools, running them, and managing workflows.",
	RunE:  runUI,
}

func init() {
	uiCmd.Flags().StringVar(&uiHost, "host", "127.0.0.1", "address to bind the UI server (use 0.0.0.0 for all interfaces)")
	uiCmd.Flags().IntVar(&uiPort, "port", 3741, "port for the UI server")
	uiCmd.Flags().BoolVar(&uiMCP, "mcp", false, "also serve MCP endpoint at /mcp")
	uiCmd.Flags().StringVar(&uiMCPToken, "mcp-token", "", "bearer token for MCP endpoint (required when --mcp is set)")
	uiCmd.Flags().BoolVar(&uiNoLaunch, "no-launch", false, "don't open the browser automatically")
}

func runUI(cmd *cobra.Command, args []string) error {
	cfg, reg, err := loadConfig()
	if err != nil {
		return err
	}

	// Set up confirm broker for routing shadow confirm prompts to the browser
	confirmBroker := ui.NewConfirmBroker()
	confirmFn := shadow.ConfirmFunc(confirmBroker.Request)
	if uiMCP {
		// When MCP is enabled, try MCP elicitation first (for agent calls),
		// fall back to browser confirm for direct UI calls
		mcpFn := shadow.ConfirmFunc(mcpElicitConfirm)
		confirmFn = func(ctx context.Context, toolName string, params map[string]string) bool {
			// If there's an MCP session, use elicitation
			if mcpserver.ClientSessionFromContext(ctx) != nil {
				return mcpFn(ctx, toolName, params)
			}
			// Otherwise route to browser
			return confirmBroker.Request(ctx, toolName, params)
		}
	}
	p, err := bootstrapProviders(cfg, reg, confirmFn)
	if err != nil {
		return err
	}
	defer p.Teardown()

	// Get vault backend (cached singleton from bootstrapProviders, won't re-prompt)
	var vaultBackend vault.Backend
	vaultBackend, _ = getCachedVault()

	// Detect project vs global vault from the cached backend
	var projectVault, globalVault vault.Backend
	projectPath := filepath.Join(".factorly", "vault.enc")
	globalPath := vault.DefaultVaultPath()
	if fb, ok := vaultBackend.(*vault.FallbackBackend); ok {
		projectVault = fb.Primary
		globalVault = fb.Secondary
	} else if vaultBackend != nil {
		// Single vault — determine which one it is
		if _, err := os.Stat(projectPath); err == nil {
			projectVault = vaultBackend
		} else if _, err := os.Stat(globalPath); err == nil {
			globalVault = vaultBackend
		} else {
			projectVault = vaultBackend
		}
	}

	// Set up activity broadcaster for live feed
	activity := ui.NewActivityBroadcaster()
	p.SetOnCall(func(e proxy.CallEvent) {
		activity.Broadcast(e)
	})

	srv, err := ui.New(ui.Options{
		Config:        cfg,
		CfgPath:       configPath,
		ToolsDir:      resolveToolsDir(configPath, cfg.ToolsDir),
		Registry:      reg,
		Proxy:         p,
		Vault:         vaultBackend,
		ProjectVault:  projectVault,
		GlobalVault:   globalVault,
		Activity:      activity,
		ConfirmBroker: confirmBroker,
	})
	if err != nil {
		return err
	}

	// Generate per-run nonce token
	token := generateNonce()

	// Optionally mount MCP endpoint on the UI server's mux
	if uiMCP {
		mcpSrv := factorlyServer.New(reg, p)
		mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)

		// Resolve MCP token: flag → env var → vault refs
		if uiMCPToken == "" {
			uiMCPToken = os.Getenv("FACTORLY_HTTP_TOKEN")
		}
		if uiMCPToken != "" && vault.HasVaultRefs(uiMCPToken) {
			if backend, vErr := getCachedLocalVault(); vErr == nil {
				resolver := vault.NewResolver()
				resolver.Register("vault", backend)
				uiMCPToken = resolveVaultRef(resolver, uiMCPToken)
			}
		}
		if uiMCPToken == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: MCP endpoint has no authentication. Use --mcp-token or FACTORLY_HTTP_TOKEN.")
		}

		// Wrap MCP handler with bearer token auth
		var mcpHandler http.Handler = mcpHTTP
		if uiMCPToken != "" {
			mcpHandler = tokenAuthMiddleware(mcpHTTP, uiMCPToken)
		}
		srv.MountMCP(mcpHandler)
	}

	// Wrap with host validation + token check
	handler := hostValidation(tokenValidation(srv.Handler(), token))

	addr := fmt.Sprintf("%s:%d", uiHost, uiPort)
	url := fmt.Sprintf("http://localhost:%d/?token=%s", uiPort, token)

	fmt.Fprintf(cmd.ErrOrStderr(), "Factorly UI running at %s\n", url)
	if uiMCP {
		fmt.Fprintf(cmd.ErrOrStderr(), "MCP endpoint at http://localhost:%d/mcp (token: %s)\n", uiPort, uiMCPToken)
	}

	if !uiNoLaunch {
		go openBrowser(url)
	}

	return http.ListenAndServe(addr, handler)
}

// hostValidation rejects requests with unexpected Host headers.
func hostValidation(next http.Handler) http.Handler {
	allowed := map[string]bool{
		"localhost":            true,
		"127.0.0.1":            true,
		"host.docker.internal": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Strip port
		if i := strings.LastIndex(host, ":"); i != -1 {
			host = host[:i]
		}
		if !allowed[host] {
			http.Error(w, "forbidden: invalid host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenValidation requires a valid token on the first request (sets a cookie),
// then validates the cookie on subsequent requests. Static assets are exempt.
func tokenValidation(next http.Handler, token string) http.Handler {
	const cookieName = "factorly_session"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets and MCP endpoint don't need cookie auth
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/mcp") {
			next.ServeHTTP(w, r)
			return
		}

		// Check cookie first
		if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value == token {
			next.ServeHTTP(w, r)
			return
		}

		// Check token query param (initial page load from browser open)
		if r.URL.Query().Get("token") == token {
			// Set session cookie and redirect to clean URL
			http.SetCookie(w, &http.Cookie{
				Name:     cookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteStrictMode,
			})
			// Redirect to strip token from URL (local path only, no open redirect)
			redirectPath := r.URL.EscapedPath()
			if redirectPath == "" {
				redirectPath = "/"
			}
			q := r.URL.Query()
			q.Del("token")
			if encoded := q.Encode(); encoded != "" {
				redirectPath += "?" + encoded
			}
			http.Redirect(w, r, redirectPath, http.StatusFound)
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func resolveToolsDir(cfgPath, toolsDir string) string {
	configDir := filepath.Dir(cfgPath)
	if toolsDir != "" {
		if filepath.IsAbs(toolsDir) {
			return toolsDir
		}
		return filepath.Join(configDir, toolsDir)
	}
	// Auto-discover .factorly/tools/ convention
	candidate := filepath.Join(configDir, "tools")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
