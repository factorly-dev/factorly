// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

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

// --- upsertConfigMapEntry / deleteConfigMapEntry tests ---

func TestUpsertConfigMapEntry_NewEntry(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	p := config.OAuthProviderConfig{
		ClientID:     "abc",
		ClientSecret: "secret",
		AuthURL:      "https://auth.example.com",
		TokenURL:     "https://token.example.com",
		Scopes:       []string{"read", "write"},
	}

	if err := upsertConfigMapEntry(cfgPath, "oauth_providers", "github", p); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if !strings.Contains(content, "oauth_providers") {
		t.Fatal("oauth_providers section not created")
	}
	if !strings.Contains(content, "github") {
		t.Fatal("github provider not found")
	}
	if !strings.Contains(content, "abc") {
		t.Fatal("client_id not written")
	}
	// Verify tools section still exists
	if !strings.Contains(content, "tools") {
		t.Fatal("tools section was lost")
	}
}

func TestUpsertConfigMapEntry_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: old\n        client_secret: old_secret\n"), 0o644)

	p := config.OAuthProviderConfig{
		ClientID:     "new_id",
		ClientSecret: "new_secret",
		AuthURL:      "https://auth.example.com",
		TokenURL:     "https://token.example.com",
	}

	if err := upsertConfigMapEntry(cfgPath, "oauth_providers", "github", p); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if strings.Contains(content, "old") {
		t.Fatal("old values should be replaced")
	}
	if !strings.Contains(content, "new_id") {
		t.Fatal("new client_id not found")
	}
}

func TestDeleteConfigMapEntry(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: abc\n    google:\n        client_id: xyz\n"), 0o644)

	if err := deleteConfigMapEntry(cfgPath, "oauth_providers", "github"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if strings.Contains(content, "github") {
		t.Fatal("github should be removed")
	}
	if !strings.Contains(content, "google") {
		t.Fatal("google should still be present")
	}
}

func TestDeleteConfigMapEntry_LastEntry(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: abc\n"), 0o644)

	if err := deleteConfigMapEntry(cfgPath, "oauth_providers", "github"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if strings.Contains(content, "github") {
		t.Fatal("github should be removed")
	}
	// oauth_providers key should still exist (empty mapping)
	if !strings.Contains(content, "oauth_providers") {
		t.Fatal("oauth_providers key should remain")
	}
}

func TestSaveOAuthProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	p := config.OAuthProviderConfig{
		ClientID:     "id123",
		ClientSecret: "{{vault:MY_SECRET}}",
		AuthURL:      "https://auth.example.com",
		TokenURL:     "https://token.example.com",
		Scopes:       []string{"read"},
	}

	if err := SaveOAuthProvider(cfgPath, "myapp", p); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)

	// Verify field order: client_id should come first (struct field order)
	clientIdx := strings.Index(content, "client_id")
	authIdx := strings.Index(content, "auth_url")
	if clientIdx == -1 || authIdx == -1 {
		t.Fatal("expected both client_id and auth_url")
	}
	if clientIdx > authIdx {
		t.Error("client_id should appear before auth_url (struct field order)")
	}
}

func TestDeleteOAuthProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: x\n"), 0o644)

	if err := DeleteOAuthProvider(cfgPath, "github"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "github") {
		t.Fatal("github should be deleted")
	}
}

func TestSaveToolToConfig_PreservesOtherSections(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: abc\ntools:\n    echo:\n        type: cli\n        command: echo\n"), 0o644)

	tc := config.ToolConfig{
		Type:    "cli",
		Command: "ls",
	}
	if err := saveToolToConfig(cfgPath, "ls.tool", tc); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if !strings.Contains(content, "oauth_providers") {
		t.Fatal("oauth_providers section was lost")
	}
	if !strings.Contains(content, "github") {
		t.Fatal("github provider was lost")
	}
	if !strings.Contains(content, "ls.tool") {
		t.Fatal("new tool not written")
	}
	if !strings.Contains(content, "echo") {
		t.Fatal("existing tool was lost")
	}
}

