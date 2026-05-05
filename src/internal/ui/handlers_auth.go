// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/oauth"
)

type authProviderView struct {
	Name        string
	Status      string // "valid", "expired_refreshable", "expired", "missing"
	StatusLabel string
	StatusColor string // "green", "amber", "red"
	Expiry      string
	Scopes      []string
	Tools       []string
	HasToken    bool
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	providers := s.buildAuthProviderViews()

	s.render(w, "auth.html", map[string]any{
		"Title":     "Auth",
		"Nav":       "auth",
		"Providers": providers,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	tokenKey := providerName + "_oauth"

	// Try to find the actual token key from config
	if pCfg, ok := s.cfg.OAuthProviders[providerName]; ok {
		_ = pCfg // default key is fine
	}
	for _, tc := range s.cfg.Tools {
		if tc.Auth != nil && tc.Auth.Type == "oauth" {
			key := config.OAuthTokenKey(tc.Auth)
			if tc.Auth.Provider == providerName && key != "" {
				tokenKey = key
				break
			}
		}
	}

	// Delete from whichever vault has it
	if s.vault != nil {
		_ = s.vault.Delete(tokenKey)
	}

	// Return updated provider list
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	providers := s.buildAuthProviderViews()
	s.renderAuthList(w, providers)
}

func (s *Server) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")

	// Find the provider config and token key
	pCfg, ok := s.cfg.OAuthProviders[providerName]
	if !ok {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}

	tokenKey := providerName + "_oauth"
	// Check tools for custom token key
	for _, tc := range s.cfg.Tools {
		if tc.Auth != nil && tc.Auth.Type == "oauth" && tc.Auth.Provider == providerName {
			if key := config.OAuthTokenKey(tc.Auth); key != "" {
				tokenKey = key
			}
			break
		}
	}

	if s.vault == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	// Read current token
	raw, err := s.vault.Get(tokenKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading token: %v", err), http.StatusInternalServerError)
		return
	}

	var bundle oauth.TokenBundle
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		http.Error(w, "invalid token data", http.StatusInternalServerError)
		return
	}

	if bundle.RefreshToken == "" {
		http.Error(w, "no refresh token available", http.StatusBadRequest)
		return
	}

	// Resolve vault refs in provider config
	clientID := s.resolveRef(pCfg.ClientID)
	clientSecret := s.resolveRef(pCfg.ClientSecret)

	providerCfg := oauth.ProviderConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     pCfg.TokenURL,
	}

	// Refresh
	newBundle, err := oauth.RefreshAccessToken(r.Context(), providerCfg, bundle.RefreshToken)
	if err != nil {
		http.Error(w, fmt.Sprintf("refresh failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Store refreshed token
	data, _ := json.Marshal(newBundle)
	if err := s.vault.Set(tokenKey, string(data)); err != nil {
		http.Error(w, fmt.Sprintf("storing token: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated list
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	providers := s.buildAuthProviderViews()
	s.renderAuthList(w, providers)
}

// resolveRef resolves a single {{vault:KEY}} reference using the vault backend.
func (s *Server) resolveRef(val string) string {
	if s.vault == nil || val == "" {
		return val
	}
	// Simple single-ref resolution: {{vault:KEY}}
	if len(val) > 10 && val[:8] == "{{vault:" && val[len(val)-2:] == "}}" {
		key := val[8 : len(val)-2]
		if resolved, err := s.vault.Get(key); err == nil {
			return resolved
		}
	}
	return val
}

func (s *Server) buildAuthProviderViews() []authProviderView {
	var views []authProviderView

	// Collect from oauth_providers config
	for name, pCfg := range s.cfg.OAuthProviders {
		tokenKey := name + "_oauth"
		view := authProviderView{
			Name:   name,
			Scopes: pCfg.Scopes,
			Tools:  s.findToolsUsingProvider(name),
		}
		s.populateTokenStatus(&view, tokenKey)
		views = append(views, view)
	}

	// Also check tools with inline OAuth that aren't in oauth_providers
	seen := make(map[string]bool)
	for _, v := range views {
		seen[v.Name] = true
	}
	for _, tc := range s.cfg.Tools {
		if tc.Auth != nil && tc.Auth.Type == "oauth" && tc.Auth.Provider != "" && !seen[tc.Auth.Provider] {
			tokenKey := config.OAuthTokenKey(tc.Auth)
			view := authProviderView{
				Name:  tc.Auth.Provider,
				Tools: s.findToolsUsingProvider(tc.Auth.Provider),
			}
			s.populateTokenStatus(&view, tokenKey)
			views = append(views, view)
			seen[tc.Auth.Provider] = true
		}
	}

	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

func (s *Server) populateTokenStatus(view *authProviderView, tokenKey string) {
	if s.vault == nil || tokenKey == "" {
		view.Status = "missing"
		view.StatusLabel = "not authenticated"
		view.StatusColor = "red"
		return
	}

	raw, err := s.vault.Get(tokenKey)
	if err != nil || raw == "" {
		view.Status = "missing"
		view.StatusLabel = "not authenticated"
		view.StatusColor = "red"
		return
	}

	var bundle oauth.TokenBundle
	if json.Unmarshal([]byte(raw), &bundle) != nil {
		view.Status = "missing"
		view.StatusLabel = "invalid token"
		view.StatusColor = "red"
		return
	}

	view.HasToken = true

	if bundle.Expiry.IsZero() {
		view.Status = "valid"
		view.StatusLabel = "valid (no expiry)"
		view.StatusColor = "green"
		return
	}

	if bundle.IsExpired(0) {
		if bundle.RefreshToken != "" {
			view.Status = "expired_refreshable"
			view.StatusLabel = "expired (will auto-refresh)"
			view.StatusColor = "amber"
			view.Expiry = relativeTime(bundle.Expiry)
		} else {
			view.Status = "expired"
			view.StatusLabel = "expired"
			view.StatusColor = "red"
			view.Expiry = relativeTime(bundle.Expiry)
		}
		return
	}

	view.Status = "valid"
	remaining := time.Until(bundle.Expiry).Truncate(time.Minute)
	view.StatusLabel = fmt.Sprintf("valid (expires in %s)", remaining)
	view.StatusColor = "green"
	view.Expiry = fmt.Sprintf("expires in %s", remaining)
}

func (s *Server) findToolsUsingProvider(providerName string) []string {
	var tools []string
	for name, tc := range s.cfg.Tools {
		if tc.Auth != nil && tc.Auth.Type == "oauth" && tc.Auth.Provider == providerName {
			tools = append(tools, name)
		}
	}
	sort.Strings(tools)
	return tools
}

func (s *Server) renderAuthList(w http.ResponseWriter, providers []authProviderView) {
	if len(providers) == 0 {
		fmt.Fprint(w, `<div class="px-5 py-8 text-center text-gray-400 text-sm">No OAuth providers configured.</div>`)
		return
	}

	for _, p := range providers {
		icon := "✓"
		if p.Status == "expired_refreshable" {
			icon = "⟳"
		} else if p.Status == "expired" || p.Status == "missing" {
			icon = "✗"
		}

		fmt.Fprintf(w, `<div class="px-5 py-4 border-b border-gray-100 last:border-b-0">
			<div class="flex items-center justify-between mb-1">
				<div class="flex items-center gap-2">
					<span class="inline-flex items-center justify-center w-5 h-5 rounded-full text-[10px] font-bold bg-%s-100 text-%s-600">%s</span>
					<span class="font-mono text-sm font-medium">%s</span>
					<span class="text-xs text-%s-600">%s</span>
				</div>
				<div class="flex items-center gap-2">`,
			p.StatusColor, p.StatusColor, icon, p.Name, p.StatusColor, p.StatusLabel)

		if !p.HasToken {
			fmt.Fprintf(w, `<span class="text-[10px] text-gray-400 font-mono bg-gray-50 px-2 py-1 rounded border border-gray-200 select-all">factorly auth login %s</span>`, p.Name)
		}
		if p.HasToken {
			fmt.Fprintf(w, `<button hx-delete="/auth/%s" hx-target="#auth-list" hx-swap="innerHTML" hx-confirm="Logout from %s?" class="text-red-400 hover:text-red-600 text-xs">logout</button>`, p.Name, p.Name)
		}

		fmt.Fprint(w, `</div></div>`)

		// Tools using this provider
		if len(p.Tools) > 0 {
			fmt.Fprint(w, `<div class="flex flex-wrap gap-1 mt-1">`)
			for _, t := range p.Tools {
				fmt.Fprintf(w, `<span class="text-[10px] font-mono text-gray-500 bg-gray-50 px-1.5 py-0.5 rounded">%s</span>`, t)
			}
			fmt.Fprint(w, `</div>`)
		}

		fmt.Fprint(w, `</div>`)
	}
}
