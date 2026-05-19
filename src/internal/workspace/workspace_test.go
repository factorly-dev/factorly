// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup creates a temp project dir with .factorly/factorly.yaml and
// returns the config path. Tests place workspace files alongside it.
func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	factDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(factDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(factDir, "factorly.yaml")
	if err := os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func writeWorkspace(t *testing.T, cfgPath, name, body string) {
	t.Helper()
	dir := workspaceDir(cfgPath)
	if dir == "" {
		t.Fatalf("workspaceDir empty for %q", cfgPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadEmptyNameReturnsNil(t *testing.T) {
	cfg := setup(t)
	ws, err := Load(cfg, "")
	if err != nil {
		t.Fatalf("expected no error for empty name, got %v", err)
	}
	if ws != nil {
		t.Errorf("expected nil workspace for empty name, got %+v", ws)
	}
}

func TestLoadHappyPath(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "staging", `description: Staging environment
vars:
  API_BASE: https://api.staging.example.com
  LOG_LEVEL: debug
`)
	ws, err := Load(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Name != "staging" {
		t.Errorf("Name = %q, want staging", ws.Name)
	}
	if ws.Description != "Staging environment" {
		t.Errorf("Description = %q", ws.Description)
	}
	if ws.Vars["API_BASE"] != "https://api.staging.example.com" {
		t.Errorf("Vars[API_BASE] = %q", ws.Vars["API_BASE"])
	}
	if ws.Vars["LOG_LEVEL"] != "debug" {
		t.Errorf("Vars[LOG_LEVEL] = %q", ws.Vars["LOG_LEVEL"])
	}
}

func TestLoadUnknownWorkspaceListsAvailable(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "staging", "vars: {}\n")
	writeWorkspace(t, cfg, "prod", "vars: {}\n")
	_, err := Load(cfg, "ghost")
	if err == nil {
		t.Fatal("expected error for unknown workspace")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"ghost"`) {
		t.Errorf("error should name the missing workspace: %s", msg)
	}
	if !strings.Contains(msg, "staging") || !strings.Contains(msg, "prod") {
		t.Errorf("error should list available workspaces: %s", msg)
	}
}

func TestLoadUnknownWorkspaceNoWorkspacesYet(t *testing.T) {
	cfg := setup(t)
	_, err := Load(cfg, "staging")
	if err == nil {
		t.Fatal("expected error when no workspaces exist")
	}
	if !strings.Contains(err.Error(), "no workspaces defined") {
		t.Errorf("error should mention no workspaces defined: %s", err)
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "broken", "vars: [not-a-map\n")
	_, err := Load(cfg, "broken")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parsing workspace") {
		t.Errorf("error should mention parsing: %s", err)
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	cfg := setup(t)
	for _, bad := range []string{"../etc", "a/b", "..", "name.with.dot"} {
		if _, err := Load(cfg, bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// TestValidateName exercises the exported validation function that
// every workspace path-builder now routes through. Callers outside
// this package (e.g. cmd/factorly) can use it to give the same error
// message everywhere a user-supplied workspace name lands.
func TestValidateName(t *testing.T) {
	good := []string{"staging", "prod", "dev-1", "team_a", "my-workspace"}
	bad := []struct {
		name, why string
	}{
		{"", "empty"},
		{"..", "traversal"},
		{"../etc", "explicit traversal"},
		{"a/b", "forward slash"},
		{"a\\b", "backslash"},
		{".hidden", "leading dot"},
		{"name.with.dot", "interior dot"},
		{"trailing.", "trailing dot"},
	}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("expected %q to validate, got %v", n, err)
		}
	}
	for _, c := range bad {
		if err := ValidateName(c.name); err == nil {
			t.Errorf("expected %q (%s) to fail validation", c.name, c.why)
		}
	}
}

func TestListEmpty(t *testing.T) {
	cfg := setup(t)
	wss, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 0 {
		t.Errorf("expected empty list, got %d", len(wss))
	}
}

func TestListSortsByName(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "prod", "vars: {API_BASE: https://prod}\n")
	writeWorkspace(t, cfg, "dev", "vars: {API_BASE: https://dev}\n")
	writeWorkspace(t, cfg, "staging", "vars: {}\n")

	wss, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 3 {
		t.Fatalf("expected 3 workspaces, got %d", len(wss))
	}
	want := []string{"dev", "prod", "staging"}
	for i, w := range wss {
		if w.Name != want[i] {
			t.Errorf("workspace %d: got %q, want %q", i, w.Name, want[i])
		}
	}
}

func TestListSkipsNonYAML(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "dev", "vars: {}\n")
	// Drop a stray non-yaml file in the same dir
	dir := workspaceDir(cfg)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	wss, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(wss) != 1 || wss[0].Name != "dev" {
		t.Errorf("expected just dev, got %+v", wss)
	}
}

func TestGlobalConfigYieldsNoWorkspaceDir(t *testing.T) {
	// A config under ~/.config/factorly/ — projectpath returns "".
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	globalCfg := filepath.Join(home, ".config", "factorly", "factorly.yaml")
	if got := workspaceDir(globalCfg); got != "" {
		t.Errorf("expected empty workspace dir for global cfg, got %q", got)
	}
}

func TestExistsReturnsTrueForExistingFile(t *testing.T) {
	cfg := setup(t)
	writeWorkspace(t, cfg, "default", "vars: {}\n")
	if !Exists(cfg, "default") {
		t.Error("expected Exists to return true for existing workspace")
	}
}

func TestExistsReturnsFalseForMissingFile(t *testing.T) {
	cfg := setup(t)
	if Exists(cfg, "default") {
		t.Error("expected Exists to return false when no workspace file present")
	}
}

func TestExistsRejectsPathTraversal(t *testing.T) {
	cfg := setup(t)
	for _, bad := range []string{"", "../etc", "a/b", "name.with.dot"} {
		if Exists(cfg, bad) {
			t.Errorf("Exists(%q) should return false", bad)
		}
	}
}

func TestExistsReturnsFalseForGlobalConfig(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	globalCfg := filepath.Join(home, ".config", "factorly", "factorly.yaml")
	if Exists(globalCfg, "default") {
		t.Error("Exists should return false for global cfg path")
	}
}
