// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// resetVaultFlags clears the package-level CLI flag variables that
// activeTier consults so each test starts from a clean slate. Without
// this, state leaks across tests in undefined order.
func resetVaultFlags(t *testing.T) {
	t.Helper()
	prevPath, prevGlobal, prevWS := vaultPath, vaultGlobal, workspaceName
	vaultPath, vaultGlobal, workspaceName = "", false, ""
	t.Cleanup(func() {
		vaultPath, vaultGlobal, workspaceName = prevPath, prevGlobal, prevWS
	})
}

// chdir cds into dir for the duration of the test and restores on
// cleanup. activeTier consults os.Stat(".factorly") so the cwd matters.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// TestActiveTierPrecedence walks the full precedence table so the
// "which tier do we target?" rule is locked down by tests. Before the
// activeTier consolidation this precedence was implemented inline in
// three different places, and easy to break in one without noticing.
func TestActiveTierPrecedence(t *testing.T) {
	// --vault-path wins over everything else.
	t.Run("vault-path beats global, env, workspace", func(t *testing.T) {
		resetVaultFlags(t)
		t.Setenv("FACTORLY_VAULT_PATH", "/from/env.enc")
		vaultPath = "/explicit.enc"
		vaultGlobal = true
		workspaceName = "staging"
		got := activeTier()
		if got.Name != "explicit:/explicit.enc" {
			t.Errorf("expected explicit tier, got %q", got.Name)
		}
		if got.Path != "/explicit.enc" {
			t.Errorf("expected explicit path, got %q", got.Path)
		}
	})

	// --global beats env, workspace, project.
	t.Run("--global beats env and workspace", func(t *testing.T) {
		resetVaultFlags(t)
		t.Setenv("FACTORLY_VAULT_PATH", "/from/env.enc")
		vaultGlobal = true
		workspaceName = "staging"
		got := activeTier()
		if got.Name != "global" {
			t.Errorf("expected global tier, got %q", got.Name)
		}
	})

	// FACTORLY_VAULT_PATH beats --workspace.
	t.Run("env path beats workspace", func(t *testing.T) {
		resetVaultFlags(t)
		t.Setenv("FACTORLY_VAULT_PATH", "/from/env.enc")
		workspaceName = "staging"
		got := activeTier()
		if got.Name != "explicit:/from/env.enc" {
			t.Errorf("expected explicit tier from env, got %q", got.Name)
		}
	})

	// --workspace beats project default.
	t.Run("workspace beats project default", func(t *testing.T) {
		resetVaultFlags(t)
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		workspaceName = "staging"
		got := activeTier()
		if got.Name != "workspace:staging" {
			t.Errorf("expected workspace tier, got %q", got.Name)
		}
	})

	// Project tier is picked when .factorly/ exists and no flag overrides.
	t.Run("project default when .factorly exists", func(t *testing.T) {
		resetVaultFlags(t)
		t.Setenv("FACTORLY_VAULT_PATH", "")
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		got := activeTier()
		if got.Name != "project" {
			t.Errorf("expected project tier, got %q", got.Name)
		}
	})

	// Global tier is the final fallback.
	t.Run("global fallback when no .factorly", func(t *testing.T) {
		resetVaultFlags(t)
		t.Setenv("FACTORLY_VAULT_PATH", "")
		// Use a temp dir that definitely has no .factorly subdir.
		chdir(t, t.TempDir())
		got := activeTier()
		if got.Name != "global" {
			t.Errorf("expected global tier, got %q", got.Name)
		}
	})
}

