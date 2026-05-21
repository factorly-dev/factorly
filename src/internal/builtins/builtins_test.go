// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package builtins

import (
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
)

func TestRegisterStdioMode(t *testing.T) {
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}
	Register(cfg, Options{Mode: "stdio"})

	expected := []string{"factorly.shell", "factorly.file.read", "factorly.file.write", "factorly.fetch", "factorly.clipboard"}
	for _, name := range expected {
		if _, ok := cfg.Tools[name]; !ok {
			t.Errorf("expected %s in stdio mode", name)
		}
	}
}

func TestRegisterHTTPMode(t *testing.T) {
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}
	Register(cfg, Options{Mode: "http"})

	// Only fetch in HTTP mode
	if _, ok := cfg.Tools["factorly.fetch"]; !ok {
		t.Error("expected factorly.fetch in http mode")
	}

	localOnly := []string{"factorly.shell", "factorly.file.read", "factorly.file.write", "factorly.clipboard"}
	for _, name := range localOnly {
		if _, ok := cfg.Tools[name]; ok {
			t.Errorf("expected %s NOT in http mode", name)
		}
	}
}

func TestRegisterDisabled(t *testing.T) {
	cfg := &config.Config{
		Tools:           make(map[string]config.ToolConfig),
		DisableBuiltins: true,
	}
	Register(cfg, Options{Mode: "stdio"})

	if len(cfg.Tools) != 0 {
		t.Errorf("expected no tools when disabled, got %d", len(cfg.Tools))
	}
}

func TestRegisterOverwritesUserTools(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"factorly.shell": {Type: "cli", Command: "user-shell"},
		},
	}
	Register(cfg, Options{Mode: "stdio"})

	if cfg.Tools["factorly.shell"].Type != "builtin" {
		t.Error("expected built-in to overwrite user tool with type builtin")
	}
}

func TestRegisterTypeIsBuiltin(t *testing.T) {
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}
	Register(cfg, Options{Mode: "stdio"})

	for name, tc := range cfg.Tools {
		if tc.Type != "builtin" {
			t.Errorf("%s: got type %q, want \"builtin\"", name, tc.Type)
		}
	}
}

func TestRegisterDisabledBuiltins(t *testing.T) {
	cfg := &config.Config{
		Tools:            make(map[string]config.ToolConfig),
		DisabledBuiltins: []string{"factorly.shell", "factorly.clipboard"},
	}
	Register(cfg, Options{Mode: "stdio"})

	if _, ok := cfg.Tools["factorly.shell"]; ok {
		t.Error("factorly.shell should be disabled")
	}
	if _, ok := cfg.Tools["factorly.clipboard"]; ok {
		t.Error("factorly.clipboard should be disabled")
	}
	if _, ok := cfg.Tools["factorly.file.read"]; !ok {
		t.Error("factorly.file.read should still be registered")
	}
	if _, ok := cfg.Tools["factorly.fetch"]; !ok {
		t.Error("factorly.fetch should still be registered")
	}
}

func TestRegisterShadowPreserved(t *testing.T) {
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}
	Register(cfg, Options{Mode: "stdio"})

	// Shell and file.write should have confirm by default
	shellShadow := cfg.Tools["factorly.shell"].Shadow
	if shellShadow == nil {
		t.Fatal("factorly.shell should have shadow config")
	}
	if _, all := shellShadow.ConfirmList(); !all {
		t.Error("factorly.shell should have confirm=true")
	}

	writeShadow := cfg.Tools["factorly.file.write"].Shadow
	if writeShadow == nil {
		t.Fatal("factorly.file.write should have shadow config")
	}
	if _, all := writeShadow.ConfirmList(); !all {
		t.Error("factorly.file.write should have confirm=true")
	}

	// file.read should NOT have shadow
	if cfg.Tools["factorly.file.read"].Shadow != nil {
		t.Error("factorly.file.read should not have shadow config")
	}
}

