package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/factorly-dev/factorly-cli/internal/templates"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	templatesDryRun bool
	templatesAll    bool
	templatesAPIKey string
)

var templatesCmd = &cobra.Command{
	Use:   "templates [name]",
	Short: "Import pre-built tool definitions for popular services",
	Long: `Import pre-built tool configurations for popular services like
GitHub, Slack, Linear, Stripe, and Notion.

List available templates:
  factorly tools import templates

Install a template (interactive):
  factorly tools import templates linear

Preview without writing:
  factorly tools import templates linear --dry-run`,
	RunE: runTemplates,
}

func runTemplates(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return listTemplates()
	}
	return installTemplate(args[0])
}

func listTemplates() error {
	all := templates.All()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCATEGORY\tAUTH\tTOOLS\tDESCRIPTION")
	for _, t := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			t.Name, t.Category, t.AuthType, len(t.Tools), t.Description)
	}
	return w.Flush()
}

func installTemplate(name string) error {
	tmpl := templates.Get(name)
	if tmpl == nil {
		return fmt.Errorf("unknown template %q — run 'factorly tools import templates' to see available templates", name)
	}

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("\n  %s — %s\n\n", tmpl.DisplayName, tmpl.Description)

	// Auth setup
	if !templatesDryRun {
		if tmpl.AuthType == "oauth" {
			if err := setupOAuthAuth(scanner, tmpl); err != nil {
				return err
			}
		} else {
			if err := setupTokenAuth(scanner, tmpl); err != nil {
				return err
			}
		}
	}

	// Tool selection
	var selectedTools []string
	if !templatesAll && !templatesDryRun {
		selectedTools = selectTools(scanner, tmpl)
	}

	// Generate configs
	toolConfigs := tmpl.ToToolConfigs(selectedTools)

	// Marshal bare tool definitions (no "tools:" wrapper — loadDir expects this)
	toolData, err := yaml.Marshal(toolConfigs)
	if err != nil {
		return fmt.Errorf("marshaling tools: %w", err)
	}

	if templatesDryRun {
		fmt.Println()
		fmt.Print(string(toolData))
		if oauthProviders := tmpl.ToOAuthProvider(); oauthProviders != nil {
			oauthData, _ := yaml.Marshal(map[string]any{"oauth_providers": oauthProviders})
			fmt.Printf("\n# Add to .factorly/factorly.yaml:\n%s", string(oauthData))
		}
		return nil
	}

	// Write tools to .factorly/tools/<name>.yaml
	outDir := filepath.Join(".factorly", "tools")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	outPath := filepath.Join(outDir, name+".yaml")

	if err := os.WriteFile(outPath, toolData, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	fmt.Printf("\n  Created %d tools:\n", len(toolConfigs))
	for toolName, tc := range toolConfigs {
		fmt.Printf("    %-30s %s\n", toolName, tc.Description)
	}

	hasShadow := false
	for _, tc := range toolConfigs {
		if tc.Shadow != nil {
			hasShadow = true
			break
		}
	}
	if hasShadow {
		fmt.Println("\n  Shadow governance applied to write/delete operations")
	}

	fmt.Printf("\n  Written to %s\n", outPath)

	// Merge oauth_providers into .factorly/factorly.yaml if needed
	if oauthProviders := tmpl.ToOAuthProvider(); oauthProviders != nil {
		if err := mergeOAuthProviders(oauthProviders); err != nil {
			return err
		}
		fmt.Printf("\n  Next: run 'factorly auth login %s' to complete OAuth setup\n", tmpl.Name)
	} else {
		fmt.Println("  Run 'factorly tools' to see all tools.")
	}
	return nil
}

func selectTools(scanner *bufio.Scanner, tmpl *templates.Template) []string {
	essentials := tmpl.EssentialTools()

	fmt.Printf("\n  Which tools to install? (%d available)\n", len(tmpl.Tools))
	fmt.Printf("  1) All (%d tools)\n", len(tmpl.Tools))
	fmt.Printf("  2) Essentials (%d tools: %s)\n", len(essentials), strings.Join(essentials, ", "))
	fmt.Printf("  3) Choose individually\n")
	fmt.Print("  > ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "2":
		return essentials
	case "3":
		return chooseIndividually(scanner, tmpl)
	default:
		return nil // nil = all tools
	}
}

func chooseIndividually(scanner *bufio.Scanner, tmpl *templates.Template) []string {
	fmt.Println()

	// Build default selection string from essentials
	var defaultNums []string
	for i, td := range tmpl.Tools {
		if td.Essential {
			defaultNums = append(defaultNums, strconv.Itoa(i+1))
		}
	}
	defaultStr := strings.Join(defaultNums, ",")

	for i, td := range tmpl.Tools {
		marker := " "
		if td.Essential {
			marker = "*"
		}
		fmt.Printf("  [%s] %d) %-30s %s\n", marker, i+1, td.Name, td.Description)
	}

	fmt.Printf("\n  Enter numbers (comma-separated), or press Enter for essentials [%s]: ", defaultStr)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	// Use defaults if empty
	if input == "" {
		input = defaultStr
	}

	// Parse selection
	selected := make(map[int]bool)
	for _, s := range strings.Split(input, ",") {
		s = strings.TrimSpace(s)
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(tmpl.Tools) {
			selected[n-1] = true
		}
	}

	// Show what was selected
	var names []string
	for i, td := range tmpl.Tools {
		if selected[i] {
			names = append(names, td.Name)
		}
	}

	fmt.Printf("\n  Selected %d tools: %s\n", len(names), strings.Join(names, ", "))

	return names
}

func setupTokenAuth(scanner *bufio.Scanner, tmpl *templates.Template) error {
	// Check if credential already exists in vault
	if templatesAPIKey == "" {
		backend, err := openVault()
		if err == nil {
			defer backend.Close()
			if backend.Has(tmpl.VaultKey) {
				fmt.Printf("  Auth: %s (credential found in vault)\n", tmpl.AuthType)
				fmt.Printf("  ✓ %s already configured\n", tmpl.VaultKey)
				return nil
			}
		}
	}

	apiKey := templatesAPIKey
	if apiKey == "" {
		fmt.Printf("  Auth: %s\n", tmpl.AuthType)
		if tmpl.AuthGuide != "" {
			fmt.Printf("  Guide: %s\n", tmpl.AuthGuide)
		}
		label := "Token"
		if tmpl.AuthType == "api_key" {
			label = "API key"
		}
		fmt.Printf("  %s: ", label)
		scanner.Scan()
		apiKey = strings.TrimSpace(scanner.Text())
		if apiKey == "" {
			return fmt.Errorf("credential is required")
		}
	}

	return storeInVault(scanner, tmpl.VaultKey, apiKey)
}

func setupOAuthAuth(scanner *bufio.Scanner, tmpl *templates.Template) error {
	clientIDKey := strings.ToUpper(tmpl.Name) + "_CLIENT_ID"
	clientSecretKey := strings.ToUpper(tmpl.Name) + "_CLIENT_SECRET"

	// Check if credentials already exist in vault
	backend, err := openVault()
	if err != nil {
		return fmt.Errorf("opening vault: %w", err)
	}
	defer backend.Close()

	hasID := backend.Has(clientIDKey)
	hasSecret := backend.Has(clientSecretKey)

	if hasID && hasSecret {
		fmt.Printf("  Auth: OAuth 2.0 (credentials found in vault)\n")
		fmt.Printf("  ✓ %s and %s already configured\n", clientIDKey, clientSecretKey)
		return nil
	}

	fmt.Printf("  Auth: OAuth 2.0\n")
	if tmpl.AuthGuide != "" {
		fmt.Printf("  Guide: %s\n", tmpl.AuthGuide)
	}

	// Client ID
	fmt.Printf("  Client ID: ")
	scanner.Scan()
	clientID := strings.TrimSpace(scanner.Text())
	if clientID == "" {
		return fmt.Errorf("client ID is required for OAuth")
	}

	// Client Secret
	fmt.Printf("  Client Secret: ")
	scanner.Scan()
	clientSecret := strings.TrimSpace(scanner.Text())
	if clientSecret == "" {
		return fmt.Errorf("client secret is required for OAuth")
	}

	if err := storeInVault(scanner, clientIDKey, clientID); err != nil {
		return err
	}
	return storeInVault(scanner, clientSecretKey, clientSecret)
}

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

// mergeOAuthProviders adds oauth_providers entries to .factorly/factorly.yaml.
// Creates the file if it doesn't exist. Preserves existing content.
func mergeOAuthProviders(providers map[string]config.OAuthProviderConfig) error {
	cfgPath := filepath.Join(".factorly", "factorly.yaml")

	// Load existing config or start fresh
	var raw map[string]any
	data, err := os.ReadFile(cfgPath)
	if err == nil {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing %s: %w", cfgPath, err)
		}
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	// Get or create oauth_providers section
	existing, _ := raw["oauth_providers"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	// Merge new providers (don't overwrite existing ones)
	for name, provider := range providers {
		if _, ok := existing[name]; ok {
			fmt.Printf("  OAuth provider %q already configured, skipping\n", name)
			continue
		}
		existing[name] = provider
		fmt.Printf("  Added OAuth provider %q to %s\n", name, cfgPath)
	}
	raw["oauth_providers"] = existing

	// Write back
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

func init() {
	templatesCmd.Flags().BoolVar(&templatesDryRun, "dry-run", false, "preview YAML without writing")
	templatesCmd.Flags().BoolVar(&templatesAll, "all", false, "install all tools (skip selection prompt)")
	templatesCmd.Flags().StringVar(&templatesAPIKey, "api-key", "", "API key or token (non-interactive)")
}
