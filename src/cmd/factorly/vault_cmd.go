// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/factorly-dev/factorly/internal/workspace"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Process-wide vault manager. Single shared instance owns the cache
// for opened backends and is consumed by the UI server (via Options)
// so CLI-side and UI-side observers always see the same state.
//
// The Manager is lazily constructed on first access. The chain opener
// dispatches on scope-key conventions:
//
//   - ""               → openSmartVault (the startup chain — workspace
//     chain when --workspace is active, else
//     project→global)
//   - "project"        → projectTier().ResolvePassword(false) + Open
//   - "global"         → globalTier().ResolvePassword(false) + Open
//   - "workspace:<n>"  → openWorkspaceChainOrNil(<n>, false), falling
//     through to the cached startup chain when the
//     workspace has no vault file
//
// Non-interactive (`prompt=false`) for project/global/workspace lookups
// because the UI's unlock dialog handles missing-password cases via
// OpenWithPassword. The startup scope ("") is the lone exception — it
// fires on first CLI use and is allowed to prompt because that's the
// expected interactive bootstrap.
var (
	vaultManagerOnce sync.Once
	vaultManager     *vault.Manager
)

func getVaultManager() *vault.Manager {
	vaultManagerOnce.Do(func() {
		vaultManager = vault.NewManager(vaultChainOpener, vaultPasswordOpener)
	})
	return vaultManager
}

// vaultChainOpener resolves a scope to a Backend using the
// non-interactive password chain for every scope except "" (the
// startup chain, where prompting is OK).
func vaultChainOpener(scope string) (vault.Backend, error) {
	switch {
	case scope == "":
		return openSmartVault()
	case scope == "project":
		t := projectTier()
		pw, err := t.ResolvePassword(false)
		if err != nil {
			return nil, err
		}
		defer zeroBytes(pw)
		return t.Open(pw)
	case scope == "global":
		t := globalTier()
		pw, err := t.ResolvePassword(false)
		if err != nil {
			return nil, err
		}
		defer zeroBytes(pw)
		return t.Open(pw)
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		if err := workspace.ValidateName(name); err != nil {
			return nil, err
		}
		if b, err := openWorkspaceChainOrNil(name, false); err != nil {
			return nil, err
		} else if b != nil {
			return b, nil
		}
		// Workspace has no vault file — return the cached startup chain
		// so callers consulting "the active vault" still get something.
		return getVaultManager().GetOrOpen("")
	}
	return nil, fmt.Errorf("vault manager: unknown scope %q", scope)
}

// vaultPasswordOpener opens a tier with an explicit password (UI
// unlock dialog). Scope dispatch mirrors vaultChainOpener.
func vaultPasswordOpener(scope string, password []byte) (vault.Backend, error) {
	switch {
	case scope == "project":
		return projectTier().Open(password)
	case scope == "global":
		return globalTier().Open(password)
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		if err := workspace.ValidateName(name); err != nil {
			return nil, err
		}
		// Same chain shape as OpenWorkspaceVaultWithPassword used to
		// return: workspace primary + project→global fallback.
		wsBackend, err := workspaceTier(name).Open(password)
		if err != nil {
			return nil, err
		}
		return &vault.FallbackBackend{
			Primary: wsBackend,
			SecondaryOpen: func() (vault.Backend, error) {
				return openFallbackVault()
			},
		}, nil
	}
	return nil, fmt.Errorf("vault manager: unknown scope %q for password unlock", scope)
}

// getCachedVault returns the startup vault backend. Thin wrapper for
// callers that don't need to know about the Manager.
func getCachedVault() (vault.Backend, error) {
	return getVaultManager().GetOrOpen("")
}

// getCachedLocalVault returns a *LocalBackend for commands that need
// the concrete type (vault set's Has check, callers using
// LocalBackend-only methods). Separately cached because openVault
// resolves to a single tier (--vault-path / --global / --workspace /
// project default), not the fallback chain that getCachedVault
// returns.
//
// Process-wide single-shot via sync.Once: the active tier is fixed
// by CLI flags, so re-resolving on subsequent calls would yield the
// same answer.
var (
	cachedLocalOnce    sync.Once
	cachedLocalBackend *vault.LocalBackend
	cachedLocalErr     error
)

func getCachedLocalVault() (*vault.LocalBackend, error) {
	cachedLocalOnce.Do(func() {
		cachedLocalBackend, cachedLocalErr = openVault()
	})
	return cachedLocalBackend, cachedLocalErr
}

