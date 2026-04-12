package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/oauth"
	"github.com/factorly-dev/factorly-cli/internal/vault"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage OAuth authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login <provider>",
	Short: "Authenticate with an OAuth provider (opens browser)",
	Args:  requireArgs(1, "factorly auth login <provider>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]

		cfg, _, err := loadConfig()
		if err != nil {
			return err
		}

		oauthCfg, tokenKey, err := resolveOAuthForProvider(cfg, providerName)
		if err != nil {
			return err
		}

		// Open vault to resolve refs in client_id/secret and to store tokens
		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		resolver := vault.NewResolver()
		resolver.Register("vault", backend)

		providerConfig := oauth.ProviderConfig{
			ClientID:     resolveVaultRef(resolver, oauthCfg.ClientID),
			ClientSecret: resolveVaultRef(resolver, oauthCfg.ClientSecret),
			AuthURL:      oauthCfg.AuthURL,
			TokenURL:     oauthCfg.TokenURL,
			Scopes:       oauthCfg.Scopes,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		bundle, err := oauth.LoginFlow(ctx, providerConfig)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		data, err := json.Marshal(bundle)
		if err != nil {
			return fmt.Errorf("encoding token bundle: %w", err)
		}

		if err := backend.Set(tokenKey, string(data)); err != nil {
			return fmt.Errorf("storing token: %w", err)
		}

		if !bundle.Expiry.IsZero() {
			remaining := time.Until(bundle.Expiry).Round(time.Minute)
			fmt.Fprintf(os.Stderr, "Authenticated with %s (expires in %s)\n", providerName, remaining)
		} else {
			fmt.Fprintf(os.Stderr, "Authenticated with %s\n", providerName)
		}
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status [provider]",
	Short: "Show OAuth token status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _, err := loadConfig()
		if err != nil {
			return err
		}

		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		// Collect all OAuth token keys from config
		tokenKeys := collectOAuthTokenKeys(cfg)
		if len(args) > 0 {
			// Filter to specific provider
			key := args[0] + "_oauth"
			if _, ok := tokenKeys[key]; !ok {
				// Try the arg as a direct token key
				key = args[0]
			}
			filtered := map[string]string{key: tokenKeys[key]}
			tokenKeys = filtered
		}

		for key, providerName := range tokenKeys {
			raw, err := backend.Get(key)
			if err != nil {
				fmt.Printf("%-20s  ✗ not authenticated (run: factorly auth login %s)\n", key, providerName)
				continue
			}
			var bundle oauth.TokenBundle
			if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
				fmt.Printf("%-20s  ✗ invalid token data\n", key)
				continue
			}
			if bundle.IsExpired(0) {
				fmt.Printf("%-20s  ✗ expired (run: factorly auth login %s)\n", key, providerName)
			} else if !bundle.Expiry.IsZero() {
				remaining := time.Until(bundle.Expiry).Round(time.Minute)
				fmt.Printf("%-20s  ✓ valid (expires in %s)\n", key, remaining)
			} else {
				fmt.Printf("%-20s  ✓ valid (no expiry)\n", key)
			}
		}
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout <provider>",
	Short: "Remove stored OAuth tokens",
	Args:  requireArgs(1, "factorly auth logout <provider>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerName := args[0]

		cfg, _, err := loadConfig()
		if err != nil {
			return err
		}

		_, tokenKey, err := resolveOAuthForProvider(cfg, providerName)
		if err != nil {
			return err
		}

		backend, err := openVault()
		if err != nil {
			return err
		}
		defer backend.Close()

		if err := backend.Delete(tokenKey); err != nil {
			return fmt.Errorf("removing token: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Logged out of %s (removed %s)\n", providerName, tokenKey)
		return nil
	},
}

// resolveOAuthForProvider finds the OAuth config and token key for a provider name.
// Checks oauth_providers first, then scans tools for inline OAuth with matching provider/token_key.
func resolveOAuthForProvider(cfg *config.Config, name string) (*config.OAuthProviderConfig, string, error) {
	// Check oauth_providers section
	if cfg.OAuthProviders != nil {
		if p, ok := cfg.OAuthProviders[name]; ok {
			return &p, name + "_oauth", nil
		}
	}

	// Scan tools for inline OAuth with matching provider or token_key
	for _, tool := range cfg.Tools {
		if tool.Auth == nil || tool.Auth.Type != "oauth" {
			continue
		}
		if tool.Auth.Provider == name {
			resolved := cfg.ResolveOAuthProvider(tool.Auth)
			return resolved, config.OAuthTokenKey(tool.Auth), nil
		}
		tokenKey := config.OAuthTokenKey(tool.Auth)
		if tokenKey == name+"_oauth" || tokenKey == name {
			resolved := cfg.ResolveOAuthProvider(tool.Auth)
			return resolved, tokenKey, nil
		}
	}

	return nil, "", fmt.Errorf("no OAuth provider %q found in config", name)
}

// collectOAuthTokenKeys returns a map of token_key → provider_name for all OAuth tools.
func collectOAuthTokenKeys(cfg *config.Config) map[string]string {
	keys := make(map[string]string)
	for _, tool := range cfg.Tools {
		if tool.Auth == nil || tool.Auth.Type != "oauth" {
			continue
		}
		tokenKey := config.OAuthTokenKey(tool.Auth)
		providerName := tool.Auth.Provider
		if providerName == "" {
			providerName = tokenKey
		}
		keys[tokenKey] = providerName
	}
	return keys
}

func init() {
	authCmd.AddCommand(authLoginCmd, authStatusCmd, authLogoutCmd)
}
