// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir cds into dir for the duration of the test and restores on
// cleanup. activeTier consults os.Stat(".factorly") for the
// project-default branch, so the cwd matters for that case.
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
//
// activeTier is pure with respect to its tierSelector input — these
// tests construct the selector directly rather than mutating package
// globals, so each subtest is independent and goroutine-safe.
func TestActiveTierPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		selector tierSelector
		// wantTier is checked via HasPrefix so explicit-tier cases can
		// match "explicit:/some/path" without spelling out the path.
		wantTier string
	}{
		{
			name: "vault-path beats global, env, workspace",
			selector: tierSelector{
				VaultPath:     "/explicit.enc",
				VaultGlobal:   true,
				EnvVaultPath:  "/from/env.enc",
				WorkspaceName: "staging",
			},
			wantTier: "explicit:/explicit.enc",
		},
		{
			name: "--global beats env and workspace",
			selector: tierSelector{
				VaultGlobal:   true,
				EnvVaultPath:  "/from/env.enc",
				WorkspaceName: "staging",
			},
			// --global resolves to explicit-tier identity (single vault,
			// no chain). Substring match on "explicit:" leaves the path
			// portion implementation-defined.
			wantTier: "explicit:",
		},
		{
			// FACTORLY_VAULT_PATH overrides only the global vault
			// location; it does NOT shadow --workspace or the
			// project-default tier. So workspace wins when both are set.
			name: "workspace beats env path",
			selector: tierSelector{
				EnvVaultPath:  "/from/env.enc",
				WorkspaceName: "staging",
			},
			wantTier: "workspace:staging",
		},
		{
			// FACTORLY_VAULT_PATH with no project and no other flag:
			// produces a global tier with the env's path. Identity stays
			// "global" so OpenChain keeps chain semantics.
			name: "env path overrides global path only",
			selector: tierSelector{
				EnvVaultPath: "/from/env.enc",
			},
			wantTier: "global",
		},
		{
			name:     "workspace selector picked",
			selector: tierSelector{WorkspaceName: "staging"},
			wantTier: "workspace:staging",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := activeTier(c.selector)
			if !strings.HasPrefix(got.Name, c.wantTier) {
				t.Errorf("got tier %q, want prefix %q", got.Name, c.wantTier)
			}
		})
	}

	// Project-default and global-fallback branches depend on cwd, so
	// each gets its own subtest with chdir handling.
	t.Run("project default when .factorly exists", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		got := activeTier(tierSelector{})
		if got.Name != "project" {
			t.Errorf("expected project tier, got %q", got.Name)
		}
	})

	t.Run("global fallback when no .factorly", func(t *testing.T) {
		chdir(t, t.TempDir())
		got := activeTier(tierSelector{})
		if got.Name != "global" {
			t.Errorf("expected global tier, got %q", got.Name)
		}
	})
}

// TestCurrentSelectorSnapshotsFlags confirms that currentSelector()
// is the seam between package globals and the pure precedence
// function — reading from the globals AND from FACTORLY_VAULT_PATH.
// If any of these inputs is omitted by currentSelector, activeTier
// would silently make decisions based on stale or missing state.
func TestCurrentSelectorSnapshotsFlags(t *testing.T) {
	prevPath, prevGlobal, prevWS := vaultPath, vaultGlobal, workspaceName
	t.Cleanup(func() {
		vaultPath, vaultGlobal, workspaceName = prevPath, prevGlobal, prevWS
	})
	vaultPath = "/from-flag.enc"
	vaultGlobal = true
	workspaceName = "ws-from-flag"
	t.Setenv("FACTORLY_VAULT_PATH", "/from-env.enc")

	s := currentSelector()
	if s.VaultPath != "/from-flag.enc" {
		t.Errorf("VaultPath: got %q", s.VaultPath)
	}
	if !s.VaultGlobal {
		t.Error("VaultGlobal: got false, want true")
	}
	if s.WorkspaceName != "ws-from-flag" {
		t.Errorf("WorkspaceName: got %q", s.WorkspaceName)
	}
	if s.EnvVaultPath != "/from-env.enc" {
		t.Errorf("EnvVaultPath: got %q", s.EnvVaultPath)
	}
}

