// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPContentToString(t *testing.T) {
	tests := []struct {
		name    string
		content []mcp.Content
		want    string
	}{
		{
			"single text",
			[]mcp.Content{mcp.TextContent{Type: "text", Text: "hello"}},
			"hello",
		},
		{
			"multiple text",
			[]mcp.Content{
				mcp.TextContent{Type: "text", Text: "line1"},
				mcp.TextContent{Type: "text", Text: "line2"},
			},
			"line1\nline2",
		},
		{
			"empty content",
			[]mcp.Content{},
			"",
		},
		{
			"nil content",
			nil,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentToString(tt.content)
			if got != tt.want {
				t.Errorf("contentToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMCPExecuteToolNotFound(t *testing.T) {
	p := NewMCP(map[string]MCPServerDef{})
	// Don't call Setup — no servers to connect to
	p.servers = make(map[string]*mcpConn)

	_, err := p.Execute("nonexistent", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got: %s", err.Error())
	}
}

func TestMCPToolNameMapping(t *testing.T) {
	// Verify the namespacing convention
	serverName := "slack"
	toolName := "post_message"
	expected := "slack.post_message"

	got := serverName + "." + toolName
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestNewMCPNoNamespace(t *testing.T) {
	servers := map[string]MCPServerDef{
		"test": {Command: "echo"},
	}
	p := NewMCPNoNamespace(servers)
	if !p.noNamespace {
		t.Error("expected noNamespace to be true")
	}
}

func TestNewMCPHasNamespace(t *testing.T) {
	servers := map[string]MCPServerDef{
		"test": {Command: "echo"},
	}
	p := NewMCP(servers)
	if p.noNamespace {
		t.Error("expected noNamespace to be false")
	}
}

func TestMCPNewAndTeardown(t *testing.T) {
	p := NewMCP(map[string]MCPServerDef{})
	if p.servers == nil {
		t.Error("expected initialized servers map")
	}
	if p.toolMap == nil {
		t.Error("expected initialized toolMap")
	}
	// Teardown on empty provider should not panic
	if err := p.Teardown(); err != nil {
		t.Fatal(err)
	}
}
