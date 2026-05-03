// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

// SaveTool writes a tool config to disk. If toolsDir is set, writes as
// individual file. Otherwise appends/updates in the main config file.
func SaveTool(cfgPath, toolsDir, name string, tc config.ToolConfig) error {
	if toolsDir != "" {
		return saveToolToDir(toolsDir, name, tc)
	}
	return saveToolToConfig(cfgPath, name, tc)
}

// DeleteTool removes a tool from disk.
func DeleteTool(cfgPath, toolsDir, name string) error {
	if toolsDir != "" {
		return deleteToolFromDir(toolsDir, name)
	}
	return deleteToolFromConfig(cfgPath, name)
}

func saveToolToDir(toolsDir, name string, tc config.ToolConfig) error {
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return err
	}
	filename := toolFilename(name)
	path := filepath.Join(toolsDir, filename)

	data := map[string]config.ToolConfig{name: tc}
	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling tool: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

func deleteToolFromDir(toolsDir, name string) error {
	filename := toolFilename(name)
	path := filepath.Join(toolsDir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveToolToConfig(cfgPath, name string, tc config.ToolConfig) error {
	// Read existing config
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	// Parse into a generic map to preserve structure
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Ensure tools map exists
	tools, ok := raw["tools"].(map[string]any)
	if !ok {
		tools = make(map[string]any)
		raw["tools"] = tools
	}

	// Marshal the tool config to generic form and insert
	toolBytes, err := yaml.Marshal(tc)
	if err != nil {
		return err
	}
	var toolMap any
	if err := yaml.Unmarshal(toolBytes, &toolMap); err != nil {
		return err
	}
	tools[name] = toolMap

	// Write back
	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

func deleteToolFromConfig(cfgPath, name string) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	tools, ok := raw["tools"].(map[string]any)
	if !ok {
		return nil
	}
	delete(tools, name)

	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

func toolFilename(name string) string {
	// Convert tool name to filename: github.list_repos → github.list_repos.yaml
	// Replace path separators for safety
	safe := strings.ReplaceAll(name, "/", "_")
	return safe + ".yaml"
}
