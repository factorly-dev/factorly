// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package registry

import (
	"testing"
)

func TestRegisterAndGet(t *testing.T) {
	reg := New()
	reg.Register(&Tool{
		Name:        "web.fetch",
		Type:        "cli",
		Description: "Fetch a webpage",
		ProviderKey: "cli",
	})

	tool, err := reg.Get("web.fetch")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "web.fetch" {
		t.Errorf("expected web.fetch, got %s", tool.Name)
	}
	if tool.Description != "Fetch a webpage" {
		t.Errorf("expected 'Fetch a webpage', got %s", tool.Description)
	}
}

func TestGetNotFound(t *testing.T) {
	reg := New()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestListSorted(t *testing.T) {
	reg := New()
	reg.Register(&Tool{Name: "z.tool", ProviderKey: "cli"})
	reg.Register(&Tool{Name: "a.tool", ProviderKey: "cli"})
	reg.Register(&Tool{Name: "m.tool", ProviderKey: "cli"})

	tools := reg.List()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	if tools[0].Name != "a.tool" {
		t.Errorf("expected a.tool first, got %s", tools[0].Name)
	}
	if tools[1].Name != "m.tool" {
		t.Errorf("expected m.tool second, got %s", tools[1].Name)
	}
	if tools[2].Name != "z.tool" {
		t.Errorf("expected z.tool third, got %s", tools[2].Name)
	}
}

func TestListEmpty(t *testing.T) {
	reg := New()
	tools := reg.List()
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
}

func TestRegisterOverwrite(t *testing.T) {
	reg := New()
	reg.Register(&Tool{Name: "test", Description: "first", ProviderKey: "cli"})
	reg.Register(&Tool{Name: "test", Description: "second", ProviderKey: "cli"})

	tool, err := reg.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Description != "second" {
		t.Errorf("expected overwritten description 'second', got %q", tool.Description)
	}
}