func TestDeleteToolFromConfig_NonExistent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	original := "tools:\n    echo:\n        type: cli\n        command: echo\n"
	_ = os.WriteFile(cfgPath, []byte(original), 0o644)

	// Deleting a non-existent tool should be a no-op
	if err := deleteToolFromConfig(cfgPath, "nonexistent"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "echo") {
		t.Fatal("existing tool should not be affected")
	}
}

func TestSaveToolToConfig_NoToolsSection(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	// Config with no tools section at all
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: x\n"), 0o644)

	tc := config.ToolConfig{
		Type:    "cli",
		Command: "echo",
	}
	if err := saveToolToConfig(cfgPath, "echo", tc); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if !strings.Contains(content, "tools") {
		t.Fatal("tools section should be created")
	}
	if !strings.Contains(content, "echo") {
		t.Fatal("echo tool should be present")
	}
	if !strings.Contains(content, "oauth_providers") {
		t.Fatal("other sections should be preserved")
	}
}

func TestRemoveToolFromFile_DeletesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.yaml")
	_ = os.WriteFile(path, []byte("only.tool:\n    type: cli\n    command: echo\n"), 0o644)

	if err := removeToolFromFile(path, "only.tool"); err != nil {
		t.Fatal(err)
	}

	// File should be deleted since it's now empty
	if _, err := os.Stat(path); err == nil {
		t.Fatal("file should be deleted when last tool is removed")
	}
}

func TestUpdateToolInFile_PreservesOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ordered.yaml")
	_ = os.WriteFile(path, []byte("first:\n    type: cli\n    command: first\nsecond:\n    type: cli\n    command: second\nthird:\n    type: cli\n    command: third\n"), 0o644)

	tc := config.ToolConfig{
		Type:    "cli",
		Command: "updated_second",
	}
	if err := updateToolInFile(path, "second", tc); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	// Verify order preserved: first, second, third
	firstIdx := strings.Index(content, "first:")
	secondIdx := strings.Index(content, "second:")
	thirdIdx := strings.Index(content, "third:")

	if firstIdx > secondIdx || secondIdx > thirdIdx {
		t.Errorf("order not preserved: first=%d second=%d third=%d", firstIdx, secondIdx, thirdIdx)
	}

	if !strings.Contains(content, "updated_second") {
		t.Error("second tool should be updated")
	}
}

func TestSafePath(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"github.list_repos", true},
		{"my-tool", true},
		{"echo", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../etc/passwd", false},
		{"foo/bar", false},
		{"foo\\bar", false},
		{"foo..bar", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := safePath(tt.input)
			if tt.ok && err != nil {
				t.Errorf("safePath(%q) should succeed, got error: %v", tt.input, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("safePath(%q) should fail, got nil error", tt.input)
			}
		})
	}
}

func TestFindToolInDir(t *testing.T) {
	dir := t.TempDir()

	// Single-tool file
	_ = os.WriteFile(filepath.Join(dir, "echo.yaml"), []byte("echo:\n    type: cli\n    command: echo\n"), 0o644)
	// Multi-tool file
	_ = os.WriteFile(filepath.Join(dir, "git.yaml"), []byte("git.log:\n    type: cli\n    command: git\ngit.status:\n    type: cli\n    command: git\n"), 0o644)

	// Find in single-tool file
	path := findToolInDir(dir, "echo")
	if path == "" {
		t.Fatal("should find echo")
	}
	if !strings.HasSuffix(path, "echo.yaml") {
		t.Errorf("expected echo.yaml, got %s", path)
	}

	// Find in multi-tool file
	path = findToolInDir(dir, "git.log")
	if path == "" {
		t.Fatal("should find git.log")
	}
	if !strings.HasSuffix(path, "git.yaml") {
		t.Errorf("expected git.yaml, got %s", path)
	}

	// Not found
	path = findToolInDir(dir, "nonexistent")
	if path != "" {
		t.Errorf("should not find nonexistent, got %s", path)
	}
}