// logVaultOp logs a vault operation to the JSONL audit trail.
// Never logs secret values — only the operation and key name.
// Uses the shared process-wide logger to maintain hash chain integrity.
func logVaultOp(op string, key string, status string) {
	log := sharedLogger
	if log == nil {
		// Fallback: vault commands can run before bootstrap (e.g., factorly vault set)
		if os.Getenv("FACTORLY_NO_LOG") != "" {
			return
		}
		fallback, err := logger.NewJSONL("")
		if err != nil {
			return
		}
		defer fallback.Close()
		log = fallback
	}

	entry := &logger.Entry{
		Timestamp: time.Now(),
		Interface: "vault",
		Tool:      "vault." + op,
		Status:    status,
	}
	if key != "" {
		entry.Params = map[string]string{"key": key}
	}
	_ = log.Log(entry)
}

var vaultPath string
var vaultGlobal bool
var vaultBackend string

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage encrypted secrets",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return checkCommandAllowed("vault")
	},
}

var vaultSetCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Store a secret in the vault",
	Args:  requireArgs(1, "factorly vault set <key> [value]"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultBackend != "" {
			return fmt.Errorf("backend %q is read-only — manage secrets in %s directly", vaultBackend, vaultBackend)
		}
		backend, err := getCachedLocalVault()
		if err != nil {
			return err
		}

		key := args[0]
		var value string
		if len(args) > 1 {
			value = args[1]
		} else {
			v, err := promptSecret("Value: ")
			if err != nil {
				return err
			}
			value = string(v)
			zeroBytes(v)
		}

		if backend.Has(key) {
			fmt.Fprintf(os.Stderr, "Key %s already exists. Overwrite? (y/n): ", key)
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(scanner.Text())), "y") {
					fmt.Fprintln(os.Stderr, "Aborted.")
					return nil
				}
			}
		}
		if err := backend.Set(key, value); err != nil {
			logVaultOp("set", key, "error")
			return err
		}
		logVaultOp("set", key, "success")
		fmt.Fprintf(os.Stderr, "Stored %s in vault\n", key)
		return nil
	},
}

var vaultGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Retrieve a secret from the vault",
	Args:  requireArgs(1, "factorly vault get <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultBackend != "" {
			return getFromExternalBackend(vaultBackend, args[0])
		}
		backend, err := getCachedVault()
		if err != nil {
			return err
		}

		value, err := backend.Get(args[0])
		if err != nil {
			logVaultOp("get", args[0], "error")
			return err
		}
		logVaultOp("get", args[0], "success")
		fmt.Print(value)
		return nil
	},
}

var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secret names in the vault",
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultBackend != "" {
			return listFromExternalBackend(vaultBackend)
		}
		backend, err := getCachedVault()
		if err != nil {
			return err
		}

		keys, err := backend.List()
		if err != nil {
			logVaultOp("list", "", "error")
			return err
		}
		logVaultOp("list", "", "success")
		for _, k := range keys {
			fmt.Println(k)
		}
		return nil
	},
}

var vaultDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Remove a secret from the vault",
	Args:  requireArgs(1, "factorly vault delete <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultBackend != "" {
			return fmt.Errorf("backend %q is read-only — manage secrets in %s directly", vaultBackend, vaultBackend)
		}
		backend, err := getCachedLocalVault()
		if err != nil {
			return err
		}

		if err := backend.Delete(args[0]); err != nil {
			logVaultOp("delete", args[0], "error")
			return err
		}
		logVaultOp("delete", args[0], "success")
		fmt.Fprintf(os.Stderr, "Deleted %s from vault\n", args[0])
		return nil
	},
}

func projectVaultPath() string {
	return filepath.Join(".factorly", "vault.enc")
}

// workspaceVaultPath returns the path to the encrypted vault file for
// the given workspace. Empty or invalid names yield an empty path —
// the caller treats that as "no workspace, no workspace vault."
//
// Validating here closes a path-traversal seam: without the
// ValidateName check, workspaceVaultPath("../escape") returned a path
// that filepath.Join + os.Stat happily resolved to anywhere on disk.
// All callers that derive a workspace vault path now route through
// here, so the seam is closed at the source.
func workspaceVaultPath(name string) string {
	if workspace.ValidateName(name) != nil {
		return ""
	}
	return filepath.Join(".factorly", "vaults", name+".enc")
}

func isProjectVault(path string) bool {
	return filepath.Base(filepath.Dir(path)) == ".factorly"
}

// isWorkspaceVault reports whether path looks like .factorly/vaults/<name>.enc.
func isWorkspaceVault(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "vaults" &&
		filepath.Base(filepath.Dir(filepath.Dir(path))) == ".factorly"
}

