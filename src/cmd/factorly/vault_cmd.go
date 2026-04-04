package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var vaultPath string

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage encrypted secrets",
}

var vaultSetCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Store a secret in the vault",
	Args:  cobra.RangeArgs(1, 2),
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

		if err := backend.Set(key, value); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Stored %s in vault\n", key)
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
	Args:  cobra.ExactArgs(1),
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

// openVault resolves the vault password and opens the local backend.
// Password sources (in order): FACTORLY_VAULT_PASSWORD env var,
// ~/.config/factorly/vault.key file, interactive prompt.
func resolveVaultPath() string {
	if vaultPath != "" {
		return vaultPath
	}
	if p := os.Getenv("FACTORLY_VAULT_PATH"); p != "" {
		return p
	}
	return vault.DefaultVaultPath()
}

func openVault() (*vault.LocalBackend, error) {
	password, err := resolveVaultPassword()
	if err != nil {
		return nil, err
	}
	path := resolveVaultPath()
	vlog("vault path: %s", path)
	return vault.OpenLocalAt(path, password)
}

func resolveVaultPassword() (string, error) {
	// 1. Environment variable
	if pw := os.Getenv("FACTORLY_VAULT_PASSWORD"); pw != "" {
		vlog("vault password from FACTORLY_VAULT_PASSWORD")
		return pw, nil
	}

	// 2. Key file
	home, err := os.UserHomeDir()
	if err == nil {
		keyFile := home + "/.config/factorly/vault.key"
		if data, err := os.ReadFile(keyFile); err == nil {
			vlog("vault password from %s", keyFile)
			return strings.TrimSpace(string(data)), nil
		}
	}

	// 3. Interactive prompt
	return promptSecret("Vault password: ")
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
	vaultCmd.PersistentFlags().StringVar(&vaultPath, "vault-path", "", "path to vault file (default: ~/.config/factorly/vault.enc)")
	vaultCmd.AddCommand(vaultSetCmd, vaultListCmd, vaultDeleteCmd)
}
