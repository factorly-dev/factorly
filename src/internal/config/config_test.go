// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/factorly-dev/factorly/internal/vault"
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
    args: ["-s", "{{url}}"]
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
    args: ["-s", "{{url}}"]
  file.read:
    type: cli
    command: cat
    args: ["{{path}}"]
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
    args: ["-H", "Authorization: {{env:FACTORLY_TEST_TOKEN}}", "{{url}}"]
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
    args: ["{{env:FACTORLY_NONEXISTENT_VAR}}"]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api"]
	if tool.Args[0] != "{{env:FACTORLY_NONEXISTENT_VAR}}" {
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
    args: ["{{url}}"]
    env:
      API_KEY: "{{env:FACTORLY_TEST_KEY}}"
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
    args: ["-s", "-o", "{{output}}", "{{url}}"]
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
    args: ["-s", "{{url}}"]
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
    args: ["{{name}}", "hello", "{{name}}"]
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
	// Empty tools are allowed — built-in tools will be added after config load
	path := writeTestConfig(t, `
tools: {}
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error for empty tools (built-ins fill them), got: %v", err)
	}
	if cfg.Tools == nil {
		t.Fatal("expected non-nil tools map")
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
	args := []string{"-s", "{{url}}", "-o", "{{output}}"}
	if !HasPlaceholder(args, "url") {
		t.Error("expected to find {{url}}")
	}
	if !HasPlaceholder(args, "output") {
		t.Error("expected to find {{output}}")
	}
	if HasPlaceholder(args, "missing") {
		t.Error("did not expect to find {{missing}}")
	}
}

// --- Tool Directory Tests ---

func writeToolDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestToolsDirLoading(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"echo.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	configContent := `
tools_dir: ` + toolsDir + `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
`
	path := writeTestConfig(t, configContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.Tools))
	}
	if _, ok := cfg.Tools["web.fetch"]; !ok {
		t.Error("expected tool web.fetch from inline config")
	}
	if _, ok := cfg.Tools["echo.test"]; !ok {
		t.Error("expected tool echo.test from tools dir")
	}
}

func TestToolsDirMultipleFiles(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"a.yaml": `
tool.a:
  type: cli
  command: echo
  args: ["a"]
`,
		"b.yml": `
tool.b:
  type: cli
  command: echo
  args: ["b"]
`,
	})

	configContent := `
tools_dir: ` + toolsDir + `
tools:
  tool.c:
    type: cli
    command: echo
    args: ["c"]
`
	path := writeTestConfig(t, configContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(cfg.Tools))
	}
}

func TestToolsDirConflictWithInline(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"web.yaml": `
web.fetch:
  type: cli
  command: wget
  args: ["{{url}}"]
`,
	})

	configContent := `
tools_dir: ` + toolsDir + `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
`
	path := writeTestConfig(t, configContent)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for duplicate tool across inline and dir")
	}
}

func TestToolsDirConflictAcrossFiles(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"a.yaml": `
dupe:
  type: cli
  command: echo
  args: ["a"]
`,
		"b.yaml": `
dupe:
  type: cli
  command: echo
  args: ["b"]
`,
	})

	configContent := `tools_dir: ` + toolsDir + `
tools: {}
`
	path := writeTestConfig(t, configContent)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for duplicate tool across dir files")
	}
}

func TestToolsDirOnly(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"tools.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	configContent := `
tools_dir: ` + toolsDir + `
tools: {}
`
	// Need at least one tool total to pass validation, so use dir-only
	path := writeTestConfig(t, configContent)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool from dir, got %d", len(cfg.Tools))
	}
}

func TestLoadDirDirectly(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"tools.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
web.fetch:
  type: cli
  command: curl
  args: ["-s", "{{url}}"]
`,
	})

	cfg, err := LoadDir(toolsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg.Tools))
	}
}