// openVault opens the single tier targeted by the current CLI flags
// (no fallback chain). Used by write operations (vault set, delete)
// where the caller wants to land in exactly one file.
//
// Returns *LocalBackend (rather than vault.Backend) so callers can
// use LocalBackend-only methods like Has() for overwrite prompts.
func openVault() (*vault.LocalBackend, error) {
	// Validate workspace name up front so an invalid --workspace
	// produces a clear error instead of silently falling through to
	// the global vault.
	if workspaceName != "" {
		if err := workspace.ValidateName(workspaceName); err != nil {
			return nil, err
		}
	}
	t := activeTier(currentSelector())
	if t.Path == "" {
		return nil, fmt.Errorf("no vault tier resolved (check --workspace name)")
	}
	vlog("vault path: %s", t.Path)
	pw, err := t.ResolvePassword(true)
	if err != nil {
		return nil, err
	}
	return vault.OpenLocalAt(t.Path, pw)
}

// errProjectVaultLocked / errGlobalVaultLocked signal that no
// non-interactive password source resolved. UI callers detect these
// via the "vault locked" substring (matching errWorkspaceVaultLocked
// for the existing workspace dialog).
var (
	errProjectVaultLocked  = fmt.Errorf("project vault locked: password required")
	errGlobalVaultLocked   = fmt.Errorf("global vault locked: password required")
	errExplicitVaultLocked = fmt.Errorf("explicit vault locked: password required")
)

// openSmartVault returns the chain shape that the current CLI flags
// target. Delegates to activeTier for "which tier?" and to OpenChain
// for "what chain shape does that tier want?" — the same two-step
// decision used by every other vault entry point.
func openSmartVault() (vault.Backend, error) {
	if workspaceName != "" {
		if err := workspace.ValidateName(workspaceName); err != nil {
			return nil, err
		}
	}
	return activeTier(currentSelector()).OpenChain(true)
}

// openWorkspaceChainOrNil opens the workspace-vault tier and nests it on
// top of the existing project→global chain. Returns (nil, nil) if the
// workspace has no vault file (caller falls through to the no-workspace
// chain). Returns an error when the vault file exists but cannot be
// opened (wrong password, etc.).
//
// Shared by openSmartVault and the UI runtime-switch flow.
//
// If prompt is false, this never blocks on user input — when no
// non-interactive password source is available, returns
// errWorkspaceVaultLocked so the caller can surface an unlock dialog
// instead of hanging on stdin.
//
// The workspace password is preserved and passed down to the
// fallback chain: if it also unlocks the project / global vault,
// the user isn't prompted again (common case where one password
// protects all the local vaults).
func openWorkspaceChainOrNil(name string, prompt bool) (vault.Backend, error) {
	if err := workspace.ValidateName(name); err != nil {
		return nil, err
	}
	t := workspaceTier(name)
	if !t.Exists() {
		return nil, nil
	}
	wsPw, err := t.ResolvePassword(prompt)
	if err != nil {
		return nil, err
	}
	// Copy before Open zeroes the password buffer — the copy is used
	// downstream by the fallback chain to attempt the same password
	// against project + global vaults.
	pwForFallback := make([]byte, len(wsPw))
	copy(pwForFallback, wsPw)

	wsBackend, err := t.Open(wsPw)
	if err != nil {
		zeroBytes(pwForFallback)
		return nil, fmt.Errorf("opening workspace vault: %w", err)
	}
	return &vault.FallbackBackend{
		Primary: wsBackend,
		SecondaryOpen: func() (vault.Backend, error) {
			vlog("falling back from workspace vault to project/global chain")
			return openFallbackVaultWithCandidate(pwForFallback)
		},
	}, nil
}

// openFallbackVault opens the project vault (if it exists) and lazily
// opens the global vault on first fallback. Only prompts for the global
// password when a key isn't found in the project vault.
func openFallbackVault() (vault.Backend, error) {
	return openFallbackVaultWithCandidate(nil)
}

