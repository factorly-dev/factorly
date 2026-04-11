package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

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
	apiKey := templatesAPIKey
	if apiKey == "" && !templatesDryRun {
		fmt.Printf("  Auth: %s\n", tmpl.AuthType)
		if tmpl.AuthGuide != "" {
			fmt.Printf("  Guide: %s\n", tmpl.AuthGuide)
		}
		fmt.Printf("  %s: ", vaultKeyLabel(tmpl))
		scanner.Scan()
		apiKey = strings.TrimSpace(scanner.Text())
		if apiKey == "" {
			return fmt.Errorf("API key is required")
		}
	}

	// Store in vault
	if apiKey != "" && !templatesDryRun {
		backend, err := openVault()
		if err != nil {
			return fmt.Errorf("opening vault: %w", err)
		}
		defer backend.Close()

		if backend.Has(tmpl.VaultKey) {
			fmt.Printf("\n  ⚠ Vault key %s already exists. Overwrite? (y/n): ", tmpl.VaultKey)
			scanner.Scan()
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(scanner.Text())), "y") {
				fmt.Printf("  Keeping existing %s\n", tmpl.VaultKey)
				apiKey = "" // skip storing
			}
		}
		if apiKey != "" {
			if err := backend.Set(tmpl.VaultKey, apiKey); err != nil {
				return fmt.Errorf("storing key in vault: %w", err)
			}
			fmt.Printf("\n  ✓ Stored in vault as %s\n", tmpl.VaultKey)
		}
	}

	// Tool selection
	var selectedTools []string
	if !templatesAll && !templatesDryRun {
		selectedTools = selectTools(scanner, tmpl)
	}

	// Generate config
	toolConfigs := tmpl.ToToolConfigs(selectedTools)

	data, err := yaml.Marshal(toolConfigs)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if templatesDryRun {
		fmt.Println()
		fmt.Print(string(data))
		return nil
	}

	// Write to .factorly/tools/<name>.yaml
	outDir := filepath.Join(".factorly", "tools")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	outPath := filepath.Join(outDir, name+".yaml")

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
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
	fmt.Println("  Run 'factorly tools' to see all tools.")
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
	selected := make(map[int]bool)
	// Pre-select essentials
	for i, td := range tmpl.Tools {
		if td.Essential {
			selected[i] = true
		}
	}

	for i, td := range tmpl.Tools {
		marker := " "
		if selected[i] {
			marker = "x"
		}
		fmt.Printf("  [%s] %d) %-30s %s\n", marker, i+1, td.Name, td.Description)
	}

	fmt.Print("\n  Enter numbers to toggle (comma-separated), or press Enter to confirm: ")
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input != "" {
		// Reset selection based on input
		selected = make(map[int]bool)
		for _, s := range strings.Split(input, ",") {
			s = strings.TrimSpace(s)
			if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(tmpl.Tools) {
				selected[n-1] = true
			}
		}
	}

	var names []string
	for i, td := range tmpl.Tools {
		if selected[i] {
			names = append(names, td.Name)
		}
	}
	return names
}

func vaultKeyLabel(tmpl *templates.Template) string {
	switch tmpl.AuthType {
	case "api_key":
		return "API key"
	case "bearer":
		return "Token"
	case "oauth":
		return "Access token"
	default:
		return "Credential"
	}
}

func init() {
	templatesCmd.Flags().BoolVar(&templatesDryRun, "dry-run", false, "preview YAML without writing")
	templatesCmd.Flags().BoolVar(&templatesAll, "all", false, "install all tools (skip selection prompt)")
	templatesCmd.Flags().StringVar(&templatesAPIKey, "api-key", "", "API key or token (non-interactive)")
}
