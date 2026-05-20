// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStorePathBuilders pins the canonical on-disk paths for each
// tier. If any of these change, callers that build paths by hand
// (e.g. backup scripts, integration tests) need to know.
func TestStorePathBuilders(t *testing.T) {
	if got := projectStorePath(); got != filepath.Join(".factorly", "store.db") {
		t.Errorf("projectStorePath() = %q, want .factorly/store.db", got)
	}
	if got := workspaceStorePath("staging"); got != filepath.Join(".factorly", "workspaces", "staging", "store.db") {
		t.Errorf("workspaceStorePath(staging) = %q", got)
	}
}

// TestWorkspaceStorePathRejectsBadNames closes the path-traversal
// seam: the validator must refuse names that would let writes
// escape .factorly/workspaces/. Mirrors the vault test for the
// same reason — store can write anywhere on disk if we don't gate.
func TestWorkspaceStorePathRejectsBadNames(t *testing.T) {
	for _, bad := range []string{"", "..", "../escape", "a/b", `back\slash`, ".hidden", "trailing."} {
		if got := workspaceStorePath(bad); got != "" {
			t.Errorf("workspaceStorePath(%q) = %q, want empty (invalid name should produce no path)", bad, got)
		}
	}
}

// TestActiveStoreTierPrecedence walks the precedence rules that
// activeStoreTier promises. activeTier (vault's analog) has the
// same shape; this test mirrors that one but for store-specific
// tiers and without the password-machinery branches that don't
// apply here.
func TestActiveStoreTierPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		selector tierSelector
		wantName string
	}{
		{
			name:     "workspace wins when set",
			selector: tierSelector{WorkspaceName: "staging"},
			wantName: "workspace:staging",
		},
		{
			name:     "no workspace, project tier when .factorly/ exists in cwd",
			selector: tierSelector{},
			wantName: "project",
		},
		{
			name:     "--global pins to global tier even inside a project",
			selector: tierSelector{StoreGlobal: true},
			wantName: "global",
		},
		{
			name:     "--global beats --workspace (mirrors vault precedence)",
			selector: tierSelector{StoreGlobal: true, WorkspaceName: "staging"},
			wantName: "global",
		},
	}
	// Project / workspace selection requires a .factorly/ in cwd; the
	// global-fallback case is exercised separately because it needs
	// a chdir to a directory WITHOUT .factorly/.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := activeStoreTier(c.selector)
			if got.Name != c.wantName {
				t.Errorf("got tier %q, want %q", got.Name, c.wantName)
			}
		})
	}

	t.Run("global fallback when no .factorly", func(t *testing.T) {
		chdir(t, t.TempDir())
		got := activeStoreTier(tierSelector{})
		if got.Name != "global" {
			t.Errorf("got %q, want global", got.Name)
		}
		// Global tier path should be under HOME if available; the
		// only contract we promise is that it's not "".
		if got.Path == "" {
			t.Error("global tier should have a non-empty path")
		}
	})
}

// TestValidateActiveStoreName confirms the up-front check that
// guards against silent fall-through. When --workspace is set to
// an invalid name, the CLI command must error before touching disk,
// not write to .factorly/workspaces/<garbage>/store.db. Symmetric
// to vault's openVault validation.
func TestValidateActiveStoreName(t *testing.T) {
	prev := workspaceName
	t.Cleanup(func() { workspaceName = prev })

	// Empty workspace name = no workspace selected = no error.
	workspaceName = ""
	if err := validateActiveStoreName(); err != nil {
		t.Errorf("empty workspaceName should pass, got %v", err)
	}

	// Valid name.
	workspaceName = "staging"
	if err := validateActiveStoreName(); err != nil {
		t.Errorf("valid name should pass, got %v", err)
	}

	// Each bad form must error.
	for _, bad := range []string{"..", "../escape", "a/b", ".hidden"} {
		workspaceName = bad
		if err := validateActiveStoreName(); err == nil {
			t.Errorf("name %q should be rejected", bad)
		} else if !strings.Contains(err.Error(), "--workspace") {
			t.Errorf("error for %q should mention --workspace, got %q", bad, err.Error())
		}
	}
}

// TestStoreTierExists confirms the Exists() probe distinguishes
// "file on disk" from "file not yet created." Used by UI handlers
// to decide whether to show "empty store" vs "store has data."
func TestStoreTierExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.db")
	t1 := storeTier{Name: "test", Path: path}
	if t1.Exists() {
		t.Error("Exists() should be false for non-existent file")
	}
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if !t1.Exists() {
		t.Error("Exists() should be true after file creation")
	}
	empty := storeTier{Name: "no-path", Path: ""}
	if empty.Exists() {
		t.Error("empty-path tier should report Exists()=false")
	}
}

// TestParseStoreTTL pins the CLI flag parsing contract:
//   - empty → (0, false, nil) so caller uses backend default
//   - "0" → (0, true, nil) for never-expire
//   - "7d" → 7*24h
//   - "24h", "30m", "5s" → standard time.ParseDuration semantics
//   - garbage → error
func TestParseStoreTTL(t *testing.T) {
	cases := []struct {
		in        string
		wantDur   string // formatted via Duration.String() for readability
		wantHas   bool
		wantError bool
	}{
		{"", "0s", false, false},
		{"0", "0s", true, false},
		{"7d", "168h0m0s", true, false},
		{"24h", "24h0m0s", true, false},
		{"30m", "30m0s", true, false},
		{"5s", "5s", true, false},
		{"garbage", "", false, true},
		{"7q", "", false, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			d, has, err := parseStoreTTL(c.in)
			if c.wantError {
				if err == nil {
					t.Errorf("expected error, got dur=%v has=%v", d, has)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if has != c.wantHas {
				t.Errorf("has = %v, want %v", has, c.wantHas)
			}
			if d.String() != c.wantDur {
				t.Errorf("dur = %s, want %s", d, c.wantDur)
			}
		})
	}
}