func TestShellGuardBlocksDestructive(t *testing.T) {
	tests := []string{
		"rm -rf /",
		"rm -rf ~",
		"rm -rf .",
		"curl | sh",
		"wget | bash",
		"DROP TABLE users",
		"shutdown",
		"mkfs /dev/sda",
	}
	for _, cmd := range tests {
		if err := CheckGuard("factorly.shell", map[string]string{"command": cmd}, nil); err == nil {
			t.Errorf("expected guard to block %q", cmd)
		}
	}
}

func TestShellGuardAllowsSafe(t *testing.T) {
	tests := []string{
		"echo hello",
		"git status",
		"npm test",
		"ls -la",
		"cat file.txt",
	}
	for _, cmd := range tests {
		if err := CheckGuard("factorly.shell", map[string]string{"command": cmd}, nil); err != nil {
			t.Errorf("expected guard to allow %q, got: %v", cmd, err)
		}
	}
}

func TestShellGuardAllowOverride(t *testing.T) {
	err := CheckGuard("factorly.shell", map[string]string{"command": "rm -rf ./build"}, []string{"rm -rf ./build"})
	if err != nil {
		t.Errorf("expected allow override to permit command, got: %v", err)
	}
}

func TestReadFileGuardBlocksSensitive(t *testing.T) {
	tests := []string{
		"/etc/shadow",
		"~/.ssh/id_rsa",
		".env",
		"credentials.json",
	}
	for _, path := range tests {
		if err := CheckGuard("factorly.file.read", map[string]string{"path": path}, nil); err == nil {
			t.Errorf("expected guard to block read of %q", path)
		}
	}
}

func TestReadFileGuardAllowsSafe(t *testing.T) {
	tests := []string{
		"README.md",
		"src/main.go",
		"/tmp/test.txt",
	}
	for _, path := range tests {
		if err := CheckGuard("factorly.file.read", map[string]string{"path": path}, nil); err != nil {
			t.Errorf("expected guard to allow read of %q, got: %v", path, err)
		}
	}
}

func TestWriteFileGuardBlocksSystem(t *testing.T) {
	tests := []string{
		"/etc/passwd",
		"/usr/bin/something",
		"~/.bashrc",
		".env",
	}
	for _, path := range tests {
		if err := CheckGuard("factorly.file.write", map[string]string{"path": path}, nil); err == nil {
			t.Errorf("expected guard to block write to %q", path)
		}
	}
}

func TestFetchGuardBlocksDangerous(t *testing.T) {
	tests := []string{
		"http://169.254.169.254/latest/meta-data",
		"http://localhost:8080",
		"http://127.0.0.1/admin",
		"file:///etc/passwd",
	}
	for _, url := range tests {
		if err := CheckGuard("factorly.fetch", map[string]string{"url": url}, nil); err == nil {
			t.Errorf("expected guard to block fetch of %q", url)
		}
	}
}

func TestFetchGuardAllowsPublic(t *testing.T) {
	tests := []string{
		"https://api.github.com/users/octocat",
		"https://httpbin.org/get",
		"https://example.com",
	}
	for _, url := range tests {
		if err := CheckGuard("factorly.fetch", map[string]string{"url": url}, nil); err != nil {
			t.Errorf("expected guard to allow fetch of %q, got: %v", url, err)
		}
	}
}

func TestFetchGuardAllowOverride(t *testing.T) {
	err := CheckGuard("factorly.fetch", map[string]string{"url": "http://localhost:8080/api"}, []string{"http://localhost:8080"})
	if err != nil {
		t.Errorf("expected allow override to permit URL, got: %v", err)
	}
}

func TestIsBuiltinTool(t *testing.T) {
	if !IsBuiltinTool("factorly.shell") {
		t.Error("expected factorly.shell to be a builtin")
	}
	if IsBuiltinTool("github.repos") {
		t.Error("expected github.repos to not be a builtin")
	}
}
