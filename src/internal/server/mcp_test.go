// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/mark3labs/mcp-go/mcp"
)

func buildTestProxy(tools map[string]provider.CLIToolDef) (*proxy.Proxy, *registry.Registry) {
	reg := registry.New()
	for name := range tools {
		reg.Register(&registry.Tool{
			Name:        name,
			Type:        "cli",
			Description: "test tool " + name,
			Parameters: []registry.Parameter{
				{Name: "msg", Description: "message", Required: true},
			},
			ProviderKey: "cli",
		})
	}
	providers := map[string]provider.Provider{
		"cli": provider.NewCLI(tools),
	}
	p := proxy.New(reg, providers, logger.NopLogger{})
	return p, reg
}

func TestConvertTool(t *testing.T) {
	tool := &registry.Tool{
		Name:        "web.fetch",
		Description: "Fetch a webpage",
		Parameters: []registry.Parameter{
			{Name: "url", Description: "URL to fetch", Required: true},
			{Name: "timeout", Description: "Timeout in seconds", Required: false},
		},
	}

	mcpTool := convertTool(tool)

	if mcpTool.Name != "web.fetch" {
		t.Errorf("expected name web.fetch, got %s", mcpTool.Name)
	}

	schema := mcpTool.InputSchema
	if _, ok := schema.Properties["url"]; !ok {
		t.Error("expected url property in schema")
	}
	if _, ok := schema.Properties["timeout"]; !ok {
		t.Error("expected timeout property in schema")
	}

	// Check required
	hasURL := false
	for _, r := range schema.Required {
		if r == "url" {
			hasURL = true
		}
	}
	if !hasURL {
		t.Error("expected url to be required")
	}
}

func TestConvertToolNoParams(t *testing.T) {
	tool := &registry.Tool{
		Name:        "simple",
		Description: "No params",
		Parameters:  nil,
	}

	mcpTool := convertTool(tool)
	if mcpTool.Name != "simple" {
		t.Errorf("expected name simple, got %s", mcpTool.Name)
	}
}

func TestMakeHandlerSuccess(t *testing.T) {
	p, _ := buildTestProxy(map[string]provider.CLIToolDef{
		"echo.test": {Command: "echo", Args: []string{"{{msg}}"}},
	})

	handler := makeHandler(p, "echo.test", nil)
	req := mcp.CallToolRequest{}
	req.Params.Name = "echo.test"
	req.Params.Arguments = map[string]any{"msg": "hello"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Error("expected success, got error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", text)
	}
}

func TestMakeHandlerToolError(t *testing.T) {
	p, _ := buildTestProxy(map[string]provider.CLIToolDef{
		"fail": {Command: "false", Args: []string{}},
	})

	handler := makeHandler(p, "fail", nil)
	req := mcp.CallToolRequest{}
	req.Params.Name = "fail"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for failing command")
	}
}

func TestMakeHandlerProxyError(t *testing.T) {
	p, _ := buildTestProxy(map[string]provider.CLIToolDef{
		"echo.test": {Command: "echo", Args: []string{"{{msg}}"}},
	})

	// Call a tool that doesn't exist
	handler := makeHandler(p, "nonexistent", nil)
	req := mcp.CallToolRequest{}
	req.Params.Name = "nonexistent"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result for missing tool")
	}
}

func TestMakeHandlerParamExtraction(t *testing.T) {
	p, _ := buildTestProxy(map[string]provider.CLIToolDef{
		"echo.test": {Command: "echo", Args: []string{"{{msg}}"}},
	})

	handler := makeHandler(p, "echo.test", nil)
	req := mcp.CallToolRequest{}
	req.Params.Name = "echo.test"
	// Test with a numeric value — should be converted to string
	req.Params.Arguments = map[string]any{"msg": 42}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "42\n" {
		t.Errorf("expected '42\\n', got %q", text)
	}
}

func TestRedactSensitiveParams(t *testing.T) {
	params := map[string]string{
		"url":           "https://example.com",
		"token":         "secret-token",
		"api_key":       "my-key",
		"Authorization": "Bearer xyz",
		"password":      "hunter2",
		"client_secret": "shh",
		"name":          "visible",
		"query":         "SELECT 1",
	}

	redacted := redactSensitiveParams(params)

	// Sensitive params should be redacted
	if redacted["token"] != "[REDACTED]" {
		t.Errorf("expected token redacted, got %q", redacted["token"])
	}
	if redacted["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key redacted, got %q", redacted["api_key"])
	}
	if redacted["Authorization"] != "[REDACTED]" {
		t.Errorf("expected Authorization redacted, got %q", redacted["Authorization"])
	}
	if redacted["password"] != "[REDACTED]" {
		t.Errorf("expected password redacted, got %q", redacted["password"])
	}
	if redacted["client_secret"] != "[REDACTED]" {
		t.Errorf("expected client_secret redacted, got %q", redacted["client_secret"])
	}

	// Non-sensitive params should be visible
	if redacted["url"] != "https://example.com" {
		t.Errorf("expected url visible, got %q", redacted["url"])
	}
	if redacted["name"] != "visible" {
		t.Errorf("expected name visible, got %q", redacted["name"])
	}
	if redacted["query"] != "SELECT 1" {
		t.Errorf("expected query visible, got %q", redacted["query"])
	}
}

func TestRedactSensitiveParamsEmpty(t *testing.T) {
	redacted := redactSensitiveParams(map[string]string{})
	if len(redacted) != 0 {
		t.Errorf("expected empty map, got %d entries", len(redacted))
	}
}

