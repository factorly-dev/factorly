// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package configyaml renders tool and blueprint definitions back to YAML.
// Used by the MCP resources surface, the UI "View YAML" page, and the
// `factorly tools show` / `factorly blueprint show` CLI subcommands.
package configyaml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

// RenderTool serializes a single tool (or workflow) config back to YAML in
// the same shape it would appear inside a loose .factorly/<file>.yaml — a
// top-level "tools:" map keyed by name.
func RenderTool(name string, tc config.ToolConfig) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("tool name is empty")
	}
	doc := map[string]map[string]config.ToolConfig{
		"tools": {name: tc},
	}
	return yaml.Marshal(doc)
}

// RenderBlueprint returns the raw bytes of the installed blueprint file at
// .factorly/blueprints/<name>.yaml. Raw bytes (not a re-marshal) preserve
// comments, key order, and any user edits.
func RenderBlueprint(cfgPath, name string) ([]byte, error) {
	safe, err := safeName(name)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(BlueprintsDir(cfgPath), safe+".yaml")
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from a sanitized name
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blueprint %q is not installed", name)
		}
		return nil, fmt.Errorf("reading blueprint %q: %w", name, err)
	}
	return data, nil
}

// BlueprintsDir returns the .factorly/blueprints/ directory for the given
// config path, regardless of whether cfgPath is inside .factorly/ already.
func BlueprintsDir(cfgPath string) string {
	cfgDir := filepath.Dir(cfgPath)
	if filepath.Base(cfgDir) == ".factorly" {
		return filepath.Join(cfgDir, "blueprints")
	}
	return filepath.Join(cfgDir, ".factorly", "blueprints")
}

// safeName rejects path traversal and separator characters so we never
// resolve a blueprint outside the blueprints directory.
func safeName(name string) (string, error) {
	s := strings.TrimSpace(name)
	if s == "" || s == "." || s == ".." {
		return "", fmt.Errorf("invalid blueprint name: %q", name)
	}
	if strings.ContainsAny(s, "/\\") || strings.Contains(s, "..") {
		return "", fmt.Errorf("invalid blueprint name: %q (contains path separator or traversal)", name)
	}
	return s, nil
}