func TestToolsDirIgnoresNonYAML(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"tools.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
		"readme.txt": `this is not a yaml file`,
		".hidden":    `also not yaml`,
	})

	cfg, err := LoadDir(toolsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool (ignoring non-yaml), got %d", len(cfg.Tools))
	}
}

func TestToolsDirParameterInference(t *testing.T) {
	toolsDir := writeToolDir(t, map[string]string{
		"tools.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}", "{{name}}"]
`,
	})

	cfg, err := LoadDir(toolsDir)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["echo.test"]
	if len(tool.Parameters) != 2 {
		t.Fatalf("expected 2 inferred params, got %d", len(tool.Parameters))
	}
}

func TestToolsDirMissing(t *testing.T) {
	configContent := `
tools_dir: /nonexistent/tools
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
`
	path := writeTestConfig(t, configContent)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing tools_dir")
	}
}

// --- REST Validation Tests ---

func TestValidationRESTMissingBaseURL(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  api:
    type: rest
    method: GET
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for rest tool missing base_url")
	}
}

func TestValidationRESTMissingMethod(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  api:
    type: rest
    base_url: https://api.example.com
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for rest tool missing method")
	}
}

func TestValidRESTConfig(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  api.get:
    type: rest
    description: "Get data"
    base_url: https://api.example.com
    method: GET
    path: /data
    auth:
      type: bearer
      token: "{{env:API_TOKEN}}"
    parameters:
      - name: limit
        in: query
        required: false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api.get"]
	if tool.BaseURL != "https://api.example.com" {
		t.Errorf("expected base_url, got %q", tool.BaseURL)
	}
	if tool.Auth == nil {
		t.Fatal("expected auth config")
	}
	if tool.Auth.Type != "bearer" {
		t.Errorf("expected bearer auth, got %s", tool.Auth.Type)
	}
	if tool.Parameters[0].In != "query" {
		t.Errorf("expected param in=query, got %q", tool.Parameters[0].In)
	}
}

// --- OAuth Config Tests ---

func TestValidOAuthWithProvider(t *testing.T) {
	path := writeTestConfig(t, `
oauth_providers:
  google:
    client_id: "test-id"
    client_secret: "test-secret"
    auth_url: https://accounts.google.com/o/oauth2/v2/auth
    token_url: https://oauth2.googleapis.com/token
    scopes: ["drive.readonly"]
tools:
  google.files:
    type: rest
    base_url: https://www.googleapis.com
    method: GET
    path: /drive/v3/files
    auth:
      type: oauth
      provider: google
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["google.files"]
	if tool.Auth.Type != "oauth" {
		t.Errorf("expected oauth auth type, got %s", tool.Auth.Type)
	}
	if tool.Auth.Provider != "google" {
		t.Errorf("expected provider google, got %s", tool.Auth.Provider)
	}
}

func TestValidOAuthInline(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  github.repos:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user/repos
    auth:
      type: oauth
      client_id: "test-id"
      client_secret: "test-secret"
      auth_url: https://github.com/login/oauth/authorize
      token_url: https://github.com/login/oauth/access_token
      scopes: ["repo"]
      token_key: github_oauth
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["github.repos"]
	if tool.Auth.ClientID != "test-id" {
		t.Errorf("expected client_id, got %q", tool.Auth.ClientID)
	}
	if tool.Auth.TokenKey != "github_oauth" {
		t.Errorf("expected token_key, got %q", tool.Auth.TokenKey)
	}
}

func TestOAuthMissingProvider(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  test:
    type: rest
    base_url: https://api.example.com
    method: GET
    auth:
      type: oauth
      provider: nonexistent
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing oauth provider ref")
	}
}

func TestOAuthInlineMissingClientID(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  test:
    type: rest
    base_url: https://api.example.com
    method: GET
    auth:
      type: oauth
      auth_url: https://example.com/auth
      token_url: https://example.com/token
      token_key: test_oauth
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing client_id in inline oauth")
	}
}

