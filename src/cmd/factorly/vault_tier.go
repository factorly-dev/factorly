// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal/vault"
)

// vaultTier is the unified abstraction over the three vault tiers
// (workspace / project / global). Each tier knows its own path,
// password sources, prompt label, and locked-sentinel error.
//
// Before this type, each tier had its own family of free functions
// (OpenX, OpenXWithPassword, tryResolveXVaultPassword,
// resolveXVaultPassword, errXVaultLocked, path-builder, etc.) — adding
// a fourth tier or changing the rules for one tier meant editing
// many small places. Identity is now a struct field rather than a
// string-inspection predicate, so absolute --vault-path values can no
// longer accidentally inherit project-tier resolution.
type vaultTier struct {
	// Name is the stable identity used in logs and error messages.
	// "workspace:<n>", "project", "global", or "explicit:<path>".
	Name string
	// PromptLabel is the human-facing label for the interactive prompt.
	PromptLabel string
	// Path is the on-disk location of the encrypted vault file.
	Path string
	// EnvVars are the ordered list of env var names to check for a
	// non-interactive password. First non-empty wins; an env var that
	// is set but empty is a user error and produces an error.
	EnvVars []string
	// KeyFile is the optional path to a 0600 file holding the password.
	// Empty string means no keyfile source for this tier.
	KeyFile string
	// LockedErr is returned by ResolvePassword(allowPrompt=false) when
	// no non-interactive source resolves. UI callers detect this to
	// surface the unlock dialog.
	LockedErr error
}

// workspaceTier returns the tier descriptor for a named workspace.
// name must be non-empty; callers that allow "no workspace selected"
// should branch before calling this.
func workspaceTier(name string) vaultTier {
	return vaultTier{
		Name:        "workspace:" + name,
		PromptLabel: fmt.Sprintf("Vault password (workspace %q): ", name),
		Path:        workspaceVaultPath(name),
		EnvVars: []string{
			"FACTORLY_WORKSPACE_VAULT_PASSWORD_" + strings.ToUpper(name),
			"FACTORLY_VAULT_PASSWORD",
		},
		KeyFile:   filepath.Join(".factorly", "vaults", name+".key"),
		LockedErr: errWorkspaceVaultLocked,
	}
}

// projectTier returns the tier descriptor for .factorly/vault.enc.
func projectTier() vaultTier {
	return vaultTier{
		Name:        "project",
		PromptLabel: "Vault password (project): ",
		Path:        projectVaultPath(),
		EnvVars: []string{
			"FACTORLY_PROJECT_VAULT_PASSWORD",
			"FACTORLY_VAULT_PASSWORD",
		},
		KeyFile:   filepath.Join(".factorly", "vault.key"),
		LockedErr: errProjectVaultLocked,
	}
}

// globalTier returns the tier descriptor for the global vault. The
// path may be overridden by FACTORLY_VAULT_PATH (handled by the
// upstream resolveVaultPath chain, not here).
func globalTier() vaultTier {
	t := vaultTier{
		Name:        "global",
		PromptLabel: "Vault password (global): ",
		Path:        vault.DefaultVaultPath(),
		EnvVars:     []string{"FACTORLY_VAULT_PASSWORD"},
		LockedErr:   errGlobalVaultLocked,
	}
	if home, err := os.UserHomeDir(); err == nil {
		t.KeyFile = filepath.Join(home, ".config", "factorly", "vault.key")
	}
	return t
}

// Exists reports whether the tier's vault file is present on disk.
// A non-existent file isn't an error here — most callers branch on
// existence before deciding whether to open or create.
func (t vaultTier) Exists() bool {
	if t.Path == "" {
		return false
	}
	_, err := os.Stat(t.Path)
	return err == nil
}

// ResolvePassword walks the non-interactive sources (env vars then
// keyfile) and optionally falls through to the interactive prompt.
// Returns LockedErr when allowPrompt is false and nothing resolved.
//
// The returned slice is owned by the caller — zero it after use.
func (t vaultTier) ResolvePassword(allowPrompt bool) ([]byte, error) {
	for _, env := range t.EnvVars {
		pw, ok := os.LookupEnv(env)
		if !ok {
			continue
		}
		if pw == "" {
			// First env var (the tier-specific one) is strict: set-but-empty
			// is a user error. Subsequent fallback env vars (shared
			// FACTORLY_VAULT_PASSWORD) silently skip when empty so a user
			// who clears the shared var doesn't break the tier-specific path.
			if env == t.EnvVars[0] {
				return nil, fmt.Errorf("%s is set but empty", env)
			}
			continue
		}
		vlog("%s vault password from %s", t.Name, env)
		return []byte(pw), nil
	}
	if t.KeyFile != "" {
		if pw, err := readKeyFile(t.KeyFile); err == nil {
			vlog("%s vault password from %s", t.Name, t.KeyFile)
			return pw, nil
		}
	}
	if !allowPrompt {
		return nil, t.LockedErr
	}
	pw, err := promptSecret(t.PromptLabel)
	if err != nil {
		return nil, err
	}
	if len(pw) == 0 {
		return nil, fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
}

// Open opens the tier's vault file with the supplied password.
// vault.OpenLocalAt zeroes the password buffer; callers that need to
// reuse the password downstream should copy it first.
func (t vaultTier) Open(pw []byte) (vault.Backend, error) {
	if t.Path == "" {
		return nil, fmt.Errorf("%s tier has no path", t.Name)
	}
	return vault.OpenLocalAt(t.Path, pw)
}

// explicitTier wraps a user-specified path (--vault-path flag or
// FACTORLY_VAULT_PATH env var). It opts out of the per-tier env-var
// chain — only FACTORLY_VAULT_PATH(_PASSWORD) and the canonical
// FACTORLY_VAULT_PASSWORD env vars are consulted — because the user
// pinning a specific path expects "no fallback magic." Identity is
// "explicit:<path>" so logs are searchable.
func explicitTier(path string) vaultTier {
	return vaultTier{
		Name:        "explicit:" + path,
		PromptLabel: fmt.Sprintf("Vault password (%s): ", filepath.Base(path)),
		Path:        path,
		EnvVars:     []string{"FACTORLY_VAULT_PASSWORD"},
		LockedErr:   errGlobalVaultLocked, // shares the "global" locked sentinel
	}
}

// activeTier returns the tier that the current CLI flags/env target
// for a Set/Get operation. Honors --vault-path, --global,
// FACTORLY_VAULT_PATH, --workspace, then falls back to project (when
// inside .factorly/) or global. This is the single source of truth
// for "which tier am I operating on?" — before this existed, the same
// precedence was repeated across resolveVaultPath, openVault, and
// openSmartVault, drifting independently.
//
// Returns the global tier as a final fallback when no other source
// applies. Callers that need to surface validation errors for
// --workspace should do so explicitly before calling this (the tier
// returned for an invalid workspace name has an empty Path).
func activeTier() vaultTier {
	if vaultPath != "" {
		return explicitTier(vaultPath)
	}
	if vaultGlobal {
		return globalTier()
	}
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		return explicitTier(p)
	}
	if workspaceName != "" {
		return workspaceTier(workspaceName)
	}
	if info, err := os.Stat(".factorly"); err == nil && info.IsDir() {
		return projectTier()
	}
	return globalTier()
}
