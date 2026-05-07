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

	// Find if tool exists in an existing file and update in-place
	existingFile := findToolInDir(toolsDir, name)
	if existingFile != "" {
		return updateToolInFile(existingFile, name, tc)
	}

	// Otherwise write a new file
	filename, err := toolFilename(name)
	if err != nil {
		return err
	}
	path := filepath.Join(toolsDir, filename)

	data := map[string]config.ToolConfig{name: tc}
	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling tool: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

// findToolInDir returns the path of the file containing the named tool, or "".
func findToolInDir(toolsDir, name string) string {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(toolsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var tools map[string]any
		if err := yaml.Unmarshal(data, &tools); err != nil {
			continue
		}
		if _, exists := tools[name]; exists {
			return path
		}
	}
	return ""
}

// updateToolInFile updates a single tool within a multi-tool YAML file,
// preserving other tools and field order.
func updateToolInFile(path, name string, tc config.ToolConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Marshal new tool config as a node
	toolBytes, err := yaml.Marshal(tc)
	if err != nil {
		return err
	}
	var toolNode yaml.Node
	if err := yaml.Unmarshal(toolBytes, &toolNode); err != nil {
		return err
	}

	// Parse existing file as node tree
	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	root := docNode.Content[0] // top-level mapping
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == name {
			root.Content[i+1] = toolNode.Content[0]
			break
		}
	}

	out, err := yaml.Marshal(&docNode)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func deleteToolFromDir(toolsDir, name string) error {
	// First try the dedicated file
	filename, err := toolFilename(name)
	if err != nil {
		return err
	}
	path := filepath.Join(toolsDir, filename)
	if err := os.Remove(path); err == nil {
		return nil
	}

	// Otherwise find and remove from a multi-tool file
	existingFile := findToolInDir(toolsDir, name)
	if existingFile == "" {
		return nil
	}
	return removeToolFromFile(existingFile, name)
}

// removeToolFromFile removes a single tool from a multi-tool YAML file.
// If the file becomes empty, it is deleted.
func removeToolFromFile(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return err
	}

	root := docNode.Content[0]
	var newContent []*yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == name {
			continue // skip this tool
		}
		newContent = append(newContent, root.Content[i], root.Content[i+1])
	}

	if len(newContent) == 0 {
		return os.Remove(path)
	}

	root.Content = newContent
	out, err := yaml.Marshal(&docNode)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
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

// SaveOAuthProvider writes an OAuth provider config to the main config file.
func SaveOAuthProvider(cfgPath, name string, p config.OAuthProviderConfig) error {
	return upsertConfigMapEntry(cfgPath, "oauth_providers", name, p)
}

// DeleteOAuthProvider removes an OAuth provider from the main config file.
func DeleteOAuthProvider(cfgPath, name string) error {
	return deleteConfigMapEntry(cfgPath, "oauth_providers", name)
}

// upsertConfigMapEntry adds or updates an entry in a top-level mapping in a YAML config file.
func upsertConfigMapEntry(cfgPath, mapKey, entryName string, value any) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	valBytes, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	var valNode yaml.Node
	if err := yaml.Unmarshal(valBytes, &valNode); err != nil {
		return err
	}

	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	root := docNode.Content[0]
	var mapNode *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == mapKey {
			mapNode = root.Content[i+1]
			break
		}
	}
	if mapNode == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: mapKey},
			&yaml.Node{Kind: yaml.MappingNode},
		)
		mapNode = root.Content[len(root.Content)-1]
	}
	mapNode.Style = 0

	found := false
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		if mapNode.Content[i].Value == entryName {
			mapNode.Content[i+1] = valNode.Content[0]
			found = true
			break
		}
	}
	if !found {
		mapNode.Content = append(mapNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: entryName},
			valNode.Content[0],
		)
	}

	out, err := yaml.Marshal(&docNode)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

// deleteConfigMapEntry removes an entry from a top-level mapping in a YAML config file.
func deleteConfigMapEntry(cfgPath, mapKey, entryName string) error {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}

	var docNode yaml.Node
	if err := yaml.Unmarshal(data, &docNode); err != nil {
		return err
	}

	root := docNode.Content[0]
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == mapKey {
			mapNode := root.Content[i+1]
			var newContent []*yaml.Node
			for j := 0; j < len(mapNode.Content)-1; j += 2 {
				if mapNode.Content[j].Value == entryName {
					continue
				}
				newContent = append(newContent, mapNode.Content[j], mapNode.Content[j+1])
			}
			mapNode.Content = newContent
			break
		}
	}

	out, err := yaml.Marshal(&docNode)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

// safePath sanitizes a user-provided name for use as a single path component.
// It rejects traversal attempts (..), path separators, and empty/dot-only names.
func safePath(name string) (string, error) {
	s := strings.TrimSpace(name)
	if s == "" || s == "." || s == ".." {
		return "", fmt.Errorf("invalid name: %q", name)
	}
	if strings.ContainsAny(s, "/\\") || strings.Contains(s, "..") {
		return "", fmt.Errorf("invalid name: %q (contains path separator or traversal)", name)
	}
	return s, nil
}

func toolFilename(name string) (string, error) {
	safe, err := safePath(name)
	if err != nil {
		return "", err
	}
	return safe + ".yaml", nil
}
