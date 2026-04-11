package main

import (
	"strings"
	"testing"
)

func TestDeriveNameFromCommand(t *testing.T) {
	tests := []struct {
		args     []string
		expected string
	}{
		{[]string{"npx", "@modelcontextprotocol/server-github"}, "github"},
		{[]string{"uvx", "mcp-server-fetch"}, "fetch"},
		{[]string{"python", "-m", "my_server"}, "my_server"},
		{[]string{"node", "server.js"}, "server"},
		{[]string{"npx", "@org/mcp-server-slack"}, "slack"},
		{[]string{"some-binary"}, "some-binary"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := deriveNameFromCommand(tt.args)
			if got != tt.expected {
				t.Errorf("deriveNameFromCommand(%v) = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

func TestDeriveNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://localhost:3001/mcp", "mcp"},
		{"http://localhost:3001/", "localhost"},
		{"https://api.example.com/v1/mcp", "mcp"},
		{"http://my-server:8080", "my-server"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := deriveNameFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("deriveNameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Server", "my-server"},
		{"test.tool", "test-tool"},
		{"UPPER", "upper"},
		{"already-clean", "already-clean"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