// openFallbackVaultWithCandidate is the same as openFallbackVault but
// tries `candidate` (a copy of a password the caller already used on
// another vault tier) before re-resolving / prompting. Lets the
// project↔global chain inherit the workspace password — common case
// is one password protecting every local vault.
//
// The candidate is consumed (zeroed) inside this function or its
// returned closures; callers should not reuse it after the call.
func openFallbackVaultWithCandidate(candidate []byte) (vault.Backend, error) {
	proj := projectTier()
	// Global tier honors FACTORLY_VAULT_PATH transparently: when the
	// env var is set the user is pinning that location, so the chain
	// uses it as the global tier.
	glob := globalTier()
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		glob.Path = p
	}

	projectExists := proj.Exists()
	globalExists := glob.Exists()

	// Neither exists — create at the active tier.
	if !projectExists && !globalExists {
		t := activeTier(currentSelector())
		if pw, ok := tryCandidate(candidate, t); ok {
			return t.Open(pw)
		}
		pw, err := t.ResolvePassword(true)
		if err != nil {
			zeroBytes(candidate)
			return nil, err
		}
		zeroBytes(candidate)
		return t.Open(pw)
	}

	// Only global exists
	if !projectExists {
		b, used, err := openWithCandidateOrPrompt(glob, candidate, "Global vault opened with shared password.")
		if err != nil {
			return nil, fmt.Errorf("global vault: %w", err)
		}
		zeroBytes(used)
		return b, nil
	}

	// Open project vault — try candidate first. Capture the password
	// that actually unlocked it so the global tier can reuse it.
	project, projectPw, err := openWithCandidateOrPrompt(proj, candidate, "Project vault opened with shared password.")
	if err != nil {
		return nil, fmt.Errorf("opening project vault: %w", err)
	}

	// Only project exists
	if !globalExists {
		zeroBytes(projectPw)
		return project, nil
	}

	// Both exist — return fallback with lazy global opening. The
	// password that opened project (whether user-typed or inherited
	// from the workspace) becomes the candidate for global, so a
	// shared password unlocks both tiers with one prompt.
	return &vault.FallbackBackend{
		Primary: project,
		SecondaryOpen: func() (vault.Backend, error) {
			vlog("falling back to global vault")
			b, used, err := openWithCandidateOrPrompt(glob, projectPw, "Global vault opened with shared password.")
			if err != nil {
				return nil, err
			}
			zeroBytes(used)
			return b, nil
		},
	}, nil
}

// tryCandidate checks whether candidate is non-empty and points at a
// tier the caller is about to open. Returns the candidate (caller
// uses it) and true when usable; false otherwise.
func tryCandidate(candidate []byte, t vaultTier) ([]byte, bool) {
	if len(candidate) == 0 {
		return nil, false
	}
	return candidate, t.Exists()
}

// openWithCandidateOrPrompt tries the candidate password against the
// tier's vault file first; on failure (or no candidate), falls
// through to the tier's full ResolvePassword chain (which prompts on
// a TTY). Logs successMsg via vlog when the candidate succeeds.
//
// The second return value is a copy of whatever password unlocked
// the vault — the caller can pass it as the candidate to the next
// tier in the chain so a user who typed their password once doesn't
// get re-prompted for downstream tiers that share it. Caller owns
// the returned slice and should zero it when done.
//
// Taking a vaultTier (not a bare path) means the password-source
// chain is the *tier's* chain — there is no path inspection or
// classification step that could pick the wrong env-var table for an
// absolute path that happens to look like a project vault.
func openWithCandidateOrPrompt(t vaultTier, candidate []byte, successMsg string) (vault.Backend, []byte, error) {
	if len(candidate) > 0 {
		// Make a working copy — Open zeroes its password buffer.
		try := make([]byte, len(candidate))
		copy(try, candidate)
		b, err := t.Open(try)
		if err == nil {
			if successMsg != "" {
				vlog(successMsg)
			}
			used := make([]byte, len(candidate))
			copy(used, candidate)
			zeroBytes(candidate)
			return b, used, nil
		}
		vlog("shared password didn't unlock %s; prompting", t.Path)
		// candidate didn't decrypt — fall through to full resolution.
		zeroBytes(candidate)
	}
	pw, err := t.ResolvePassword(true)
	if err != nil {
		return nil, nil, err
	}
	// Snapshot before Open zeroes pw.
	used := make([]byte, len(pw))
	copy(used, pw)
	b, err := t.Open(pw)
	if err != nil {
		zeroBytes(used)
		return nil, nil, err
	}
	return b, used, nil
}

// errWorkspaceVaultLocked signals that no automatic password source
// was usable for a workspace vault and the caller should ask the user
// (via the UI unlock dialog, or fall through to a prompt). The UI's
// isVaultLocked() unwraps this to detect the case.
var errWorkspaceVaultLocked = fmt.Errorf("workspace vault locked: password required")

