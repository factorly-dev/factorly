// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package blueprints

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestBundledLoadsWithoutError(t *testing.T) {
	if err := BundledLoadError(); err != nil {
		t.Fatalf("bundled blueprints failed to load: %v", err)
	}
}

func TestBundledIsNonEmpty(t *testing.T) {
	list := Bundled()
	if len(list) == 0 {
		t.Fatal("expected at least one bundled blueprint")
	}
}

func TestBundledIsSorted(t *testing.T) {
	list := Bundled()
	names := make([]string, len(list))
	for i, b := range list {
		names[i] = b.Header.Name
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("Bundled() not sorted: %v", names)
	}
}

func TestBundledByNameLinear(t *testing.T) {
	// Linear is a representative api_key/bearer blueprint. If any of these
	// header fields drift, the catalog UI will misrender.
	bp := BundledByName("linear")
	if bp == nil {
		t.Fatal("bundled blueprint 'linear' not found")
	}
	h := bp.Header
	if h.Name != "linear" {
		t.Errorf("Name = %q, want linear", h.Name)
	}
	if h.DisplayName != "Linear" {
		t.Errorf("DisplayName = %q, want Linear", h.DisplayName)
	}
	if h.Category != "engineering" {
		t.Errorf("Category = %q, want engineering", h.Category)
	}
	if h.AuthType != "api_key" {
		t.Errorf("AuthType = %q, want api_key", h.AuthType)
	}
	if h.AuthGuide == "" {
		t.Error("AuthGuide empty")
	}
	if h.Filename != "linear.yaml" {
		t.Errorf("Filename = %q, want linear.yaml", h.Filename)
	}
	if bp.YAML == "" {
		t.Error("YAML body empty")
	}
}

func TestBundledByNameOAuthShape(t *testing.T) {
	// Gmail is OAuth; the catalog distinguishes those via AuthType.
	bp := BundledByName("gmail")
	if bp == nil {
		t.Fatal("bundled blueprint 'gmail' not found")
	}
	if bp.Header.AuthType != "oauth" {
		t.Errorf("AuthType = %q, want oauth", bp.Header.AuthType)
	}
}

func TestBundledByNameMissing(t *testing.T) {
	if bp := BundledByName("nonexistent-blueprint-xyz"); bp != nil {
		t.Errorf("expected nil for missing blueprint, got %+v", bp)
	}
}

// TestBundledAllInstallable proves every bundled blueprint passes through the
// full Install dry-run pipeline. Catches malformed YAML, broken references,
// or missing headers in any of the bundled files at test time rather than at
// catalog-render time.
func TestBundledAllInstallable(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "factorly.yaml")
	if err := os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, bp := range Bundled() {
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			res, err := Install(InstallOptions{
				Content: []byte(bp.YAML),
				CfgPath: cfgPath,
				DryRun:  true,
			})
			if err != nil {
				t.Fatalf("dry-run install: %v", err)
			}
			if res == nil {
				t.Fatal("nil InstallResult")
			}
			if res.Header.Name != bp.Header.Name {
				t.Errorf("Header.Name = %q, want %q", res.Header.Name, bp.Header.Name)
			}
			if len(res.ToolsAdded)+len(res.WorkflowsAdded) == 0 {
				t.Errorf("blueprint %q has no tools or workflows", bp.Header.Name)
			}
		})
	}
}

// TestBundledHeaderInvariants ports the per-template field checks from the
// old templates package. Each bundled blueprint must declare the metadata
// the catalog UI relies on. Subtests so a single bad file is easy to find.
func TestBundledHeaderInvariants(t *testing.T) {
	for _, bp := range Bundled() {
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			h := bp.Header
			if h.Name == "" {
				t.Error("Name empty")
			}
			if h.DisplayName == "" {
				t.Error("DisplayName empty")
			}
			if h.Description == "" {
				t.Error("Description empty")
			}
			if h.Category == "" {
				t.Error("Category empty")
			}
			if h.AuthType == "" {
				t.Error("AuthType empty")
			}
			// AuthGuide is required for credential-bearing blueprints; "none"
			// (CLI tools that need no auth) is exempt.
			if h.AuthType != "none" && h.AuthGuide == "" {
				t.Error("AuthGuide empty")
			}
			if h.Version == "" {
				t.Error("Version empty")
			}
		})
	}
}

