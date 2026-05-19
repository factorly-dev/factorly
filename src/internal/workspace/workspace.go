// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package workspace loads named variable bundles that overlay the
// project config at config-load time. A workspace is a YAML file at
// .factorly/workspaces/<name>.yaml whose Vars map populates the env
// backend's overrides so {{env:NAME}} references in factorly.yaml
// resolve to the workspace's value before falling back to os.Getenv.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/factorly-dev/factorly/internal/projectpath"
)

// Workspace is a named bundle of variables that overlays the project
// config. Name is derived from the filename (sans .yaml extension);
// it isn't carried in the file itself.
type Workspace struct {
	Name        string            `yaml:"-"`
	Description string            `yaml:"description,omitempty"`
	Vars        map[string]string `yaml:"vars,omitempty"`
}

// ValidateName rejects workspace names that would let an attacker
// escape the .factorly/ tree or create surprising filenames. Empty
// names, path separators (`/` `\`), and dots (`.` covers both `..`
// traversal and hidden-file names) are all rejected. Callers that
// treat empty as "no workspace selected" should branch before
// calling this.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}
	if strings.ContainsAny(name, "/\\.") {
		return fmt.Errorf("workspace name %q must not contain path separators or dots", name)
	}
	return nil
}

// Load reads .factorly/workspaces/<name>.yaml relative to the active
// config and returns the parsed workspace. Empty name returns nil,
// nil — the caller treats "no workspace selected" as no overlay.
func Load(cfgPath, name string) (*Workspace, error) {
	if name == "" {
		return nil, nil
	}
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	dir := workspaceDir(cfgPath)
	if dir == "" {
		return nil, fmt.Errorf("workspace %q: no project directory (a project config is required)", name)
	}
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			available, _ := List(cfgPath)
			if len(available) == 0 {
				return nil, fmt.Errorf("workspace %q not found at %s (no workspaces defined)", name, path)
			}
			names := make([]string, 0, len(available))
			for _, w := range available {
				names = append(names, w.Name)
			}
			return nil, fmt.Errorf("workspace %q not found at %s (available: %s)", name, path, strings.Join(names, ", "))
		}
		return nil, fmt.Errorf("reading workspace %q: %w", name, err)
	}
	var ws Workspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("parsing workspace %q at %s: %w", name, path, err)
	}
	ws.Name = name
	if ws.Vars == nil {
		ws.Vars = map[string]string{}
	}
	return &ws, nil
}

// Exists reports whether a workspace file with the given name lives
// at .factorly/workspaces/<name>.yaml. Used by the bootstrap to decide
// whether to auto-select "default" when no --workspace flag is set.
func Exists(cfgPath, name string) bool {
	if ValidateName(name) != nil {
		return false
	}
	dir := workspaceDir(cfgPath)
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, name+".yaml"))
	return err == nil
}

// List returns every workspace file in .factorly/workspaces/ sorted by
// name. Missing directory is not an error — workspaces are optional.
func List(cfgPath string) ([]*Workspace, error) {
	dir := workspaceDir(cfgPath)
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing workspaces dir %s: %w", dir, err)
	}
	var out []*Workspace
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var ws Workspace
		if err := yaml.Unmarshal(data, &ws); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		ws.Name = name
		if ws.Vars == nil {
			ws.Vars = map[string]string{}
		}
		out = append(out, &ws)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save writes the workspace to .factorly/workspaces/<name>.yaml.
// The Name field is the source of truth for the filename; Description
// and Vars round-trip. Path traversal in Name is rejected.
func Save(cfgPath string, ws *Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}
	if err := ValidateName(ws.Name); err != nil {
		return err
	}
	dir := workspaceDir(cfgPath)
	if dir == "" {
		return fmt.Errorf("save workspace %q: no project directory", ws.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	// Stable output: marshal a struct without the unexported Name field.
	type onDisk struct {
		Description string            `yaml:"description,omitempty"`
		Vars        map[string]string `yaml:"vars,omitempty"`
	}
	data, err := yaml.Marshal(onDisk{Description: ws.Description, Vars: ws.Vars})
	if err != nil {
		return fmt.Errorf("marshal workspace %q: %w", ws.Name, err)
	}
	path := filepath.Join(dir, ws.Name+".yaml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Delete removes the workspace YAML file. The workspace's vault file
// (if any) is intentionally left alone — deleting both would be
// destructive and irreversible. Returns nil if the file didn't exist.
func Delete(cfgPath, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	dir := workspaceDir(cfgPath)
	if dir == "" {
		return fmt.Errorf("delete workspace %q: no project directory", name)
	}
	path := filepath.Join(dir, name+".yaml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// workspaceDir returns the absolute path to .factorly/workspaces/ for
// the given config, or "" when the config is global (no project to
// scope workspaces to).
func workspaceDir(cfgPath string) string {
	// projectpath.Resolve returns the global fallback for global configs;
	// we use an empty fallback so a global config yields no workspace dir.
	resolved := projectpath.Resolve(cfgPath, "workspaces", "")
	if resolved == "" {
		return ""
	}
	return resolved
}
