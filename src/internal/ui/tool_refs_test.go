// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
)

// TestToolReferenceCounts_BasicAuthRef confirms a single tool with a
// {{vault:KEY}} reference in auth bumps the key by 1.
func TestToolReferenceCounts_BasicAuthRef(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"some.tool": {
				Type:    "rest",
				BaseURL: "https://api.example.com",
				Method:  "GET",
				Path:    "/v1/things",
				Auth: &config.AuthConfig{
					Type:  "bearer",
					Token: "{{vault:EXAMPLE_API_KEY}}",
				},
			},
		},
	}
	got := toolReferenceCounts(cfg, "vault")
	if got["EXAMPLE_API_KEY"] != 1 {
		t.Errorf("EXAMPLE_API_KEY count = %d, want 1; full map: %v", got["EXAMPLE_API_KEY"], got)
	}
}

// TestToolReferenceCounts_OneToolOneIncrement a tool that mentions
// the same key in three fields still counts as 1 toward that key.
func TestToolReferenceCounts_OneToolOneIncrement(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"chatty": {
				Type:    "rest",
				BaseURL: "https://{{vault:HOST}}/api",
				Method:  "POST",
				Path:    "/v1/{{vault:HOST}}",
				Headers: map[string]string{
					"X-Host": "{{vault:HOST}}",
				},
			},
		},
	}
	got := toolReferenceCounts(cfg, "vault")
	if got["HOST"] != 1 {
		t.Errorf("HOST count = %d, want 1 (single tool); got map %v", got["HOST"], got)
	}
}

// TestToolReferenceCounts_DistinctTools two tools referencing the
// same key produce a count of 2.
func TestToolReferenceCounts_DistinctTools(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"a": {
				Type:    "rest",
				BaseURL: "https://x",
				Method:  "GET",
				Auth:    &config.AuthConfig{Type: "bearer", Token: "{{vault:SHARED}}"},
			},
			"b": {
				Type:    "rest",
				BaseURL: "https://y",
				Method:  "GET",
				Auth:    &config.AuthConfig{Type: "bearer", Token: "{{vault:SHARED}}"},
			},
		},
	}
	got := toolReferenceCounts(cfg, "vault")
	if got["SHARED"] != 2 {
		t.Errorf("SHARED count = %d, want 2; got map %v", got["SHARED"], got)
	}
}

// TestToolReferenceCounts_BackendIsolation a {{store:K}} reference
// must NOT bump the vault count for K, and vice versa.
func TestToolReferenceCounts_BackendIsolation(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"mixed": {
				Type:    "rest",
				BaseURL: "https://x",
				Method:  "GET",
				Headers: map[string]string{
					"X-Vault": "{{vault:K}}",
					"X-Store": "{{store:K}}",
				},
			},
		},
	}
	vaultCounts := toolReferenceCounts(cfg, "vault")
	storeCounts := toolReferenceCounts(cfg, "store")
	if vaultCounts["K"] != 1 {
		t.Errorf("vault K count = %d, want 1", vaultCounts["K"])
	}
	if storeCounts["K"] != 1 {
		t.Errorf("store K count = %d, want 1", storeCounts["K"])
	}
}

// TestToolReferenceCounts_NoReferences returns empty map when no
// tools reference the backend at all.
func TestToolReferenceCounts_NoReferences(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"plain": {Type: "rest", BaseURL: "https://x", Method: "GET"},
		},
	}
	got := toolReferenceCounts(cfg, "vault")
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// TestToolReferenceCounts_NilConfig safe against nil — used to guard
// the early-render case where s.cfg might briefly be nil during boot.
func TestToolReferenceCounts_NilConfig(t *testing.T) {
	got := toolReferenceCounts(nil, "vault")
	if len(got) != 0 {
		t.Errorf("nil cfg should return empty map, got %v", got)
	}
}

// TestOAuthProviderReferenceCounts_BasicRef one provider referencing
// {{vault:GH_CLIENT_SECRET}} bumps that key by 1.
func TestOAuthProviderReferenceCounts_BasicRef(t *testing.T) {
	cfg := &config.Config{
		OAuthProviders: map[string]config.OAuthProviderConfig{
			"github": {
				ClientID:     "abc",
				ClientSecret: "{{vault:GH_CLIENT_SECRET}}",
				AuthURL:      "https://github.com/login/oauth/authorize",
				TokenURL:     "https://github.com/login/oauth/access_token",
			},
		},
	}
	got := oauthProviderReferenceCounts(cfg, "vault")
	if got["GH_CLIENT_SECRET"] != 1 {
		t.Errorf("GH_CLIENT_SECRET count = %d, want 1; full map: %v", got["GH_CLIENT_SECRET"], got)
	}
}

// TestOAuthProviderReferenceCounts_DedupWithinProvider one provider
// mentioning the same key in two fields still counts once.
func TestOAuthProviderReferenceCounts_DedupWithinProvider(t *testing.T) {
	cfg := &config.Config{
		OAuthProviders: map[string]config.OAuthProviderConfig{
			"weird": {
				ClientID:     "{{vault:K}}",
				ClientSecret: "{{vault:K}}",
			},
		},
	}
	got := oauthProviderReferenceCounts(cfg, "vault")
	if got["K"] != 1 {
		t.Errorf("K count = %d, want 1 (single provider); got map %v", got["K"], got)
	}
}

// TestOAuthProviderReferenceCounts_AcrossProviders two providers
// referencing the same key produce a count of 2.
func TestOAuthProviderReferenceCounts_AcrossProviders(t *testing.T) {
	cfg := &config.Config{
		OAuthProviders: map[string]config.OAuthProviderConfig{
			"a": {ClientSecret: "{{vault:SHARED}}"},
			"b": {ClientSecret: "{{vault:SHARED}}"},
		},
	}
	got := oauthProviderReferenceCounts(cfg, "vault")
	if got["SHARED"] != 2 {
		t.Errorf("SHARED count = %d, want 2; got map %v", got["SHARED"], got)
	}
}

// TestOAuthProviderReferenceCounts_NilConfig safe against nil.
func TestOAuthProviderReferenceCounts_NilConfig(t *testing.T) {
	got := oauthProviderReferenceCounts(nil, "vault")
	if len(got) != 0 {
		t.Errorf("nil cfg should return empty map, got %v", got)
	}
}