func TestNewRegistersAllTools(t *testing.T) {
	reg := registry.New()
	reg.Register(&registry.Tool{Name: "tool.a", Type: "cli", ProviderKey: "cli"})
	reg.Register(&registry.Tool{Name: "tool.b", Type: "cli", ProviderKey: "cli"})
	reg.Register(&registry.Tool{Name: "tool.c", Type: "cli", ProviderKey: "cli"})

	cliTools := map[string]provider.CLIToolDef{
		"tool.a": {Command: "echo", Args: []string{"a"}},
		"tool.b": {Command: "echo", Args: []string{"b"}},
		"tool.c": {Command: "echo", Args: []string{"c"}},
	}
	providers := map[string]provider.Provider{
		"cli": provider.NewCLI(cliTools),
	}
	p := proxy.New(reg, providers, logger.NopLogger{})

	s := New(reg, p, nil, "")
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	// The server was created with 3 tools — we can't easily inspect
	// the internal state, but verify it doesn't panic and returns non-nil
}

// resourceTestFixture builds a temp .factorly/ tree with one tool, one
// workflow, and one installed blueprint, returning the cfg + cfgPath the
// MCP server should be wired against.
func resourceTestFixture(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	factorlyDir := filepath.Join(dir, ".factorly")
	bpDir := filepath.Join(factorlyDir, "blueprints")
	if err := os.MkdirAll(bpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(factorlyDir, "factorly.yaml")
	if err := os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bpDir, "gmail.yaml"), []byte("name: gmail\nversion: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool":     {Type: "cli", Description: "list", Command: "ls"},
			"my.workflow": {Type: "workflow", Description: "wf", Steps: []config.StepConfig{{Tool: "my.tool"}}},
		},
	}
	return cfg, cfgPath
}

func TestNew_RegistersResources(t *testing.T) {
	cfg, cfgPath := resourceTestFixture(t)
	p, reg := buildTestProxy(map[string]provider.CLIToolDef{"my.tool": {Command: "ls"}})
	_ = reg

	s := New(reg, p, cfg, cfgPath)

	resources := s.ListResources()
	want := []string{
		"factorly://tools/my.tool",
		"factorly://workflows/my.workflow",
		"factorly://blueprints/gmail",
	}
	for _, uri := range want {
		if _, ok := resources[uri]; !ok {
			t.Errorf("expected resource %q registered, got URIs: %v", uri, resourceURIs(resources))
		}
	}
}

func TestRegisteredResource_ReadReturnsYAML(t *testing.T) {
	cfg, cfgPath := resourceTestFixture(t)
	p, reg := buildTestProxy(map[string]provider.CLIToolDef{"my.tool": {Command: "ls"}})

	s := New(reg, p, cfg, cfgPath)

	cases := []struct {
		uri         string
		mustContain []string
	}{
		{"factorly://tools/my.tool", []string{"my.tool", "command: ls"}},
		{"factorly://workflows/my.workflow", []string{"my.workflow", "type: workflow"}},
		{"factorly://blueprints/gmail", []string{"name: gmail", "version: 1.0.0"}},
	}
	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			res, ok := s.ListResources()[tc.uri]
			if !ok {
				t.Fatalf("resource %q not registered", tc.uri)
			}
			req := mcp.ReadResourceRequest{}
			req.Params.URI = tc.uri
			contents, err := res.Handler(context.Background(), req)
			if err != nil {
				t.Fatalf("handler: %v", err)
			}
			if len(contents) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(contents))
			}
			tc2, ok := contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("expected TextResourceContents, got %T", contents[0])
			}
			if tc2.MIMEType != "application/yaml" {
				t.Errorf("MIME = %q, want application/yaml", tc2.MIMEType)
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(tc2.Text, want) {
					t.Errorf("body missing %q\ngot:\n%s", want, tc2.Text)
				}
			}
		})
	}
}

func TestRefreshResources_AddsAndRemoves(t *testing.T) {
	cfg, cfgPath := resourceTestFixture(t)
	p, reg := buildTestProxy(map[string]provider.CLIToolDef{"my.tool": {Command: "ls"}})

	s := New(reg, p, cfg, cfgPath)

	// Add a new tool and remove the workflow; refresh should reflect both.
	delete(cfg.Tools, "my.workflow")
	cfg.Tools["new.tool"] = config.ToolConfig{Type: "cli", Command: "pwd"}

	RefreshResources(s, cfg, cfgPath)

	resources := s.ListResources()
	if _, ok := resources["factorly://tools/new.tool"]; !ok {
		t.Error("expected factorly://tools/new.tool after refresh")
	}
	if _, ok := resources["factorly://workflows/my.workflow"]; ok {
		t.Error("factorly://workflows/my.workflow should have been removed after refresh")
	}
	if _, ok := resources["factorly://tools/my.tool"]; !ok {
		t.Error("factorly://tools/my.tool should still be registered after refresh")
	}
	if _, ok := resources["factorly://blueprints/gmail"]; !ok {
		t.Error("factorly://blueprints/gmail should still be registered after refresh")
	}

	// Install a new blueprint by writing a file; refresh again.
	bpDir := filepath.Join(filepath.Dir(cfgPath), "blueprints")
	if err := os.WriteFile(filepath.Join(bpDir, "slack.yaml"), []byte("name: slack\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RefreshResources(s, cfg, cfgPath)
	if _, ok := s.ListResources()["factorly://blueprints/slack"]; !ok {
		t.Error("expected factorly://blueprints/slack after second refresh")
	}
}

// resourceURIs returns the set of registered URIs for assertion messages.
// Typed as a generic map so the test stays decoupled from the mcp-go value
// type (which collides namespace-wise with our package name).
func resourceURIs[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
