package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
)

func TestSaveToolToDir_TypeFirst(t *testing.T) {
	dir := t.TempDir()

	tc := config.ToolConfig{
		Type:        "cli",
		Description: "lists files",
		Command:     "ls",
		Args:        []string{"-la"},
	}

	if err := saveToolToDir(dir, "fs.list", tc); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fs.list.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(data), "\n")
	// First line is the tool name key, second should be type
	var typeLine string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "type:") {
			typeLine = trimmed
			break
		}
		// Skip the tool name line
		if strings.HasSuffix(trimmed, ":") || trimmed == "" {
			continue
		}
		// If we hit a non-type field first, fail
		t.Fatalf("expected 'type:' as first field, got: %q", trimmed)
	}

	if typeLine != "type: cli" {
		t.Fatalf("expected 'type: cli', got: %q", typeLine)
	}
}

func TestSaveToolToConfig_TypeFirst(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")

	// Write a minimal config
	initial := []byte("tools: {}\n")
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	tc := config.ToolConfig{
		Type:        "rest",
		Description: "fetch data",
		Method:      "GET",
		BaseURL:     "https://api.example.com",
		Path:        "/data",
	}

	if err := saveToolToConfig(cfgPath, "api.fetch", tc); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	// Find the tool section and verify type comes first
	content := string(data)
	toolIdx := strings.Index(content, "api.fetch:")
	if toolIdx == -1 {
		t.Fatal("tool 'api.fetch' not found in output")
	}

	afterTool := content[toolIdx:]
	lines := strings.Split(afterTool, "\n")
	// Skip the "api.fetch:" line itself
	for _, l := range lines[1:] {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "type:") {
			if trimmed != "type: rest" {
				t.Fatalf("expected 'type: rest', got: %q", trimmed)
			}
			return
		}
		t.Fatalf("expected 'type:' as first field under tool, got: %q", trimmed)
	}
	t.Fatal("'type:' field not found")
}

func TestSaveToolToDir_NoDuplicate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	toolsDir := filepath.Join(dir, "tools")

	// Write config with an inline tool
	initial := []byte(`tools:
    my.tool:
        type: cli
        command: echo
`)
	if err := os.WriteFile(cfgPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	tc := config.ToolConfig{
		Type:        "cli",
		Description: "updated",
		Command:     "echo",
		Args:        []string{"hello"},
	}

	// SaveTool should remove from inline and write to dir
	if err := SaveTool(cfgPath, toolsDir, "my.tool", tc); err != nil {
		t.Fatal(err)
	}

	// Verify inline no longer has the tool
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "my.tool") {
		t.Fatal("tool should have been removed from inline config")
	}

	// Verify dir file exists
	dirFile := filepath.Join(toolsDir, "my.tool.yaml")
	if _, err := os.Stat(dirFile); err != nil {
		t.Fatalf("expected tool file in dir: %v", err)
	}
}

func TestSaveToolToDir_MultiToolFile(t *testing.T) {
	toolsDir := t.TempDir()

	// Write a multi-tool file
	multiFile := filepath.Join(toolsDir, "git.yaml")
	initial := []byte(`git.log:
    type: cli
    command: git
    args:
        - log
git.status:
    type: cli
    command: git
    args:
        - status
`)
	if err := os.WriteFile(multiFile, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	// Update git.log — should update in-place in git.yaml, not create git.log.yaml
	tc := config.ToolConfig{
		Type:    "cli",
		Command: "git",
		Args:    []string{"log", "--oneline"},
	}
	if err := saveToolToDir(toolsDir, "git.log", tc); err != nil {
		t.Fatal(err)
	}

	// Should NOT have created a new file
	if _, err := os.Stat(filepath.Join(toolsDir, "git.log.yaml")); err == nil {
		t.Fatal("should not have created git.log.yaml — tool exists in git.yaml")
	}

	// Verify git.yaml still has both tools
	data, err := os.ReadFile(multiFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "git.log") {
		t.Fatal("git.log should still be in git.yaml")
	}
	if !strings.Contains(content, "git.status") {
		t.Fatal("git.status should still be in git.yaml")
	}
	if !strings.Contains(content, "--oneline") {
		t.Fatal("git.log should have been updated with --oneline")
	}
}

func TestDeleteToolFromDir_MultiToolFile(t *testing.T) {
	toolsDir := t.TempDir()

	// Write a multi-tool file
	multiFile := filepath.Join(toolsDir, "git.yaml")
	initial := []byte(`git.log:
    type: cli
    command: git
git.status:
    type: cli
    command: git
`)
	if err := os.WriteFile(multiFile, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	// Delete git.log — should remove from file, keep git.status
	if err := deleteToolFromDir(toolsDir, "git.log"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(multiFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "git.log") {
		t.Fatal("git.log should have been removed")
	}
	if !strings.Contains(content, "git.status") {
		t.Fatal("git.status should still be present")
	}
}
