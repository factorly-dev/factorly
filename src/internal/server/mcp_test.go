// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package server

import (
	"context"
	"testing"

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

	s := New(reg, p)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	// The server was created with 3 tools — we can't easily inspect
	// the internal state, but verify it doesn't panic and returns non-nil
}
