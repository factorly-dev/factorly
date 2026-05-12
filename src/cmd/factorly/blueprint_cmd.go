// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/factorly-dev/factorly/internal/blueprints"
	"github.com/factorly-dev/factorly/internal/config"
)

var (
	blueprintInstallDryRun   bool
	blueprintInstallNoPrompt bool
)

var blueprintCmd = &cobra.Command{
	Use:   "blueprint",
	Short: "Install and manage sharable tool/workflow blueprints",
	Long: `A blueprint is a single YAML file that bundles tools, workflows, OAuth
provider definitions, and vault-key requirements into one shareable unit.

Run 'factorly blueprint install <source>' to install one from a GitHub repo,
URL, or local file. After install, the blueprint lives at
.factorly/blueprints/<name>.yaml and its tools/workflows/oauth_providers
merge into the project config on next load.`,
}

var blueprintInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a blueprint from a GitHub repo, URL, or file",
	Long: `Install a sharable blueprint.

Sources:
  Local file:        ./blueprints/gmail.yaml
  Raw URL:           https://raw.githubusercontent.com/widefido/gmail/main/blueprint.yaml
  GitHub shorthand:  github.com/widefido/factorly-gmail
                     github.com/widefido/factorly-gmail@v1.0.0
                     github.com/widefido/factorly-gmail/blueprints/search.yaml

Examples:
  factorly blueprint install github.com/widefido/factorly-gmail
  factorly blueprint install ./blueprints/gmail.yaml
  factorly blueprint install ./blueprints/gmail.yaml --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runBlueprintInstall,
}

var blueprintUninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Remove an installed blueprint",
	Long: `Remove a previously-installed blueprint by its name (as shown in
'factorly blueprint list').

