package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "factorly.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadBasicConfig(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  web.fetch:
    type: cli
    description: "Fetch a webpage"
    command: curl
    args: ["-s", "{url}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cfg.Tools))
	}

	tool, ok := cfg.Tools["web.fetch"]
	if !ok {
		t.Fatal("expected tool web.fetch")
	}
	if tool.Type != "cli" {
		t.Errorf("expected type cli, got %s", tool.Type)
	}
	if tool.Command != "curl" {
		t.Errorf("expected command curl, got %s", tool.Command)
	}
	if tool.Description != "Fetch a webpage" {
		t.Errorf("expected description 'Fetch a webpage', got %q", tool.Description)
	}
}

func TestLoadMultipleTools(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{url}"]
  file.read:
    type: cli
    command: cat
    args: ["{path}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.Tools))
	}
}

func TestEnvVarResolution(t *testing.T) {
	t.Setenv("FACTORLY_TEST_TOKEN", "secret123")

	path := writeTestConfig(t, `
tools:
  api:
    type: cli
    command: curl
    args: ["-H", "Authorization: ${FACTORLY_TEST_TOKEN}", "{url}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api"]
	if tool.Args[1] != "Authorization: secret123" {
		t.Errorf("expected resolved env var, got %q", tool.Args[1])
	}
}

func TestEnvVarUnsetLeftAsIs(t *testing.T) {
	os.Unsetenv("FACTORLY_NONEXISTENT_VAR")

	path := writeTestConfig(t, `
tools:
  api:
    type: cli
    command: curl
    args: ["${FACTORLY_NONEXISTENT_VAR}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api"]
	if tool.Args[0] != "${FACTORLY_NONEXISTENT_VAR}" {
		t.Errorf("expected unresolved placeholder, got %q", tool.Args[0])
	}
}

func TestEnvVarInEnvMap(t *testing.T) {
	t.Setenv("FACTORLY_TEST_KEY", "mykey")

	path := writeTestConfig(t, `
tools:
  api:
    type: cli
    command: curl
    args: ["{url}"]
    env:
      API_KEY: "${FACTORLY_TEST_KEY}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api"]
	if tool.Env["API_KEY"] != "mykey" {
		t.Errorf("expected resolved env in env map, got %q", tool.Env["API_KEY"])
	}
}

func TestParameterInference(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "-o", "{output}", "{url}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["web.fetch"]
	if len(tool.Parameters) != 2 {
		t.Fatalf("expected 2 inferred params, got %d", len(tool.Parameters))
	}

	names := tool.ParamNames()
	// Order depends on regex match order in args
	hasOutput := false
	hasURL := false
	for _, n := range names {
		if n == "output" {
			hasOutput = true
		}
		if n == "url" {
			hasURL = true
		}
	}
	if !hasOutput || !hasURL {
		t.Errorf("expected params [output, url], got %v", names)
	}
}

func TestParameterInferenceSkipsExplicit(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{url}"]
    parameters:
      - name: url
        description: "The URL to fetch"
        required: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["web.fetch"]
	if len(tool.Parameters) != 1 {
		t.Fatalf("expected 1 explicit param, got %d", len(tool.Parameters))
	}
	if tool.Parameters[0].Description != "The URL to fetch" {
		t.Errorf("expected explicit description, got %q", tool.Parameters[0].Description)
	}
}

func TestParameterDeduplication(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  test:
    type: cli
    command: echo
    args: ["{name}", "hello", "{name}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["test"]
	if len(tool.Parameters) != 1 {
		t.Fatalf("expected 1 deduplicated param, got %d", len(tool.Parameters))
	}
}

func TestValidationNoTools(t *testing.T) {
	path := writeTestConfig(t, `
tools: {}
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty tools")
	}
}

func TestValidationMissingType(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  broken:
    command: echo
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestValidationUnknownType(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  broken:
    type: graphql
    command: echo
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidationCLIMissingCommand(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  broken:
    type: cli
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for cli tool missing command")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/factorly.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  - this is not valid
  : broken yaml {{{}
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestHasPlaceholder(t *testing.T) {
	args := []string{"-s", "{url}", "-o", "{output}"}
	if !HasPlaceholder(args, "url") {
		t.Error("expected to find {url}")
	}
	if !HasPlaceholder(args, "output") {
		t.Error("expected to find {output}")
	}
	if HasPlaceholder(args, "missing") {
		t.Error("did not expect to find {missing}")
	}
}
