// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import (
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

func TestAllTemplatesHaveRequiredFields(t *testing.T) {
	for _, tmpl := range All() {
		if tmpl.Name == "" {
			t.Error("template has empty name")
		}
		if tmpl.DisplayName == "" {
			t.Errorf("template %q has empty display name", tmpl.Name)
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", tmpl.Name)
		}
		if tmpl.AuthType == "" {
			t.Errorf("template %q has empty auth type", tmpl.Name)
		}
		if tmpl.AuthType != "oauth" && tmpl.VaultKey == "" {
			t.Errorf("template %q has empty vault key", tmpl.Name)
		}
		if tmpl.AuthType == "oauth" && tmpl.OAuthConfig == nil {
			t.Errorf("template %q uses oauth but has no OAuthConfig", tmpl.Name)
		}
		if tmpl.YAML == "" {
			t.Errorf("template %q has empty YAML", tmpl.Name)
		}
	}
}

func TestAllTemplatesHaveTools(t *testing.T) {
	for _, tmpl := range All() {
		if tmpl.ToolCount() == 0 {
			t.Errorf("template %q has no tools", tmpl.Name)
		}
	}
}

func TestAllTemplatesYAMLValid(t *testing.T) {
	for _, tmpl := range All() {
		var tools map[string]config.ToolConfig
		if err := yaml.Unmarshal([]byte(tmpl.YAML), &tools); err != nil {
			t.Errorf("template %q has invalid YAML: %v", tmpl.Name, err)
			continue
		}
		for name, tc := range tools {
			if tc.Type != "rest" {
				t.Errorf("template %q tool %s: type=%q, expected rest", tmpl.Name, name, tc.Type)
			}
			if tc.BaseURL == "" {
				t.Errorf("template %q tool %s: empty base_url", tmpl.Name, name)
			}
			if tc.Method == "" {
				t.Errorf("template %q tool %s: empty method", tmpl.Name, name)
			}
			if tc.Auth == nil {
				t.Errorf("template %q tool %s: missing auth", tmpl.Name, name)
			}
		}
	}
}

func TestGetTemplate(t *testing.T) {
	tmpl := Get("linear")
	if tmpl == nil {
		t.Fatal("expected to find linear template")
	}
	if tmpl.DisplayName != "Linear" {
		t.Errorf("expected Linear, got %q", tmpl.DisplayName)
	}
}

func TestGetTemplateNotFound(t *testing.T) {
	tmpl := Get("nonexistent")
	if tmpl != nil {
		t.Error("expected nil for nonexistent template")
	}
}

func TestFilterYAML(t *testing.T) {
	tmpl := Get("github")
	if tmpl == nil {
		t.Fatal("expected to find github template")
	}

	// Filter to specific tools
	allNames := tmpl.ToolNames()
	if len(allNames) < 2 {
		t.Skip("github template has fewer than 2 tools")
	}

	selected := allNames[:2]
	filtered := tmpl.FilterYAML(selected)

	var tools map[string]any
	if err := yaml.Unmarshal([]byte(filtered), &tools); err != nil {
		t.Fatalf("failed to parse filtered YAML: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools in filtered YAML, got %d", len(tools))
	}

	// Empty selection returns all
	full := tmpl.FilterYAML(nil)
	var allTools map[string]any
	if err := yaml.Unmarshal([]byte(full), &allTools); err != nil {
		t.Fatalf("failed to parse full YAML: %v", err)
	}
	if len(allTools) != tmpl.ToolCount() {
		t.Errorf("expected %d tools in full YAML, got %d", tmpl.ToolCount(), len(allTools))
	}
}

func TestToolNames(t *testing.T) {
	tmpl := Get("github")
	if tmpl == nil {
		t.Fatal("expected to find github template")
	}
	names := tmpl.ToolNames()
	if len(names) == 0 {
		t.Error("expected tool names")
	}
	// Verify sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("tool names not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestTemplateCount(t *testing.T) {
	all := All()
	if len(all) != 36 {
		t.Errorf("expected 36 templates, got %d", len(all))
	}
}