func TestOAuthInlineMissingTokenKey(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  test:
    type: rest
    base_url: https://api.example.com
    method: GET
    auth:
      type: oauth
      client_id: "test"
      auth_url: https://example.com/auth
      token_url: https://example.com/token
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing token_key in inline oauth")
	}
}

func TestResolveOAuthProviderFromRef(t *testing.T) {
	cfg := &Config{
		OAuthProviders: map[string]OAuthProviderConfig{
			"google": {
				ClientID:     "provider-id",
				ClientSecret: "provider-secret",
				AuthURL:      "https://google.com/auth",
				TokenURL:     "https://google.com/token",
				Scopes:       []string{"drive"},
			},
		},
	}
	auth := &AuthConfig{Type: "oauth", Provider: "google"}
	resolved := cfg.ResolveOAuthProvider(auth)

	if resolved.ClientID != "provider-id" {
		t.Errorf("expected provider-id, got %s", resolved.ClientID)
	}
	if resolved.AuthURL != "https://google.com/auth" {
		t.Errorf("expected auth URL from provider, got %s", resolved.AuthURL)
	}
}

func TestResolveOAuthProviderInlineOverride(t *testing.T) {
	cfg := &Config{
		OAuthProviders: map[string]OAuthProviderConfig{
			"google": {
				ClientID: "provider-id",
				AuthURL:  "https://google.com/auth",
				TokenURL: "https://google.com/token",
			},
		},
	}
	auth := &AuthConfig{
		Type:     "oauth",
		Provider: "google",
		ClientID: "override-id", // inline overrides provider
	}
	resolved := cfg.ResolveOAuthProvider(auth)

	if resolved.ClientID != "override-id" {
		t.Errorf("expected override-id, got %s", resolved.ClientID)
	}
	if resolved.AuthURL != "https://google.com/auth" {
		t.Errorf("expected auth URL from provider, got %s", resolved.AuthURL)
	}
}

func TestOAuthTokenKeyDefault(t *testing.T) {
	auth := &AuthConfig{Type: "oauth", Provider: "google"}
	key := OAuthTokenKey(auth)
	if key != "google_oauth" {
		t.Errorf("expected google_oauth, got %s", key)
	}
}

func TestOAuthTokenKeyExplicit(t *testing.T) {
	auth := &AuthConfig{Type: "oauth", Provider: "google", TokenKey: "my_custom_key"}
	key := OAuthTokenKey(auth)
	if key != "my_custom_key" {
		t.Errorf("expected my_custom_key, got %s", key)
	}
}

// --- New Tool Fields ---

func TestParseNewToolFields(t *testing.T) {
	yaml := `
tools:
  api.fetch:
    type: rest
    base_url: "https://api.example.com"
    method: GET
    timeout: "30s"
    max_output: 50000
    compress:
      - json
      - logs
`
	dir := t.TempDir()
	path := filepath.Join(dir, "factorly.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	tool := cfg.Tools["api.fetch"]
	if tool.Timeout != "30s" {
		t.Errorf("expected timeout '30s', got %q", tool.Timeout)
	}
	if tool.MaxOutput != 50000 {
		t.Errorf("expected max_output 50000, got %d", tool.MaxOutput)
	}
	if len(tool.Compress) != 2 || tool.Compress[0] != "json" || tool.Compress[1] != "logs" {
		t.Errorf("expected compress [json, logs], got %v", tool.Compress)
	}
}

func TestWorkflowConfigValid(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  pipeline:
    type: workflow
    description: Test pipeline
    steps:
      - tool: step1
        params:
          key: value
        store: result
      - tool: step2
        params:
          input: "{{result}}"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tool := cfg.Tools["pipeline"]
	if tool.Type != "workflow" {
		t.Errorf("expected type workflow, got %s", tool.Type)
	}
	if len(tool.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(tool.Steps))
	}
	if tool.Steps[0].Tool != "step1" {
		t.Errorf("expected step1, got %s", tool.Steps[0].Tool)
	}
	if tool.Steps[0].Store != "result" {
		t.Errorf("expected store=result, got %s", tool.Steps[0].Store)
	}
	if tool.Steps[1].Params["input"] != "{{result}}" {
		t.Errorf("expected {{result}} param, got %s", tool.Steps[1].Params["input"])
	}
}