// TestExplicitTierIdentity verifies that a user-pinned path does NOT
// inherit project/workspace tier env-var resolution, even when the
// path looks like a project vault. This is the safety property that
// Step 1's struct-based identity bought us: a path argument can't
// accidentally trigger FACTORLY_PROJECT_VAULT_PASSWORD lookup just
// because its filename ends in /.factorly/vault.enc.
func TestExplicitTierIdentity(t *testing.T) {
	// A path that *looks* like a project vault, but the user pinned it.
	pinned := filepath.Join(t.TempDir(), ".factorly", "vault.enc")
	got := activeTier(tierSelector{VaultPath: pinned})
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
		{"explicit", explicitTier("/tmp/some.enc"), errExplicitVaultLocked},
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

// TestTierForPathClassification documents the path-shape classifier
// used by ui_cmd.go's extractVaultTiers (which walks an open chain
// and asks LocalBackend.Path() what each opened file is).
//
// CLI password resolution does NOT use this function — that path was
// removed in Step 3 of the concession fix-up. CLI flows hold a
// vaultTier directly (constructed from the active flags/env via
// activeTier), so an absolute --vault-path can never accidentally
// inherit project-tier env-var lookup.
//
// The /tmp/.factorly/ row that previously documented a "known edge
// case" is gone with it — the path-shape ambiguity has no production
// CLI consumer to mislead. extractVaultTiers only sees paths that
// came from a LocalBackend we opened, so the provenance is trusted.
func TestTierForPathClassification(t *testing.T) {
	cases := []struct {
		path     string
		wantTier string
	}{
		{filepath.Join(".factorly", "vault.enc"), "project"},
		{filepath.Join(".factorly", "vaults", "staging.enc"), "workspace:staging"},
		{filepath.Join("/tmp", "vault.enc"), "global"},
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

// TestEnvSourceStrictness locks in the contract that "strict-on-empty"
// is a *field* on envSource, not a positional convention. Reordering
// the EnvVars slice must not flip which entry rejects an empty value.
//
// This is the test the refactor was missing: previously the rule was
// "the first env var is strict," encoded by index, so a refactor that
// happened to reorder the slice would silently swap the strictness
// rule with no test feedback.
func TestEnvSourceStrictness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"AAA", "BBB"} {
		t.Setenv(k, "__unset__")
		os.Unsetenv(k)
	}

	// Build a synthetic tier whose strict entry is in position 1, not 0.
	// If strictness were positional ("first wins"), AAA would be strict;
	// because it's a field, BBB is strict regardless of position.
	tier := vaultTier{
		Name:        "test",
		PromptLabel: "test: ",
		Path:        "/nope.enc",
		EnvVars: []envSource{
			{Name: "AAA", Strict: false},
			{Name: "BBB", Strict: true},
		},
		LockedErr: errGlobalVaultLocked,
	}

	// AAA empty: should silently skip (not strict). With BBB unset,
	// we fall through to LockedErr.
	t.Setenv("AAA", "")
	if _, err := tier.ResolvePassword(false); !errors.Is(err, errGlobalVaultLocked) {
		t.Errorf("AAA empty (non-strict): want LockedErr, got %v", err)
	}

	// BBB empty: should error (strict), even though it's not first.
	os.Unsetenv("AAA")
	t.Setenv("BBB", "")
	_, err := tier.ResolvePassword(false)
	if err == nil || !strings.Contains(err.Error(), "BBB is set but empty") {
		t.Errorf("BBB empty (strict): want 'BBB is set but empty', got %v", err)
	}

	// Both empty: AAA scanned first, skipped; BBB errors.
	t.Setenv("AAA", "")
	t.Setenv("BBB", "")
	_, err = tier.ResolvePassword(false)
	if err == nil || !strings.Contains(err.Error(), "BBB") {
		t.Errorf("AAA+BBB empty: strict BBB should error, got %v", err)
	}

	// BBB set, AAA empty: BBB wins.
	t.Setenv("AAA", "")
	t.Setenv("BBB", "from-bbb")
	pw, err := tier.ResolvePassword(false)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if string(pw) != "from-bbb" {
		t.Errorf("got %q, want from-bbb", string(pw))
	}
}
