// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
		// Race MCP elicitation and browser confirm — first response wins.
		// This ensures prompts always show in the browser UI AND the agent
		// can respond via elicitation, whichever is faster.
		mcpFn := shadow.ConfirmFunc(mcpElicitConfirm)
		confirmFn = func(ctx context.Context, toolName string, params map[string]string) bool {
			// Timeout: deny if no response within 1 minute
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			// Always send to browser
			browserCtx, browserCancel := context.WithCancel(ctx)
			defer browserCancel()

			type confirmResult struct {
				approved bool
				source   string
			}
			result := make(chan confirmResult, 2)

			channels := []string{"browser"}
			go func() {
				result <- confirmResult{confirmBroker.Request(browserCtx, toolName, params), "browser"}
			}()

			// If there's an MCP session, also try elicitation
			if mcpserver.ClientSessionFromContext(ctx) != nil {
				channels = append(channels, "elicitation")
				go func() {
					result <- confirmResult{mcpFn(ctx, toolName, params), "elicitation"}
				}()
			}

			vlog("[confirm] %s: waiting for approval (%s)", toolName, strings.Join(channels, ", "))

			// First response wins
			select {
			case r := <-result:
				action := "approved"
				if !r.approved {
					action = "denied"
				}
				vlog("[confirm] %s: %s via %s", toolName, action, r.source)
				return r.approved
			case <-ctx.Done():
				vlog("[confirm] %s: denied (timed out after 60s)", toolName)
				return false
			}
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
		Resolver:      getCachedResolver(),
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
	var uiWarnings []string

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
			uiWarnings = append(uiWarnings, "MCP endpoint has no authentication. Use --mcp-token or FACTORLY_HTTP_TOKEN.")
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

	if uiHost != "127.0.0.1" && uiHost != "localhost" {
		uiWarnings = append(uiWarnings, "The UI is bound to "+uiHost+" — intended for local development only. Do not expose to the internet.")
	}
	if len(uiWarnings) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "⚠ WARNING: %s\n", strings.Join(uiWarnings, " "))
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Factorly UI running at %s\n", url)
	if uiMCP {
		fmt.Fprintf(cmd.ErrOrStderr(), "MCP endpoint at http://localhost:%d/mcp (token: %s)\n", uiPort, uiMCPToken)
	}

	// Start listener first, then open browser to avoid race condition
	// where the browser connects before the server is ready
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if !uiNoLaunch {
		go openBrowser(url)
	}

	return http.Serve(ln, handler)
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