// TestExplicitTierIdentity verifies that a user-pinned path does NOT
// inherit project/workspace tier env-var resolution, even when the
// path looks like a project vault. This is the safety property that
// Step 1's struct-based identity bought us: a path argument can't
// accidentally trigger FACTORLY_PROJECT_VAULT_PASSWORD lookup just
// because its filename ends in /.factorly/vault.enc.
func TestExplicitTierIdentity(t *testing.T) {
	resetVaultFlags(t)
	// A path that *looks* like a project vault, but the user pinned it.
	pinned := filepath.Join(t.TempDir(), ".factorly", "vault.enc")
	vaultPath = pinned
	got := activeTier()
	if !strings.HasPrefix(got.Name, "explicit:") {
		t.Errorf("expected explicit tier identity, got %q", got.Name)
	}
	// Explicit tier consults only FACTORLY_VAULT_PASSWORD, not the
	// project-specific variant. Set the project variant to a tripwire
	// and confirm it is NOT consulted.
	t.Setenv("FACTORLY_PROJECT_VAULT_PASSWORD", "project-only")
	t.Setenv("FACTORLY_VAULT_PASSWORD", "shared")
	pw, err := got.ResolvePassword(false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if string(pw) != "shared" {
		t.Errorf("explicit tier picked up the wrong env var: got %q, want shared", string(pw))
	}
}

// TestTierResolvePasswordPrecedence runs every tier through its
// non-interactive env-var precedence rules. One table per tier; rows
// model the "set X, expect Y" combinations.
func TestTierResolvePasswordPrecedence(t *testing.T) {
	type envSet struct {
		key, val string
	}

	cases := []struct {
		name    string
		tier    func() vaultTier
		envs    []envSet
		want    string
		wantErr bool
	}{
		// --- workspace tier ---
		{
			name: "workspace_specific_env_wins",
			tier: func() vaultTier { return workspaceTier("staging") },
			envs: []envSet{
				{"FACTORLY_WORKSPACE_VAULT_PASSWORD_STAGING", "ws-specific"},
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			want: "ws-specific",
		},
		{
			name: "workspace_falls_back_to_shared",
			tier: func() vaultTier { return workspaceTier("staging") },
			envs: []envSet{
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			want: "shared",
		},
		{
			name: "workspace_specific_empty_is_error",
			tier: func() vaultTier { return workspaceTier("staging") },
			envs: []envSet{
				{"FACTORLY_WORKSPACE_VAULT_PASSWORD_STAGING", ""},
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			wantErr: true,
		},
		// --- project tier ---
		{
			name: "project_specific_env_wins",
			tier: projectTier,
			envs: []envSet{
				{"FACTORLY_PROJECT_VAULT_PASSWORD", "proj-only"},
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			want: "proj-only",
		},
		{
			name: "project_falls_back_to_shared",
			tier: projectTier,
			envs: []envSet{
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			want: "shared",
		},
		{
			name: "project_specific_empty_is_error",
			tier: projectTier,
			envs: []envSet{
				{"FACTORLY_PROJECT_VAULT_PASSWORD", ""},
			},
			wantErr: true,
		},
		{
			name: "project_shared_empty_skips_silently",
			tier: projectTier,
			envs: []envSet{
				// shared env set to empty should NOT error; the strict-empty
				// rule only applies to the tier-specific env var.
				{"FACTORLY_VAULT_PASSWORD", ""},
			},
			wantErr: true, // no other source either -> LockedErr
		},
		// --- global tier ---
		{
			name: "global_env_wins",
			tier: globalTier,
			envs: []envSet{
				{"FACTORLY_VAULT_PASSWORD", "shared"},
			},
			want: "shared",
		},
		{
			name: "global_empty_is_error",
			tier: globalTier,
			envs: []envSet{
				{"FACTORLY_VAULT_PASSWORD", ""},
			},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Clear all relevant env vars so values from a previous case
			// or the dev environment don't leak in.
			for _, k := range []string{
				"FACTORLY_VAULT_PASSWORD",
				"FACTORLY_PROJECT_VAULT_PASSWORD",
				"FACTORLY_WORKSPACE_VAULT_PASSWORD_STAGING",
			} {
				t.Setenv(k, "__unset__")
				os.Unsetenv(k)
			}
			for _, e := range c.envs {
				t.Setenv(e.key, e.val)
			}
			// Use HOME pointed at an empty dir so the global keyfile
			// fallback can't surprise the test.
			t.Setenv("HOME", t.TempDir())

			tier := c.tier()
			// Re-derive tier after setting HOME — globalTier reads
			// UserHomeDir() to build its keyfile path.
			pw, err := tier.ResolvePassword(false)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got pw=%q", string(pw))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(pw) != c.want {
				t.Errorf("got %q, want %q", string(pw), c.want)
			}
		})
	}
}

// TestTierResolvePasswordSurfacesLockedErr verifies that each tier
// returns its own LockedErr sentinel when no non-interactive source
// resolves. UI callers detect these to surface unlock dialogs.
func TestTierResolvePasswordSurfacesLockedErr(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{
		"FACTORLY_VAULT_PASSWORD",
		"FACTORLY_PROJECT_VAULT_PASSWORD",
		"FACTORLY_WORKSPACE_VAULT_PASSWORD_STAGING",
	} {
		t.Setenv(k, "__unset__")
		os.Unsetenv(k)
	}
	cases := []struct {
		name string
		tier vaultTier
		want error
	}{
		{"workspace", workspaceTier("staging"), errWorkspaceVaultLocked},
		{"project", projectTier(), errProjectVaultLocked},
		{"global", globalTier(), errGlobalVaultLocked},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.tier.ResolvePassword(false)
			if !errors.Is(err, c.want) {
				t.Errorf("got %v, want %v", err, c.want)
			}
		})
	}
}

// TestTierForPathClassification verifies that the path -> tier mapping
// is path-shape sensitive (so absolute paths from --vault-path that
// happen to live under .factorly/ don't get classified as project
// tier and inherit FACTORLY_PROJECT_VAULT_PASSWORD lookup).
//
// Note: today tierForPath is still string-based (Step 4 stopped short
// of fully retiring it because the chain composition still needs to
// dispatch on path). The cases here lock in the current behavior so
// any future tweaks to isProjectVault / isWorkspaceVault are noticed.
func TestTierForPathClassification(t *testing.T) {
	cases := []struct {
		path     string
		wantTier string
	}{
		{filepath.Join(".factorly", "vault.enc"), "project"},
		{filepath.Join(".factorly", "vaults", "staging.enc"), "workspace:staging"},
		{filepath.Join("/tmp", "vault.enc"), "global"},
	}
	if runtime.GOOS != "windows" {
		// Absolute paths that happen to live inside a .factorly/ named
		// dir under /tmp DO currently classify as project tier — this
		// is a known edge case (the path-shape matcher can't tell the
		// difference). Document it here so the behavior change in a
		// future hardening pass is intentional.
		cases = append(cases, struct {
			path     string
			wantTier string
		}{filepath.Join("/tmp", ".factorly", "vault.enc"), "project"})
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := tierForPath(c.path)
			if got.Name != c.wantTier {
				t.Errorf("path %s: tier %q, want %q", c.path, got.Name, c.wantTier)
			}
		})
	}
}
