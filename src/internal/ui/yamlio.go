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
// individual file and removes any inline definition to avoid duplicates.
// Otherwise appends/updates in the main config file.
func SaveTool(cfgPath, toolsDir, name string, tc config.ToolConfig) error {
	if toolsDir != "" {
		// Remove from inline config to prevent duplicate errors
		_ = deleteToolFromConfig(cfgPath, name)
		return saveToolToDir(toolsDir, name, tc)
	}
	return saveToolToConfig(cfgPath, name, tc)
}

// DeleteTool removes a tool from disk (both inline and dir file).
func DeleteTool(cfgPath, toolsDir, name string) error {
	_ = deleteToolFromConfig(cfgPath, name)
	if toolsDir != "" {
		return deleteToolFromDir(toolsDir, name)
	}
	return nil
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

	// Marshal the tool config to a yaml.Node to preserve field order
	toolBytes, err := yaml.Marshal(tc)
	if err != nil {
		return err
	}
	var toolNode yaml.Node
	if err := yaml.Unmarshal(toolBytes, &toolNode); err != nil {
		return err
	}

	// Re-read the full file as a yaml.Node tree to preserve ordering
	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return fmt.Errorf("parsing config as node: %w", err)
	}

	// Find or create the tools mapping node
	root := docNode.Content[0] // document root mapping
	var toolsNode *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "tools" {
			toolsNode = root.Content[i+1]
			break
		}
	}
	if toolsNode == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "tools"},
			&yaml.Node{Kind: yaml.MappingNode},
		)
		toolsNode = root.Content[len(root.Content)-1]
	}

	// Ensure block style for readable output
	toolsNode.Style = 0

	// Replace or append the tool entry
	found := false
	for i := 0; i < len(toolsNode.Content)-1; i += 2 {
		if toolsNode.Content[i].Value == name {
			toolsNode.Content[i+1] = toolNode.Content[0]
			found = true
			break
		}
	}
	if !found {
		toolsNode.Content = append(toolsNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: name},
			toolNode.Content[0],
		)
	}

	out, err := yaml.Marshal(&docNode)
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
