// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"os"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/promote"
	codeprov "github.com/factorly-dev/factorly/internal/provider/code"
	"github.com/spf13/cobra"
)

var (
	promoteFromSHA     string
	promoteName        string
	promoteDescription string
	promoteOverwrite   bool
	promoteNoConfirm   bool
)

var toolsPromoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote a factorly.code audit-log entry into a named tool",
	Long: `Promote scans the audit log for a factorly.code run identified by
its source_sha prefix, recovers the script, and writes a registerable
type:code tool YAML. The script is compile-checked before being written
so a broken promotion can never land on disk.

Parameters are inferred from the params the run was called with: keys
other than 'code' become tool parameters, with their actual run-time
values as defaults. Refine in the UI's edit page if needed.

Example:
    factorly tools promote --from-sha 8e3c --name trello.factorly_cards`,
	RunE: runToolsPromote,
}

func init() {
	toolsPromoteCmd.Flags().StringVar(&promoteFromSHA, "from-sha", "",
		"source_sha prefix of the factorly.code entry to promote (>= 4 chars)")
	toolsPromoteCmd.Flags().StringVar(&promoteName, "name", "",
		"name for the new tool (e.g. mycategory.do_thing)")
	toolsPromoteCmd.Flags().StringVar(&promoteDescription, "description", "",
		"description for the new tool (default: auto-generated from run metadata)")
	toolsPromoteCmd.Flags().BoolVar(&promoteOverwrite, "overwrite", false,
		"replace an existing tool of the same name")
	toolsPromoteCmd.Flags().BoolVar(&promoteNoConfirm, "no-confirm", false,
		"don't set shadow.confirm:true on the saved tool (default: confirm is on)")
}

func runToolsPromote(cmd *cobra.Command, args []string) error {
	if promoteFromSHA == "" {
		return fmt.Errorf("--from-sha is required")
	}
	if promoteName == "" {
		return fmt.Errorf("--name is required")
	}

	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	// 1. Recover the source from the audit log.
	logPath := resolveLogPath()
	res, err := promote.FromLog(logPath, promoteFromSHA)
	if err != nil {
		return err
	}

	// 2. Compile-check. Refuse to write a broken tool.
	if err := codeprov.Validate(res.Source); err != nil {
		return fmt.Errorf("script doesn't compile, refusing to write a broken tool: %w", err)
	}

	// 3. Name-conflict check. The duplicate check in writeNewTool /
	// SaveTool would silently overwrite or error in inconsistent ways
	// depending on the storage mode, so do it explicitly here.
	if _, exists := cfg.Tools[promoteName]; exists && !promoteOverwrite {
		return fmt.Errorf("tool %q already exists; pass --overwrite to replace it", promoteName)
	}

	// 4. Build the ToolConfig.
	desc := promoteDescription
	if desc == "" {
		when := res.OriginalRun.Timestamp.Format("2006-01-02")
		desc = fmt.Sprintf("Promoted from factorly.code run on %s (sha %s)", when, shortSHA(res.SHA))
	}

	tc := config.ToolConfig{
		Type:        "code",
		Description: desc,
		Code:        res.Source,
		Parameters:  toParamConfigSlice(res.Parameters),
	}
	if !promoteNoConfirm {
		tc.Shadow = &config.ShadowConfig{Confirm: true}
	}

	// 5. Persist using the same path-resolution logic `factorly tools
	// add` uses, so promote behaves consistently with that command.
	outPath, err := writeNewTool(promoteName, tc, cfg)
	if err != nil {
		return fmt.Errorf("writing tool: %w", err)
	}

	// 6. Friendly summary on stdout.
	fmt.Fprintf(os.Stderr, "Promoted factorly.code run %s → tool %q\n", shortSHA(res.SHA), promoteName)
	fmt.Fprintf(os.Stderr, "  Wrote %s\n", outPath)
	if len(res.Parameters) > 0 {
		fmt.Fprintf(os.Stderr, "  Inferred %d parameter(s) from the run:\n", len(res.Parameters))
		for _, p := range res.Parameters {
			fmt.Fprintf(os.Stderr, "    - %s = %q\n", p.Name, p.Default)
		}
		fmt.Fprintln(os.Stderr, "  Refine descriptions/types in the UI: `factorly ui` → /tools/"+promoteName)
	}
	if !promoteNoConfirm {
		fmt.Fprintln(os.Stderr, "  shadow.confirm: true (the tool will prompt before each call)")
	}
	return nil
}

// shortSHA returns the first 8 hex chars of a SHA, the standard
// abbreviation used elsewhere in factorly's CLI output.
func shortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// toParamConfigSlice copies the inferred parameter list so callers
// downstream can mutate without affecting the promote.Result. The
// types match already; this is purely a defensive copy.
func toParamConfigSlice(in []config.ParamConfig) []config.ParamConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]config.ParamConfig, len(in))
	copy(out, in)
	return out
}
