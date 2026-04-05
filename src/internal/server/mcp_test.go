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
		"echo.test": {Command: "echo", Args: []string{"{msg}"}},
	})

	handler := makeHandler(p, "echo.test")
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

	handler := makeHandler(p, "fail")
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
		"echo.test": {Command: "echo", Args: []string{"{msg}"}},
	})

	// Call a tool that doesn't exist
	handler := makeHandler(p, "nonexistent")
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
		"echo.test": {Command: "echo", Args: []string{"{msg}"}},
	})

	handler := makeHandler(p, "echo.test")
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