// TestBundledUniqueNames catches a copy-paste bug where two bundled files
// claim the same name (would silently overwrite each other in the index).
func TestBundledUniqueNames(t *testing.T) {
	seen := make(map[string]string)
	for _, bp := range Bundled() {
		if existing, ok := seen[bp.Header.Name]; ok {
			t.Errorf("duplicate bundled name %q in both %s and %s", bp.Header.Name, existing, bp.Header.Filename)
		}
		seen[bp.Header.Name] = bp.Header.Filename
	}
}

// TestBundledAuthShapes verifies each blueprint's tools match its declared
// auth_type. api_key/bearer blueprints must use vault refs in tool auth
// tokens; oauth blueprints must declare an oauth_providers block whose key
// matches what tools reference.
func TestBundledAuthShapes(t *testing.T) {
	for _, bp := range Bundled() {
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			parsed, err := ParseBlueprint([]byte(bp.YAML))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			switch bp.Header.AuthType {
			case "api_key", "bearer":
				for name, tc := range parsed.Tools {
					if tc.Auth == nil {
						continue
					}
					if tc.Auth.Token != "" && !strings.Contains(tc.Auth.Token, "{{vault:") {
						t.Errorf("tool %s: bearer token should reference vault, got %q", name, tc.Auth.Token)
					}
				}
			case "oauth":
				if len(parsed.OAuthProviders) == 0 {
					t.Error("AuthType=oauth but no oauth_providers declared")
				}
				// Tools that use auth.type=oauth must reference a provider
				// that's defined in this same blueprint.
				for name, tc := range parsed.Tools {
					if tc.Auth == nil || tc.Auth.Type != "oauth" {
						continue
					}
					if tc.Auth.Provider == "" {
						t.Errorf("tool %s: oauth auth missing provider name", name)
						continue
					}
					if _, ok := parsed.OAuthProviders[tc.Auth.Provider]; !ok {
						t.Errorf("tool %s: references provider %q not in this blueprint", name, tc.Auth.Provider)
					}
				}
			}
		})
	}
}

// TestBundledToolsHaveDescriptions and TestBundledParametersHaveDescriptions
// preserve the per-tool/per-param descriptive copy that drives the UI tool
// detail page and MCP-client UX.
func TestBundledToolsHaveDescriptions(t *testing.T) {
	for _, bp := range Bundled() {
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			parsed, err := ParseBlueprint([]byte(bp.YAML))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for name, tc := range parsed.Tools {
				if tc.Description == "" {
					t.Errorf("tool %s: empty description", name)
				}
			}
		})
	}
}

func TestBundledParametersHaveDescriptions(t *testing.T) {
	for _, bp := range Bundled() {
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			parsed, err := ParseBlueprint([]byte(bp.YAML))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for toolName, tc := range parsed.Tools {
				for _, p := range tc.Parameters {
					if p.Name == "" {
						t.Errorf("tool %s: parameter with empty name", toolName)
					}
					if p.Description == "" {
						t.Errorf("tool %s param %s: missing description", toolName, p.Name)
					}
				}
			}
		})
	}
}

// TestBundledOAuthEndpoints ensures every oauth blueprint declares the
// provider config the OAuth flow needs (auth_url, token_url, scopes, vault-
// referenced client_id/secret).
func TestBundledOAuthEndpoints(t *testing.T) {
	for _, bp := range Bundled() {
		if bp.Header.AuthType != "oauth" {
			continue
		}
		bp := bp
		t.Run(bp.Header.Name, func(t *testing.T) {
			parsed, err := ParseBlueprint([]byte(bp.YAML))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for name, p := range parsed.OAuthProviders {
				if p.AuthURL == "" {
					t.Errorf("oauth_providers.%s: missing auth_url", name)
				}
				if p.TokenURL == "" {
					t.Errorf("oauth_providers.%s: missing token_url", name)
				}
				if len(p.Scopes) == 0 {
					t.Errorf("oauth_providers.%s: missing scopes", name)
				}
				if !strings.Contains(p.ClientID, "{{vault:") {
					t.Errorf("oauth_providers.%s: client_id should reference vault, got %q", name, p.ClientID)
				}
				if !strings.Contains(p.ClientSecret, "{{vault:") {
					t.Errorf("oauth_providers.%s: client_secret should reference vault, got %q", name, p.ClientSecret)
				}
			}
		})
	}
}
