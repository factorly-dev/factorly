// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/factorly-dev/factorly/internal/workspace"
)

// storeTier is the per-scope descriptor for the bbolt-backed
// agent-writable store. It mirrors vaultTier's shape minus the
// password machinery (no EnvVars, no KeyFile, no LockedErr) —
// store has no encryption, no locked state, no per-tier credential
// resolution. Just identity + on-disk path.
//
// One file per scope (parallel to vault) gives us cleanly-isolated
// concurrency boundaries: bbolt's exclusive file lock per scope
// means project writes and workspace writes never contend with
// each other, and backing up a single workspace is one file.
type storeTier struct {
	// Name is the stable identity used in logs and audit entries.
	// "project", "workspace:<n>", or "global".
	Name string
	// Path is the on-disk location of the bbolt database file.
	Path string
}

// Exists reports whether the tier's database file is on disk yet.
// A non-existent file isn't an error — bbolt will create it on
// first write, and reads of a missing tier are simply empty.
// Callers that care about "has anyone written here yet?" use this.
func (t storeTier) Exists() bool {
	if t.Path == "" {
		return false
	}
	_, err := os.Stat(t.Path)
	return err == nil
}

// projectStoreTier returns the descriptor for the project-scoped
// store at .factorly/store.db. This is the default destination
// when no --workspace flag is active and the cwd contains a
// .factorly/ directory.
func projectStoreTier() storeTier {
	return storeTier{
		Name: "project",
		Path: projectStorePath(),
	}
}

// workspaceStoreTier returns the descriptor for a named workspace's
// store file. Name validation matches the vault pattern — invalid
// names yield an empty Path so the caller treats "no store for this
// workspace" as the failure mode rather than scribbling outside
// .factorly/.
func workspaceStoreTier(name string) storeTier {
	return storeTier{
		Name: "workspace:" + name,
		Path: workspaceStorePath(name),
	}
}

// globalStoreTier returns the descriptor for ~/.config/factorly/store.db.
// Used when no project / workspace is active, mirroring how the global
// vault is the final fallback for credentials.
func globalStoreTier() storeTier {
	t := storeTier{Name: "global"}
	if home, err := os.UserHomeDir(); err == nil {
		t.Path = filepath.Join(home, ".config", "factorly", "store.db")
	}
	return t
}

// projectStorePath returns the canonical on-disk path for the
// project-scoped store. Lives at .factorly/store.db alongside the
// project vault file.
func projectStorePath() string {
	return filepath.Join(".factorly", "store.db")
}

// workspaceStorePath returns the path for a named workspace's
// store. Empty or invalid names yield an empty path — the caller
// treats that as "no workspace store possible," matching the
// vault's path-builder contract.
//
// Path traversal prevention is delegated to workspace.ValidateName,
// the same gate used for workspace vault paths and workspace YAML
// files.
func workspaceStorePath(name string) string {
	if workspace.ValidateName(name) != nil {
		return ""
	}
	return filepath.Join(".factorly", "workspaces", name, "store.db")
}

// activeStoreTier picks the store tier the current CLI invocation
// targets, mirroring vault's activeTier precedence:
//
//   - --global wins when set (pin to global store)
//   - else --workspace when set
//   - else project default (when .factorly/ exists in cwd)
//   - else global (~/.config/factorly/store.db)
//
// Reuses the vault's tierSelector struct so --workspace /
// FACTORLY_WORKSPACE precedence is identical across vault and store.
// The VaultPath / VaultGlobal / EnvVaultPath fields of the selector
// are ignored — they're vault-specific.
func activeStoreTier(s tierSelector) storeTier {
	if s.StoreGlobal {
		return globalStoreTier()
	}
	if s.WorkspaceName != "" {
		return workspaceStoreTier(s.WorkspaceName)
	}
	if info, err := os.Stat(".factorly"); err == nil && info.IsDir() {
		return projectStoreTier()
	}
	return globalStoreTier()
}

// validateActiveStoreName surfaces a clear error when --workspace is
// set to an invalid name (path traversal etc.), instead of silently
// falling through to the global tier. Called by command handlers
// before any open attempt.
func validateActiveStoreName() error {
	if workspaceName == "" {
		return nil
	}
	if err := workspace.ValidateName(workspaceName); err != nil {
		return fmt.Errorf("--workspace %q: %w", workspaceName, err)
	}
	return nil
}