// tierForPath classifies a vault file path into a tier descriptor.
//
// This is for UI use only — specifically extractVaultTiers in
// ui_cmd.go, which walks a FallbackBackend chain and asks
// LocalBackend.Path() what file each tier opened. Path provenance is
// trusted at those callsites because the path came from a backend
// the caller just opened.
//
// Do NOT call this from CLI password-resolution paths. Doing so would
// reintroduce the bug that Step 3 of the concession fix-up closed:
// an absolute --vault-path pointing at e.g. /tmp/.factorly/vault.enc
// would silently classify as the project tier and inherit
// FACTORLY_PROJECT_VAULT_PASSWORD lookup. CLI callers should hold a
// vaultTier directly and pass it to openWithCandidateOrPrompt /
// vault.Open / etc.
func tierForPath(path string) vaultTier {
	if isWorkspaceVault(path) {
		// .factorly/vaults/<name>.enc — derive workspace name from filename.
		name := strings.TrimSuffix(filepath.Base(path), ".enc")
		return workspaceTier(name)
	}
	if isProjectVault(path) {
		return projectTier()
	}
	return globalTier()
}

func readKeyFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		return nil, fmt.Errorf("vault key file %s has insecure permissions %04o (must be 0600)", path, perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading vault key file: %w", err)
	}
	pw := bytes.TrimSpace(data)
	if len(pw) == 0 {
		return nil, fmt.Errorf("vault key file %s is empty", path)
	}
	return pw, nil
}

// stdinScanner is the shared scanner for piped-stdin password reads.
// Each promptSecret() call used to create a new bufio.Scanner, which
// buffers ahead — the first call would read line 1 AND swallow line
// 2+ into its internal buffer, then get GC'd. Subsequent calls saw
// empty stdin. A shared scanner reads lines in sequence so multi-
// prompt flows (e.g., workspace + project + global passwords) work
// when stdin is piped.
var stdinScanner *bufio.Scanner

func getStdinScanner() *bufio.Scanner {
	if stdinScanner == nil {
		stdinScanner = bufio.NewScanner(os.Stdin)
	}
	return stdinScanner
}

func promptSecret(label string) ([]byte, error) {
	fmt.Fprint(os.Stderr, label)

	// Try to read without echo from terminal
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		pw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}
		return pw, nil
	}

	// Fallback: read from stdin (piped input). Use a process-wide
	// scanner so multiple promptSecret() calls don't each gobble the
	// rest of stdin into a discarded buffer.
	scanner := getStdinScanner()
	if scanner.Scan() {
		return []byte(strings.TrimSpace(scanner.Text())), nil
	}
	return nil, fmt.Errorf("no input received")
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func getFromExternalBackend(name, key string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	backendCfg, ok := cfg.VaultBackends[name]
	if !ok {
		return fmt.Errorf("unknown vault backend %q — check vault_backends in your config", name)
	}
	backend := vault.NewExternalBackend(name, backendCfg)
	value, err := backend.Get(key)
	if err != nil {
		return err
	}
	fmt.Print(value)
	return nil
}

func listFromExternalBackend(name string) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	backendCfg, ok := cfg.VaultBackends[name]
	if !ok {
		return fmt.Errorf("unknown vault backend %q — check vault_backends in your config", name)
	}
	backend := vault.NewExternalBackend(name, backendCfg)
	keys, err := backend.List()
	if err != nil {
		return err
	}
	for _, k := range keys {
		fmt.Println(k)
	}
	return nil
}

func init() {
	vaultCmd.PersistentFlags().StringVar(&vaultPath, "vault-path", "", "path to vault file (overrides auto-detection)")
	vaultCmd.PersistentFlags().BoolVar(&vaultGlobal, "global", false, "use global vault (~/.config/factorly/vault.enc) instead of project vault")
	vaultCmd.PersistentFlags().StringVar(&vaultBackend, "backend", "", "use an external vault backend (e.g., op, aws, gcp)")
	vaultCmd.AddCommand(vaultSetCmd, vaultGetCmd, vaultListCmd, vaultDeleteCmd)
}

// storeInVault writes a key/value into the local vault, prompting for
// overwrite if the key already exists. Shared by interactive command
// flows that collect credentials (e.g. blueprint install).
func storeInVault(scanner *bufio.Scanner, key, value string) error {
	backend, err := openVault()
	if err != nil {
		return fmt.Errorf("opening vault: %w", err)
	}
	defer backend.Close()

	if backend.Has(key) {
		fmt.Printf("\n  Vault key %s already exists. Overwrite? (y/n): ", key)
		scanner.Scan()
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(scanner.Text())), "y") {
			fmt.Printf("  Keeping existing %s\n", key)
			return nil
		}
	}

	if err := backend.Set(key, value); err != nil {
		return fmt.Errorf("storing in vault: %w", err)
	}
	fmt.Printf("  Stored in vault as %s\n", key)
	return nil
}
