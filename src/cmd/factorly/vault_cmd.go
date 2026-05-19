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

// Process-wide vault cache — first open prompts for password;
// subsequent opens within the same process reuse the backend.
var (
	cachedVaultOnce    sync.Once
	cachedVaultBackend vault.Backend
	cachedVaultErr     error
	cachedLocalOnce    sync.Once
	cachedLocalBackend *vault.LocalBackend
	cachedLocalErr     error
)

// getCachedVault returns a cached Backend (may be FallbackBackend).
// First call opens the vault normally; subsequent calls reuse it.
func getCachedVault() (vault.Backend, error) {
	cachedVaultOnce.Do(func() {
		cachedVaultBackend, cachedVaultErr = openSmartVault()
	})
	return cachedVaultBackend, cachedVaultErr
}

// getCachedLocalVault returns a cached *LocalBackend.
// For commands that need the concrete type (vault set with Has check).
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

// resolveVaultPath determines which vault file to use.
// Priority: --vault-path flag → --global flag → FACTORLY_VAULT_PATH env →
// --workspace (.factorly/vaults/<name>.enc) → project vault → global vault.
//
// Delegates entirely to activeTier so there's exactly one source of
// truth for the precedence. Returns an empty string when --workspace
// is set to an invalid name (path traversal etc.) — callers translate
// that into a hard error so the user doesn't silently fall through to
// the global vault.
func resolveVaultPath() string {
	return activeTier().Path
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
	t := activeTier()
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

// OpenProjectVault opens (or creates) .factorly/vault.enc using only
// non-interactive password sources (env vars, keyfile). Returns
// errProjectVaultLocked if no password is available — UI callers
// translate that into an unlock dialog. Used by the UI's vault page
// so a project secret can be stored even when the project vault
// didn't exist at server startup.
func OpenProjectVault() (vault.Backend, error) {
	t := projectTier()
	pw, err := t.ResolvePassword(false)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(pw)
	return t.Open(pw)
}

// OpenProjectVaultWithPassword opens .factorly/vault.enc using an
// explicit password (bypassing env-var/keyfile resolution). Used by
// the UI when the user enters a password through the unlock dialog.
func OpenProjectVaultWithPassword(password []byte) (vault.Backend, error) {
	return projectTier().Open(password)
}

// OpenGlobalVault opens (or creates) ~/.config/factorly/vault.enc with
// non-interactive password resolution.
func OpenGlobalVault() (vault.Backend, error) {
	t := globalTier()
	pw, err := t.ResolvePassword(false)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(pw)
	return t.Open(pw)
}

// OpenGlobalVaultWithPassword opens ~/.config/factorly/vault.enc with
// an explicit password.
func OpenGlobalVaultWithPassword(password []byte) (vault.Backend, error) {
	return globalTier().Open(password)
}

// errProjectVaultLocked / errGlobalVaultLocked signal that no
// non-interactive password source resolved. UI callers detect these
// via the "vault locked" substring (matching errWorkspaceVaultLocked
// for the existing workspace dialog).
var (
	errProjectVaultLocked = fmt.Errorf("project vault locked: password required")
	errGlobalVaultLocked  = fmt.Errorf("global vault locked: password required")
)

// tryResolveProjectVaultPassword mirrors resolveProjectVaultPassword
// but skips the interactive prompt. Returns errProjectVaultLocked
// when no non-interactive source resolves.
func tryResolveProjectVaultPassword() ([]byte, error) {
	return projectTier().ResolvePassword(false)
}

// tryResolveGlobalVaultPassword mirrors resolveGlobalVaultPassword
// without the interactive prompt.
func tryResolveGlobalVaultPassword() ([]byte, error) {
	return globalTier().ResolvePassword(false)
}

// openSmartVault returns a vault backend that searches project vault first,
// then falls back to global. For explicit --global or --vault-path, returns
// a single vault with no fallback.
func openSmartVault() (vault.Backend, error) {
	// Explicit flag = single vault, no fallback
	if vaultPath != "" || vaultGlobal {
		return openVault()
	}
	if workspaceName != "" {
		if b, err := openWorkspaceChainOrNil(workspaceName, true); err != nil {
			return nil, err
		} else if b != nil {
			return b, nil
		}
	}
	return openFallbackVault()
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

// OpenWorkspaceVaultWithPassword opens the workspace vault file at
// .factorly/vaults/<name>.enc using the supplied password and chains it
// to the project→global fallback. Used by the UI when the user enters
// a password through the unlock dialog (bypassing env-var/keyfile/prompt
// resolution).
func OpenWorkspaceVaultWithPassword(name string, password []byte) (vault.Backend, error) {
	if err := workspace.ValidateName(name); err != nil {
		return nil, err
	}
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

// OpenWorkspaceVaultUpsert opens (or creates) the workspace vault at
// .factorly/vaults/<name>.enc using only non-interactive password
// sources. Returns errWorkspaceVaultLocked if no password is
// available — caller should surface the UI unlock dialog instead of
// hanging. Unlike OpenWorkspaceChain, this returns the workspace
// vault *alone* (no FallbackBackend wrapping), so a Set writes
// directly to vaults/<name>.enc without falling through to the
// project vault.
func OpenWorkspaceVaultUpsert(name string) (vault.Backend, error) {
	if err := workspace.ValidateName(name); err != nil {
		return nil, err
	}
	t := workspaceTier(name)
	pw, err := t.ResolvePassword(false)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(pw)
	return t.Open(pw)
}

// OpenWorkspaceChain is the UI-facing entry point for opening the
// vault chain associated with a named workspace. Empty name returns
// the no-workspace chain (cached project → global, set up at server
// startup). Never blocks on stdin — when a workspace vault exists
// but no non-interactive password source is available, returns
// errWorkspaceVaultLocked so the UI surfaces its unlock dialog.
func OpenWorkspaceChain(name string) (vault.Backend, error) {
	if name == "" {
		// Reuse the process-wide vault opened at startup so the UI
		// never re-prompts for the project/global vault password.
		return getCachedVault()
	}
	if b, err := openWorkspaceChainOrNil(name, false); err != nil {
		return nil, err
	} else if b != nil {
		return b, nil
	}
	// Workspace has no vault file — reuse the cached project/global chain.
	return getCachedVault()
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
	projectPath := projectTier().Path
	// Global tier honors FACTORLY_VAULT_PATH transparently: when the
	// env var is set the user is pinning that location, so the chain
	// uses it as the global tier. Reading once here keeps the
	// precedence rule centralized in activeTier/explicitTier rather
	// than spread across multiple helpers.
	globalPath := vault.DefaultVaultPath()
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		globalPath = p
	}

	_, projectExists := os.Stat(projectPath)
	_, globalExists := os.Stat(globalPath)

	// Neither exists — create at the best location
	if projectExists != nil && globalExists != nil {
		createPath := resolveVaultPath()
		if pw, ok := tryCandidate(candidate, createPath); ok {
			return vault.OpenLocalAt(createPath, pw)
		}
		pw, err := resolveVaultPassword(createPath)
		if err != nil {
			zeroBytes(candidate)
			return nil, err
		}
		zeroBytes(candidate)
		return vault.OpenLocalAt(createPath, pw)
	}

	// Only global exists
	if projectExists != nil {
		b, used, err := openWithCandidateOrPrompt(globalPath, candidate, "Global vault opened with shared password.")
		if err != nil {
			return nil, fmt.Errorf("global vault: %w", err)
		}
		zeroBytes(used)
		return b, nil
	}

	// Open project vault — try candidate first. Capture the password
	// that actually unlocked it so the global tier can reuse it.
	project, projectPw, err := openWithCandidateOrPrompt(projectPath, candidate, "Project vault opened with shared password.")
	if err != nil {
		return nil, fmt.Errorf("opening project vault: %w", err)
	}

	// Only project exists
	if globalExists != nil {
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
			b, used, err := openWithCandidateOrPrompt(globalPath, projectPw, "Global vault opened with shared password.")
			if err != nil {
				return nil, err
			}
			zeroBytes(used)
			return b, nil
		},
	}, nil
}

// tryCandidate checks whether candidate is non-empty and points at a
// path the caller is about to open. Returns the candidate (caller
// uses it) and true when usable; false otherwise.
func tryCandidate(candidate []byte, path string) ([]byte, bool) {
	if len(candidate) == 0 {
		return nil, false
	}
	_, err := os.Stat(path)
	return candidate, err == nil
}

// openWithCandidateOrPrompt tries the candidate password first; on
// failure (or when there is no candidate), falls through to the full
// resolveVaultPassword chain (which prompts on a TTY). Logs `successMsg`
// to stderr when the candidate succeeded, so the user knows their
// initial password was reused.
//
// The second return value is a copy of whatever password ended up
// unlocking the vault — caller can pass it as the candidate to the
// next tier in the chain so a user who typed their password once
// doesn't get re-prompted for downstream tiers that happen to share
// it. Caller owns the returned slice and should zero it when done.
func openWithCandidateOrPrompt(path string, candidate []byte, successMsg string) (vault.Backend, []byte, error) {
	if len(candidate) > 0 {
		// Make a working copy — OpenLocalAt zeroes its password buffer.
		try := make([]byte, len(candidate))
		copy(try, candidate)
		b, err := vault.OpenLocalAt(path, try)
		if err == nil {
			if successMsg != "" {
				vlog(successMsg)
			}
			used := make([]byte, len(candidate))
			copy(used, candidate)
			zeroBytes(candidate)
			return b, used, nil
		}
		vlog("shared password didn't unlock %s; prompting", path)
		// candidate didn't decrypt — fall through to full resolution.
		zeroBytes(candidate)
	}
	pw, err := resolveVaultPassword(path)
	if err != nil {
		return nil, nil, err
	}
	// Snapshot before OpenLocalAt zeroes pw.
	used := make([]byte, len(pw))
	copy(used, pw)
	b, err := vault.OpenLocalAt(path, pw)
	if err != nil {
		zeroBytes(used)
		return nil, nil, err
	}
	return b, used, nil
}

// resolveVaultPassword resolves the password for a vault at the given path
// by classifying the path into a tier and delegating to vaultTier.ResolvePassword.
// Returns []byte so the caller can zero it after use.
func resolveVaultPassword(path string) ([]byte, error) {
	return tierForPath(path).ResolvePassword(true)
}

// errWorkspaceVaultLocked signals that no automatic password source
// was usable for a workspace vault and the caller should ask the user
// (via the UI unlock dialog, or fall through to a prompt). The UI's
// isVaultLocked() unwraps this to detect the case.
var errWorkspaceVaultLocked = fmt.Errorf("workspace vault locked: password required")

// tierForPath classifies a vault path into a vaultTier. Used by
// resolveVaultPassword to pick the right password-source table for
// the file the caller is about to open.
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

// resolveWorkspaceVaultPassword is the prompting variant. Preserved as a
// thin wrapper for callsites that explicitly target a workspace tier.
func resolveWorkspaceVaultPassword(name string) ([]byte, error) {
	return workspaceTier(name).ResolvePassword(true)
}

// tryResolveWorkspaceVaultPassword is the non-interactive variant.
func tryResolveWorkspaceVaultPassword(name string) ([]byte, error) {
	return workspaceTier(name).ResolvePassword(false)
}

func resolveProjectVaultPassword() ([]byte, error) {
	return projectTier().ResolvePassword(true)
}

func resolveGlobalVaultPassword() ([]byte, error) {
	return globalTier().ResolvePassword(true)
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