func TestWorkflowConfigNoSteps(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  empty:
    type: workflow
    description: Empty workflow
`)
	_, err := Load(path)
	if err != nil {
		t.Fatalf("workflow with no steps should be valid, got: %v", err)
	}
}

func TestWorkflowConfigStepMissingTool(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  bad:
    type: workflow
    steps:
      - params:
          key: value
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for step missing tool name")
	}
}

func TestWorkflowConfigSwitchValid(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  echo:
    type: cli
    command: echo
    args: ["hi"]
  test:
    type: workflow
    steps:
      - switch:
          - condition: "true"
            tool: echo
`)
	_, err := Load(path)
	if err != nil {
		t.Fatalf("valid switch config should load: %v", err)
	}
}

func TestWorkflowConfigSwitchMissingCondition(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  echo:
    type: cli
    command: echo
    args: ["hi"]
  test:
    type: workflow
    steps:
      - switch:
          - tool: echo
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for switch case missing condition")
	}
}

func TestWorkflowConfigSwitchMissingTool(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  test:
    type: workflow
    steps:
      - switch:
          - condition: "true"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for switch case missing tool")
	}
}

func TestWorkflowConfigIfWithTool(t *testing.T) {
	path := writeTestConfig(t, `
tools:
  echo:
    type: cli
    command: echo
    args: ["hi"]
  test:
    type: workflow
    steps:
      - tool: echo
        if: "x == 'y'"
`)
	_, err := Load(path)
	if err != nil {
		t.Fatalf("if with tool should be valid: %v", err)
	}
}

// --- Blueprint format tests ---