The blueprint file in .factorly/blueprints/<name>.yaml is deleted. Any vault
keys or oauth provider credentials the blueprint used are left intact;
remove them manually with 'factorly vault remove' if desired.`,
	Args: cobra.ExactArgs(1),
	RunE: runBlueprintUninstall,
}

var blueprintListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed blueprints",
	RunE:  runBlueprintList,
}

func runBlueprintInstall(cmd *cobra.Command, args []string) error {
	source := args[0]
	cfgPath := resolveCfgPath()

	opts := blueprints.InstallOptions{
		Source:  source,
		CfgPath: cfgPath,
		DryRun:  blueprintInstallDryRun,
	}
	res, err := blueprints.Install(opts)
	if err != nil {
		// Print whatever the result tells us (conflicts, missing requires)
		// even when install fails, so the user has actionable context.
		if res != nil {
			printInstallSummary(res)
		}
		return err
	}

	printInstallSummary(res)

	if blueprintInstallDryRun {
		fmt.Println()
		fmt.Println("  Dry run — no changes written.")
		return nil
	}

	// Vault keys: collect values interactively if any are reported missing.
	if len(res.VaultKeysMissing) > 0 && !blueprintInstallNoPrompt {
		fmt.Println()
		fmt.Printf("  This blueprint uses %d vault key(s). Provide values now to enable it.\n", len(res.VaultKeysMissing))
		if err := promptAndStoreVaultKeys(res.VaultKeysMissing); err != nil {
			return err
		}
	} else if len(res.VaultKeysMissing) > 0 && blueprintInstallNoPrompt {
		fmt.Println()
		fmt.Printf("  Vault keys not set (--no-prompt): %s\n", strings.Join(res.VaultKeysMissing, ", "))
		fmt.Println("  Use 'factorly vault set <key> <value>' to provide them.")
	}

	fmt.Println()
	fmt.Printf("  Installed %s → %s\n", res.Header.Name, res.FilePath)
	return nil
}

func runBlueprintUninstall(cmd *cobra.Command, args []string) error {
	cfgPath := resolveCfgPath()
	name := args[0]
	if err := blueprints.Uninstall(cfgPath, name); err != nil {
		return err
	}
	fmt.Printf("Uninstalled %s\n", name)
	return nil
}

func runBlueprintList(cmd *cobra.Command, args []string) error {
	cfgPath := resolveCfgPath()
	list, err := blueprints.List(cfgPath)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("No blueprints installed.")
		fmt.Println("Try: factorly blueprint install github.com/<owner>/<repo>")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION")
	for _, p := range list {
		desc := p.Description
		if desc == "" {
			desc = "—"
		}
		version := p.Version
		if version == "" {
			version = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, version, desc)
	}
	return w.Flush()
}

// printInstallSummary prints the human-readable preview/result of an install.
// Called both on success and on failure (with whatever partial result is
// available), so the user sees conflicts/missing requires inline with the
// error message.
func printInstallSummary(res *blueprints.InstallResult) {
	if res == nil {
		return
	}
	h := res.Header
	fmt.Println()
	if h.Name != "" {
		title := h.Name
		if h.Version != "" {
			title = fmt.Sprintf("%s %s", h.Name, h.Version)
		}
		fmt.Printf("  %s\n", title)
		if h.Description != "" {
			fmt.Printf("  %s\n", h.Description)
		}
	} else {
		fmt.Printf("  (unnamed blueprint)\n")
	}

	if len(res.ToolsAdded) > 0 {
		fmt.Printf("\n  Tools (%d):\n", len(res.ToolsAdded))
		for _, t := range res.ToolsAdded {
			fmt.Printf("    + %s\n", t)
		}
	}
	if len(res.WorkflowsAdded) > 0 {
		fmt.Printf("\n  Workflows (%d):\n", len(res.WorkflowsAdded))
		for _, t := range res.WorkflowsAdded {
			fmt.Printf("    + %s\n", t)
		}
	}
	if len(res.ProvidersAdded) > 0 {
		fmt.Printf("\n  OAuth providers (%d):\n", len(res.ProvidersAdded))
		for _, p := range res.ProvidersAdded {
			fmt.Printf("    + %s\n", p)
		}
	}
	if len(res.VaultBackends) > 0 {
		fmt.Printf("\n  Vault backends (%d):\n", len(res.VaultBackends))
		for _, b := range res.VaultBackends {
			fmt.Printf("    + %s\n", b)
		}
	}
	if len(res.Conflicts) > 0 {
		fmt.Printf("\n  Conflicts (%d) — resolve before installing:\n", len(res.Conflicts))
		for _, c := range res.Conflicts {
			fmt.Printf("    ✗ %s %q already defined\n", c.Kind, c.Name)
		}
	}
	if len(res.RequiresMissing) > 0 {
		fmt.Printf("\n  Missing dependencies (%d):\n", len(res.RequiresMissing))
		for _, r := range res.RequiresMissing {
			fmt.Printf("    ✗ %s %q not installed\n", r.Kind, r.Name)
		}
	}
	if len(res.VaultKeysMissing) > 0 {
		fmt.Printf("\n  Vault keys required (%d):\n", len(res.VaultKeysMissing))
		for _, k := range res.VaultKeysMissing {
			fmt.Printf("    • %s\n", k)
		}
	}
	if res.AlreadyInstalled {
		fmt.Printf("\n  Already installed — uninstall first with 'factorly blueprint uninstall %s'.\n", res.Header.Name)
	}
}

// promptAndStoreVaultKeys interactively asks for each missing vault key's
// value and writes it to the local vault. Reuses storeInVault from
// templates_cmd.go so the overwrite-prompt UX is consistent.
func promptAndStoreVaultKeys(keys []string) error {
	scanner := bufio.NewScanner(os.Stdin)
	for _, k := range keys {
		fmt.Printf("  %s: ", k)
		if !scanner.Scan() {
			return errors.New("install: stdin closed before all vault keys collected")
		}
		val := strings.TrimSpace(scanner.Text())
		if val == "" {
			fmt.Printf("  (skipped %s — set later with 'factorly vault set %s <value>')\n", k, k)
			continue
		}
		if err := storeInVault(scanner, k, val); err != nil {
			return err
		}
	}
	return nil
}

// resolveCfgPath finds the config file the blueprint commands should target.
// Falls back to a sensible default if no config is found, so a fresh project
// can install its first blueprint without 'factorly init'.
func resolveCfgPath() string {
	if p := config.FindConfig(); p != "" {
		return p
	}
	// No config file found — default to ./.factorly/factorly.yaml so the
	// blueprint writes into a sensible location relative to CWD.
	return ".factorly/factorly.yaml"
}

func init() {
	blueprintInstallCmd.Flags().BoolVar(&blueprintInstallDryRun, "dry-run", false, "preview without writing")
	blueprintInstallCmd.Flags().BoolVar(&blueprintInstallNoPrompt, "no-prompt", false, "skip interactive vault key prompts")
	blueprintCmd.AddCommand(blueprintInstallCmd, blueprintUninstallCmd, blueprintListCmd)
}
