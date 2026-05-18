// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os"
	"regexp"
	"text/tabwriter"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/workspace"
	"github.com/spf13/cobra"
)

var workspacesCmd = &cobra.Command{
	Use:   "workspaces",
	Short: "List and inspect named workspace overlays",
}

var workspacesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = config.FindConfig()
		}
		wss, err := workspace.List(cfgPath)
		if err != nil {
			return err
		}
		if len(wss) == 0 {
			fmt.Println("No workspaces defined.")
			fmt.Println("Create one at .factorly/workspaces/<name>.yaml.")
			return nil
		}
		active := workspaceName
		if active == "" {
			active = os.Getenv("FACTORLY_WORKSPACE")
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVARS\tDESCRIPTION")
		for _, ws := range wss {
			name := ws.Name
			if name == active {
				name = name + " *"
			}
			fmt.Fprintf(w, "%s\t%d\t%s\n", name, len(ws.Vars), ws.Description)
		}
		_ = w.Flush()
		if active != "" {
			fmt.Println()
			fmt.Printf("* active workspace (--workspace %s or FACTORLY_WORKSPACE=%s)\n", active, active)
		}
		return nil
	},
}

var workspacesShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print a workspace's variables (secrets masked)",
	Args:  requireArgs(1, "factorly workspaces show <name>"),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = config.FindConfig()
		}
		ws, err := workspace.Load(cfgPath, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("workspace: %s\n", ws.Name)
		if ws.Description != "" {
			fmt.Printf("description: %s\n", ws.Description)
		}
		if len(ws.Vars) == 0 {
			fmt.Println("vars: (none)")
			return nil
		}
		fmt.Println("vars:")
		w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
		// Sort keys for stable output. Reuse sorted iteration via a slice.
		keys := make([]string, 0, len(ws.Vars))
		for k := range ws.Vars {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			v := ws.Vars[k]
			if looksSecret(k) {
				v = maskValue(v)
			}
			fmt.Fprintf(w, "  %s\t= %s\n", k, v)
		}
		_ = w.Flush()
		return nil
	},
}

// secretKeyPattern matches keys that smell secret. Used by `workspaces
// show` to mask values — workspace var files aren't a great place to
// stash secrets (vault is), but if a user puts one there we should at
// least not print it back at them in plaintext.
var secretKeyPattern = regexp.MustCompile(`(?i)(secret|token|key|password|passwd|pwd|api[_-]?key|auth)`)

func looksSecret(key string) bool {
	return secretKeyPattern.MatchString(key)
}

func maskValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + "****" + v[len(v)-2:]
}

func sortStrings(ss []string) {
	// Tiny insertion sort to avoid importing sort just for this; lists
	// are typically <20 entries.
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j-1] > ss[j]; j-- {
			ss[j-1], ss[j] = ss[j], ss[j-1]
		}
	}
}

func init() {
	workspacesCmd.AddCommand(workspacesListCmd, workspacesShowCmd)
}
