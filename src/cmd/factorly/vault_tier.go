// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
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
	// EnvVars are the ordered list of env-var sources for a non-interactive
	// password. The first non-empty value wins; Strict entries also reject
	// "set but empty" as a user error (Strict=false silently skips empty).
	// Strictness is a field, not a position, so re-ordering this slice
	// can't accidentally flip which env var rejects empty values.
	EnvVars []envSource
	// KeyFile is the optional path to a 0600 file holding the password.
	// Empty string means no keyfile source for this tier.
	KeyFile string
	// LockedErr is returned by ResolvePassword(allowPrompt=false) when
	// no non-interactive source resolves. UI callers detect this to
	// surface the unlock dialog.
	LockedErr error
}

// envSource is one entry in a tier's password-resolution chain.
// Strict=true means a set-but-empty value is rejected as a user error
// (the tier-specific variable). Strict=false means an empty value is
// silently skipped so the chain can continue (the shared convenience
// variable). See vaultTier.EnvVars.
type envSource struct {
	Name   string
	Strict bool
}

// workspaceTier returns the tier descriptor for a named workspace.
// name must be non-empty; callers that allow "no workspace selected"
// should branch before calling this.
func workspaceTier(name string) vaultTier {
	return vaultTier{
		Name:        "workspace:" + name,
		PromptLabel: fmt.Sprintf("Vault password (workspace %q): ", name),
		Path:        workspaceVaultPath(name),
		EnvVars: []envSource{
			{Name: "FACTORLY_WORKSPACE_VAULT_PASSWORD_" + strings.ToUpper(name), Strict: true},
			{Name: "FACTORLY_VAULT_PASSWORD", Strict: false},
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
		EnvVars: []envSource{
			{Name: "FACTORLY_PROJECT_VAULT_PASSWORD", Strict: true},
			{Name: "FACTORLY_VAULT_PASSWORD", Strict: false},
		},
		KeyFile:   filepath.Join(".factorly", "vault.key"),
		LockedErr: errProjectVaultLocked,
	}
}

// globalTier returns the tier descriptor for the global vault. The
// path may be overridden by FACTORLY_VAULT_PATH; that override is
// applied in activeTier (for the chain-selection path) and in
// openFallbackVaultWithCandidate (for the chain-composition path).
func globalTier() vaultTier {
	t := vaultTier{
		Name:        "global",
		PromptLabel: "Vault password (global): ",
		Path:        vault.DefaultVaultPath(),
		EnvVars:     []envSource{{Name: "FACTORLY_VAULT_PASSWORD", Strict: true}},
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
// Strict env entries reject set-but-empty as a user error. Non-strict
// entries (typically the shared FACTORLY_VAULT_PASSWORD convenience
// var) silently skip when empty so clearing the shared var doesn't
// break the tier-specific path.
//
// The returned Secret is owned by the caller — `defer pw.Zero()`.
func (t vaultTier) ResolvePassword(allowPrompt bool) (vault.Secret, error) {
	for _, e := range t.EnvVars {
		pw, ok := os.LookupEnv(e.Name)
		if !ok {
			continue
		}
		if pw == "" {
			if e.Strict {
				return vault.Secret{}, fmt.Errorf("%s is set but empty", e.Name)
			}
			continue
		}
		vlog("%s vault password from %s", t.Name, e.Name)
		return vault.SecretFromString(pw), nil
	}
	if t.KeyFile != "" {
		if pw, err := readKeyFile(t.KeyFile); err == nil {
			vlog("%s vault password from %s", t.Name, t.KeyFile)
			return pw, nil
		}
	}
	if !allowPrompt {
		return vault.Secret{}, t.LockedErr
	}
	pw, err := promptSecret(t.PromptLabel)
	if err != nil {
		return vault.Secret{}, err
	}
	if pw.Empty() {
		return vault.Secret{}, fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
}

// Open opens the tier's vault file with the supplied password. The
// caller owns pw and is responsible for zeroing it (typically via
// `defer pw.Zero()` at the call site). Open does not consume pw —
// callers can reuse the Secret afterward (e.g. clone it for fan-out
// to a chain).
func (t vaultTier) Open(pw vault.Secret) (vault.Backend, error) {
	if t.Path == "" {
		return nil, fmt.Errorf("%s tier has no path", t.Name)
	}
	return vault.OpenLocalAt(t.Path, pw)
}

// vaultPromptAttempts caps the number of interactive password tries
// before the CLI gives up. Matches sudo-style "you get three tries."
// Non-interactive sources (env / keyfile) get one attempt — retrying
// would re-derive the same key from the same source and re-fail.
const vaultPromptAttempts = 3

// ResolveAndOpen ties the prompt loop to the open call so a wrong
// password at the interactive prompt re-prompts up to vaultPromptAttempts
// times before failing. Env-var and key-file sources fail on the
// first wrong password — retrying static sources can't change the
// answer, and silently falling through to prompt would confuse
// scripted callers.
//
// Returns the opened backend AND the Secret that unlocked it. Caller
// owns the Secret and must Zero() it. On failure, no Secret is
// returned (caller has nothing to zero).
//
// allowPrompt mirrors ResolvePassword's flag: pass false for paths
// that must not block on stdin (the UI unlock-dialog source path).
func (t vaultTier) ResolveAndOpen(allowPrompt bool) (vault.Backend, vault.Secret, error) {
	// Phase 1: non-interactive sources. If env or keyfile resolves,
	// we open exactly once. A wrong password from these sources is a
	// surface-it-loudly error — no retry, because retrying the same
	// static value would just re-fail.
	if pw, source, ok, err := t.resolveNonInteractive(); err != nil {
		return nil, vault.Secret{}, err
	} else if ok {
		b, err := t.Open(pw)
		if err != nil {
			pw.Zero()
			if errors.Is(err, vault.ErrWrongPassword) {
				return nil, vault.Secret{}, fmt.Errorf("%s vault: %s did not unlock %s", t.Name, source, t.Path)
			}
			return nil, vault.Secret{}, err
		}
		return b, pw, nil
	}

	if !allowPrompt {
		return nil, vault.Secret{}, t.LockedErr
	}

	// Phase 2: interactive prompt with retry. Each iteration prompts
	// fresh — we never reuse a Secret across attempts.
	var lastErr error
	for attempt := 1; attempt <= vaultPromptAttempts; attempt++ {
		pw, err := promptSecret(t.PromptLabel)
		if err != nil {
			return nil, vault.Secret{}, err
		}
		if pw.Empty() {
			pw.Zero()
			lastErr = fmt.Errorf("vault password cannot be empty")
			fmt.Fprintln(os.Stderr, lastErr)
			continue
		}
		b, err := t.Open(pw)
		if err == nil {
			return b, pw, nil
		}
		pw.Zero()
		lastErr = err
		if !errors.Is(err, vault.ErrWrongPassword) {
			// Non-password failure (corrupt file, I/O). Fail hard;
			// the next prompt isn't going to fix it.
			return nil, vault.Secret{}, err
		}
		remaining := vaultPromptAttempts - attempt
		if remaining > 0 {
			fmt.Fprintf(os.Stderr, "Incorrect password, try again (%d %s left).\n",
				remaining, plural(remaining, "attempt", "attempts"))
		}
	}
	return nil, vault.Secret{}, fmt.Errorf("vault unlock failed after %d attempts: %w", vaultPromptAttempts, lastErr)
}

// resolveNonInteractive checks env vars and key file (in that order)
// and returns (secret, source-name, found, error). Splits the
// non-prompt half of ResolvePassword out so ResolveAndOpen can
// branch on the password source. We can't just call ResolvePassword
// because we need to know which source the password came from.
func (t vaultTier) resolveNonInteractive() (vault.Secret, string, bool, error) {
	for _, e := range t.EnvVars {
		pw, ok := os.LookupEnv(e.Name)
		if !ok {
			continue
		}
		if pw == "" {
			if e.Strict {
				return vault.Secret{}, "", false, fmt.Errorf("%s is set but empty", e.Name)
			}
			continue
		}
		vlog("%s vault password from %s", t.Name, e.Name)
		return vault.SecretFromString(pw), e.Name, true, nil
	}
	if t.KeyFile != "" {
		if pw, err := readKeyFile(t.KeyFile); err == nil {
			vlog("%s vault password from %s", t.Name, t.KeyFile)
			return pw, t.KeyFile, true, nil
		}
	}
	return vault.Secret{}, "", false, nil
}

// plural picks the singular or plural form based on n. Tiny helper —
// "1 attempt left" vs "2 attempts left" reads more naturally than
// "1 attempts left."
func plural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// OpenChain opens the tier and returns the right chain shape for it.
//
//   - explicit tiers (--vault-path / FACTORLY_VAULT_PATH) → single
//     vault, no fallback. The user pinned a path; honoring that pin is
//     the entire point.
//   - workspace tier → workspace vault as Primary with a lazy
//     project→global fallback Secondary; or the bare project→global
//     chain when the workspace has no vault file.
//   - project / global tier → the lazy project→global FallbackBackend.
//
// This is the chain-shape decision that openSmartVault used to make
// inline. Routing through tier identity means the decision is colocated
// with the tier descriptor, not duplicated across helpers.
//
// allowPrompt is forwarded to ResolvePassword for the leading tier;
// non-interactive callers (the UI) pass false so a locked vault
// returns LockedErr instead of blocking on stdin.
func (t vaultTier) OpenChain(allowPrompt bool) (vault.Backend, error) {
	switch {
	case strings.HasPrefix(t.Name, "explicit:"):
		// Single vault, no chain. Honors the user's pin exactly.
		// ResolveAndOpen handles the prompt-retry-on-wrong-password
		// loop for interactive callers.
		b, pw, err := t.ResolveAndOpen(allowPrompt)
		if err != nil {
			return nil, err
		}
		pw.Zero()
		return b, nil
	case strings.HasPrefix(t.Name, "workspace:"):
		// Workspace name lives in t.Name after the prefix.
		name := strings.TrimPrefix(t.Name, "workspace:")
		if b, err := openWorkspaceChainOrNil(name, allowPrompt); err != nil {
			return nil, err
		} else if b != nil {
			return b, nil
		}
		// Workspace has no vault file — fall through to project/global chain.
		return openFallbackVault()
	default:
		// project / global / anything else → the lazy project→global chain.
		return openFallbackVault()
	}
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
		EnvVars:     []envSource{{Name: "FACTORLY_VAULT_PASSWORD", Strict: true}},
		LockedErr:   errExplicitVaultLocked,
	}
}

// tierSelector is the input record for activeTier — the package-level
// inputs that determine which tier the current CLI invocation targets.
// Production code calls activeTier(currentSelector()); tests construct
// a selector directly so they don't have to mutate package globals.
//
// EnvVaultPath captures the value of FACTORLY_VAULT_PATH at the time
// the selector was built. Putting it in the struct (rather than
// re-reading inside activeTier) makes the function pure and the
// precedence rule entirely captured by the selector value.
type tierSelector struct {
	VaultPath     string
	VaultGlobal   bool
	WorkspaceName string
	EnvVaultPath  string
	// StoreGlobal pins store operations to the global tier
	// (~/.config/factorly/store.db). Consumed by activeStoreTier
	// only; activeTier ignores it.
	StoreGlobal bool
}

// currentSelector snapshots the package-level flag and env state.
// All production callers of activeTier use this — it's the seam
// between the package-global CLI state and the pure precedence
// function.
func currentSelector() tierSelector {
	return tierSelector{
		VaultPath:     vaultPath,
		VaultGlobal:   vaultGlobal,
		WorkspaceName: workspaceName,
		EnvVaultPath:  os.Getenv("FACTORLY_VAULT_PATH"),
		StoreGlobal:   storeGlobal,
	}
}

// activeTier returns the tier that the supplied selector targets for
// a single-vault operation. Honors --vault-path, --global,
// FACTORLY_VAULT_PATH, --workspace, then falls back to project (when
// inside .factorly/) or global. This is the single source of truth
// for "which tier am I operating on?" — before this existed, the same
// precedence was repeated across openVault, openSmartVault, and the
// per-tier path builders, drifting independently.
//
// activeTier is pure with respect to its inputs (the selector), with
// one exception: the "project default" branch consults os.Stat to see
// if a .factorly/ directory exists in cwd. That's an unavoidable
// filesystem dependency — the precedence rule itself requires
// "project tier wins when we're inside a project."
//
// Returns the global tier as a final fallback when no other source
// applies. Callers that need to surface validation errors for
// --workspace should do so explicitly before calling this (the tier
// returned for an invalid workspace name has an empty Path).
func activeTier(s tierSelector) vaultTier {
	if s.VaultPath != "" {
		return explicitTier(s.VaultPath)
	}
	if s.VaultGlobal {
		// --global means "use the global vault, no fallback chain."
		// Wrap as explicit so OpenChain takes the no-fallback branch.
		// The path is the same as globalTier(), but identity is
		// explicit so the chain composition stays single-vault.
		return explicitTier(vault.DefaultVaultPath())
	}
	if s.WorkspaceName != "" {
		return workspaceTier(s.WorkspaceName)
	}
	if info, err := os.Stat(".factorly"); err == nil && info.IsDir() {
		return projectTier()
	}
	// FACTORLY_VAULT_PATH was deliberately ranked lower than --global,
	// --workspace, and the project default: it's a "set the global
	// vault location" knob, not an "explicit pin." Reads done through
	// OpenChain still consult the project tier first when one exists;
	// the env var only changes where the global tier lives in the
	// chain. (openFallbackVaultWithCandidate consults the env directly
	// for the same reason.)
	if s.EnvVaultPath != "" {
		// Build a global tier with overridden path. No explicit-tier
		// wrap: we still want chain semantics, just with a custom
		// global path.
		g := globalTier()
		g.Path = s.EnvVaultPath
		return g
	}
	return globalTier()
}
