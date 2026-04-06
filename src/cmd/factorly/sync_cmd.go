package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var syncHTTP string
var syncCommand string
var syncRemove bool
var syncGlobal bool

type clientDef struct {
	Name       string
	ConfigPath string
	AlwaysSync bool   // true = always create/update; false = only if DirCheck exists
	DirCheck   string // only sync if this directory exists (empty = always)
}

func getClients() []clientDef {
	if syncGlobal {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "~"
		}
		// Claude Code user-level MCP config is inside ~/.claude.json
		// Cursor uses ~/.cursor/mcp.json
		// Codex global config uses TOML — not supported yet
		return []clientDef{
			{Name: "Claude Code", ConfigPath: filepath.Join(home, ".claude.json"), AlwaysSync: true},
			{Name: "Cursor", ConfigPath: filepath.Join(home, ".cursor", "mcp.json"), DirCheck: filepath.Join(home, ".cursor")},
			{Name: "Codex", ConfigPath: "", DirCheck: filepath.Join(home, ".codex")},
		}
	}
	return []clientDef{
		{Name: "Claude Code", ConfigPath: ".mcp.json", AlwaysSync: true},
		{Name: "Cursor", ConfigPath: ".cursor/mcp.json", DirCheck: ".cursor"},
		{Name: "Codex", ConfigPath: ".codex/mcp.json", DirCheck: ".codex"},
	}
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Push Factorly MCP config into AI client config files",
	Long: `Detect installed AI clients and write the Factorly MCP server entry
into their config files (.mcp.json, .cursor/mcp.json, etc.).

Existing MCP server entries in the config files are preserved.`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	entry := buildFactorlyEntry()
	synced := 0
	activeClients := getClients()

	fmt.Fprintln(os.Stderr)
	if syncGlobal {
		fmt.Fprintln(os.Stderr, "  Detected clients (global):")
	} else {
		fmt.Fprintln(os.Stderr, "  Detected clients:")
	}

	for _, c := range activeClients {
		if !c.AlwaysSync && c.DirCheck != "" {
			if info, err := os.Stat(c.DirCheck); err != nil || !info.IsDir() {
				fmt.Fprintf(os.Stderr, "  - %-14s %s not found, skipping\n", c.Name, c.DirCheck+"/")
				continue
			}
		}

		if c.ConfigPath == "" {
			fmt.Fprintf(os.Stderr, "  ✗ %-14s global sync not supported (uses TOML config)\n", c.Name)
			continue
		}

		if syncRemove {
			removed, err := removeFactorlyFromJSON(c.ConfigPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %-14s %v\n", c.Name, err)
				continue
			}
			if removed {
				fmt.Fprintf(os.Stderr, "  ✓ %-14s removed from %s\n", c.Name, c.ConfigPath)
				synced++
			} else {
				fmt.Fprintf(os.Stderr, "  - %-14s not configured in %s\n", c.Name, c.ConfigPath)
			}
		} else {
			if err := mergeFactorlyIntoJSON(c.ConfigPath, entry); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %-14s %v\n", c.Name, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "  ✓ %-14s wrote %s\n", c.Name, c.ConfigPath)
			synced++
		}
	}

	fmt.Fprintln(os.Stderr)
	if syncRemove {
		fmt.Fprintf(os.Stderr, "  Removed from %d clients.\n", synced)
	} else {
		fmt.Fprintf(os.Stderr, "  Synced %d clients.\n", synced)
	}
	return nil
}

func buildFactorlyEntry() map[string]any {
	if syncHTTP != "" {
		// HTTP mode
		url := syncHTTP
		if url[0] != 'h' {
			url = "http://" + url
		}
		if filepath.Ext(url) == "" && url[len(url)-1] != '/' {
			url += "/mcp"
		}
		return map[string]any{
			"type": "streamable-http",
			"url":  url,
		}
	}

	// Stdio mode
	cmd := syncCommand
	if cmd == "" {
		cmd = "factorly"
	}

	serveArgs := []string{"serve"}

	// Add config flag if using non-default config location
	if configPath != "" {
		serveArgs = append(serveArgs, "-c", configPath)
	}

	return map[string]any{
		"command": cmd,
		"args":    serveArgs,
	}
}

func mergeFactorlyIntoJSON(path string, entry map[string]any) error {
	var root map[string]any

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		root = make(map[string]any)
	} else if len(strings.TrimSpace(string(data))) == 0 {
		root = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	// Ensure mcpServers exists
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	servers["factorly"] = entry
	root["mcpServers"] = servers

	// Ensure parent directory exists
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling: %w", err)
	}
	out = append(out, '\n')

	return os.WriteFile(path, out, 0o644)
}

func removeFactorlyFromJSON(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("parsing %s: %w", path, err)
	}

	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return false, nil
	}

	if _, exists := servers["factorly"]; !exists {
		return false, nil
	}

	delete(servers, "factorly")
	root["mcpServers"] = servers

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

func init() {
	syncCmd.Flags().StringVar(&syncHTTP, "http", "", "sync HTTP mode with this address (e.g. localhost:3000)")
	syncCmd.Flags().StringVar(&syncCommand, "command", "", "custom factorly binary path")
	syncCmd.Flags().BoolVar(&syncRemove, "remove", false, "remove factorly entry from client configs")
	syncCmd.Flags().BoolVar(&syncGlobal, "global", false, "sync to user-level config (~/.claude/, ~/.cursor/, ~/.codex/)")
}
