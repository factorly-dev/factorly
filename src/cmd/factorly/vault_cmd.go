// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var vaultPath string
var vaultGlobal bool

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
		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		key := args[0]
		var value string
		if len(args) > 1 {
			value = args[1]
		} else {
			v, err := promptSecret("Value: ")
			if err != nil {
				return err
			}
			value = v
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
			return err
		}
		fmt.Fprintf(os.Stderr, "Stored %s in vault\n", key)
		return nil
	},
}

var vaultGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Retrieve a secret from the vault",
	Args:  requireArgs(1, "factorly vault get <key>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := openSmartVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		value, err := backend.Get(args[0])
		if err != nil {
			return err
		}
		fmt.Print(value)
		return nil
	},
}

var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secret names in the vault",
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		keys, err := backend.List()
		if err != nil {
			return err
		}
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
		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		if err := backend.Delete(args[0]); err != nil {
			return err
		}
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

	// Neither exists
	if projectExists != nil && globalExists != nil {
		return nil, fmt.Errorf("no vault found (run 'factorly vault set' to create one)")
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
	project, err := vault.OpenLocalAt(projectPath, pw)
	if err != nil {
		return nil, fmt.Errorf("opening project vault: %w", err)
	}

	// Only project exists
	if globalExists != nil {
		return project, nil
	}

	// Both exist — return fallback with lazy global opening
	return &vault.FallbackBackend{
		Primary: project,
		SecondaryOpen: func() (vault.Backend, error) {
			vlog("falling back to global vault")
			gpw, err := resolveVaultPassword(globalPath)
			if err != nil {
				return nil, err
			}
			return vault.OpenLocalAt(globalPath, gpw)
		},
	}, nil
}

// resolveVaultPassword resolves the password for a vault at the given path.
// Project vaults use FACTORLY_PROJECT_VAULT_PASSWORD and .factorly/vault.key.
// Global vaults use FACTORLY_VAULT_PASSWORD and ~/.config/factorly/vault.key.
func resolveVaultPassword(path string) (string, error) {
	if isProjectVault(path) {
		return resolveProjectVaultPassword()
	}
	return resolveGlobalVaultPassword()
}

func resolveProjectVaultPassword() (string, error) {
	// 1. Project-specific env var
	if pw, ok := os.LookupEnv("FACTORLY_PROJECT_VAULT_PASSWORD"); ok {
		if pw == "" {
			return "", fmt.Errorf("FACTORLY_PROJECT_VAULT_PASSWORD is set but empty")
		}
		vlog("project vault password from FACTORLY_PROJECT_VAULT_PASSWORD")
		return pw, nil
	}

	// 2. Shared env var (convenience — one password for both)
	if pw, ok := os.LookupEnv("FACTORLY_VAULT_PASSWORD"); ok {
		if pw != "" {
			vlog("project vault password from FACTORLY_VAULT_PASSWORD")
			return pw, nil
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
		return "", err
	}
	if pw == "" {
		return "", fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
}

func resolveGlobalVaultPassword() (string, error) {
	// 1. Environment variable
	if pw, ok := os.LookupEnv("FACTORLY_VAULT_PASSWORD"); ok {
		if pw == "" {
			return "", fmt.Errorf("FACTORLY_VAULT_PASSWORD is set but empty")
		}
		vlog("vault password from FACTORLY_VAULT_PASSWORD")
		return pw, nil
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
		return "", err
	}
	if pw == "" {
		return "", fmt.Errorf("vault password cannot be empty")
	}
	return pw, nil
}

func readKeyFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		return "", fmt.Errorf("vault key file %s has insecure permissions %04o (must be 0600)", path, perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading vault key file: %w", err)
	}
	pw := strings.TrimSpace(string(data))
	if pw == "" {
		return "", fmt.Errorf("vault key file %s is empty", path)
	}
	return pw, nil
}

func promptSecret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)

	// Try to read without echo from terminal
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		pw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return "", fmt.Errorf("reading password: %w", err)
		}
		return string(pw), nil
	}

	// Fallback: read from stdin (piped input)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	return "", fmt.Errorf("no input received")
}

func init() {
	vaultCmd.PersistentFlags().StringVar(&vaultPath, "vault-path", "", "path to vault file (overrides auto-detection)")
	vaultCmd.PersistentFlags().BoolVar(&vaultGlobal, "global", false, "use global vault (~/.config/factorly/vault.enc) instead of project vault")
	vaultCmd.AddCommand(vaultSetCmd, vaultGetCmd, vaultListCmd, vaultDeleteCmd)
}