// Writes a loose YAML file inside a .factorly/ directory and returns the
// project root (parent of .factorly/), suitable for passing to LoadDir-via-Load
// patterns that look for .factorly/ siblings.
func writeLooseFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	factorlyDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(factorlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factorlyDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also write a minimal factorly.yaml at the root so Load() has an entry point.
	rootCfg := filepath.Join(dir, "factorly.yaml")
	if err := os.WriteFile(rootCfg, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return rootCfg
}

func TestLooseFileFlatMapStillWorks(t *testing.T) {
	// Existing users have .factorly/my-tools.yaml in the flat-map shape.
	// Backward compatibility: this must keep loading without migration.
	path := writeLooseFile(t, "my-tools.yaml", `
my.tool:
  type: cli
  command: echo
  description: hi
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("loading flat-map loose file: %v", err)
	}
	if _, ok := cfg.Tools["my.tool"]; !ok {
		t.Fatalf("expected my.tool to load; got %v", cfg.Tools)
	}
}

func TestLooseFileBlueprintShapeParses(t *testing.T) {
	path := writeLooseFile(t, "gmail.yaml", `
name: gmail-toolkit
version: 1.0.0
description: Gmail integration
author: factorly
license: MIT
tools:
  gmail.search:
    type: cli
    command: echo
    description: search
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("loading pack shape: %v", err)
	}
	if _, ok := cfg.Tools["gmail.search"]; !ok {
		t.Fatalf("expected gmail.search to load; got %v", cfg.Tools)
	}
}

func TestLooseFileWithOAuthProvider(t *testing.T) {
	// A pack file shipping its own oauth_providers entry should merge into
	// the main config — that's the whole point of the format change.
	path := writeLooseFile(t, "linear.yaml", `
name: linear
oauth_providers:
  linear:
    client_id: cid
    client_secret: csecret
    auth_url: https://example/auth
    token_url: https://example/token
tools:
  linear.list:
    type: cli
    command: echo
    description: list
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("loading loose file with oauth provider: %v", err)
	}
	if _, ok := cfg.OAuthProviders["linear"]; !ok {
		t.Fatalf("expected oauth_providers.linear to load; got %v", cfg.OAuthProviders)
	}
}

func TestLooseFileDuplicateOAuthProviderErrors(t *testing.T) {
	dir := t.TempDir()
	factorlyDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(factorlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := `
oauth_providers:
  dup:
    client_id: x
    auth_url: https://x
    token_url: https://x
tools: {}
`
	for _, n := range []string{"a.yaml", "b.yaml"} {
		if err := os.WriteFile(filepath.Join(factorlyDir, n), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rootCfg := filepath.Join(dir, "factorly.yaml")
	if err := os.WriteFile(rootCfg, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(rootCfg); err == nil {
		t.Fatalf("expected duplicate oauth provider error")
	}
}

func TestValidateReferencesWorkflowToolMissing(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{
			"my.workflow": {
				Type: "workflow",
				Steps: []StepConfig{
					{Tool: "does.not.exist"},
				},
			},
		},
	}
	if err := ValidateReferences(cfg, nil); err == nil {
		t.Fatalf("expected error for missing workflow ref")
	}
}

func TestValidateReferencesWorkflowToolResolvesViaBuiltins(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{
			"my.workflow": {
				Type: "workflow",
				Steps: []StepConfig{
					{Tool: "factorly.fetch"},
				},
			},
		},
	}
	builtins := map[string]bool{"factorly.fetch": true}
	if err := ValidateReferences(cfg, builtins); err != nil {
		t.Fatalf("expected workflow ref to resolve via builtins; got %v", err)
	}
}

func TestValidateReferencesWorkflowSwitchMissing(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{
			"my.workflow": {
				Type: "workflow",
				Steps: []StepConfig{
					{Switch: []SwitchCase{{Condition: "x", Tool: "ghost"}}},
				},
			},
		},
	}
	if err := ValidateReferences(cfg, nil); err == nil {
		t.Fatalf("expected error for missing switch tool")
	}
}

func TestValidateReferencesRequiresToolMissing(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{},
		Requires: &Requires{
			Tools: []string{"ghost"},
		},
	}
	if err := ValidateReferences(cfg, nil); err == nil {
		t.Fatalf("expected error for missing required tool")
	}
}

func TestValidateReferencesRequiresOAuthMissing(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{},
		Requires: &Requires{
			OAuthProviders: []string{"ghost"},
		},
	}
	if err := ValidateReferences(cfg, nil); err == nil {
		t.Fatalf("expected error for missing required oauth provider")
	}
}

func TestValidateReferencesPasses(t *testing.T) {
	cfg := &Config{
		Tools: map[string]ToolConfig{
			"a": {Type: "cli", Command: "echo"},
			"b": {Type: "workflow", Steps: []StepConfig{{Tool: "a"}}},
		},
	}
	if err := ValidateReferences(cfg, nil); err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
}

// --- Blueprint format unit gaps ---

func TestBlueprintsSubdirAutoLoaded(t *testing.T) {
	// .factorly/blueprints/*.yaml should be picked up automatically by Load,
	// alongside (and equivalent to) loose files in .factorly/.
	dir := t.TempDir()
	factorlyDir := filepath.Join(dir, ".factorly")
	blueprintsDir := filepath.Join(factorlyDir, "blueprints")
	if err := os.MkdirAll(blueprintsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rootCfg := filepath.Join(dir, "factorly.yaml")
	if err := os.WriteFile(rootCfg, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bpYAML := `
name: auto
tools:
  auto.tool:
    type: cli
    command: echo
    description: from blueprints subdir
`
	if err := os.WriteFile(filepath.Join(blueprintsDir, "auto.yaml"), []byte(bpYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(rootCfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Tools["auto.tool"]; !ok {
		t.Fatalf("expected auto.tool from .factorly/blueprints/, got %v", cfg.Tools)
	}
}

func TestBlueprintsSubdirMergedWhenConfigInsideFactorly(t *testing.T) {
	// When the config file lives in .factorly/factorly.yaml, the sibling
	// blueprints/ subdirectory must also be scanned. This is a separate code
	// path from the root-config case above.
	dir := t.TempDir()
	factorlyDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(filepath.Join(factorlyDir, "blueprints"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(factorlyDir, "factorly.yaml")
	if err := os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bpYAML := `
name: inside
tools:
  inside.tool:
    type: cli
    command: echo
    description: from inside-factorly blueprints
`
	if err := os.WriteFile(filepath.Join(factorlyDir, "blueprints", "inside.yaml"), []byte(bpYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Tools["inside.tool"]; !ok {
		t.Fatalf("expected inside.tool from .factorly/blueprints/, got %v", cfg.Tools)
	}
}

func TestMergeConfigsVaultBackends(t *testing.T) {
	dst := &Config{}
	src := &Config{
		VaultBackends: map[string]vault.ExternalBackendConfig{
			"op": {Type: "cli"},
		},
	}
	if err := mergeConfigs(dst, src, "src.yaml"); err != nil {
		t.Fatalf("mergeConfigs: %v", err)
	}
	if _, ok := dst.VaultBackends["op"]; !ok {
		t.Fatalf("expected vault backend 'op' merged, got %v", dst.VaultBackends)
	}
}

func TestMergeConfigsDuplicateVaultBackendErrors(t *testing.T) {
	dst := &Config{
		VaultBackends: map[string]vault.ExternalBackendConfig{
			"op": {Type: "cli"},
		},
	}
	src := &Config{
		VaultBackends: map[string]vault.ExternalBackendConfig{
			"op": {Type: "cli"},
		},
	}
	if err := mergeConfigs(dst, src, "src.yaml"); err == nil {
		t.Fatal("expected duplicate vault backend error")
	}
}

func TestMergeConfigsDoesNotMutateSource(t *testing.T) {
	src := &Config{
		Tools: map[string]ToolConfig{
			"a": {Type: "cli", Command: "echo"},
		},
		OAuthProviders: map[string]OAuthProviderConfig{
			"p": {ClientID: "x"},
		},
	}
	dst := &Config{}
	if err := mergeConfigs(dst, src, "src.yaml"); err != nil {
		t.Fatalf("mergeConfigs: %v", err)
	}
	// Mutating dst should not affect src.
	dst.Tools["a"] = ToolConfig{Type: "cli", Command: "MUTATED"}
	if src.Tools["a"].Command != "echo" {
		t.Fatalf("merge leaked mutation back to source: %v", src.Tools["a"])
	}
}

func TestHasStructuredKeysDetectsEachField(t *testing.T) {
	// Each top-level Config field should flip hasStructuredKeys to true,
	// so a loose file using only that field doesn't fall back to flat-map.
	cases := map[string]*Config{
		"name":             {Name: "x"},
		"version":          {Version: "1"},
		"description":      {Description: "d"},
		"author":           {Author: "a"},
		"homepage":         {Homepage: "h"},
		"license":          {License: "MIT"},
		"requires":         {Requires: &Requires{}},
		"tools":            {Tools: map[string]ToolConfig{"t": {}}},
		"oauth_providers":  {OAuthProviders: map[string]OAuthProviderConfig{"p": {}}},
		"vault_backends":   {VaultBackends: map[string]vault.ExternalBackendConfig{"v": {}}},
		"tools_dir":        {ToolsDir: "x"},
		"disable_builtins": {DisableBuiltins: true},
	}
	for name, cfg := range cases {
		if !hasStructuredKeys(cfg) {
			t.Errorf("hasStructuredKeys returned false for %s; got %+v", name, cfg)
		}
	}
	// Empty Config should be false.
	if hasStructuredKeys(&Config{}) {
		t.Error("hasStructuredKeys returned true for empty Config")
	}
}
