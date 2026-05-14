// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package configyaml

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

func TestRenderTool_CLI(t *testing.T) {
	tc := config.ToolConfig{
		Type:        "cli",
		Description: "list files in cwd",
		Command:     "ls",
		Args:        []string{"-la"},
	}
	out, err := RenderTool("ls.list", tc)
	if err != nil {
		t.Fatalf("RenderTool: %v", err)
	}
	got := string(out)
	if !strings.HasPrefix(got, "tools:") {
		t.Errorf("expected top-level tools: key, got:\n%s", got)
	}
	if !strings.Contains(got, "ls.list:") {
		t.Errorf("expected tool name as nested key, got:\n%s", got)
	}
	if !strings.Contains(got, "command: ls") {
		t.Errorf("expected command field, got:\n%s", got)
	}

	// Round-trip back through the loader and assert the nested ToolConfig
	// matches the original.
	var doc struct {
		Tools map[string]config.ToolConfig `yaml:"tools"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if !reflect.DeepEqual(doc.Tools["ls.list"], tc) {
		t.Errorf("round-trip mismatch\nwant: %+v\ngot:  %+v", tc, doc.Tools["ls.list"])
	}
}

func TestRenderTool_REST(t *testing.T) {
	tc := config.ToolConfig{
		Type:        "rest",
		Description: "list issues",
		BaseURL:     "https://api.linear.app",
		Method:      "POST",
		Path:        "/graphql",
		Headers:     map[string]string{"Content-Type": "application/json"},
	}
	out, err := RenderTool("linear.issues", tc)
	if err != nil {
		t.Fatalf("RenderTool: %v", err)
	}
	var doc struct {
		Tools map[string]config.ToolConfig `yaml:"tools"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if !reflect.DeepEqual(doc.Tools["linear.issues"], tc) {
		t.Errorf("round-trip mismatch\nwant: %+v\ngot:  %+v", tc, doc.Tools["linear.issues"])
	}
}

func TestRenderTool_Workflow(t *testing.T) {
	tc := config.ToolConfig{
		Type:        "workflow",
		Description: "morning prep",
		Steps: []config.StepConfig{
			{Tool: "factorly.fetch", Params: map[string]string{"url": "https://example.com"}, Store: "data"},
		},
	}
	out, err := RenderTool("prep.daily", tc)
	if err != nil {
		t.Fatalf("RenderTool: %v", err)
	}
	if !strings.Contains(string(out), "type: workflow") {
		t.Errorf("expected type: workflow, got:\n%s", out)
	}

	var doc struct {
		Tools map[string]config.ToolConfig `yaml:"tools"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if !reflect.DeepEqual(doc.Tools["prep.daily"], tc) {
		t.Errorf("round-trip mismatch\nwant: %+v\ngot:  %+v", tc, doc.Tools["prep.daily"])
	}
}

func TestRenderTool_EmptyName(t *testing.T) {
	if _, err := RenderTool("", config.ToolConfig{Type: "cli"}); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := RenderTool("   ", config.ToolConfig{Type: "cli"}); err == nil {
		t.Error("expected error for whitespace-only name")
	}
}

func TestRenderBlueprint_RawBytes(t *testing.T) {
	dir := t.TempDir()
	bpDir := filepath.Join(dir, ".factorly", "blueprints")
	if err := os.MkdirAll(bpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".factorly", "factorly.yaml")

	// A blueprint file with a comment + custom field ordering — RenderBlueprint
	// must return these bytes verbatim, not re-marshal them.
	source := []byte(`# my custom comment
name: gmail
version: 1.0.0
tools:
  gmail.list:
    type: rest
    description: list messages
`)
	if err := os.WriteFile(filepath.Join(bpDir, "gmail.yaml"), source, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := RenderBlueprint(cfgPath, "gmail")
	if err != nil {
		t.Fatalf("RenderBlueprint: %v", err)
	}
	if string(got) != string(source) {
		t.Errorf("blueprint not returned byte-for-byte\nwant:\n%s\ngot:\n%s", source, got)
	}
}

func TestRenderBlueprint_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".factorly", "factorly.yaml")
	if _, err := RenderBlueprint(cfgPath, "nope"); err == nil {
		t.Error("expected error for missing blueprint")
	}
}

func TestRenderBlueprint_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".factorly", "factorly.yaml")
	cases := []string{"../etc/passwd", "..", "", ".", "foo/bar", `foo\bar`}
	for _, name := range cases {
		if _, err := RenderBlueprint(cfgPath, name); err == nil {
			t.Errorf("expected rejection for %q", name)
		}
	}
}

func TestBlueprintsDir(t *testing.T) {
	// cfgPath inside .factorly/ → use sibling blueprints/
	got := BlueprintsDir("/proj/.factorly/factorly.yaml")
	if got != "/proj/.factorly/blueprints" {
		t.Errorf("inside .factorly: got %q", got)
	}
	// cfgPath outside .factorly/ → add .factorly/blueprints/
	got = BlueprintsDir("/proj/factorly.yaml")
	if got != "/proj/.factorly/blueprints" {
		t.Errorf("outside .factorly: got %q", got)
	}
}
