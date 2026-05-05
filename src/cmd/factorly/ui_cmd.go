// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/factorly-dev/factorly/internal/ui"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/spf13/cobra"
)

var uiPort int

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the Factorly web UI",
	Long:  "Starts a localhost web server with a visual interface for configuring tools, running them, and managing workflows.",
	RunE:  runUI,
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 3741, "port for the UI server")
}

func runUI(cmd *cobra.Command, args []string) error {
	cfg, reg, err := loadConfig()
	if err != nil {
		return err
	}

	p, err := bootstrapProviders(cfg, reg)
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

	srv, err := ui.New(ui.Options{
		Config:       cfg,
		CfgPath:      configPath,
		ToolsDir:     cfg.ToolsDir,
		Registry:     reg,
		Proxy:        p,
		Vault:        vaultBackend,
		ProjectVault: projectVault,
		GlobalVault:  globalVault,
	})
	if err != nil {
		return err
	}

	// Generate per-run nonce token
	token := generateNonce()

	// Bind to loopback only
	addr := fmt.Sprintf("127.0.0.1:%d", uiPort)
	url := fmt.Sprintf("http://localhost:%d/?token=%s", uiPort, token)

	fmt.Fprintf(cmd.ErrOrStderr(), "Factorly UI running at %s\n", url)

	// Open browser with token
	go openBrowser(url)

	// Wrap handler with host validation + token check
	handler := hostValidation(tokenValidation(srv.Handler(), token))

	return http.ListenAndServe(addr, handler)
}

// hostValidation rejects requests with unexpected Host headers.
func hostValidation(next http.Handler) http.Handler {
	allowed := map[string]bool{
		"localhost": true,
		"127.0.0.1": true,
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
		// Static assets don't need auth
		if strings.HasPrefix(r.URL.Path, "/static/") {
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
				SameSite: http.SameSiteStrictMode,
			})
			// Redirect to strip token from URL
			clean := r.URL
			q := clean.Query()
			q.Del("token")
			clean.RawQuery = q.Encode()
			http.Redirect(w, r, clean.String(), http.StatusFound)
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
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
