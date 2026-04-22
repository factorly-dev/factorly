// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

//go:build integration

package test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

var yamlBlockPattern = regexp.MustCompile("(?s)```yaml\\s*\n(.*?)```")

// TestExampleYAMLValid extracts all YAML code blocks from docs/examples/*.md
// and validates that they parse as valid Factorly configs.
func TestExampleYAMLValid(t *testing.T) {
	examplesDir := findExamplesDir(t)

	files, err := filepath.Glob(filepath.Join(examplesDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no example files found")
	}

	for _, file := range files {
		name := filepath.Base(file)
		if name == "README.md" {
			continue
		}

		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			blocks := yamlBlockPattern.FindAllStringSubmatch(string(data), -1)
			if len(blocks) == 0 {
				return // no YAML to validate — command-only examples are tested elsewhere
			}

			for i, match := range blocks {
				yamlContent := match[1]

				// Skip blocks that are clearly not Factorly configs
				// (e.g., .mcp.json examples, shell output, directory trees)
				if isNonConfigYAML(yamlContent) {
					continue
				}

				t.Run(blockName(i, yamlContent), func(t *testing.T) {
					validateYAMLBlock(t, yamlContent, name)
				})
			}
		})
	}
}

func validateYAMLBlock(t *testing.T, yamlContent, filename string) {
	t.Helper()

	// Try as full config (with tools: key)
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(yamlContent), &cfg); err == nil && len(cfg.Tools) > 0 {
		for toolName, tc := range cfg.Tools {
			validateTool(t, toolName, tc, filename)
		}
		return
	}

	// Try as bare tool map (tool files in .factorly/tools/)
	var tools map[string]config.ToolConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &tools); err == nil && len(tools) > 0 {
		for toolName, tc := range tools {
			validateTool(t, toolName, tc, filename)
		}
		return
	}

	// Could be a partial config (oauth_providers only, vault_backends only, etc.)
	// — that's fine, skip validation
}

func validateTool(t *testing.T, name string, tc config.ToolConfig, filename string) {
	t.Helper()

	// Built-in tools (factorly.*) may not have type in shadow override examples
	if tc.Type == "" {
		if strings.HasPrefix(name, "factorly.") {
			return // skip validation — built-in tools are auto-registered
		}
		t.Errorf("[%s] tool %q missing type", filename, name)
		return
	}

	validTypes := map[string]bool{"cli": true, "rest": true, "mcp": true, "workflow": true}
	if !validTypes[tc.Type] {
		t.Errorf("[%s] tool %q has invalid type %q", filename, name, tc.Type)
	}

	switch tc.Type {
	case "cli":
		if tc.Command == "" {
			t.Errorf("[%s] cli tool %q missing command", filename, name)
		}
	case "rest":
		if tc.BaseURL == "" {
			t.Errorf("[%s] rest tool %q missing base_url", filename, name)
		}
		if tc.Method == "" {
			t.Errorf("[%s] rest tool %q missing method", filename, name)
		}
	case "mcp":
		if tc.Command == "" && tc.URL == "" {
			t.Errorf("[%s] mcp tool %q needs either command or url", filename, name)
		}
	case "workflow":
		if len(tc.Steps) == 0 {
			t.Errorf("[%s] workflow tool %q has no steps", filename, name)
		}
	}
}

func isNonConfigYAML(content string) bool {
	trimmed := strings.TrimSpace(content)
	// Skip JSON-like blocks (.mcp.json examples)
	if strings.HasPrefix(trimmed, "{") {
		return true
	}
	// Skip blocks that are just comments
	lines := strings.Split(trimmed, "\n")
	allComments := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			allComments = false
			break
		}
	}
	if allComments {
		return true
	}
	// Skip directory tree examples
	if strings.Contains(trimmed, "├──") || strings.Contains(trimmed, "└──") {
		return true
	}
	// Skip shell output examples (lines starting with $)
	if strings.HasPrefix(trimmed, "$") {
		return true
	}
	// Skip partial auth snippets (comparison examples showing just auth: block)
	if strings.HasPrefix(trimmed, "auth:") {
		return true
	}
	// Skip JSONL log entry examples
	if strings.HasPrefix(trimmed, "{\"") {
		return true
	}
	return false
}

func blockName(index int, content string) string {
	// Use first meaningful line as the test name
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			if len(line) > 40 {
				line = line[:40]
			}
			return strings.ReplaceAll(line, " ", "_")
		}
	}
	return "block_" + strings.Repeat("0", 2-len(string(rune('0'+index)))) + string(rune('0'+index))
}

func findExamplesDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../docs/examples",
		"docs/examples",
		"../../docs/examples",
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				return abs
			}
		}
	}
	t.Skip("docs/examples directory not found")
	return ""
}
