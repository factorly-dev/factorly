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
// project vault (.factorly/vault.enc if .factorly/ exists) → global vault.
func resolveVaultPath() string {
	if vaultPath != "" {
		return vaultPath
	}
	if vaultGlobal {
		return vault.DefaultVaultPath()
	}
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		return p
	}
	// Default: project vault if .factorly/ directory exists
	if info, err := os.Stat(".factorly"); err == nil && info.IsDir() {
		return projectVaultPath()
	}
	return vault.DefaultVaultPath()
}

func projectVaultPath() string {
	return filepath.Join(".factorly", "vault.enc")
}

func isProjectVault(path string) bool {
	return filepath.Base(filepath.Dir(path)) == ".factorly"
}

// openVault opens the vault at the resolved path.
// When both project and global vaults exist (and no explicit flag),
// returns a FallbackBackend that checks project first, global second.
func openVault() (*vault.LocalBackend, error) {
	path := resolveVaultPath()
	vlog("vault path: %s", path)
	password, err := resolveVaultPassword(path)
	if err != nil {
		return nil, err
	}
	return vault.OpenLocalAt(path, password)
}

// openSmartVault returns a vault backend that searches project vault first,
// then falls back to global. For explicit --global or --vault-path, returns
// a single vault with no fallback.
func openSmartVault() (vault.Backend, error) {
	// Explicit flag = single vault, no fallback
	if vaultPath != "" || vaultGlobal {
		return openVault()
	}
	return openFallbackVault()
}

// openFallbackVault opens the project vault (if it exists) and lazily
// opens the global vault on first fallback. Only prompts for the global
// password when a key isn't found in the project vault.
func openFallbackVault() (vault.Backend, error) {
	projectPath := projectVaultPath()
	globalPath := vault.DefaultVaultPath()
	// Respect FACTORLY_VAULT_PATH for global vault location
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		globalPath = p
	}

	_, projectExists := os.Stat(projectPath)
	_, globalExists := os.Stat(globalPath)

	// Neither exists — create at the best location
	if projectExists != nil && globalExists != nil {
		createPath := resolveVaultPath()
		pw, err := resolveVaultPassword(createPath)
		if err != nil {
			return nil, err
		}
		return vault.OpenLocalAt(createPath, pw)
	}

	// Only global exists
	if projectExists != nil {
		pw, err := resolveVaultPassword(globalPath)
		if err != nil {
			return nil, fmt.Errorf("global vault: %w", err)
		}
		return vault.OpenLocalAt(globalPath, pw)
	}

	// Open project vault
	pw, err := resolveVaultPassword(projectPath)
	if err != nil {
		return nil, fmt.Errorf("project vault: %w", err)
	}
	// Copy password before OpenLocalAt zeroes it — needed for trying on global vault
	pwForGlobal := make([]byte, len(pw))
	copy(pwForGlobal, pw)
	project, err := vault.OpenLocalAt(projectPath, pw)
	if err != nil {
		zeroBytes(pwForGlobal)
		return nil, fmt.Errorf("opening project vault: %w", err)
	}

	// Only project exists
	if globalExists != nil {
		zeroBytes(pwForGlobal)
		return project, nil
	}

	// Both exist — return fallback with lazy global opening.
	// Try the project password first to avoid a second prompt when
	// both vaults share the same password (common case).
	return &vault.FallbackBackend{
		Primary: project,
		SecondaryOpen: func() (vault.Backend, error) {
			vlog("falling back to global vault")
			// Try project password on global vault first
			global, err := vault.OpenLocalAt(globalPath, pwForGlobal)
			if err == nil {
				fmt.Fprintln(os.Stderr, "Global vault opened.")
				return global, nil
			}
			// pwForGlobal is now zeroed by OpenLocalAt (even on failure via deriveKey)
			// Different password — prompt for global
			vlog("project password didn't work for global vault, prompting")
			gpw, err := resolveVaultPassword(globalPath)
			if err != nil {
				return nil, err
			}
			return vault.OpenLocalAt(globalPath, gpw)
		},
	}, nil
}

// resolveVaultPassword resolves the password for a vault at the given path.
// Returns []byte so the caller can zero it after use.
func resolveVaultPassword(path string) ([]byte, error) {
	if isProjectVault(path) {
		return resolveProjectVaultPassword()
	}
	return resolveGlobalVaultPassword()
}

func resolveProjectVaultPassword() ([]byte, error) {
	// 1. Project-specific env var
	if pw, ok := os.LookupEnv("FACTORLY_PROJECT_VAULT_PASSWORD"); ok {
		if pw == "" {
			return nil, fmt.Errorf("FACTORLY_PROJECT_VAULT_PASSWORD is set but empty")
		}
		vlog("project vault password from FACTORLY_PROJECT_VAULT_PASSWORD")
		return []byte(pw), nil
	}

	// 2. Shared env var (convenience — one password for both)
	if pw, ok := os.LookupEnv("FACTORLY_VAULT_PASSWORD"); ok {
		if pw != "" {
			vlog("project vault password from FACTORLY_VAULT_PASSWORD")
			return []byte(pw), nil
		}
	}

	// 3. Project key file (.factorly/vault.key)
	keyFile := filepath.Join(".factorly", "vault.key")
	if pw, err := readKeyFile(keyFile); err == nil {
		vlog("project vault password from %s", keyFile)
		return pw, nil
	}

	// 4. Interactive prompt
	pw, err := promptSecret("Vault password (project): ")
	if err != nil {
		return nil, err
	}
	if len(pw) == 0 {
		return nil, fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
}

func resolveGlobalVaultPassword() ([]byte, error) {
	// 1. Environment variable
	if pw, ok := os.LookupEnv("FACTORLY_VAULT_PASSWORD"); ok {
		if pw == "" {
			return nil, fmt.Errorf("FACTORLY_VAULT_PASSWORD is set but empty")
		}
		vlog("vault password from FACTORLY_VAULT_PASSWORD")
		return []byte(pw), nil
	}

	// 2. Global key file
	home, err := os.UserHomeDir()
	if err == nil {
		keyFile := filepath.Join(home, ".config", "factorly", "vault.key")
		if pw, err := readKeyFile(keyFile); err == nil {
			vlog("vault password from %s", keyFile)
			return pw, nil
		}
	}

	// 3. Interactive prompt
	pw, err := promptSecret("Vault password (global): ")
	if err != nil {
		return nil, err
	}
	if len(pw) == 0 {
		return nil, fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
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

	// Fallback: read from stdin (piped input)
	scanner := bufio.NewScanner(os.Stdin)
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
