package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/factorly-dev/factorly-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var removeCmd = &cobra.Command{
	Use:   "remove <tool-name>",
	Short: "Remove a tool from the config",
	Args:  requireArgs(1, "factorly tools remove <tool-name>"),
	RunE:  runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Verify tool exists
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Tools[toolName]; !ok {
		return fmt.Errorf("tool %q not found in config", toolName)
	}

	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfig()
	}

	// Search for the tool across all config locations
	removed, removedFrom, err := searchAndRemove(toolName, cfgPath, cfg)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("tool %q exists in config but could not be located in any file", toolName)
	}

	fmt.Fprintf(os.Stderr, "Removed %s from %s\n", toolName, removedFrom)
	return nil
}

func searchAndRemove(toolName, cfgPath string, cfg *config.Config) (bool, string, error) {
	// 1. Check primary config file
	found, err := removeFromConfigFile(cfgPath, toolName)
	if err != nil {
		return false, "", err
	}
	if found {
		return true, cfgPath, nil
	}

	// 2. Check tools_dir
	if cfg.ToolsDir != "" {
		dir := cfg.ToolsDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(cfgPath), dir)
		}
		found, path, err := removeFromDir(dir, toolName)
		if err != nil {
			return false, "", err
		}
		if found {
			return true, path, nil
		}
	}

	// 3. Check .factorly/ directory
	configDir := filepath.Dir(cfgPath)
	if filepath.Base(configDir) == ".factorly" {
		// Config is in .factorly/ — check loose files there
		found, path, err := removeFromDir(configDir, toolName)
		if err != nil {
			return false, "", err
		}
		if found {
			return true, path, nil
		}
	} else {
		// Config is outside .factorly/ — check .factorly/ subdirectory
		factorlyDir := filepath.Join(configDir, ".factorly")
		if info, err := os.Stat(factorlyDir); err == nil && info.IsDir() {
			// Check .factorly/factorly.yaml
			projectConfig := filepath.Join(factorlyDir, "factorly.yaml")
			found, err := removeFromConfigFile(projectConfig, toolName)
			if err != nil {
				return false, "", err
			}
			if found {
				return true, projectConfig, nil
			}

			// Check loose files
			found2, path, err := removeFromDir(factorlyDir, toolName)
			if err != nil {
				return false, "", err
			}
			if found2 {
				return true, path, nil
			}
		}
	}

	return false, "", nil
}

// removeFromConfigFile removes a tool from a primary config file (has tools: key).
func removeFromConfigFile(path, toolName string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var cfgMap map[string]any
	if err := yaml.Unmarshal(data, &cfgMap); err != nil {
		return false, nil // can't parse, skip
	}

	toolsRaw, ok := cfgMap["tools"]
	if !ok {
		return false, nil
	}
	toolsMap, ok := toolsRaw.(map[string]any)
	if !ok {
		return false, nil
	}

	if _, exists := toolsMap[toolName]; !exists {
		return false, nil
	}

	delete(toolsMap, toolName)

	outData, err := yaml.Marshal(cfgMap)
	if err != nil {
		return false, fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, outData, 0o644); err != nil {
		return false, fmt.Errorf("writing config: %w", err)
	}
	return true, nil
}

// removeFromDir searches YAML files in a directory for a tool and removes it.
func removeFromDir(dir, toolName string) (bool, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, "", nil // directory doesn't exist or can't be read
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		if name == "factorly.yaml" || name == "factorly.yml" {
			continue // skip primary configs, handled separately
		}

		path := filepath.Join(dir, name)
		found, err := removeFromToolFile(path, toolName)
		if err != nil {
			return false, "", err
		}
		if found {
			return true, path, nil
		}
	}

	return false, "", nil
}

// removeFromToolFile removes a tool from a flat tool definition file.
// If the file becomes empty, deletes it.
func removeFromToolFile(path, toolName string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}

	var tools map[string]any
	if err := yaml.Unmarshal(data, &tools); err != nil {
		return false, nil // can't parse as tool map, skip
	}

	if _, exists := tools[toolName]; !exists {
		return false, nil
	}

	delete(tools, toolName)

	if len(tools) == 0 {
		// File is empty — delete it
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("deleting empty file: %w", err)
		}
		return true, nil
	}

	// Re-marshal remaining tools
	outData, err := yaml.Marshal(tools)
	if err != nil {
		return false, fmt.Errorf("marshaling tools: %w", err)
	}
	if err := os.WriteFile(path, outData, 0o644); err != nil {
		return false, fmt.Errorf("writing tools: %w", err)
	}
	return true, nil
}
