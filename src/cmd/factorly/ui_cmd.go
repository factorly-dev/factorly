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

	"github.com/factorly-dev/factorly/internal/provider"
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
	// Set up ask broker BEFORE bootstrapProviders. The
	// factorly.ask builtin handler reads `activeAskBroker` at call
	// time, so as long as we assign it before bootstrap registers
	// the late-bound handler closure, the wiring is correct. The
	// same broker also gets injected into the UI server below so
	// the SSE / POST routes route to the same queue.
	askBroker := ui.NewAskBroker()
	SetActiveAskBroker(askBroker)
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

	// Inherit each tier the CLI startup already unlocked. The chain
	// returned from getCachedVault is either:
	//   - workspace→project→global, when --workspace is active and a
	//     workspace vault file exists (the outermost FallbackBackend's
	//     Primary is the workspace vault),
	//   - project→global, when no workspace is active,
	//   - a single LocalBackend, when only one tier exists.
	//
	// SecondaryOpen closures fire lazily on first Get-miss in normal
	// operation, but we want each tier surfaced to the UI as already-
	// unlocked at startup so the user doesn't see "locked" badges
	// after typing the password on the CLI. Force the lazy opens now.
	workspaceVault, projectVault, globalVault := extractVaultTiers(vaultBackend, workspaceName != "")

	// Set up activity broadcaster for live feed
	activity := ui.NewActivityBroadcaster()
	p.SetOnCall(func(e proxy.CallEvent) {
		activity.Broadcast(e)
	})
	p.SetOnWorkflowStep(func(workflow string, ev provider.StepEvent) {
		activity.BroadcastStep(workflow, ev)
	})

	mgr := getVaultManager()
	// Pre-seed the Manager with tiers the CLI startup already opened
	// (extractVaultTiers walked the chain and surfaced the per-tier
	// LocalBackends). Without this, the UI's vault-page would re-open
	// project / global on first access — and re-prompt for passwords
	// the user already typed at CLI startup.
	if projectVault != nil {
		mgr.Put("project", projectVault)
	}
	if globalVault != nil {
		mgr.Put("global", globalVault)
	}

	srv, err := ui.New(ui.Options{
		Config:          cfg,
		CfgPath:         configPath,
		ToolsDir:        resolveToolsDir(configPath, cfg.ToolsDir),
		Registry:        reg,
		Proxy:           p,
		Vault:           vaultBackend,
		Resolver:        getCachedResolver(),
		Activity:        activity,
		ConfirmBroker:   confirmBroker,
		AskBroker:       askBroker,
		ActiveWorkspace: workspaceName,
		WorkspaceVault:  workspaceVault,
		VaultManager:    mgr,
		StoreOpener:     openStore,
	})
	if err != nil {
		return err
	}

	// Make OAuth token refreshes follow the active UI workspace. The
	// proxy was built with a token store pinned to the startup vault;
	// swap its backend resolver so reads/writes always hit
	// vaults/<active>.enc when a workspace is active.
	if restProv, ok := p.Provider("rest").(*provider.RESTProvider); ok && restProv.TokenStore() != nil {
		if vts, ok := restProv.TokenStore().(*vaultTokenStore); ok {
			vts.SetGetBackend(srv.ActiveVault)
		}
	}

	// Generate per-run nonce token
	token := generateNonce()
	var uiWarnings []string

	// Optionally mount MCP endpoint on the UI server's mux
	if uiMCP {
		mcpSrv := factorlyServer.New(reg, p, cfg, configPath)
		mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)
		// Re-register MCP resources whenever the UI reloads config so the
		// list_changed notification fires for connected clients.
		srv.OnReload = func() {
			factorlyServer.RefreshResources(mcpSrv, srv.Config(), srv.CfgPath())
		}

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

// extractVaultTiers walks the chain returned by getCachedVault and
// pulls out each tier's backend so the UI can surface them as
// individually-unlocked (rather than re-prompting). Eagerly fires
// any lazy SecondaryOpen closures so a one-time password the user
// typed on stdin gets propagated to project + global tiers via the
// shared-password candidate fallback we built earlier.
//
// hasWorkspace tells us whether to expect a workspace tier at the
// top of the chain (true when --workspace was active).
//
// Returns (workspace, project, global) — any tier that wasn't
// present or didn't unlock is nil.
func extractVaultTiers(root vault.Backend, hasWorkspace bool) (ws, proj, glob vault.Backend) {
	if root == nil {
		return nil, nil, nil
	}

	// Detect the file each LocalBackend points at so we can sort
	// tiers by what they are, not by chain position. (More robust:
	// chain shape can vary with --vault-path, --global, etc.)
	//
	// The path-shape classification is safe to use here because the
	// path came from a LocalBackend we just opened — provenance is
	// trusted. Step 3 of the concession fix-up forbade tierForPath
	// from CLI password resolution paths, where the same ambiguity
	// would silently mis-route env-var lookups.
	classify := func(b vault.Backend) (kind string) {
		lb, ok := b.(*vault.LocalBackend)
		if !ok {
			return ""
		}
		t := tierForPath(lb.Path())
		switch {
		case strings.HasPrefix(t.Name, "workspace:"):
			return "workspace"
		case t.Name == "project":
			return "project"
		default:
			return "global"
		}
	}

	assign := func(b vault.Backend) {
		switch classify(b) {
		case "workspace":
			if ws == nil {
				ws = b
			}
		case "project":
			if proj == nil {
				proj = b
			}
		case "global":
			if glob == nil {
				glob = b
			}
		}
	}

	// Walk the chain. Each FallbackBackend has a Primary (an opened
	// backend) and either Secondary (already open) or SecondaryOpen
	// (lazy). Fire lazy opens so the next layer is reachable.
	cur := root
	for cur != nil {
		fb, ok := cur.(*vault.FallbackBackend)
		if !ok {
			assign(cur)
			break
		}
		if fb.Primary != nil {
			assign(fb.Primary)
		}
		// Fire lazy open if needed. EnsureSecondary returns (nil, err)
		// on failure — log the reason so a misconfigured tier doesn't
		// silently look "absent" in the UI's per-tier status, then move
		// on (the tier simply stays unassigned, matching prior behavior).
		next, openErr := fb.EnsureSecondary()
		if openErr != nil {
			vlog("warming secondary tier failed: %v", openErr)
		}
		if next == nil {
			break
		}
		cur = next
	}

	// If --workspace wasn't active, the chain's "workspace" slot
	// should be unused.
	if !hasWorkspace {
		ws = nil
	}
	return ws, proj, glob
}
