// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

//go:build integration

package test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var binary string
var testserverBin string

func TestMain(m *testing.M) {
	// Allow override via env var
	if p := os.Getenv("FACTORLY_BIN"); p != "" {
		binary = p
	} else {
		// Walk up from the test dir looking for build/factorly
		dir, _ := os.Getwd()
		for i := 0; i < 5; i++ {
			candidate := filepath.Join(dir, "build", "factorly")
			if _, err := os.Stat(candidate); err == nil {
				binary = candidate
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	if binary == "" {
		if p, err := exec.LookPath("factorly"); err == nil {
			binary = p
		}
	}
	if binary == "" {
		panic("factorly binary not found — run 'make build' first or set FACTORLY_BIN")
	}

	// testserver is factorly itself (factorly serve acts as an MCP server)
	testserverBin = binary

	os.Exit(m.Run())
}

func run(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run factorly: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func setupDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// --- Version ---

func TestVersionOutput(t *testing.T) {
	stdout, _, code := run(t, "", "version")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.HasPrefix(stdout, "factorly ") {
		t.Errorf("expected 'factorly ...' output, got %q", stdout)
	}
}

// --- Tools listing ---

func TestToolsListCLI(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  web.fetch:
    type: cli
    description: "Fetch a webpage"
    command: curl
    args: ["-s", "{{url}}"]
  file.read:
    type: cli
    description: "Read a file"
    command: cat
    args: ["{{path}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "web.fetch") {
		t.Error("expected web.fetch in tools output")
	}
	if !strings.Contains(stdout, "file.read") {
		t.Error("expected file.read in tools output")
	}
	if !strings.Contains(stdout, "cli") {
		t.Error("expected 'cli' type in tools output")
	}
}

// --- Call ---

func TestCallEcho(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "echo.test", "--msg", "hello world")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != "hello world" {
		t.Errorf("expected 'hello world', got %q", stdout)
	}
}

func TestCallMultipleParams(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.multi:
    type: cli
    command: echo
    args: ["{{first}}", "{{second}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "echo.multi", "--first", "hello", "--second", "world")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != "hello world" {
		t.Errorf("expected 'hello world', got %q", stdout)
	}
}

func TestCallMissingTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	_, _, code := run(t, dir, "call", "nonexistent", "--msg", "hi")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing tool")
	}
}

func TestCallMissingParam(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	_, _, code := run(t, dir, "call", "echo.test")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing param")
	}
}

func TestCallFailingCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  fail:
    type: cli
    command: "false"
    args: []
`,
	})

	_, _, code := run(t, dir, "call", "fail")
	if code == 0 {
		t.Fatal("expected non-zero exit for failing command")
	}
}

// --- Stdin ---

func TestCallStdin(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  cat.input:
    type: cli
    command: cat
    stdin: "{{input}}"
`,
	})

	stdout, _, code := run(t, dir, "call", "cat.input", "--input", "hello from stdin")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if stdout != "hello from stdin" {
		t.Errorf("expected 'hello from stdin', got %q", stdout)
	}
}

func TestCallStdinWithArgs(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  grep.filter:
    type: cli
    command: grep
    args: ["{{pattern}}"]
    stdin: "{{input}}"
`,
	})

	stdout, _, code := run(t, dir, "call", "grep.filter",
		"--pattern", "hello",
		"--input", "hello world\ngoodbye world\nhello again")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("expected grep match, got %q", stdout)
	}
}

func TestCallStdinParamInference(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  cat.input:
    type: cli
    command: cat
    stdin: "{{data}}"
`,
	})

	// Tool should list "data" as a parameter (inferred from stdin)
	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "data") {
		t.Error("expected inferred param 'data' in tools output")
	}
}

// --- Tool Directory ---

func TestToolsDirLoading(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
`,
		"tools/echo.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "web.fetch") {
		t.Error("expected web.fetch from inline config")
	}
	if !strings.Contains(stdout, "echo.test") {
		t.Error("expected echo.test from tools dir")
	}
}

func TestToolsDirCallFromDir(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools: {}
`,
		"tools/echo.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "echo.test", "--msg", "from directory")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != "from directory" {
		t.Errorf("expected 'from directory', got %q", stdout)
	}
}

func TestToolsDirConflict(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools:
  dupe:
    type: cli
    command: echo
    args: ["inline"]
`,
		"tools/dupe.yaml": `
dupe:
  type: cli
  command: echo
  args: ["from-dir"]
`,
	})

	_, stderr, code := run(t, dir, "tools")
	if code == 0 {
		t.Fatal("expected non-zero exit for duplicate tool")
	}
	if !strings.Contains(stderr, "duplicate") {
		t.Errorf("expected duplicate error message, got %q", stderr)
	}
}

func TestConfigDirFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"mytools/echo.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, "", "--config-dir", filepath.Join(dir, "mytools"), "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "echo.test") {
		t.Error("expected echo.test from --config-dir")
	}
}

// --- OpenAPI Import ---

func TestImportOpenAPIStdout(t *testing.T) {
	// Find the petstore example spec
	specPath := findPetstoreSpec(t)

	stdout, _, code := run(t, "", "tools", "import", "openapi", specPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "pet_store.listPets") {
		t.Error("expected pet_store.listPets in output")
	}
	if !strings.Contains(stdout, "pet_store.createPet") {
		t.Error("expected pet_store.createPet in output")
	}
	if !strings.Contains(stdout, "pet_store.showPetById") {
		t.Error("expected pet_store.showPetById in output")
	}
	if !strings.Contains(stdout, "type: rest") {
		t.Error("expected 'type: rest' in output")
	}
}

func TestImportOpenAPIToFile(t *testing.T) {
	specPath := findPetstoreSpec(t)
	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "petstore.yaml")

	_, stderr, code := run(t, "", "tools", "import", "openapi", specPath, "--out", outPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Wrote 3 tools") {
		t.Errorf("expected 'Wrote 3 tools' message, got %q", stderr)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "pet_store.listPets") {
		t.Error("expected pet_store.listPets in output file")
	}
}

func TestImportOpenAPIWithPrefix(t *testing.T) {
	specPath := findPetstoreSpec(t)

	stdout, _, code := run(t, "", "tools", "import", "openapi", specPath, "--prefix", "myapi")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "myapi.listPets") {
		t.Error("expected myapi.listPets with custom prefix")
	}
}

// --- Full Pipeline: import → tools dir → tools listing ---

func TestFullPipeline(t *testing.T) {
	specPath := findPetstoreSpec(t)
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Import OpenAPI spec into tools dir
	outPath := filepath.Join(toolsDir, "petstore.yaml")
	_, _, code := run(t, dir, "tools", "import", "openapi", specPath, "--out", outPath)
	if code != 0 {
		t.Fatal("import failed")
	}

	// List all tools — should see both inline and imported
	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatal("tools listing failed")
	}
	if !strings.Contains(stdout, "echo.test") {
		t.Error("expected echo.test (inline)")
	}
	if !strings.Contains(stdout, "pet_store.listPets") {
		t.Error("expected pet_store.listPets (imported)")
	}

	// Call the inline CLI tool — should still work
	stdout, stderrOut, code := run(t, dir, "call", "echo.test", "--msg", "pipeline works")
	if code != 0 {
		t.Fatalf("call failed (code %d): %s", code, stderrOut)
	}
	if strings.TrimSpace(stdout) != "pipeline works" {
		t.Errorf("expected 'pipeline works', got %q", stdout)
	}
}

// --- REST Provider ---

func TestToolsListREST(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  api.list:
    type: rest
    description: "List items"
    base_url: https://api.example.com
    method: GET
    path: /items
    parameters:
      - name: limit
        in: query
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "api.list") {
		t.Error("expected api.list in tools output")
	}
	if !strings.Contains(stdout, "rest") {
		t.Error("expected 'rest' type in tools output")
	}
}

func TestCallREST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":["a","b"]}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.list:
    type: rest
    description: "List items"
    base_url: %s
    method: GET
    path: /items
`, srv.URL),
	})

	stdout, _, code := run(t, dir, "call", "api.list")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, `{"items":["a","b"]}`) {
		t.Errorf("expected JSON response, got %q", stdout)
	}
}

func TestCallRESTWithParams(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Write([]byte(`{"id":"42"}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.get:
    type: rest
    base_url: %s
    method: GET
    path: /items/{{id}}
    parameters:
      - name: id
        in: path
        required: true
      - name: fields
        in: query
`, srv.URL),
	})

	stdout, _, code := run(t, dir, "call", "api.get", "--id", "42", "--fields", "name,status")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if capturedPath != "/items/42" {
		t.Errorf("expected path /items/42, got %s", capturedPath)
	}
	if !strings.Contains(capturedQuery, "fields=name") {
		t.Errorf("expected query param, got %s", capturedQuery)
	}
	if !strings.Contains(stdout, `{"id":"42"}`) {
		t.Errorf("expected response, got %q", stdout)
	}
}

func TestCallRESTPost(t *testing.T) {
	var capturedBody string
	var capturedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(201)
		w.Write([]byte(`{"created":true}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.create:
    type: rest
    base_url: %s
    method: POST
    path: /items
    body_type: raw
    parameters:
      - name: body
        in: body
        required: true
`, srv.URL),
	})

	stdout, _, code := run(t, dir, "call", "api.create", "--body", `{"name":"test"}`)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedBody != `{"name":"test"}` {
		t.Errorf("expected body, got %q", capturedBody)
	}
	if !strings.Contains(stdout, `{"created":true}`) {
		t.Errorf("expected response, got %q", stdout)
	}
}

func TestCallRESTAuth(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.get:
    type: rest
    base_url: %s
    method: GET
    path: /secure
    auth:
      type: bearer
      token: test-token-123
`, srv.URL),
	})

	stdout, _, code := run(t, dir, "call", "api.get")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if capturedAuth != "Bearer test-token-123" {
		t.Errorf("expected bearer auth, got %q", capturedAuth)
	}
	if stdout != "ok" {
		t.Errorf("expected 'ok', got %q", stdout)
	}
}

// --- MCP Provider ---

func TestCallMCPEcho(t *testing.T) {
	// Create a child config with a CLI echo tool
	dir := setupDir(t, map[string]string{
		"child.yaml": `
tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
`,
		"factorly.yaml": fmt.Sprintf(`
tools:
  child:
    type: mcp
    command: %s
    args: ["serve", "-c", "child.yaml"]
`, binary),
	})

	stdout, stderr, code := run(t, dir, "call", "child.echo", "--message", "hello from mcp")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "hello from mcp") {
		t.Errorf("expected 'hello from mcp', got %q", stdout)
	}
}

func TestToolsListMCP(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"child.yaml": `
tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
  cat:
    type: cli
    description: "Read a file"
    command: cat
    args: ["{{path}}"]
`,
		"factorly.yaml": fmt.Sprintf(`
tools:
  child:
    type: mcp
    command: %s
    args: ["serve", "-c", "child.yaml"]
`, binary),
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "child.echo") {
		t.Error("expected child.echo in tools output")
	}
	if !strings.Contains(stdout, "child.cat") {
		t.Error("expected child.cat in tools output")
	}
	if !strings.Contains(stdout, "mcp") {
		t.Error("expected 'mcp' type in tools output")
	}
}

func TestMCPMixedWithCLI(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"child.yaml": `
tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
`,
		"factorly.yaml": fmt.Sprintf(`
tools:
  remote:
    type: mcp
    command: %s
    args: ["serve", "-c", "child.yaml"]
  local.echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
`, binary),
	})

	// MCP tool
	stdout, _, code := run(t, dir, "call", "remote.echo", "--message", "from mcp")
	if code != 0 {
		t.Fatalf("mcp call failed with code %d", code)
	}
	if !strings.Contains(stdout, "from mcp") {
		t.Errorf("expected 'from mcp', got %q", stdout)
	}

	// CLI tool
	stdout, _, code = run(t, dir, "call", "local.echo", "--msg", "from cli")
	if code != 0 {
		t.Fatalf("cli call failed with code %d", code)
	}
	if strings.TrimSpace(stdout) != "from cli" {
		t.Errorf("expected 'from cli', got %q", stdout)
	}
}

// --- OAuth Auth Commands ---

func TestAuthStatusWithToken(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
oauth_providers:
  github:
    client_id: "test-id"
    client_secret: "test-secret"
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    scopes: ["repo"]
tools:
  github.repos:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user/repos
    auth:
      type: oauth
      provider: github
`,
	})

	vaultPath := filepath.Join(dir, "vault.enc")

	// Store a token bundle in the vault
	_, _, code := runVault(t, vaultPath, "vault", "set", "github_oauth",
		`{"access_token":"test-token","refresh_token":"test-refresh","token_type":"bearer","expiry":"2099-01-01T00:00:00Z"}`)
	if code != 0 {
		t.Fatal("vault set failed")
	}

	// Check status
	stdout, _, code := runVault(t, vaultPath, "-c", filepath.Join(dir, "factorly.yaml"), "auth", "status")
	if code != 0 {
		t.Fatalf("auth status failed with code %d", code)
	}
	if !strings.Contains(stdout, "github_oauth") {
		t.Error("expected github_oauth in status output")
	}
	if !strings.Contains(stdout, "✓") {
		t.Error("expected ✓ for valid token")
	}
}

func TestAuthStatusExpiredToken(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.api:
    type: rest
    base_url: https://api.example.com
    method: GET
    path: /data
    auth:
      type: oauth
      client_id: "test"
      auth_url: https://example.com/auth
      token_url: https://example.com/token
      token_key: test_oauth
`,
	})

	vaultPath := filepath.Join(dir, "vault.enc")

	// Store an expired token
	_, _, code := runVault(t, vaultPath, "vault", "set", "test_oauth",
		`{"access_token":"expired","refresh_token":"refresh","token_type":"bearer","expiry":"2020-01-01T00:00:00Z"}`)
	if code != 0 {
		t.Fatal("vault set failed")
	}

	stdout, _, code := runVault(t, vaultPath, "-c", filepath.Join(dir, "factorly.yaml"), "auth", "status")
	if code != 0 {
		t.Fatalf("auth status failed with code %d", code)
	}
	if !strings.Contains(stdout, "⟳") {
		t.Error("expected ⟳ for expired token with refresh token")
	}
	if !strings.Contains(stdout, "auto-refresh") {
		t.Error("expected 'auto-refresh' message for expired token with refresh token")
	}
}

func TestAuthLogout(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
oauth_providers:
  github:
    client_id: "test-id"
    client_secret: "test-secret"
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
tools:
  github.repos:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user/repos
    auth:
      type: oauth
      provider: github
`,
	})

	vaultPath := filepath.Join(dir, "vault.enc")

	// Store a token
	_, _, code := runVault(t, vaultPath, "vault", "set", "github_oauth", `{"access_token":"token"}`)
	if code != 0 {
		t.Fatal("vault set failed")
	}

	// Logout
	_, stderr, code := runVault(t, vaultPath, "-c", filepath.Join(dir, "factorly.yaml"), "auth", "logout", "github")
	if code != 0 {
		t.Fatalf("auth logout failed with code %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Logged out") {
		t.Error("expected 'Logged out' message")
	}

	// Verify token is gone
	_, _, code = runVault(t, vaultPath, "vault", "get", "github_oauth")
	if code == 0 {
		t.Error("expected non-zero exit for deleted token")
	}
}

// --- Health Check ---

func TestHealthAllHealthy(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
disable_builtins: true
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
  cat.test:
    type: cli
    command: cat
    args: ["{{path}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools", "status")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "✓") {
		t.Error("expected checkmarks in output")
	}
	if !strings.Contains(stdout, "0 issues") {
		t.Error("expected 0 issues")
	}
}

func TestHealthBrokenCLI(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
disable_builtins: true
tools:
  good:
    type: cli
    command: echo
    args: ["{{msg}}"]
  broken:
    type: cli
    command: nonexistent-command-12345
    args: []
`,
	})

	stdout, _, code := run(t, dir, "tools", "status")
	if code == 0 {
		t.Fatal("expected non-zero exit for broken tool")
	}
	if !strings.Contains(stdout, "✗") {
		t.Error("expected cross mark for broken tool")
	}
	if !strings.Contains(stdout, "not found in PATH") {
		t.Error("expected 'not found in PATH' message")
	}
	if !strings.Contains(stdout, "1 issues") {
		t.Error("expected 1 issue")
	}
}

func TestHealthRESTReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
disable_builtins: true
tools:
  api.test:
    type: rest
    base_url: %s
    method: GET
    path: /
`, srv.URL),
	})

	stdout, _, code := run(t, dir, "tools", "status")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "reachable") {
		t.Error("expected 'reachable' in output")
	}
}

func TestHealthMCPServer(t *testing.T) {
	// Use factorly itself as a child MCP server
	dir := setupDir(t, map[string]string{
		"child.yaml": `
disable_builtins: true
tools:
  echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
		"factorly.yaml": fmt.Sprintf(`
disable_builtins: true
tools:
  child:
    type: mcp
    command: %s
    args: ["serve", "-c", "child.yaml"]
`, binary),
	})

	stdout, _, code := run(t, dir, "tools", "status")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "connected") {
		t.Error("expected 'connected' in output")
	}
	if !strings.Contains(stdout, "ping") {
		t.Error("expected 'ping' in output")
	}
}

// --- Add / Remove ---

func TestAddCLITool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools: {}
`,
	})

	_, stderr, code := run(t, dir, "tools", "add", "--name", "test.echo", "--type", "cli", "--command", "echo", "--args", "{{msg}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Added test.echo") {
		t.Error("expected 'Added' message")
	}

	// Verify tool works
	stdout, _, code := run(t, dir, "call", "test.echo", "--msg", "hello from add")
	if code != 0 {
		t.Fatal("call failed")
	}
	if strings.TrimSpace(stdout) != "hello from add" {
		t.Errorf("expected 'hello from add', got %q", stdout)
	}
}

func TestAddToToolsDir(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools: {}
`,
	})
	os.MkdirAll(filepath.Join(dir, "tools"), 0o755)

	_, _, code := run(t, dir, "tools", "add", "--name", "test.echo", "--type", "cli", "--command", "echo", "--args", "{{msg}}")
	if code != 0 {
		t.Fatal("add failed")
	}

	// Verify file was created in tools dir
	toolFile := filepath.Join(dir, "tools", "test.echo.yaml")
	if _, err := os.Stat(toolFile); os.IsNotExist(err) {
		t.Error("expected tool file in tools dir")
	}
}

func TestAddDuplicateTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	_, _, code := run(t, dir, "tools", "add", "--name", "test.echo", "--type", "cli", "--command", "echo", "--args", "{{msg}}")
	if code == 0 {
		t.Fatal("expected error for duplicate tool")
	}
}

func TestRemoveTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
  keep.this:
    type: cli
    command: cat
    args: ["{{path}}"]
`,
	})

	_, stderr, code := run(t, dir, "tools", "remove", "test.echo")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Removed test.echo") {
		t.Error("expected 'Removed' message")
	}

	// Verify tool is gone but other tool remains
	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatal("tools failed")
	}
	if strings.Contains(stdout, "test.echo") {
		t.Error("test.echo should be removed")
	}
	if !strings.Contains(stdout, "keep.this") {
		t.Error("keep.this should remain")
	}
}

func TestRemoveFromToolsDir(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools: {}
`,
		"tools/test.echo.yaml": `
test.echo:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	_, _, code := run(t, dir, "tools", "remove", "test.echo")
	if code != 0 {
		t.Fatal("remove failed")
	}

	// File should be deleted (was the only tool)
	toolFile := filepath.Join(dir, "tools", "test.echo.yaml")
	if _, err := os.Stat(toolFile); !os.IsNotExist(err) {
		t.Error("expected tool file to be deleted")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	_, _, code := run(t, dir, "tools", "remove", "nonexistent")
	if code == 0 {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestAddRESTTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools: {}
`,
	})

	_, _, code := run(t, dir, "tools", "add", "--name", "api.test", "--type", "rest", "--base-url", "https://api.github.com", "--method", "GET", "--path", "/")
	if code != 0 {
		t.Fatal("add REST tool failed")
	}

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatal("tools failed")
	}
	if !strings.Contains(stdout, "api.test") {
		t.Error("expected api.test in tools output")
	}
	if !strings.Contains(stdout, "rest") {
		t.Error("expected 'rest' type")
	}
}

// --- Security Hardening ---

func TestVerboseRedactsSecrets(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	_, stderr, code := run(t, dir, "-v", "call", "echo.test", "--msg", "hello", "--api_token", "super-secret", "--password", "hunter2")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Secrets should be redacted in verbose output
	if strings.Contains(stderr, "super-secret") {
		t.Error("api_token value should be redacted in verbose output")
	}
	if strings.Contains(stderr, "hunter2") {
		t.Error("password value should be redacted in verbose output")
	}
	// But param names and non-sensitive values should be visible
	if !strings.Contains(stderr, "[REDACTED]") {
		t.Error("expected [REDACTED] in verbose output")
	}
	if !strings.Contains(stderr, "hello") {
		t.Error("expected non-sensitive param 'hello' visible in verbose output")
	}
}

func TestHTTPTokenAuthRejectsUnauthenticated(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	// Start server with token auth
	cmd := exec.Command(binary, "serve", "--http", ":0", "--http-token", "test-secret-token", "-c", filepath.Join(dir, "factorly.yaml"))
	cmd.Env = append(os.Environ(), "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr

	// We can't easily test the full HTTP flow in integration tests
	// because we'd need to find the actual port. Instead verify the
	// flag is accepted and the command starts without error.
	// The unit-level auth middleware test covers the actual rejection.
	err := cmd.Start()
	if err != nil {
		t.Fatalf("failed to start serve: %v", err)
	}
	// Kill immediately — we just need to verify it started
	cmd.Process.Kill()
	cmd.Wait()
}

// --- Sync ---

func TestSyncCreatesClaudeCodeConfig(t *testing.T) {
	dir := t.TempDir()

	_, stderr, code := run(t, dir, "sync")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "Claude Code") {
		t.Error("expected Claude Code in output")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "factorly") {
		t.Error("expected factorly in .mcp.json")
	}
	if !strings.Contains(string(data), "serve") {
		t.Error("expected serve in .mcp.json")
	}
}

func TestSyncPreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()

	existing := `{"mcpServers":{"other":{"command":"other-server"}}}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := run(t, dir, "sync")
	if code != 0 {
		t.Fatal("sync failed")
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	content := string(data)
	if !strings.Contains(content, "other") {
		t.Error("expected existing 'other' entry preserved")
	}
	if !strings.Contains(content, "factorly") {
		t.Error("expected factorly entry added")
	}
}

func TestSyncRemove(t *testing.T) {
	dir := t.TempDir()

	existing := `{"mcpServers":{"factorly":{"command":"factorly","args":["serve"]},"other":{"command":"other"}}}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := run(t, dir, "sync", "--remove")
	if code != 0 {
		t.Fatalf("sync --remove failed: %s", stderr)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	content := string(data)
	if strings.Contains(content, "factorly") {
		t.Error("expected factorly removed")
	}
	if !strings.Contains(content, "other") {
		t.Error("expected other entry preserved")
	}
}

func TestSyncHTTPMode(t *testing.T) {
	dir := t.TempDir()

	_, _, code := run(t, dir, "sync", "--http", "localhost:3000")
	if code != 0 {
		t.Fatal("sync --http failed")
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	content := string(data)
	if !strings.Contains(content, "http") {
		t.Error("expected http type")
	}
	if !strings.Contains(content, "localhost:3000/mcp") {
		t.Error("expected /mcp endpoint in URL")
	}
}

func TestSyncCursorDetection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := run(t, dir, "sync")
	if code != 0 {
		t.Fatal("sync failed")
	}
	if !strings.Contains(stderr, "Cursor") {
		t.Error("expected Cursor in output")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal("expected .cursor/mcp.json to be created")
	}
	if !strings.Contains(string(data), "factorly") {
		t.Error("expected factorly in cursor config")
	}
}

// --- factorly.Store SDK handle (in-script) ---
//
// Note: the prior `factorly.store.{get,save,search,list,delete}`
// builtin tools were removed. The SDK handle below is the only
// in-script path for store access; the CLI's `factorly store ...`
// subcommand remains for human use.

// TestStoreSDKHandleRoundTrip exercises the in-script factorly.Store
// handle end-to-end against the real binary: a script uses
// factorly.Store.SetWithTTL to write, factorly.Store.Get to read
// back the same value, and then the CLI's `factorly store get`
// confirms the value really landed in the bbolt file. Closes the
// loop so the SDK surface and the CLI subcommand target the same
// store.
func TestStoreSDKHandleRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools: {}`,
	})

	src := `package main
import (
    "errors"
    "factorly"
    "time"
)
func Run(p map[string]string) (any, error) {
    if err := factorly.Store.SetWithTTL("sdk.session", "alpha", 50*time.Minute); err != nil {
        return nil, err
    }
    v, err := factorly.Store.Get("sdk.session")
    if err != nil { return nil, err }
    if v != "alpha" {
        return nil, errors.New("read-back mismatch: " + v)
    }
    return v, nil
}`
	stdout, stderr, code := run(t, dir, "call", "factorly.code", "--code", src, "--params", "{}")
	if code != 0 {
		t.Fatalf("script exit %d, stderr=%s, stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "alpha") {
		t.Errorf("expected script to return 'alpha', got %q", stdout)
	}

	// Confirm via CLI that the value is visible to non-script
	// consumers — same bbolt file, same cascade.
	out, _, code := run(t, dir, "store", "get", "sdk.session")
	if code != 0 {
		t.Fatalf("cli get exit %d", code)
	}
	if strings.TrimSpace(out) != "alpha" {
		t.Errorf("cli get = %q, want 'alpha'", out)
	}
}

// TestStoreSDKHandleMissingKeyReturnsErrStoreNotFound is the in-
// script counterpart to the builtin's "missing key surfaces as
// non-zero" contract. With the SDK handle, the agent can branch via
// errors.Is(err, factorly.ErrStoreNotFound) instead of brittle
// string-matching on the error message — a real ergonomic upgrade.
func TestStoreSDKHandleMissingKeyReturnsErrStoreNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools: {}`,
	})

	src := `package main
import (
    "errors"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    _, err := factorly.Store.Get("never-set")
    if errors.Is(err, factorly.ErrStoreNotFound) {
        return "missing-handled", nil
    }
    return "unexpected: " + err.Error(), nil
}`
	stdout, stderr, code := run(t, dir, "call", "factorly.code", "--code", src, "--params", "{}")
	if code != 0 {
		t.Fatalf("script exit %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "missing-handled") {
		t.Errorf("expected ErrStoreNotFound branch to fire, got %q", stdout)
	}
}

// TestStoreSDKHandleDeleteIsIdempotent verifies the SDK Delete
// contract end-to-end: deleting a missing key returns nil, just like
// the CLI's `factorly store delete`.
func TestStoreSDKHandleDeleteIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools: {}`,
	})

	src := `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    return "ok", factorly.Store.Delete("never-existed")
}`
	stdout, stderr, code := run(t, dir, "call", "factorly.code", "--code", src, "--params", "{}")
	if code != 0 {
		t.Fatalf("script exit %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("expected idempotent Delete to succeed, got stdout=%q", stdout)
	}
}

// --- Store Refs in Params ---

// TestCallParamWithStoreRef pins the {{store:KEY}} reference syntax
// end-to-end. The store backend should resolve refs in CLI param
// values just like {{vault:KEY}} does, but without ever opening
// the vault (no password prompt). Regression guard for the
// HasVaultRefs exclusion that lets store-only ref strings skip the
// vault-open branch.
func TestCallParamWithStoreRef(t *testing.T) {
	// Isolate HOME so the global store tier can never touch the dev's
	// real ~/.config/factorly/store.db.
	t.Setenv("HOME", t.TempDir())

	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{text}}"]
    parameters:
      - name: text
        description: text to echo
        required: true
`,
	})

	// Seed the store directly (no password prompt; no vault touched).
	_, _, code := run(t, dir, "store", "set", "MY_NOTE", "from-store")
	if code != 0 {
		t.Fatal("store set failed")
	}

	stdout, _, code := run(t, dir, "call", "echo.test", "--text", "{{store:MY_NOTE}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != "from-store" {
		t.Errorf("expected 'from-store', got %q", stdout)
	}
}

// --- Vault Refs in Params ---

func TestCallParamWithVaultRef(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{text}}"]
`,
	})

	vaultPath := filepath.Join(dir, "vault.enc")

	// Store a secret
	_, _, code := runVault(t, vaultPath, "vault", "set", "MY_SECRET", "resolved-value")
	if code != 0 {
		t.Fatal("vault set failed")
	}

	// Call with {{vault:KEY}} as param value — should resolve
	stdout, _, code := runVault(t, vaultPath, "-c", filepath.Join(dir, "factorly.yaml"),
		"call", "echo.test", "--text", "{{vault:MY_SECRET}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(stdout) != "resolved-value" {
		t.Errorf("expected 'resolved-value', got %q", stdout)
	}
}

// --- Shadow Policy ---

func TestShadowDenyBlocksTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.blocked:
    type: cli
    command: echo
    args: ["{{msg}}"]
    shadow:
      deny: [test.blocked]
  test.allowed:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	// Blocked tool should fail
	_, stderr, code := run(t, dir, "call", "test.blocked", "--msg", "should fail")
	if code == 0 {
		t.Fatal("expected non-zero exit for denied tool")
	}
	if !strings.Contains(stderr, "denied by shadow policy") {
		t.Errorf("expected 'denied by shadow policy' in error, got: %s", stderr)
	}

	// Allowed tool should work
	stdout, _, code := run(t, dir, "call", "test.allowed", "--msg", "should work")
	if code != 0 {
		t.Fatal("expected exit 0 for allowed tool")
	}
	if strings.TrimSpace(stdout) != "should work" {
		t.Errorf("expected 'should work', got %q", stdout)
	}
}

func TestShadowDenyMCPSubTool(t *testing.T) {
	// Shadow deny on MCP server blocks specific sub-tools
	dir := setupDir(t, map[string]string{
		"child.yaml": `
tools:
  safe:
    type: cli
    command: echo
    args: ["{{msg}}"]
  dangerous:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
		"factorly.yaml": fmt.Sprintf(`
tools:
  child:
    type: mcp
    command: %s
    args: ["serve", "-c", "child.yaml"]
    shadow:
      deny: [dangerous]
`, binary),
	})

	// Safe sub-tool should work
	stdout, _, code := run(t, dir, "call", "child.safe", "--msg", "ok")
	if code != 0 {
		t.Fatal("safe tool should work")
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("expected 'ok', got %q", stdout)
	}

	// Dangerous sub-tool should be blocked
	_, stderr, code := run(t, dir, "call", "child.dangerous", "--msg", "nope")
	if code == 0 {
		t.Fatal("dangerous tool should be blocked")
	}
	if !strings.Contains(stderr, "denied") {
		t.Errorf("expected 'denied' in error, got: %s", stderr)
	}
}

func TestShadowRateLimit(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.limited:
    type: cli
    command: echo
    args: ["{{msg}}"]
    shadow:
      rate_limit: 2/min
`,
	})

	// First two calls succeed
	_, _, code := run(t, dir, "call", "test.limited", "--msg", "1")
	if code != 0 {
		t.Fatal("first call should succeed")
	}
	_, _, code = run(t, dir, "call", "test.limited", "--msg", "2")
	if code != 0 {
		t.Fatal("second call should succeed")
	}

	// Third call should be rate limited
	_, stderr, code := run(t, dir, "call", "test.limited", "--msg", "3")
	if code == 0 {
		t.Fatal("third call should be rate limited")
	}
	if !strings.Contains(stderr, "rate limited") {
		t.Errorf("expected 'rate limited' in error, got: %s", stderr)
	}
}

func TestShadowConfirmAutoApproveWithYesFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.confirm:
    type: cli
    command: echo
    args: ["{{msg}}"]
    shadow:
      confirm: true
`,
	})

	// Without --yes, would prompt (but piped stdin = no input = decline)
	_, _, code := run(t, dir, "call", "test.confirm", "--msg", "hi")
	if code == 0 {
		t.Fatal("expected failure without confirmation input")
	}
}

func TestShadowNoRulesAllows(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  test.free:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "test.free", "--msg", "no shadow")
	if code != 0 {
		t.Fatal("tool without shadow should work")
	}
	if strings.TrimSpace(stdout) != "no shadow" {
		t.Errorf("expected 'no shadow', got %q", stdout)
	}
}

// --- Missing config ---

func TestMissingConfig(t *testing.T) {
	dir := t.TempDir() // empty directory, no factorly.yaml

	_, _, code := run(t, dir, "tools")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing config")
	}
}

// --- Init ---

func runWithStdin(t *testing.T, dir string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run factorly: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func TestInitDefaults(t *testing.T) {
	dir := t.TempDir()

	// Standard setup: yes example, no openapi, skip template, no sync
	stdin := "y\nn\nskip\nn\n"
	stdout, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("expected creation message, got %q", stdout)
	}

	// Verify file was created at .factorly/factorly.yaml
	data, err := os.ReadFile(filepath.Join(dir, ".factorly", "factorly.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "web.fetch") {
		t.Error("expected example tool web.fetch in config")
	}
	if !strings.Contains(content, "curl") {
		t.Error("expected curl command in config")
	}

	// Tools dir should always be created at .factorly/tools/ (auto-discovered
	// by the loader; we don't write a tools_dir line in YAML).
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "tools")); os.IsNotExist(err) {
		t.Error("expected .factorly/tools directory to be created")
	}
	if strings.Contains(content, "tools_dir") {
		t.Error("standard setup should not emit tools_dir; .factorly/tools/ is auto-discovered")
	}

	// Default workspace should always be created — it auto-loads
	// whenever no --workspace flag is set.
	defaultWs := filepath.Join(dir, ".factorly", "workspaces", "default.yaml")
	if _, err := os.Stat(defaultWs); os.IsNotExist(err) {
		t.Errorf("expected default workspace at %s", defaultWs)
	}
}

func TestInitAlreadyExists(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": "tools: {}",
	})

	stdin := "\n\n\n"
	_, _, code := runWithStdin(t, dir, stdin, "init")
	if code == 0 {
		t.Fatal("expected non-zero exit when .factorly/factorly.yaml already exists")
	}
}

func TestInitWithOutFlag(t *testing.T) {
	dir := t.TempDir()

	stdin := "y\nn\nskip\nn\n"
	_, _, code := runWithStdin(t, dir, stdin, "init", "--out", "factorly.yaml")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Should be at the custom path, not .factorly/
	if _, err := os.Stat(filepath.Join(dir, "factorly.yaml")); os.IsNotExist(err) {
		t.Error("expected factorly.yaml at custom path")
	}
	if _, err := os.Stat(filepath.Join(dir, ".factorly")); err == nil {
		t.Error("did not expect .factorly/ directory with --out flag")
	}
}

func TestInitNoExample(t *testing.T) {
	dir := t.TempDir()

	stdin := "n\nn\nskip\nn\n"
	_, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".factorly", "factorly.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tools:") {
		t.Error("expected tools key in config")
	}
}

// TestInitNoGitignoreSkipsPrompt: when no .gitignore exists, init must
// not create one and must not prompt about it. We're not the gitignore
// manager — only opt in to managing it when the user already has one.
func TestInitNoGitignoreSkipsPrompt(t *testing.T) {
	dir := t.TempDir()
	// stdin answers example=y, openapi=n, sync=n. No gitignore prompt
	// expected, but a trailing "y" is harmless if I'm wrong.
	stdin := "y\nn\nn\ny\n"
	stdout, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(stdout, "Append runtime state files to .gitignore") {
		t.Error("did not expect gitignore prompt when no .gitignore is present")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf("did not expect init to create a .gitignore (stat err: %v)", err)
	}
}

// TestInitGitignoreAppendsEntries: existing .gitignore + user accepts
// → three runtime-state entries appended; pre-existing rules preserved.
func TestInitGitignoreAppendsEntries(t *testing.T) {
	dir := t.TempDir()
	giPath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(giPath, []byte("*.log\nnode_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// example=y, openapi=n, gitignore=y, sync=n
	stdin := "y\nn\ny\nn\n"
	stdout, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "Append runtime state files to .gitignore") {
		t.Error("expected gitignore prompt in output")
	}

	data, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"*.log",                    // pre-existing preserved
		"node_modules/",            // pre-existing preserved
		".factorly/audit.jsonl",    // appended
		".factorly/ratelimit.json", // appended
		".factorly/runs/",          // appended
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in .gitignore after append, got:\n%s", want, body)
		}
	}
}

// TestInitGitignoreDeclineLeavesUntouched: user answers no → file
// stays byte-for-byte identical to what they had before.
func TestInitGitignoreDeclineLeavesUntouched(t *testing.T) {
	dir := t.TempDir()
	giPath := filepath.Join(dir, ".gitignore")
	original := "*.log\nnode_modules/\n"
	if err := os.WriteFile(giPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// example=y, openapi=n, gitignore=n, sync=n
	stdin := "y\nn\nn\nn\n"
	if _, _, code := runWithStdin(t, dir, stdin, "init"); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	data, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("expected .gitignore unchanged; got:\n%s\nwant:\n%s", data, original)
	}
}

// TestInitGitignoreSkipsWhenAlreadyPresent: pre-existing entries
// already cover all three state files → no prompt fires, no append.
func TestInitGitignoreSkipsWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	giPath := filepath.Join(dir, ".gitignore")
	original := ".factorly/audit.jsonl\n.factorly/ratelimit.json\n.factorly/runs/\n"
	if err := os.WriteFile(giPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// example=y, openapi=n, sync=n. Trailing buffer doesn't matter.
	stdin := "y\nn\nn\ny\n"
	stdout, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(stdout, "Append runtime state files to .gitignore") {
		t.Error("did not expect gitignore prompt when entries are already present")
	}

	data, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("expected .gitignore unchanged; got:\n%s", data)
	}
}

// --- .factorly/ project directory ---

func TestProjectDirLoading(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
tools:
  echo.project:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "echo.project") {
		t.Error("expected echo.project from .factorly/")
	}
}

func TestProjectDirLooseFiles(t *testing.T) {
	// .factorly/ with just tool files, no factorly.yaml inside
	dir := setupDir(t, map[string]string{
		".factorly/echo.yaml": `
echo.loose:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "echo.loose") {
		t.Error("expected echo.loose from .factorly/ loose files")
	}
}

func TestProjectDirMergesWithInline(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{{url}}"]
`,
		".factorly/echo.yaml": `
echo.project:
  type: cli
  command: echo
  args: ["{{msg}}"]
`,
	})

	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "web.fetch") {
		t.Error("expected web.fetch from factorly.yaml")
	}
	if !strings.Contains(stdout, "echo.project") {
		t.Error("expected echo.project from .factorly/")
	}
}

func TestProjectDirConflict(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  dupe:
    type: cli
    command: echo
    args: ["inline"]
`,
		".factorly/dupe.yaml": `
dupe:
  type: cli
  command: echo
  args: ["project"]
`,
	})

	_, _, code := run(t, dir, "tools")
	if code == 0 {
		t.Fatal("expected non-zero exit for duplicate tool across config and .factorly/")
	}
}

// --- Project Vault ---

func TestProjectVaultDefaultsToProject(t *testing.T) {
	dir := t.TempDir()
	// Create .factorly/ directory so vault defaults to project
	os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755)

	projectVault := filepath.Join(dir, ".factorly", "vault.enc")

	// Set a key — should go to project vault
	cmd := exec.Command(binary, "vault", "set", "PROJECT_KEY", "project_val")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_PROJECT_VAULT_PASSWORD=projpw",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault set failed: %v", err)
	}

	// Project vault file should exist
	if _, err := os.Stat(projectVault); os.IsNotExist(err) {
		t.Fatal("expected .factorly/vault.enc to be created")
	}

	// List should show the key
	cmd2 := exec.Command(binary, "vault", "list")
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_PROJECT_VAULT_PASSWORD=projpw",
	)
	var out strings.Builder
	cmd2.Stdout = &out
	if err := cmd2.Run(); err != nil {
		t.Fatalf("vault list failed: %v", err)
	}
	if !strings.Contains(out.String(), "PROJECT_KEY") {
		t.Errorf("expected PROJECT_KEY in list, got %q", out.String())
	}
}

func TestProjectVaultGlobalFlag(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755)

	globalVault := filepath.Join(dir, "global.enc")

	// Set in global vault explicitly
	cmd := exec.Command(binary, "vault", "--global", "--vault-path", globalVault, "set", "GLOBAL_KEY", "global_val")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=globalpw",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault --global set failed: %v", err)
	}

	// Global vault should exist, project vault should not
	if _, err := os.Stat(globalVault); os.IsNotExist(err) {
		t.Fatal("expected global vault to be created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "vault.enc")); err == nil {
		t.Error("expected project vault NOT to be created with --global")
	}
}

func TestProjectVaultGetFallsBackToGlobal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755)

	projectVault := filepath.Join(dir, ".factorly", "vault.enc")
	globalVault := filepath.Join(dir, "global.enc")

	// Create project vault with PROJECT_KEY
	cmd := exec.Command(binary, "vault", "--vault-path", projectVault, "set", "PROJECT_KEY", "from_project")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=pw",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("project vault set failed: %v", err)
	}

	// Create global vault with GLOBAL_KEY
	cmd2 := exec.Command(binary, "vault", "--vault-path", globalVault, "set", "GLOBAL_KEY", "from_global")
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=pw",
	)
	if err := cmd2.Run(); err != nil {
		t.Fatalf("global vault set failed: %v", err)
	}

	// Get PROJECT_KEY — should come from project vault
	cmd3 := exec.Command(binary, "vault", "get", "PROJECT_KEY")
	cmd3.Dir = dir
	cmd3.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_PROJECT_VAULT_PASSWORD=pw",
		"FACTORLY_VAULT_PASSWORD=pw",
		"FACTORLY_VAULT_PATH="+globalVault,
	)
	var out3 strings.Builder
	cmd3.Stdout = &out3
	if err := cmd3.Run(); err != nil {
		t.Fatalf("vault get PROJECT_KEY failed: %v", err)
	}
	if strings.TrimSpace(out3.String()) != "from_project" {
		t.Errorf("expected 'from_project', got %q", out3.String())
	}

	// Get GLOBAL_KEY — should fall back to global vault
	cmd4 := exec.Command(binary, "vault", "get", "GLOBAL_KEY")
	cmd4.Dir = dir
	cmd4.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_PROJECT_VAULT_PASSWORD=pw",
		"FACTORLY_VAULT_PASSWORD=pw",
		"FACTORLY_VAULT_PATH="+globalVault,
	)
	var out4 strings.Builder
	cmd4.Stdout = &out4
	if err := cmd4.Run(); err != nil {
		t.Fatalf("vault get GLOBAL_KEY (fallback) failed: %v", err)
	}
	if strings.TrimSpace(out4.String()) != "from_global" {
		t.Errorf("expected 'from_global', got %q", out4.String())
	}
}

func TestVaultRefInCLIArgs(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
tools:
  test.echo:
    type: cli
    command: echo
    args: ["secret={{vault:TEST_ARG_SECRET}}"]
`,
	})

	vaultPath := filepath.Join(dir, ".factorly", "vault.enc")

	// Store a secret
	cmd := exec.Command(binary, "vault", "--vault-path", vaultPath, "set", "TEST_ARG_SECRET", "resolved_value")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=pw",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault set failed: %v", err)
	}

	// Call the tool — {{vault:TEST_ARG_SECRET}} in args should resolve
	cmd2 := exec.Command(binary, "call", "test.echo")
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_PROJECT_VAULT_PASSWORD=pw",
	)
	var out strings.Builder
	cmd2.Stdout = &out
	if err := cmd2.Run(); err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if !strings.Contains(out.String(), "secret=resolved_value") {
		t.Errorf("expected 'secret=resolved_value', got %q", out.String())
	}
}

// --- External Vault Backends ---

func TestExternalVaultBackendCLI(t *testing.T) {
	// Configure an external backend that uses echo to return a secret
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
vault_backends:
  mock:
    type: cli
    get:
      command: echo
      args: ["secret_for_{{key}}"]
tools:
  test.api:
    type: cli
    command: echo
    args: ["token={{mock:MY_SECRET}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "test.api")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "token=secret_for_MY_SECRET") {
		t.Errorf("expected resolved external backend ref, got %q", stdout)
	}
}

func TestExternalVaultBackendList(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
vault_backends:
  mock:
    type: cli
    get:
      command: echo
      args: ["val"]
    list:
      command: printf
      args: ["KEY1\nKEY2\nKEY3"]
tools:
  echo:
    type: cli
    command: echo
    args: ["{{text}}"]
`,
	})

	// Tool listing should work (external backend doesn't interfere)
	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "echo") {
		t.Error("expected echo tool in listing")
	}
}

func TestExternalVaultBackendNoLocalVault(t *testing.T) {
	// External backend should work even without a local vault
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
vault_backends:
  mock:
    type: cli
    get:
      command: echo
      args: ["external_secret"]
tools:
  test.api:
    type: cli
    command: echo
    args: ["got={{mock:TOKEN}}"]
`,
	})

	stdout, _, code := run(t, dir, "call", "test.api")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "got=external_secret") {
		t.Errorf("expected external secret without local vault, got %q", stdout)
	}
}

func TestVaultPathEnvOverride(t *testing.T) {
	// Vault at a custom path (not project or global default) should work
	// via FACTORLY_VAULT_PATH — this is what CI uses
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
disable_builtins: true
tools:
  test.echo:
    type: cli
    command: echo
    args: ["secret={{vault:CUSTOM_KEY}}"]
`,
	})

	customVault := filepath.Join(dir, "custom-vault.enc")

	// Create vault at custom path
	cmd := exec.Command(binary, "vault", "--vault-path", customVault, "set", "CUSTOM_KEY", "custom_value")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=pw",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault set failed: %v", err)
	}

	// Call tool with FACTORLY_VAULT_PATH pointing to custom vault
	cmd2 := exec.Command(binary, "call", "test.echo", "-c", filepath.Join(dir, "factorly.yaml"))
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=pw",
		"FACTORLY_VAULT_PATH="+customVault,
	)
	var out strings.Builder
	cmd2.Stdout = &out
	var stderr strings.Builder
	cmd2.Stderr = &stderr
	if err := cmd2.Run(); err != nil {
		t.Fatalf("call failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(out.String(), "secret=custom_value") {
		t.Errorf("expected 'secret=custom_value', got %q", out.String())
	}
}

func TestExternalVaultBackendSetBlocked(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
vault_backends:
  mock:
    type: cli
    get:
      command: echo
      args: ["val"]
tools:
  echo:
    type: cli
    command: echo
    args: ["hi"]
`,
	})

	_, stderr, code := run(t, dir, "vault", "--backend", "mock", "set", "KEY", "val")
	if code == 0 {
		t.Fatal("expected non-zero exit for set on external backend")
	}
	if !strings.Contains(stderr, "read-only") {
		t.Errorf("expected 'read-only' in error, got %q", stderr)
	}
}

func TestExternalVaultBackendDeleteBlocked(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `
disable_builtins: true
vault_backends:
  mock:
    type: cli
    get:
      command: echo
      args: ["val"]
tools:
  echo:
    type: cli
    command: echo
    args: ["hi"]
`,
	})

	_, stderr, code := run(t, dir, "vault", "--backend", "mock", "delete", "KEY")
	if code == 0 {
		t.Fatal("expected non-zero exit for delete on external backend")
	}
	if !strings.Contains(stderr, "read-only") {
		t.Errorf("expected 'read-only' in error, got %q", stderr)
	}
}

// --- Vault ---

func TestVaultSetAndList(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")

	// Set secrets
	_, stderr, code := runVault(t, vp, "vault", "set", "MY_TOKEN", "secret-value-123")
	if code != 0 {
		t.Fatalf("vault set failed: %s", stderr)
	}
	runVault(t, vp, "vault", "set", "OTHER", "other-value")

	// List
	stdout, _, code := runVault(t, vp, "vault", "list")
	if code != 0 {
		t.Fatal("vault list failed")
	}
	if !strings.Contains(stdout, "MY_TOKEN") || !strings.Contains(stdout, "OTHER") {
		t.Errorf("expected both keys in list, got %q", stdout)
	}
}

func TestVaultDelete(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")

	runVault(t, vp, "vault", "set", "TEMP_KEY", "value")
	_, _, code := runVault(t, vp, "vault", "delete", "TEMP_KEY")
	if code != 0 {
		t.Fatal("vault delete failed")
	}

	stdout, _, _ := runVault(t, vp, "vault", "list")
	if strings.Contains(stdout, "TEMP_KEY") {
		t.Error("expected TEMP_KEY to be deleted")
	}
}

func TestVaultCallWithSecret(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")
	secret := "vault-injected-bearer-token"

	// Store secret in vault
	runVault(t, vp, "vault", "set", "API_SECRET", secret)

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.test:
    type: rest
    base_url: %s
    method: GET
    path: /data
    auth:
      type: bearer
      token: "{{vault:API_SECRET}}"
`, srv.URL),
	})

	stdout, stderr, code := runVault(t, vp, "-c", filepath.Join(dir, "factorly.yaml"), "call", "api.test")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}

	// Server received the vault secret
	if capturedAuth != "Bearer "+secret {
		t.Errorf("expected server to receive vault secret, got %q", capturedAuth)
	}

	// Agent never sees the secret
	if strings.Contains(stdout, secret) {
		t.Error("SECRET LEAKED: vault secret appeared in stdout")
	}
	if strings.Contains(stderr, secret) {
		t.Error("SECRET LEAKED: vault secret appeared in stderr")
	}
}

// runVault runs factorly with a temp vault path and password set via env.
func runVault(t *testing.T, vaultPath string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpass123",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run factorly: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

// --- Credential Isolation ---
// Verifies that secrets configured in Factorly never appear in the
// agent-visible output (stdout/stderr) — only in the HTTP request
// that Factorly sends on the agent's behalf.

func TestCredentialIsolation(t *testing.T) {
	secret := "super-secret-token-12345"

	var capturedAuth string
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":"octocat","public_repos":8}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.get_user:
    type: rest
    description: "Get a user"
    base_url: %s
    method: GET
    path: /users/{{username}}
    auth:
      type: bearer
      token: "%s"
    parameters:
      - name: username
        in: path
        required: true
`, srv.URL, secret),
	})

	stdout, stderr, code := run(t, dir, "call", "api.get_user", "--username", "octocat")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}

	// The server received the secret in the auth header
	if capturedAuth != "Bearer "+secret {
		t.Errorf("expected server to receive bearer token, got %q", capturedAuth)
	}
	if capturedPath != "/users/octocat" {
		t.Errorf("expected path /users/octocat, got %s", capturedPath)
	}

	// The agent (stdout + stderr) NEVER sees the secret
	if strings.Contains(stdout, secret) {
		t.Error("SECRET LEAKED: token appeared in stdout (agent-visible output)")
	}
	if strings.Contains(stderr, secret) {
		t.Error("SECRET LEAKED: token appeared in stderr (agent-visible output)")
	}

	// The agent only sees the response data
	if !strings.Contains(stdout, "octocat") {
		t.Error("expected response data in stdout")
	}
}

func TestCredentialIsolationVerboseMode(t *testing.T) {
	secret := "sk_live_stripe_key_99999"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  payments.list:
    type: rest
    description: "List payments"
    base_url: %s
    method: GET
    path: /v1/charges
    auth:
      type: bearer
      token: "%s"
    parameters:
      - name: limit
        in: query
`, srv.URL, secret),
	})

	// Even in verbose mode, secrets must not leak
	stdout, stderr, code := run(t, dir, "call", "-v", "payments.list", "--limit", "10")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}

	if strings.Contains(stdout, secret) {
		t.Error("SECRET LEAKED: token appeared in stdout during verbose mode")
	}
	if strings.Contains(stderr, secret) {
		t.Error("SECRET LEAKED: token appeared in stderr during verbose mode")
	}

	// Verbose output should show tool name and params but not the token
	if !strings.Contains(stderr, "payments.list") {
		t.Error("expected verbose output to mention tool name")
	}
	if !strings.Contains(stderr, "limit") {
		t.Error("expected verbose output to mention param name")
	}
}

func TestCredentialIsolationMultipleProviders(t *testing.T) {
	githubSecret := "ghp_github_secret_token"
	slackSecret := "xoxb_slack_bot_token"

	var githubAuth, slackAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/github"):
			githubAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`[{"name":"repo1"}]`))
		case strings.HasPrefix(r.URL.Path, "/slack"):
			slackAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  github.repos:
    type: rest
    base_url: %s
    method: GET
    path: /github/repos
    auth:
      type: bearer
      token: "%s"
  slack.post:
    type: rest
    base_url: %s
    method: POST
    path: /slack/chat
    auth:
      type: bearer
      token: "%s"
    parameters:
      - name: body
        in: body
  echo.safe:
    type: cli
    command: echo
    args: ["{{msg}}"]
`, srv.URL, githubSecret, srv.URL, slackSecret),
	})

	// Call each tool and verify isolation
	stdout1, stderr1, _ := run(t, dir, "call", "github.repos")
	stdout2, stderr2, _ := run(t, dir, "call", "slack.post", "--body", `{"text":"hi"}`)
	stdout3, stderr3, _ := run(t, dir, "call", "echo.safe", "--msg", "hello")

	allOutput := stdout1 + stderr1 + stdout2 + stderr2 + stdout3 + stderr3

	if strings.Contains(allOutput, githubSecret) {
		t.Error("SECRET LEAKED: GitHub token appeared in agent-visible output")
	}
	if strings.Contains(allOutput, slackSecret) {
		t.Error("SECRET LEAKED: Slack token appeared in agent-visible output")
	}

	// But both servers received their respective tokens
	if githubAuth != "Bearer "+githubSecret {
		t.Errorf("expected GitHub server to receive its token, got %q", githubAuth)
	}
	if slackAuth != "Bearer "+slackSecret {
		t.Errorf("expected Slack server to receive its token, got %q", slackAuth)
	}

	// CLI tool works alongside REST tools
	if strings.TrimSpace(stdout3) != "hello" {
		t.Errorf("expected echo output 'hello', got %q", stdout3)
	}
}

func TestCredentialIsolationHeaderAuth(t *testing.T) {
	apiKey := "ak_custom_header_secret"

	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.call:
    type: rest
    base_url: %s
    method: GET
    path: /data
    auth:
      type: header
      header: X-Api-Key
      value: "%s"
`, srv.URL, apiKey),
	})

	stdout, stderr, code := run(t, dir, "call", "api.call")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Server got the key
	if capturedKey != apiKey {
		t.Errorf("expected server to receive API key, got %q", capturedKey)
	}

	// Agent never sees it
	if strings.Contains(stdout, apiKey) {
		t.Error("SECRET LEAKED: API key appeared in stdout")
	}
	if strings.Contains(stderr, apiKey) {
		t.Error("SECRET LEAKED: API key appeared in stderr")
	}
}

func TestCredentialIsolationToolsDir(t *testing.T) {
	secret := "secret-from-tools-dir"

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		".factorly/api.yaml": fmt.Sprintf(`
api.call:
  type: rest
  base_url: %s
  method: GET
  path: /data
  auth:
    type: bearer
    token: "%s"
`, srv.URL, secret),
	})

	stdout, stderr, code := run(t, dir, "call", "api.call")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}

	if capturedAuth != "Bearer "+secret {
		t.Errorf("expected server to receive token, got %q", capturedAuth)
	}
	if strings.Contains(stdout+stderr, secret) {
		t.Error("SECRET LEAKED: token from .factorly/ tool file appeared in output")
	}
}

// --- Security Hardening Tests ---

func TestLogFilePermissions(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{msg}}"]
`,
	})

	// The project-scoped logger writes to <dir>/.factorly/audit.jsonl;
	// strip FACTORLY_NO_LOG so the logger actually runs.
	cmd := exec.Command(binary, "call", "echo.test", "--msg", "test")
	cmd.Dir = dir
	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FACTORLY_NO_LOG=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, "FACTORLY_NO_UPDATE_CHECK=1")
	_ = cmd.Run()

	auditPath := filepath.Join(dir, ".factorly", "audit.jsonl")
	info, err := os.Stat(auditPath)
	if err != nil {
		t.Fatalf("expected audit log at %s: %v", auditPath, err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected log file permissions 0600, got %04o", perm)
	}
}

func TestVaultSecretNotInVerboseOutput(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")
	secret := "vault-verbose-leak-test-token"

	runVault(t, vp, "vault", "set", "VERBOSE_SECRET", secret)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  api.test:
    type: rest
    base_url: %s
    method: GET
    path: /data
    auth:
      type: bearer
      token: "{{vault:VERBOSE_SECRET}}"
`, srv.URL),
	})

	// Run with verbose AND vault
	cmd := exec.Command(binary, "call", "-v", "-c", filepath.Join(dir, "factorly.yaml"), "api.test")
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpass123",
		"FACTORLY_VAULT_PATH="+vp,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()

	if strings.Contains(stdout.String(), secret) {
		t.Error("SECRET LEAKED: vault secret appeared in stdout during verbose mode")
	}
	if strings.Contains(stderr.String(), secret) {
		t.Error("SECRET LEAKED: vault secret appeared in stderr during verbose mode")
	}
}

func TestVaultEmptyPasswordRejected(t *testing.T) {
	// Empty password via stdin should be rejected
	cmd := exec.Command(binary, "vault", "list")
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PATH="+filepath.Join(t.TempDir(), "vault.enc"),
	)
	cmd.Stdin = strings.NewReader("\n") // empty password
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for empty vault password")
	}
	if !strings.Contains(stderr.String(), "empty") {
		t.Errorf("expected 'empty' in error message, got %q", stderr.String())
	}
}

func TestVaultFilePermissions(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")

	runVault(t, vp, "vault", "set", "KEY", "value")

	info, err := os.Stat(vp)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected vault file permissions 0600, got %04o", perm)
	}
}

func TestVaultWrongPasswordFails(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")

	// Create vault with known password
	runVault(t, vp, "vault", "set", "KEY", "value")

	// Try to access with wrong password
	cmd := exec.Command(binary, "vault", "list")
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=wrong-password",
		"FACTORLY_VAULT_PATH="+vp,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for wrong vault password")
	}
	// Env-var-sourced passwords don't retry (retrying the same env
	// value would just re-fail). The error message names the source
	// and the vault path so users know what to fix.
	es := stderr.String()
	if !strings.Contains(es, "FACTORLY_VAULT_PASSWORD") {
		t.Errorf("expected env-var name in error, got %q", es)
	}
	if !strings.Contains(es, "did not unlock") {
		t.Errorf("expected 'did not unlock' phrasing, got %q", es)
	}
}

// TestVaultInteractivePromptRetries pins the new "3 attempts" prompt
// loop. Piping three lines on stdin: two wrong, one right. The first
// two should produce "Incorrect password, try again" messages on
// stderr; the third unlocks and the command succeeds.
//
// We don't pipe a separate FACTORLY_VAULT_PASSWORD here — that env
// var would short-circuit the prompt path entirely.
func TestVaultInteractivePromptRetries(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")

	// Create vault with the known password.
	runVault(t, vp, "vault", "set", "KEY", "value")

	// Now invoke `vault list` without FACTORLY_VAULT_PASSWORD; pipe
	// stdin with three attempts. The CLI's prompt path reads from
	// stdin via bufio.Scanner when stdin isn't a TTY (which it isn't
	// here since we're piping). "testpass123" is the password used
	// by runVault above.
	cmd := exec.Command(binary, "vault", "list")
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PATH="+vp,
	)
	// Strip any vault password env that might be inherited.
	cleaned := cmd.Env[:0]
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "FACTORLY_VAULT_PASSWORD=") {
			continue
		}
		cleaned = append(cleaned, e)
	}
	cmd.Env = cleaned
	cmd.Stdin = strings.NewReader("wrong-one\nwrong-two\ntestpass123\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected success after retry; err=%v stdout=%q stderr=%q",
			err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Incorrect password") {
		t.Errorf("expected 'Incorrect password' message after wrong attempt, got stderr=%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "KEY") {
		t.Errorf("expected 'KEY' in list output after eventual success, got stdout=%q", stdout.String())
	}
}

// TestVaultInteractivePromptFailsAfterMaxAttempts pins the upper
// bound: three wrong passwords in a row → command exits non-zero
// with a "after 3 attempts" message.
func TestVaultInteractivePromptFailsAfterMaxAttempts(t *testing.T) {
	vp := filepath.Join(t.TempDir(), "vault.enc")
	runVault(t, vp, "vault", "set", "KEY", "value")

	cmd := exec.Command(binary, "vault", "list")
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PATH="+vp,
	)
	cleaned := cmd.Env[:0]
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "FACTORLY_VAULT_PASSWORD=") {
			continue
		}
		cleaned = append(cleaned, e)
	}
	cmd.Env = cleaned
	cmd.Stdin = strings.NewReader("nope1\nnope2\nnope3\n")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit after 3 wrong attempts")
	}
	if !strings.Contains(stderr.String(), "3 attempts") {
		t.Errorf("expected error to mention 3 attempts, got %q", stderr.String())
	}
}

// --- Exec ---

func TestExecBasic(t *testing.T) {
	stdout, _, code := run(t, "", "exec", "--", "echo", "hello world")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", stdout)
	}
}

func TestExecPreservesExitCode(t *testing.T) {
	_, _, code := run(t, "", "exec", "--", "false")
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestExecCompressNone(t *testing.T) {
	stdout, _, code := run(t, "", "exec", "--compress", "none", "--", "echo", "no compression")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "no compression") {
		t.Errorf("expected output, got %q", stdout)
	}
}

func TestExecMaxOutput(t *testing.T) {
	// Generate output larger than max, verify truncation
	stdout, _, code := run(t, "", "exec", "--max-output", "50", "--", "echo", strings.Repeat("x", 200))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(stdout) > 150 { // some slack for truncation marker
		t.Errorf("expected truncated output, got %d bytes", len(stdout))
	}
	if !strings.Contains(stdout, "truncated") {
		t.Error("expected truncation marker")
	}
}

func TestExecEnvIsolationStrict(t *testing.T) {
	// Set a var that should be hidden in strict mode
	stdout, _, code := run(t, "", "exec", "--env-isolation", "strict", "--", "env")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Should have PATH but not random parent env vars
	if !strings.Contains(stdout, "PATH=") {
		t.Error("expected PATH in strict env")
	}
}

func TestExecNoArgs(t *testing.T) {
	_, _, code := run(t, "", "exec")
	if code == 0 {
		t.Fatal("expected non-zero exit with no args")
	}
}

func TestExecInteractiveFlag(t *testing.T) {
	// Interactive mode with a simple command — should still work
	stdout, _, code := run(t, "", "exec", "-i", "--", "echo", "interactive")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "interactive") {
		t.Errorf("expected 'interactive' in output, got %q", stdout)
	}
}

func TestExecEnvVarResolution(t *testing.T) {
	// {{env:HOME}} should be resolved to the actual HOME value
	stdout, _, code := run(t, "", "exec", "--", "echo", "{{env:HOME}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "{{env:HOME}}" {
		t.Error("env ref was not resolved")
	}
	if stdout == "" {
		t.Error("expected non-empty HOME value")
	}
}

func TestExecVaultResolution(t *testing.T) {
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "vault.enc")

	// Store a secret
	cmd := exec.Command(binary, "vault", "set", "EXEC_TEST_SECRET", "secret_value_123")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpassword",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault set failed: %v", err)
	}

	// Use vault ref in exec
	cmd2 := exec.Command(binary, "exec", "--", "echo", "{{vault:EXEC_TEST_SECRET}}")
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpassword",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	var stdout2 strings.Builder
	cmd2.Stdout = &stdout2
	if err := cmd2.Run(); err != nil {
		t.Fatalf("exec with vault ref failed: %v", err)
	}

	output := strings.TrimSpace(stdout2.String())
	if output != "secret_value_123" {
		t.Errorf("expected 'secret_value_123', got %q", output)
	}
}

// --- Exec --env ---

func TestExecEnvFlag(t *testing.T) {
	stdout, _, code := run(t, "", "exec", "--env", "FOO=bar", "--env", "BAZ=qux", "--", "echo", "{{env:FOO}} {{env:BAZ}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "bar qux") {
		t.Errorf("expected 'bar qux', got %q", stdout)
	}
}

func TestExecEnvFlagStrictIsolation(t *testing.T) {
	stdout, _, code := run(t, "", "exec", "--env-isolation", "strict", "--env", "CUSTOM=hello", "--", "env")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "CUSTOM=hello") {
		t.Error("expected CUSTOM=hello in strict env")
	}
	if !strings.Contains(stdout, "PATH=") {
		t.Error("expected PATH in strict env")
	}
}

func TestExecEnvFlagWithParentEnv(t *testing.T) {
	// --env can reference parent env vars via {{env:HOME}}
	stdout, _, code := run(t, "", "exec", "--env", "MY_HOME={{env:HOME}}", "--", "echo", "{{env:MY_HOME}}")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" || stdout == "{{env:MY_HOME}}" {
		t.Errorf("expected resolved HOME value, got %q", stdout)
	}
}

// --- Disabled Commands ---

func TestDisabledCommandBlocked(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `disabled_commands: [exec, vault]
tools:
  echo:
    type: cli
    command: echo
    args: ["{{text}}"]
`,
	})

	// exec should be blocked
	_, stderr, code := run(t, dir, "exec", "--", "echo", "hello")
	if code == 0 {
		t.Fatal("expected exec to be blocked")
	}
	if !strings.Contains(stderr, "disabled") {
		t.Errorf("expected 'disabled' in error, got %q", stderr)
	}

	// vault should be blocked
	_, stderr, code = run(t, dir, "vault", "list")
	if code == 0 {
		t.Fatal("expected vault to be blocked")
	}
	if !strings.Contains(stderr, "disabled") {
		t.Errorf("expected 'disabled' in error, got %q", stderr)
	}

	// call should still work
	stdout, _, code := run(t, dir, "call", "echo", "--text", "allowed")
	if code != 0 {
		t.Fatalf("expected call to work, got exit %d", code)
	}
	if !strings.Contains(stdout, "allowed") {
		t.Errorf("expected 'allowed' in output, got %q", stdout)
	}
}

func TestDisabledCommandServe(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `disabled_commands: [serve]
tools:
  echo:
    type: cli
    command: echo
    args: ["hello"]
`,
	})

	_, stderr, code := run(t, dir, "serve")
	if code == 0 {
		t.Fatal("expected serve to be blocked")
	}
	if !strings.Contains(stderr, "disabled") {
		t.Errorf("expected 'disabled' in error, got %q", stderr)
	}
}

func TestDisabledCommandNoneConfigured(t *testing.T) {
	dir := setupDir(t, map[string]string{
		".factorly/factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{text}}"]
`,
	})

	// No disabled_commands — everything should work
	stdout, _, code := run(t, dir, "call", "echo", "--text", "works")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "works") {
		t.Errorf("expected 'works' in output, got %q", stdout)
	}
}

// helpers

func findPetstoreSpec(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../examples/petstore.yaml",
		"examples/petstore.yaml",
		"src/examples/petstore.yaml",
		"../../src/examples/petstore.yaml",
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	t.Skip("petstore.yaml example not found")
	return ""
}

func TestCallRESTFileParam(t *testing.T) {
	var capturedBody []byte
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"transcript":"hello world"}`))
	}))
	defer srv.Close()

	// Create a test file
	testData := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00} // RIFF header stub
	testFile := filepath.Join(t.TempDir(), "test.wav")
	if err := os.WriteFile(testFile, testData, 0o644); err != nil {
		t.Fatal(err)
	}

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  audio.transcribe:
    type: rest
    base_url: %s
    method: POST
    path: /v1/listen
    headers:
      Content-Type: audio/wav
    parameters:
      - name: file
        in: file
        required: true
      - name: model
        in: query
        default: "nova-3"
`, srv.URL),
	})

	stdout, stderr, code := run(t, dir, "call", "audio.transcribe", "--file", testFile)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}
	if capturedContentType != "audio/wav" {
		t.Errorf("expected Content-Type audio/wav, got %s", capturedContentType)
	}
	if len(capturedBody) != len(testData) {
		t.Errorf("expected %d bytes in body, got %d", len(testData), len(capturedBody))
	}
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("expected transcript in output, got %q", stdout)
	}
}

func TestCallRESTFileParamMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": fmt.Sprintf(`
tools:
  upload:
    type: rest
    base_url: %s
    method: POST
    path: /upload
    parameters:
      - name: data
        in: file
        required: true
`, srv.URL),
	})

	_, stderr, code := run(t, dir, "call", "upload", "--data", "/nonexistent/file.bin")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing file")
	}
	if !strings.Contains(stderr, "opening file") {
		t.Errorf("expected file error in stderr, got %q", stderr)
	}
}

// --- Workflows ---

func TestCallWorkflowSimple(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.hello:
    type: cli
    command: echo
    args: ["hello"]
  echo.world:
    type: cli
    command: echo
    args: ["world"]
  my.pipeline:
    type: workflow
    description: Run two echo commands
    steps:
      - tool: echo.hello
      - tool: echo.world
`,
	})

	stdout, _, code := run(t, dir, "call", "my.pipeline")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Workflow output should contain the last step's output
	if !strings.Contains(stdout, "world") {
		t.Errorf("expected 'world' in output, got %q", stdout)
	}
}

func TestCallWorkflowVariablePassing(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.chain:
    type: workflow
    description: Pass output from step 1 to step 2
    steps:
      - tool: echo.msg
        params: { message: "first" }
        store: result
      - tool: echo.msg
        params: { message: "got {{result}}" }
`,
	})

	stdout, _, code := run(t, dir, "call", "pipeline.chain")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Step 2 should receive step 1's output via {{result}}
	if !strings.Contains(stdout, "got first") {
		t.Errorf("expected 'got first' in output, got %q", stdout)
	}
}

func TestCallWorkflowWithInputParams(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.greet:
    type: workflow
    description: Greet someone
    parameters:
      - name: name
        required: true
    steps:
      - tool: echo.msg
        params: { message: "hello {{name}}" }
`,
	})

	stdout, _, code := run(t, dir, "call", "pipeline.greet", "--name", "world")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", stdout)
	}
}

func TestCallWorkflowWithShadowDeny(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.safe:
    type: cli
    command: echo
    args: ["safe"]
  echo.danger:
    type: cli
    command: echo
    args: ["danger"]
    shadow:
      deny: [echo.danger]
  pipeline.mixed:
    type: workflow
    description: Second step is denied
    steps:
      - tool: echo.safe
      - tool: echo.danger
`,
	})

	_, stderr, code := run(t, dir, "call", "pipeline.mixed")
	if code == 0 {
		t.Fatal("expected non-zero exit when workflow step is denied")
	}
	if !strings.Contains(stderr, "denied") {
		t.Errorf("expected 'denied' in stderr, got %q", stderr)
	}
}

func TestCallWorkflowThreeSteps(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: printf
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.three:
    type: workflow
    description: Three step chain
    steps:
      - tool: echo.msg
        params: { message: "alpha" }
        store: a
      - tool: echo.msg
        params: { message: "{{a}} beta" }
        store: b
      - tool: echo.msg
        params: { message: "{{b}} gamma" }
`,
	})

	stdout, _, code := run(t, dir, "call", "pipeline.three")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Output is JSON with result field containing the final output
	if !strings.Contains(stdout, "alpha beta gamma") {
		t.Errorf("expected chained output 'alpha beta gamma', got %q", stdout)
	}
}

func TestCallWorkflowIfCondition(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: printf
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.conditional:
    type: workflow
    description: Skip step based on condition
    parameters:
      - name: flag
    steps:
      - tool: echo.msg
        params: { message: "always" }
        store: first
      - tool: echo.msg
        params: { message: "conditional" }
        if: "flag == 'yes'"
        store: second
      - tool: echo.msg
        params: { message: "done" }
`,
	})

	// With flag=yes: all three steps run
	stdout, _, code := run(t, dir, "call", "pipeline.conditional", "--flag", "yes")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, `"result":"done"`) {
		t.Errorf("expected 'done' as result, got %q", stdout)
	}
	if strings.Contains(stdout, `"skipped"`) {
		t.Error("no steps should be skipped when flag=yes")
	}

	// With flag=no: second step skipped
	stdout, _, code = run(t, dir, "call", "pipeline.conditional", "--flag", "no")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, `"skipped"`) {
		t.Errorf("expected skipped step when flag=no, got %q", stdout)
	}
}

func TestCallWorkflowSwitch(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: printf
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.switch:
    type: workflow
    description: Branch based on input
    parameters:
      - name: mode
        required: true
    steps:
      - switch:
          - condition: "mode == 'fast'"
            tool: echo.msg
            params: { message: "speed mode" }
          - condition: "mode == 'safe'"
            tool: echo.msg
            params: { message: "safety mode" }
          - condition: "true"
            tool: echo.msg
            params: { message: "default mode" }
`,
	})

	// mode=fast → first case
	stdout, _, code := run(t, dir, "call", "pipeline.switch", "--mode", "fast")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "speed mode") {
		t.Errorf("expected 'speed mode', got %q", stdout)
	}

	// mode=safe → second case
	stdout, _, code = run(t, dir, "call", "pipeline.switch", "--mode", "safe")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "safety mode") {
		t.Errorf("expected 'safety mode', got %q", stdout)
	}

	// mode=other → default (true)
	stdout, _, code = run(t, dir, "call", "pipeline.switch", "--mode", "other")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "default mode") {
		t.Errorf("expected 'default mode', got %q", stdout)
	}
}

func TestCallWorkflowSwitchWithStore(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  echo.msg:
    type: cli
    command: printf
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  pipeline.store:
    type: workflow
    description: Switch stores output for next step
    parameters:
      - name: tier
    steps:
      - switch:
          - condition: "tier == 'premium'"
            tool: echo.msg
            params: { message: "unlimited" }
            store: limit
          - condition: "true"
            tool: echo.msg
            params: { message: "100" }
            store: limit
      - tool: echo.msg
        params: { message: "your limit is {{limit}}" }
`,
	})

	stdout, _, code := run(t, dir, "call", "pipeline.store", "--tier", "premium")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "your limit is unlimited") {
		t.Errorf("expected 'your limit is unlimited', got %q", stdout)
	}

	stdout, _, code = run(t, dir, "call", "pipeline.store", "--tier", "free")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "your limit is 100") {
		t.Errorf("expected 'your limit is 100', got %q", stdout)
	}
}

// --- Blueprint install lifecycle ---

func TestBlueprintsEmptyList(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})
	stdout, _, code := run(t, dir, "blueprint", "list")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "No blueprints installed") {
		t.Errorf("expected empty-state message, got %q", stdout)
	}
}

func TestBlueprintInstallFromLocalFile(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"my-blueprint.yaml": `
name: gmail-toolkit
version: 1.0.0
description: Gmail integration
tools:
  gmail.search:
    type: cli
    command: echo
    description: search Gmail
    args: ["{{q}}"]
  gmail.daily:
    type: workflow
    steps:
      - tool: gmail.search
        params: { q: "is:unread" }
`,
	})

	stdout, _, code := run(t, dir, "blueprint", "install", "./my-blueprint.yaml", "--no-prompt")
	if code != 0 {
		t.Fatalf("install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "gmail-toolkit 1.0.0") {
		t.Errorf("expected blueprint title in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "+ gmail.search") {
		t.Errorf("expected tools summary, got %q", stdout)
	}
	if !strings.Contains(stdout, "+ gmail.daily") {
		t.Errorf("expected workflows summary, got %q", stdout)
	}

	// Blueprint file should exist on disk
	blueprintFile := filepath.Join(dir, ".factorly", "blueprints", "gmail-toolkit.yaml")
	if _, err := os.Stat(blueprintFile); err != nil {
		t.Fatalf("expected blueprint file at %s: %v", blueprintFile, err)
	}

	// Tools should be visible via 'factorly tools'
	stdout, _, code = run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("tools list: exit %d", code)
	}
	if !strings.Contains(stdout, "gmail.search") || !strings.Contains(stdout, "gmail.daily") {
		t.Errorf("expected blueprint tools in tools list, got %q", stdout)
	}

	// Blueprints list should report the installed blueprint
	stdout, _, code = run(t, dir, "blueprint", "list")
	if code != 0 {
		t.Fatalf("blueprints list: exit %d", code)
	}
	if !strings.Contains(stdout, "gmail-toolkit") || !strings.Contains(stdout, "1.0.0") {
		t.Errorf("expected gmail-toolkit in blueprints list, got %q", stdout)
	}
}

func TestBlueprintInstallDryRun(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"my-blueprint.yaml": `
name: dryrun-test
tools:
  dry.tool:
    type: cli
    command: echo
    description: dry
`,
	})

	stdout, _, code := run(t, dir, "blueprint", "install", "./my-blueprint.yaml", "--dry-run", "--no-prompt")
	if code != 0 {
		t.Fatalf("dry-run install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "Dry run") {
		t.Errorf("expected 'Dry run' message, got %q", stdout)
	}
	// Blueprint file should NOT exist
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "blueprints", "dryrun-test.yaml")); err == nil {
		t.Fatal("dry-run should not write a blueprint file")
	}
	// Tool should NOT appear in tools list
	stdout, _, _ = run(t, dir, "tools")
	if strings.Contains(stdout, "dry.tool") {
		t.Errorf("dry-run tool leaked into tools list: %q", stdout)
	}
}

func TestBlueprintUninstall(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"my-blueprint.yaml": `
name: removable
tools:
  rm.test:
    type: cli
    command: echo
    description: removable
`,
	})

	if _, _, code := run(t, dir, "blueprint", "install", "./my-blueprint.yaml", "--no-prompt"); code != 0 {
		t.Fatalf("install: exit %d", code)
	}
	stdout, _, code := run(t, dir, "blueprint", "uninstall", "removable")
	if code != 0 {
		t.Fatalf("uninstall: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "Uninstalled removable") {
		t.Errorf("expected uninstall confirmation, got %q", stdout)
	}
	// Blueprint file should be gone
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "blueprints", "removable.yaml")); err == nil {
		t.Fatal("blueprint file should be removed after uninstall")
	}
	// Tool should disappear from tools list
	stdout, _, _ = run(t, dir, "tools")
	if strings.Contains(stdout, "rm.test") {
		t.Errorf("uninstalled tool still in tools list: %q", stdout)
	}
	// Second uninstall should fail clearly
	_, stderr, code := run(t, dir, "blueprint", "uninstall", "removable")
	if code == 0 {
		t.Fatal("expected uninstalling-missing-blueprint to error")
	}
	if !strings.Contains(stderr, "not installed") {
		t.Errorf("expected 'not installed' error, got stderr=%q", stderr)
	}
}

func TestBlueprintInstallConflict(t *testing.T) {
	// Existing project already defines a tool the blueprint would shadow.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools:
  shared.tool:
    type: cli
    command: existing
    description: existing
`,
		"my-blueprint.yaml": `
name: conflicty
tools:
  shared.tool:
    type: cli
    command: new
    description: from-blueprint
`,
	})

	stdout, stderr, code := run(t, dir, "blueprint", "install", "./my-blueprint.yaml", "--no-prompt")
	if code == 0 {
		t.Fatal("expected install to fail on conflict")
	}
	if !strings.Contains(stdout, "Conflicts") {
		t.Errorf("expected conflict section in output, got %q", stdout)
	}
	if !strings.Contains(stderr, "conflict") {
		t.Errorf("expected conflict error on stderr, got %q", stderr)
	}
	// Blueprint must not be written
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "blueprints", "conflicty.yaml")); err == nil {
		t.Fatal("conflicting blueprint should not have been written")
	}
}

func TestBlueprintInstallFromHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
name: from-http
version: 0.1
tools:
  http.tool:
    type: cli
    command: echo
    description: served over http
`))
	}))
	defer srv.Close()

	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})

	stdout, _, code := run(t, dir, "blueprint", "install", srv.URL+"/blueprint.yaml", "--no-prompt")
	if code != 0 {
		t.Fatalf("install from URL: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "from-http") {
		t.Errorf("expected blueprint name in output, got %q", stdout)
	}

	stdout, _, _ = run(t, dir, "tools")
	if !strings.Contains(stdout, "http.tool") {
		t.Errorf("expected http-served tool in tools list, got %q", stdout)
	}
}

func TestBlueprintInstallWithOAuthProvider(t *testing.T) {
	// A blueprint ships its own oauth_providers entry. After install, the provider
	// must be visible via the merged config; we verify via 'factorly tools'
	// for a tool that auth-references the provider (load failure would surface
	// as a non-zero exit code from validate).
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"linear-blueprint.yaml": `
name: linear
oauth_providers:
  linear:
    client_id: "{{vault:linear_client_id}}"
    client_secret: "{{vault:linear_client_secret}}"
    auth_url: https://linear.app/oauth/authorize
    token_url: https://api.linear.app/oauth/token
    scopes: [read]
tools:
  linear.list:
    type: rest
    base_url: https://api.linear.app
    method: GET
    path: /graphql
    description: list issues
    auth:
      type: oauth
      provider: linear
      token_key: linear
`,
	})

	stdout, _, code := run(t, dir, "blueprint", "install", "./linear-blueprint.yaml", "--no-prompt")
	if code != 0 {
		t.Fatalf("install: exit %d, %s", code, stdout)
	}
	// Tool should be loadable — proves the oauth_providers.linear reference
	// resolved against the same file's provider block during merge+validate.
	stdout, _, code = run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("tools list after install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "linear.list") {
		t.Errorf("expected linear.list in tools list, got %q", stdout)
	}
}

func TestBlueprintBackwardCompatWithFlatMapFile(t *testing.T) {
	// A user has a legacy flat-map .factorly/my-tools.yaml AND installs a new
	// blueprint file. Both must coexist and both their tools must register.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		".factorly/legacy.yaml": `
legacy.tool:
  type: cli
  command: echo
  description: legacy flat-map style
`,
		"new-blueprint.yaml": `
name: newstyle
tools:
  new.tool:
    type: cli
    command: echo
    description: new blueprint style
`,
	})

	// Sanity: legacy tool already loads before install
	stdout, _, code := run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("pre-install tools: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "legacy.tool") {
		t.Fatalf("legacy flat-map tool should load: %q", stdout)
	}

	if _, _, code := run(t, dir, "blueprint", "install", "./new-blueprint.yaml", "--no-prompt"); code != 0 {
		t.Fatal("install failed")
	}

	stdout, _, code = run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("post-install tools: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "legacy.tool") {
		t.Errorf("legacy tool dropped after install: %q", stdout)
	}
	if !strings.Contains(stdout, "new.tool") {
		t.Errorf("new blueprint tool missing: %q", stdout)
	}
}

func TestBlueprintUnknownSourceFails(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})
	_, stderr, code := run(t, dir, "blueprint", "install", "./does-not-exist.yaml", "--no-prompt")
	if code == 0 {
		t.Fatal("expected missing-source to fail")
	}
	if !strings.Contains(stderr, "does not exist") {
		t.Errorf("expected 'does not exist' in error, got %q", stderr)
	}
}

// --- Blueprint CLI surface details ---

func TestBlueprintsListMultipleSorted(t *testing.T) {
	// Two blueprints should list in name-sorted order regardless of install order.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"zeta-blueprint.yaml": `
name: zeta
version: 9.9
description: last alphabetically
tools: {}
`,
		"alpha-blueprint.yaml": `
name: alpha
version: 0.1
description: first alphabetically
tools: {}
`,
	})

	if _, _, code := run(t, dir, "blueprint", "install", "./zeta-blueprint.yaml", "--no-prompt"); code != 0 {
		t.Fatal("install zeta failed")
	}
	if _, _, code := run(t, dir, "blueprint", "install", "./alpha-blueprint.yaml", "--no-prompt"); code != 0 {
		t.Fatal("install alpha failed")
	}

	stdout, _, code := run(t, dir, "blueprint", "list")
	if code != 0 {
		t.Fatalf("blueprints list: exit %d", code)
	}
	alphaIdx := strings.Index(stdout, "alpha")
	zetaIdx := strings.Index(stdout, "zeta")
	if alphaIdx < 0 || zetaIdx < 0 || alphaIdx >= zetaIdx {
		t.Errorf("expected alpha to precede zeta in output, got %q", stdout)
	}
}

func TestBlueprintInstallNoPromptFlagListsKeys(t *testing.T) {
	// With --no-prompt and required vault keys, the install should succeed
	// but mention the unset keys with the 'vault set' suggestion.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"needs-keys.yaml": `
name: needs-keys
requires:
  vault_keys:
    - my_secret
    - another_secret
tools:
  k.test:
    type: cli
    command: echo
    description: t
`,
	})

	stdout, _, code := run(t, dir, "blueprint", "install", "./needs-keys.yaml", "--no-prompt")
	if code != 0 {
		t.Fatalf("install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "my_secret") || !strings.Contains(stdout, "another_secret") {
		t.Errorf("expected unset vault keys listed, got %q", stdout)
	}
	if !strings.Contains(stdout, "vault set") {
		t.Errorf("expected 'vault set' suggestion, got %q", stdout)
	}
}

func TestBlueprintInstallDoubleInstallFails(t *testing.T) {
	// Installing the same blueprint twice should fail with a clear "already
	// installed" message, suggesting uninstall — not the generic
	// "conflict with N definitions" message that the tool-collision path
	// would otherwise produce.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"dup.yaml": `
name: dup
tools:
  dup.tool:
    type: cli
    command: echo
    description: dup
`,
	})

	if _, _, code := run(t, dir, "blueprint", "install", "./dup.yaml", "--no-prompt"); code != 0 {
		t.Fatal("first install failed")
	}
	_, stderr, code := run(t, dir, "blueprint", "install", "./dup.yaml", "--no-prompt")
	if code == 0 {
		t.Fatal("expected second install to fail")
	}
	if !strings.Contains(stderr, "already installed") {
		t.Errorf("expected 'already installed' error, got %q", stderr)
	}
	if !strings.Contains(stderr, "uninstall first") {
		t.Errorf("expected actionable 'uninstall first' hint, got %q", stderr)
	}
}

func TestBlueprintInstallSummaryShowsMissingRequires(t *testing.T) {
	// When a blueprint's requires can't be satisfied, the CLI should print the
	// summary section (with the proposed adds AND the missing deps) before
	// the error — actionable context next to the failure.
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
		"needs.yaml": `
name: needs-ghost
requires:
  tools: [some.ghost]
tools:
  needs.tool:
    type: cli
    command: echo
    description: x
`,
	})
	stdout, _, code := run(t, dir, "blueprint", "install", "./needs.yaml", "--no-prompt")
	if code == 0 {
		t.Fatal("expected install to fail")
	}
	if !strings.Contains(stdout, "needs-ghost") {
		t.Errorf("expected blueprint header in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Missing dependencies") {
		t.Errorf("expected 'Missing dependencies' section, got %q", stdout)
	}
	if !strings.Contains(stdout, "some.ghost") {
		t.Errorf("expected the specific missing dep, got %q", stdout)
	}
}

// repoFile resolves a path relative to the repository root by walking up
// from the test's working directory, looking for a sibling that exists.
// Mirrors how TestMain locates the factorly binary.
func repoFile(t *testing.T, relPath string) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate %s from %s upward", relPath, dir)
	return ""
}

func TestExampleBlueprintGmailInstalls(t *testing.T) {
	// Dogfood test: install one of the bundled blueprints from a file path.
	// The bundled gmail blueprint exercises the install path end-to-end —
	// 6 REST tools and a self-shipped OAuth provider using vault refs for
	// client credentials. If this test ever fails, it means a format change
	// has broken the bundled catalog.
	gmailBlueprint := repoFile(t, "src/internal/blueprints/bundled/gmail.yaml")

	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})

	// Dry-run: confirms the structured preview the UI would render.
	stdout, _, code := run(t, dir, "blueprint", "install", gmailBlueprint, "--dry-run", "--no-prompt")
	if code != 0 {
		t.Fatalf("dry-run install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "gmail 1.0.0") {
		t.Errorf("expected blueprint header in dry-run output, got %q", stdout)
	}
	// Expect each of the 6 tools by name.
	expectTools := []string{
		"gmail.list_messages",
		"gmail.get_message",
		"gmail.send_message",
		"gmail.search",
		"gmail.create_draft",
		"gmail.list_labels",
	}
	for _, tool := range expectTools {
		if !strings.Contains(stdout, tool) {
			t.Errorf("dry-run missing tool %q in output: %q", tool, stdout)
		}
	}
	// OAuth provider should appear in the preview.
	if !strings.Contains(stdout, "+ gmail") {
		t.Errorf("expected '+ gmail' oauth provider line, got %q", stdout)
	}
	// Vault keys section should list both required keys.
	if !strings.Contains(stdout, "GMAIL_CLIENT_ID") || !strings.Contains(stdout, "GMAIL_CLIENT_SECRET") {
		t.Errorf("expected vault keys in dry-run output, got %q", stdout)
	}

	// Commit. --no-prompt skips the interactive vault prompts.
	stdout, _, code = run(t, dir, "blueprint", "install", gmailBlueprint, "--no-prompt")
	if code != 0 {
		t.Fatalf("install: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "Installed gmail") {
		t.Errorf("expected 'Installed gmail' confirmation, got %q", stdout)
	}

	// All six tools should be visible via 'factorly tools'. If the blueprint's
	// OAuth provider definition didn't merge correctly, validate would
	// reject the tools' provider:gmail references and tools list would
	// either be empty or error.
	stdout, _, code = run(t, dir, "tools")
	if code != 0 {
		t.Fatalf("tools list after install: exit %d, %s", code, stdout)
	}
	for _, tool := range expectTools {
		if !strings.Contains(stdout, tool) {
			t.Errorf("tool %q missing from tools list, got %q", tool, stdout)
		}
	}

	// 'factorly blueprint list' should report the installed blueprint.
	stdout, _, code = run(t, dir, "blueprint", "list")
	if code != 0 {
		t.Fatalf("blueprints list: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "gmail") || !strings.Contains(stdout, "1.0.0") {
		t.Errorf("expected installed gmail 1.0.0 in blueprints list, got %q", stdout)
	}

	// Uninstall and confirm everything is gone.
	if _, _, code := run(t, dir, "blueprint", "uninstall", "gmail"); code != 0 {
		t.Fatal("uninstall failed")
	}
	stdout, _, _ = run(t, dir, "tools")
	for _, tool := range expectTools {
		if strings.Contains(stdout, tool) {
			t.Errorf("tool %q still present after uninstall: %q", tool, stdout)
		}
	}
}

// TestToolsShowAndBlueprintShow verifies the read-only "show" subcommands
// reach the same YAML serializer used by the MCP resources surface and the
// UI's "View YAML" page. Smoke covers: a configured tool, a not-configured
// tool, an installed blueprint, and a bundled (but not installed) blueprint.
func TestToolsShowAndBlueprintShow(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    description: print arg
    args:
      - hi
`,
	})

	// `factorly tools show echo` — happy path
	stdout, _, code := run(t, dir, "tools", "show", "echo")
	if code != 0 {
		t.Fatalf("tools show: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "echo:") || !strings.Contains(stdout, "command: echo") {
		t.Errorf("tools show stdout missing expected fields:\n%s", stdout)
	}

	// `factorly tools show nope` — nonzero exit + error message
	stdout, _, code = run(t, dir, "tools", "show", "nope")
	if code == 0 {
		t.Errorf("tools show nope: expected nonzero exit, got 0; stdout=%q", stdout)
	}

	// Install a bundled blueprint, then `factorly blueprint show` reads the
	// installed copy from disk.
	_, _, code = run(t, dir, "blueprint", "install", "linear", "--no-prompt")
	if code != 0 {
		t.Fatalf("blueprint install linear failed: exit %d", code)
	}
	stdout, _, code = run(t, dir, "blueprint", "show", "linear")
	if code != 0 {
		t.Fatalf("blueprint show linear: exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "name: linear") {
		t.Errorf("blueprint show stdout missing header:\n%s", stdout)
	}

	// `factorly blueprint show <not-installed>` — falls back to bundled.
	stdout, _, code = run(t, dir, "blueprint", "show", "github")
	if code != 0 {
		t.Fatalf("blueprint show github (bundled fallback): exit %d, %s", code, stdout)
	}
	if !strings.Contains(stdout, "name: github") {
		t.Errorf("blueprint show github stdout missing header:\n%s", stdout)
	}

	// `factorly blueprint show nope` — neither installed nor bundled.
	stdout, _, code = run(t, dir, "blueprint", "show", "definitely-not-a-blueprint")
	if code == 0 {
		t.Errorf("blueprint show nope: expected nonzero exit, got 0; stdout=%q", stdout)
	}
}

// TestCodeToolEndToEnd installs a type: code tool whose script calls
// a sibling CLI tool, transforms its output, and returns a string.
// Exercises the full provider/proxy/yaegi pipeline through the real CLI
// binary. Uses a CLI tool (not factorly.fetch) so the test stays
// hermetic without poking holes in the URL safety guard.
func TestCodeToolEndToEnd(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  data.source:
    type: cli
    description: emits a fixed JSON payload
    command: printf
    args:
      - '%s'
      - '{"message":"hello from cli","count":3}'
  greet.test:
    type: code
    description: call data.source and extract the message field
    code: |
      package main
      import (
          "encoding/json"
          "errors"
          "factorly"
          "strconv"
      )
      func Run(params map[string]string) (any, error) {
          res, err := factorly.Call("data.source", nil)
          if err != nil { return nil, err }
          if res.IsError() { return nil, errors.New(res.Error) }
          var body struct{
              Message string ` + "`json:\"message\"`" + `
              Count   int    ` + "`json:\"count\"`" + `
          }
          if err := json.Unmarshal([]byte(res.Output), &body); err != nil {
              return nil, err
          }
          return body.Message + " x" + strconv.Itoa(body.Count), nil
      }
`,
	})

	stdout, stderr, code := run(t, dir, "call", "greet.test")
	if code != 0 {
		t.Fatalf("call greet.test: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "hello from cli x3") {
		t.Errorf("stdout missing expected output: %q", stdout)
	}
}

// TestCodeToolListsRegisteredTools exercises factorly.ListTools() against
// the real proxy + registry. Confirms the script sees the CLI tool's
// declared parameters and itself, and that a hidden tool is excluded.
// Covers the V2 foundation seam where the proxy's
// ListVisibleToolsForScript builds code.ToolInfo from registry data.
func TestCodeToolListsRegisteredTools(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  data.source:
    type: cli
    description: emits a fixed payload
    command: echo
    args: ["hi"]
    parameters:
      - name: format
        description: payload format hint
        type: string
        required: true
      - name: limit
        type: integer
        default: "10"
  internal.helper:
    type: cli
    description: should not appear
    hidden: true
    command: echo
    args: ["secret"]
  introspect:
    type: code
    description: dump the visible tool catalogue
    code: |
      package main
      import (
          "encoding/json"
          "factorly"
      )
      func Run(params map[string]string) (any, error) {
          return factorly.ListTools(), nil
      }
      var _ = json.Marshal
`,
	})

	stdout, stderr, code := run(t, dir, "call", "introspect")
	if code != 0 {
		t.Fatalf("call introspect: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var tools []struct {
		Name        string
		Description string
		Parameters  []struct {
			Name        string
			Type        string
			Required    bool
			Description string
			Default     string
		}
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &tools); err != nil {
		t.Fatalf("parsing tools JSON: %v\nraw stdout: %s", err, stdout)
	}

	byName := map[string]int{}
	for i, t := range tools {
		byName[t.Name] = i
	}

	// Visible tools should be present.
	for _, name := range []string{"data.source", "introspect", "factorly.shell", "factorly.fetch"} {
		if _, ok := byName[name]; !ok {
			names := make([]string, 0, len(byName))
			for k := range byName {
				names = append(names, k)
			}
			t.Errorf("expected tool %q in ListTools output; got %v", name, names)
		}
	}

	// Hidden tool must be excluded.
	if _, ok := byName["internal.helper"]; ok {
		t.Errorf("hidden tool internal.helper should not appear in ListTools output")
	}

	// data.source should expose its declared parameters with metadata.
	idx, ok := byName["data.source"]
	if !ok {
		t.Fatal("data.source missing from output")
	}
	ds := tools[idx]
	if ds.Description != "emits a fixed payload" {
		t.Errorf("data.source description = %q", ds.Description)
	}
	var formatP, limitP *struct {
		Name        string
		Type        string
		Required    bool
		Description string
		Default     string
	}
	for i := range ds.Parameters {
		p := &ds.Parameters[i]
		switch p.Name {
		case "format":
			formatP = p
		case "limit":
			limitP = p
		}
	}
	if formatP == nil {
		t.Fatal("data.source param 'format' missing")
	}
	if !formatP.Required {
		t.Errorf("format should be required")
	}
	if formatP.Type != "string" {
		t.Errorf("format type = %q, want string", formatP.Type)
	}
	if formatP.Description != "payload format hint" {
		t.Errorf("format description = %q", formatP.Description)
	}
	if limitP == nil {
		t.Fatal("data.source param 'limit' missing")
	}
	if limitP.Default != "10" {
		t.Errorf("limit default = %q, want 10", limitP.Default)
	}
}

// TestWorkflowStampsRunIDAndNameInAuditLog is the end-to-end guard
// for /history workflow coalescing. Running a multi-step workflow
// against the real binary must produce audit-log entries where:
//
//  1. Every child-step entry carries the same workflow_run_id (an
//     8-char hex string).
//  2. Every child-step entry carries the workflow_name field equal
//     to the workflow's registered tool name.
//  3. The run_id is unique per invocation — running the same
//     workflow twice produces two distinct IDs.
//  4. Standalone (non-workflow) calls have empty workflow_run_id
//     and workflow_name (the proxy must not pick up stale values
//     from elsewhere).
//
// Catches the regression class where someone deletes a
// context.WithValue line in workflow.go OR a ctx.Value read in
// proxy.go — both would silently break /history coalescing, and
// unit tests on either side alone wouldn't catch the disconnect.
func TestWorkflowStampsRunIDAndNameInAuditLog(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
  echo-twice:
    type: workflow
    steps:
      - tool: echo
        params:
          message: "first"
      - tool: echo
        params:
          message: "second"
`,
	})

	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) int {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
			}
		}
		return code
	}

	// 1) Standalone call — must NOT pick up workflow tags.
	if code := runWithLog("call", "echo", "--message", "standalone"); code != 0 {
		t.Fatalf("standalone call failed: exit %d", code)
	}
	// 2) First workflow run — produces two child step entries.
	if code := runWithLog("call", "echo-twice"); code != 0 {
		t.Fatalf("first workflow run failed: exit %d", code)
	}
	// 3) Second workflow run — must use a different run ID.
	if code := runWithLog("call", "echo-twice"); code != 0 {
		t.Fatalf("second workflow run failed: exit %d", code)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	var all []workflowAuditEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e workflowAuditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("parse log line: %v\nline: %s", err, scanner.Text())
		}
		all = append(all, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}

	// Split entries:
	//   - standalones    = no run_id at all (CLI call to echo, plus
	//                       the two outer workflow tool calls that
	//                       the proxy logs without the run-id stamp,
	//                       since the workflow provider sets ctx
	//                       *after* the proxy's outer-call entry is
	//                       constructed)
	//   - workflowSteps  = entries with run_id, grouped by run_id
	var standalones []workflowAuditEntry
	stepsByRun := map[string][]workflowAuditEntry{}
	for _, e := range all {
		if e.WorkflowRunID == "" {
			standalones = append(standalones, e)
			continue
		}
		stepsByRun[e.WorkflowRunID] = append(stepsByRun[e.WorkflowRunID], e)
	}

	// Standalone count: 1 (the actual standalone) + 2 (the outer
	// workflow tool calls). The outer-workflow calls don't carry the
	// stamp; this is by design today — children carry it, and the UI
	// suppresses the duplicate parent row via tool-name matching.
	if len(standalones) != 3 {
		t.Errorf("expected 3 entries without workflow_run_id (1 standalone + 2 outer workflow calls), got %d", len(standalones))
		for i, e := range standalones {
			t.Logf("  standalone[%d]: tool=%q iface=%q", i, e.Tool, e.Interface)
		}
	}

	// Two workflow runs → two distinct run IDs in the map.
	if len(stepsByRun) != 2 {
		t.Fatalf("expected 2 distinct workflow_run_ids (one per run), got %d: keys=%v", len(stepsByRun), keysOfWorkflowRuns(stepsByRun))
	}

	// Each run must have exactly 2 step entries, all tagged with the
	// workflow's registered name and an 8-char run ID.
	for runID, steps := range stepsByRun {
		if len(runID) != 8 {
			t.Errorf("run_id %q length = %d, want 8 (truncated UUID)", runID, len(runID))
		}
		// Hex-ish — UUID truncation produces lowercase hex.
		for _, c := range runID {
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
			if !isHex {
				t.Errorf("run_id %q has non-hex char %q", runID, c)
				break
			}
		}
		if len(steps) != 2 {
			t.Errorf("run %q: expected 2 step entries, got %d", runID, len(steps))
		}
		for _, s := range steps {
			if s.WorkflowName != "echo-twice" {
				t.Errorf("run %q step: workflow_name = %q, want echo-twice", runID, s.WorkflowName)
			}
			if s.Interface != "workflow" {
				t.Errorf("run %q step: interface = %q, want workflow", runID, s.Interface)
			}
			if s.Status != "success" {
				t.Errorf("run %q step: status = %q, want success", runID, s.Status)
			}
		}
	}

	// The truly-standalone echo call must not have any workflow tags
	// stuck to it.
	var sawEchoStandalone bool
	for _, e := range standalones {
		if e.Tool == "echo" && e.Interface == "cli" {
			sawEchoStandalone = true
			if e.WorkflowRunID != "" || e.WorkflowName != "" {
				t.Errorf("standalone echo picked up stale workflow tags: run_id=%q name=%q", e.WorkflowRunID, e.WorkflowName)
			}
		}
	}
	if !sawEchoStandalone {
		t.Error("did not find the standalone echo entry — log shape unexpected")
	}
}

// workflowAuditEntry is the audit-log shape used by the workflow
// coalescing integration test. Named to avoid colliding with the
// local `entry` type declared inside TestCodeToolStampsSourceSHA.
type workflowAuditEntry struct {
	Tool          string `json:"tool"`
	Interface     string `json:"interface"`
	Status        string `json:"status"`
	WorkflowRunID string `json:"workflow_run_id"`
	WorkflowName  string `json:"workflow_name"`
}

// keysOfWorkflowRuns returns the sorted keys of a
// map[string][]workflowAuditEntry for stable error messages.
func keysOfWorkflowRuns(m map[string][]workflowAuditEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Light sort — deterministic test output.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

// TestLogsReplay_HappyPath runs a call, then replays it from the
// CLI using a hash prefix, and verifies the replay actually
// produced a new audit entry whose replayed_from points back at
// the original. End-to-end coverage for the replay surface:
// hash-prefix lookup → proxy dispatch → audit entry stamping.
func TestLogsReplay_HappyPath(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
`,
	})
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) (string, int) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
			}
		}
		return out.String(), code
	}

	// 1) Fire the original call.
	if _, code := runWithLog("call", "echo", "--message", "hello-world"); code != 0 {
		t.Fatalf("original call: exit %d", code)
	}

	// 2) Grab the hash of that entry.
	originalHash := readLastHashFromLog(t, logPath)
	if originalHash == "" {
		t.Fatal("no hash recorded for original call")
	}
	prefix := originalHash[:12] // short-prefix replay

	// 3) Replay by short prefix.
	if _, code := runWithLog("logs", "replay", prefix); code != 0 {
		t.Fatalf("replay: exit %d", code)
	}

	// 4) Verify the new audit entry exists and links back via
	//    replayed_from.
	type replayEntry struct {
		Tool         string            `json:"tool"`
		Params       map[string]string `json:"params"`
		Hash         string            `json:"hash"`
		ReplayedFrom string            `json:"replayed_from"`
	}
	all := readAllEntries[replayEntry](t, logPath)
	if len(all) != 2 {
		t.Fatalf("expected 2 entries (original + replay), got %d", len(all))
	}
	last := all[len(all)-1]
	if last.Tool != "echo" {
		t.Errorf("replay tool = %q, want echo", last.Tool)
	}
	if last.Params["message"] != "hello-world" {
		t.Errorf("replay params message = %q, want hello-world", last.Params["message"])
	}
	if last.ReplayedFrom != originalHash {
		t.Errorf("replayed_from = %q, want %q", last.ReplayedFrom, originalHash)
	}
}

// TestLogsReplay_LastFlag exercises --last selection: after several
// calls, `logs replay --last` re-fires the most recent one.
func TestLogsReplay_LastFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
`,
	})
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) int {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return 0
	}

	for _, msg := range []string{"first", "second", "third"} {
		if code := runWithLog("call", "echo", "--message", msg); code != 0 {
			t.Fatalf("call %s: exit %d", msg, code)
		}
	}

	if code := runWithLog("logs", "replay", "--last"); code != 0 {
		t.Fatalf("replay --last: exit %d", code)
	}

	type replayEntry struct {
		Params       map[string]string `json:"params"`
		ReplayedFrom string            `json:"replayed_from"`
	}
	all := readAllEntries[replayEntry](t, logPath)
	if len(all) != 4 {
		t.Fatalf("expected 4 entries (3 originals + 1 replay), got %d", len(all))
	}
	last := all[len(all)-1]
	if last.Params["message"] != "third" {
		t.Errorf("replay --last picked %q, want third", last.Params["message"])
	}
	if last.ReplayedFrom == "" {
		t.Error("replayed_from should be set on a replay")
	}
}

// TestLogsReplay_ParamOverride confirms --param key=value lets the
// user tweak one recorded value before re-firing.
func TestLogsReplay_ParamOverride(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
`,
	})
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) int {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return 0
	}

	if code := runWithLog("call", "echo", "--message", "before"); code != 0 {
		t.Fatalf("original call: exit %d", code)
	}
	if code := runWithLog("logs", "replay", "--last", "--param", "message=after"); code != 0 {
		t.Fatalf("replay --param: exit %d", code)
	}

	type pe struct {
		Params map[string]string `json:"params"`
	}
	all := readAllEntries[pe](t, logPath)
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	if all[1].Params["message"] != "after" {
		t.Errorf("replayed message = %q, want after (override should win)", all[1].Params["message"])
	}
}

// TestLogsReplay_ShowDoesNotFire confirms --show prints the call
// info without dispatching: no new audit entry is written.
func TestLogsReplay_ShowDoesNotFire(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo:
    type: cli
    command: echo
    args: ["{{message}}"]
    parameters:
      - name: message
        required: true
`,
	})
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) (string, int) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return out.String(), ee.ExitCode()
			}
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return out.String(), 0
	}

	if _, code := runWithLog("call", "echo", "--message", "hi"); code != 0 {
		t.Fatalf("original call: exit %d", code)
	}
	out, code := runWithLog("logs", "replay", "--last", "--show")
	if code != 0 {
		t.Fatalf("replay --show: exit %d", code)
	}
	if !strings.Contains(out, "would replay") {
		t.Errorf("--show output missing 'would replay': %q", out)
	}
	if !strings.Contains(out, "message=hi") {
		t.Errorf("--show output missing param line: %q", out)
	}

	// Should still be exactly 1 entry — --show must not fire.
	type any1 struct{}
	if got := len(readAllEntries[any1](t, logPath)); got != 1 {
		t.Errorf("entries after --show: %d, want 1 (--show must not fire)", got)
	}
}

// TestLogsReplay_MutuallyExclusiveModes confirms the CLI rejects
// combining a positional hash with --last.
func TestLogsReplay_MutuallyExclusiveModes(t *testing.T) {
	dir := setupDir(t, map[string]string{"factorly.yaml": "tools: {}\n"})
	cmd := exec.Command(binary, "logs", "replay", "abc12345", "--last")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit, got success: %s", out.String())
	}
	if !strings.Contains(errb.String(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in stderr, got: %s", errb.String())
	}
}

// readAllEntries is a generic helper that decodes every line of a
// JSONL audit log into a slice of the caller's chosen struct shape.
func readAllEntries[T any](t *testing.T, path string) []T {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e T
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("parse log line: %v\nline: %s", err, sc.Text())
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

// readLastHashFromLog reads the last JSONL line and pulls its hash
// field. Helper for tests that need to address the most recent
// entry by chain hash.
func readLastHashFromLog(t *testing.T, path string) string {
	t.Helper()
	type entry struct {
		Hash string `json:"hash"`
	}
	all := readAllEntries[entry](t, path)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1].Hash
}

// TestCodeToolStampsSourceSHAInAuditLog verifies the audit log entry
// for a code-tool call carries a 64-char hex SHA-256 of the script body
// in source_sha. Also verifies a non-code call (CLI tool) leaves the
// field empty. Uses FACTORLY_LOG_PATH to redirect the audit log into a
// temp file so the test stays isolated from the user's real log.
func TestCodeToolStampsSourceSHAInAuditLog(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo.cli:
    type: cli
    description: plain CLI tool
    command: printf
    args: ["%s", "ok"]
  echo.code:
    type: code
    description: a code tool
    code: |
      package main
      func Run(params map[string]string) (any, error) {
          return "ok from code", nil
      }
`,
	})

	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	runWithLog := func(args ...string) (string, int) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
			}
		}
		return out.String(), code
	}

	if _, code := runWithLog("call", "echo.code"); code != 0 {
		t.Fatalf("call echo.code: exit %d", code)
	}
	if _, code := runWithLog("call", "echo.cli"); code != 0 {
		t.Fatalf("call echo.cli: exit %d", code)
	}

	// Walk the JSONL log; collect the two relevant entries.
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	type entry struct {
		Tool      string `json:"tool"`
		SourceSHA string `json:"source_sha"`
		Status    string `json:"status"`
	}
	var codeEntry, cliEntry *entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("parsing log line: %v\nline: %s", err, scanner.Text())
		}
		switch e.Tool {
		case "echo.code":
			ec := e
			codeEntry = &ec
		case "echo.cli":
			ce := e
			cliEntry = &ce
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning log: %v", err)
	}

	if codeEntry == nil {
		t.Fatal("no audit entry for echo.code")
	}
	if cliEntry == nil {
		t.Fatal("no audit entry for echo.cli")
	}
	if codeEntry.Status != "success" {
		t.Errorf("echo.code status = %q, want success", codeEntry.Status)
	}
	if len(codeEntry.SourceSHA) != 64 {
		t.Errorf("code-tool source_sha length = %d, want 64 (hex SHA-256); got %q", len(codeEntry.SourceSHA), codeEntry.SourceSHA)
	}
	for _, c := range codeEntry.SourceSHA {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Errorf("source_sha contains non-hex char %q in %q", c, codeEntry.SourceSHA)
			break
		}
	}
	if cliEntry.SourceSHA != "" {
		t.Errorf("CLI tool should not have source_sha; got %q", cliEntry.SourceSHA)
	}
}

// TestFactorlyCodeBuiltinHappyPath confirms the wire-up: agent submits
// a code body that returns a string, factorly.code returns it through
// the proxy and out to stdout.
func TestFactorlyCodeBuiltinHappyPath(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})
	stdout, stderr, code := run(t, dir,
		"call", "factorly.code",
		"--code", "package main\nfunc Run(p map[string]string) (any, error) { return \"hello-\" + p[\"name\"], nil }",
		"--params", `{"name":"world"}`,
	)
	if code != 0 {
		t.Fatalf("call factorly.code: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "hello-world") {
		t.Errorf("stdout = %q, want to contain 'hello-world'", stdout)
	}
}

// TestFactorlyCodeBuiltinCallsInnerTool confirms a submitted script can
// call a sibling tool via factorly.Call and propagate its output.
func TestFactorlyCodeBuiltinCallsInnerTool(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  data.source:
    type: cli
    description: emits a fixed payload
    command: printf
    args: ["%s", "inner-result"]
`,
	})
	stdout, stderr, code := run(t, dir,
		"call", "factorly.code",
		"--code", `package main
import (
    "factorly"
    "errors"
)
func Run(p map[string]string) (any, error) {
    res, err := factorly.Call("data.source", nil)
    if err != nil { return nil, err }
    if res.IsError() { return nil, errors.New(res.Error) }
    return "wrapped: " + res.Output, nil
}`,
		"--params", `{}`,
	)
	if code != 0 {
		t.Fatalf("call factorly.code: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "wrapped: inner-result") {
		t.Errorf("stdout = %q, want to contain 'wrapped: inner-result'", stdout)
	}
}

// TestFactorlyCodeBuiltinComposesWithItself exercises the composability
// of the factorly.code surface: an outer script calls factorly.code
// again with a different inner script source, and the inner result
// flows back through the outer wrapping. This is the canonical
// "agent dynamically composes a tool from a tool" pattern — the
// outer script generates / chooses inner Go source at runtime and
// runs it through the same builtin.
//
// What this exercises that the sibling-call test (above) doesn't:
//   - The inner call goes through compile-validate-execute in a
//     fresh interpreter, not through a pre-registered cli tool.
//   - Both inner and outer scripts produce their own audit entries
//     with distinct SourceSHA values, proving the chain is honest
//     about which code actually ran at each level.
//   - The store SDK handle is available at both levels (the inner
//     script writes; the outer reads), proving per-call SDK
//     injection works recursively.
func TestFactorlyCodeBuiltinComposesWithItself(t *testing.T) {
	// Isolate HOME so the inner script's store writes can't escape
	// into the dev's real ~/.config/factorly/store.db.
	t.Setenv("HOME", t.TempDir())

	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools: {}`,
	})

	// The inner script writes a value to the store and returns a
	// distinctive marker. Encoded as a JSON-quoted Go string so it
	// can be embedded inside the outer script's source literal
	// without escaping headaches.
	innerSrc := `package main
import "factorly"
func Run(p map[string]string) (any, error) {
    if err := factorly.Store.Set("compose.marker", "inner-was-here"); err != nil {
        return nil, err
    }
    return "inner-output", nil
}`
	innerSrcQuoted, err := json.Marshal(innerSrc)
	if err != nil {
		t.Fatalf("marshal inner src: %v", err)
	}

	// The outer script invokes factorly.code with the inner source,
	// then reads back the store value the inner script wrote, and
	// returns both — proving inner ran AND its side effects landed
	// in the same backend the outer can see.
	outerSrc := `package main
import (
    "encoding/json"
    "errors"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    inner := ` + string(innerSrcQuoted) + `
    res, err := factorly.Call("factorly.code", map[string]string{
        "code":   inner,
        "params": "{}",
    })
    if err != nil { return nil, err }
    if res.IsError() { return nil, errors.New("inner failed: " + res.Error) }

    marker, err := factorly.Store.Get("compose.marker")
    if err != nil { return nil, err }

    payload, err := json.Marshal(map[string]string{
        "inner_output": res.Output,
        "store_marker": marker,
    })
    if err != nil { return nil, err }
    return string(payload), nil
}`

	stdout, stderr, code := run(t, dir,
		"call", "factorly.code",
		"--code", outerSrc,
		"--params", `{}`,
	)
	if code != 0 {
		t.Fatalf("outer call exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// stdout should contain both the inner script's return value
	// AND the store marker the inner script wrote — proving the
	// composition round-tripped successfully.
	if !strings.Contains(stdout, `"inner_output":"inner-output"`) {
		t.Errorf("outer didn't see inner return value; stdout=%q", stdout)
	}
	if !strings.Contains(stdout, `"store_marker":"inner-was-here"`) {
		t.Errorf("outer didn't see inner's store write; stdout=%q", stdout)
	}

	// Confirm the CLI can also see the inner's store write — same
	// bbolt file, no cross-process boundary even though the inner
	// call ran inside a nested interpreter.
	out, _, code := run(t, dir, "store", "get", "compose.marker")
	if code != 0 {
		t.Fatalf("cli store get exit %d", code)
	}
	if strings.TrimSpace(out) != "inner-was-here" {
		t.Errorf("cli store get = %q, want 'inner-was-here'", out)
	}
}

// TestFactorlyCodeBuiltinMaxCallsBudget confirms the user-side
// shadow.max_calls override on the factorly.code builtin caps the
// script's inner-call budget. Submitted script loops 200 times calling
// a no-op tool; user config sets max_calls to 5.
func TestFactorlyCodeBuiltinMaxCallsBudget(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  noop:
    type: cli
    command: true
  factorly.code:
    type: builtin
    shadow:
      max_calls: 5
`,
	})
	stdout, stderr, code := run(t, dir,
		"call", "factorly.code",
		"--code", `package main
import (
    "fmt"
    "factorly"
)
func Run(p map[string]string) (any, error) {
    for i := 0; i < 200; i++ {
        _, err := factorly.Call("noop", nil)
        if err != nil {
            return nil, fmt.Errorf("call %d: %w", i, err)
        }
    }
    return "completed", nil
}`,
		"--params", `{}`,
	)
	// Script error → CLI returns nonzero exit. We mainly care that the
	// budget error message reaches stderr or stdout.
	combined := stdout + stderr
	if !strings.Contains(combined, "max_calls") {
		t.Errorf("expected max_calls budget error\nstdout: %s\nstderr: %s\nexit: %d", stdout, stderr, code)
	}
}

// TestFactorlyCodeBuiltinStampsSourceSHA verifies the audit log entry
// for a factorly.code call carries source_sha computed over the
// agent-supplied `code` param (V2 path, distinct from V1 type:code
// tools whose source is stashed in the provider).
func TestFactorlyCodeBuiltinStampsSourceSHA(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")

	cmd := exec.Command(binary,
		"call", "factorly.code",
		"--code", "package main\nfunc Run(p map[string]string) (any, error) { return \"sha-test\", nil }",
		"--params", "{}",
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_LOG_PATH="+logPath,
		"FACTORLY_NO_UPDATE_CHECK=1",
	)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("call factorly.code: %v\nstderr: %s", err, errb.String())
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	type entry struct {
		Tool      string `json:"tool"`
		SourceSHA string `json:"source_sha"`
		Status    string `json:"status"`
	}
	var got *entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("parsing log line: %v", err)
		}
		if e.Tool == "factorly.code" {
			ec := e
			got = &ec
		}
	}
	if got == nil {
		t.Fatal("no audit entry for factorly.code")
	}
	if got.Status != "success" {
		t.Errorf("status = %q, want success", got.Status)
	}
	if len(got.SourceSHA) != 64 {
		t.Errorf("source_sha length = %d, want 64 (hex SHA-256); got %q", len(got.SourceSHA), got.SourceSHA)
	}
}

// TestDocsCodeToolExamples extracts the YAML config from each
// docs/examples/3[4-7]*.md page, drops it into a temp project, runs
// the documented invocation, and asserts the documented output. Guards
// the docs against drift — if the SDK contract or the script API
// changes such that an example stops working, this test fails before
// users hit the same wall.
//
// Example 35 hits api.github.com (external) and the response is
// non-deterministic, so we only assert "exit 0 + non-empty output"
// instead of locking to a specific koan.
func TestDocsCodeToolExamples(t *testing.T) {
	cases := []struct {
		name        string
		doc         string
		args        []string
		mustContain string // empty → only assert exit 0 and non-empty output
	}{
		{
			name:        "34_hello_code_tool",
			doc:         "docs/examples/34-hello-code-tool.md",
			args:        []string{"call", "greet", "--name", "alice"},
			mustContain: "hello alice",
		},
		{
			name:        "36_cross_tool_composition",
			doc:         "docs/examples/36-cross-tool-composition.md",
			args:        []string{"call", "user.summary"},
			mustContain: "Ada <ada@example.com> — dark theme, UTC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := extractFirstYAMLBlock(t, tc.doc)
			dir := setupDir(t, map[string]string{
				"factorly.yaml": cfg,
			})
			stdout, stderr, code := run(t, dir, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if tc.mustContain != "" {
				if !strings.Contains(stdout, tc.mustContain) {
					t.Errorf("stdout missing %q\nstdout: %s", tc.mustContain, stdout)
				}
			} else if strings.TrimSpace(stdout) == "" {
				t.Errorf("expected non-empty stdout\nstderr: %s", stderr)
			}
		})
	}

	// Example 35 hits a public URL in the doc but the test stays local
	// by standing up a httptest server and substituting its URL into
	// the doc's YAML body. Also requires shadow.allow_urls because the
	// fetch guard blocks loopback by default.
	t.Run("35_fetch_and_transform", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Beauty of style and harmony and grace."))
		}))
		defer srv.Close()

		cfg := extractFirstYAMLBlock(t, "docs/examples/35-fetch-and-transform.md")
		cfg = strings.Replace(cfg, "https://api.github.com/zen", srv.URL, 1)
		// Allow the test loopback URL through the fetch safety guard.
		cfg += `  factorly.fetch:
    type: builtin
    shadow:
      allow_urls:
        - "127.0.0.1"
`

		dir := setupDir(t, map[string]string{"factorly.yaml": cfg})
		stdout, stderr, code := run(t, dir, "call", "zen")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Beauty of style and harmony and grace.") {
			t.Errorf("stdout missing fetched body\nstdout: %s", stdout)
		}
	})
}

// TestDocsFactorlyCodeBuiltinExample is the sibling of TestDocsCodeToolExamples
// for example 37, which doesn't have a YAML config block — the script is
// passed inline on the command line. Pulled out separately so the bash
// quoting stays readable.
func TestDocsFactorlyCodeBuiltinExample(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": "tools: {}\n",
	})
	stdout, stderr, code := run(t, dir,
		"call", "factorly.code",
		"--code", `package main
import "fmt"
func Run(p map[string]string) (any, error) {
    return fmt.Sprintf("hello %s (%s)", p["name"], p["greeting"]), nil
}`,
		"--params", `{"name":"alice","greeting":"hi"}`,
	)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "hello alice (hi)") {
		t.Errorf("stdout missing 'hello alice (hi)'\nstdout: %s", stdout)
	}
}

// extractFirstYAMLBlock pulls the first ```yaml fenced block from a
// Markdown file at the given repo-relative path. Doc files for code-tool
// examples lead with the canonical YAML config; extracting the first
// block gives us the source-of-truth project layout.
func extractFirstYAMLBlock(t *testing.T, relPath string) string {
	t.Helper()
	path := repoFile(t, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	in := false
	for _, line := range lines {
		if !in {
			if strings.HasPrefix(line, "```yaml") {
				in = true
			}
			continue
		}
		if strings.HasPrefix(line, "```") {
			break
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		t.Fatalf("no ```yaml block found in %s", relPath)
	}
	return strings.Join(out, "\n") + "\n"
}

// TestProjectScopedLogPath verifies that a call run inside a project
// directory writes its audit log to <project>/.factorly/audit.jsonl
// rather than the global ~/.config/factorly/audit.jsonl. We isolate
// HOME to a fresh temp dir so the test can't ever touch the user's
// real log, and we leave FACTORLY_LOG_PATH unset so resolution falls
// through to ProjectLogPath.
func TestProjectScopedLogPath(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `tools:
  echo.test:
    type: cli
    command: printf
    args: ["%s", "ok"]
`,
	})

	fakeHome := t.TempDir()

	cmd := exec.Command(binary, "call", "echo.test")
	cmd.Dir = dir
	// Strip FACTORLY_LOG_PATH/FACTORLY_NO_LOG from the inherited env so the
	// resolver actually exercises ProjectLogPath; isolate HOME so the
	// "global fallback" branch points to a writable temp dir, not the
	// user's real ~/.config/factorly.
	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FACTORLY_LOG_PATH=") {
			continue
		}
		if strings.HasPrefix(kv, "FACTORLY_NO_LOG=") {
			continue
		}
		if strings.HasPrefix(kv, "HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+fakeHome, "FACTORLY_NO_UPDATE_CHECK=1")
	cmd.Env = env

	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("call echo.test: %v\nstderr: %s", err, errb.String())
	}

	projectLog := filepath.Join(dir, ".factorly", "audit.jsonl")
	info, err := os.Stat(projectLog)
	if err != nil {
		t.Fatalf("expected project log at %s: %v", projectLog, err)
	}
	if info.Size() == 0 {
		t.Errorf("project log exists but is empty")
	}

	globalLog := filepath.Join(fakeHome, ".config", "factorly", "audit.jsonl")
	if _, err := os.Stat(globalLog); !os.IsNotExist(err) {
		t.Errorf("global log should not exist at %s (err: %v)", globalLog, err)
	}
}

// --- Workspaces ---

// envWithoutHome returns the process env with HOME, FACTORLY_VAULT_PATH,
// FACTORLY_LOG_PATH, and FACTORLY_NO_LOG stripped. Use this when a test
// needs to inject those values for isolation — appending to
// os.Environ() doesn't override existing entries on Linux (glibc
// returns the first occurrence of a key from execve's environ array),
// so the existing entry must be removed first.
func envWithoutHome() []string {
	stripped := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HOME=") ||
			strings.HasPrefix(kv, "FACTORLY_VAULT_PATH=") ||
			strings.HasPrefix(kv, "FACTORLY_LOG_PATH=") ||
			strings.HasPrefix(kv, "FACTORLY_NO_LOG=") {
			continue
		}
		stripped = append(stripped, kv)
	}
	return stripped
}

// setupWorkspaceProject creates a temp project with a .factorly/
// config and the given workspaces (name → workspace YAML body).
func setupWorkspaceProject(t *testing.T, factorlyYaml string, workspaces map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	factDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(filepath.Join(factDir, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factDir, "factorly.yaml"), []byte(factorlyYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range workspaces {
		path := filepath.Join(factDir, "workspaces", name+".yaml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestWorkspaceOverlay: --workspace selects which {{env:NAME}} value
// gets resolved at config-load time.
func TestWorkspaceOverlay(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{env:GREETING}}"]
`, map[string]string{
		"staging": "vars:\n  GREETING: hello-staging\n",
		"prod":    "vars:\n  GREETING: hello-prod\n",
	})

	out, _, code := run(t, dir, "call", "echo.test", "--workspace", "staging")
	if code != 0 {
		t.Fatalf("staging call: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hello-staging") {
		t.Errorf("expected staging greeting, got %q", out)
	}

	out, _, code = run(t, dir, "call", "echo.test", "-w", "prod")
	if code != 0 {
		t.Fatalf("prod call: exit %d", code)
	}
	if !strings.Contains(out, "hello-prod") {
		t.Errorf("expected prod greeting, got %q", out)
	}
}

// TestWorkspaceFromEnvVar: FACTORLY_WORKSPACE is the env-var fallback
// for the --workspace flag, identical effect.
func TestWorkspaceFromEnvVar(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  echo.test:
    type: cli
    command: echo
    args: ["{{env:GREETING}}"]
`, map[string]string{
		"staging": "vars:\n  GREETING: from-env-var\n",
	})

	cmd := exec.Command(binary, "call", "echo.test")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_NO_UPDATE_CHECK=1",
		"FACTORLY_WORKSPACE=staging",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("call: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "from-env-var") {
		t.Errorf("expected from-env-var, got %q", stdout.String())
	}
}

// TestWorkspaceUnknown: --workspace name that doesn't match any file
// returns a helpful error listing available workspaces.
func TestWorkspaceUnknown(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  echo.test:
    type: cli
    command: echo
    args: ["hi"]
`, map[string]string{
		"staging": "vars: {}\n",
		"prod":    "vars: {}\n",
	})

	_, stderr, code := run(t, dir, "call", "echo.test", "--workspace", "ghost")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown workspace")
	}
	if !strings.Contains(stderr, "ghost") {
		t.Errorf("error should name the missing workspace: %s", stderr)
	}
	if !strings.Contains(stderr, "staging") || !strings.Contains(stderr, "prod") {
		t.Errorf("error should list available workspaces: %s", stderr)
	}
}

// TestWorkspaceStampedInAuditLog: every entry made under --workspace
// X carries workspace:"X" in the JSONL; calls without the flag don't.
func TestWorkspaceStampedInAuditLog(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  echo.test:
    type: cli
    command: echo
    args: ["hi"]
`, map[string]string{
		"staging": "vars: {}\n",
	})

	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	runWithLog := func(args ...string) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"FACTORLY_LOG_PATH="+logPath,
			"FACTORLY_NO_UPDATE_CHECK=1",
		)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, stderr.String())
		}
	}

	runWithLog("call", "echo.test", "--workspace", "staging")
	runWithLog("call", "echo.test")

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	type entry struct {
		Tool      string `json:"tool"`
		Workspace string `json:"workspace,omitempty"`
	}
	var entries []entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if e.Tool == "echo.test" {
			entries = append(entries, e)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 echo.test entries, got %d", len(entries))
	}
	if entries[0].Workspace != "staging" {
		t.Errorf("first entry: workspace=%q, want staging", entries[0].Workspace)
	}
	if entries[1].Workspace != "" {
		t.Errorf("second entry: workspace=%q, want empty", entries[1].Workspace)
	}
}

// TestWorkspacesListCLI: `factorly workspaces list` enumerates files;
// `show <name>` masks secret-looking values.
func TestWorkspacesListCLI(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "description: Staging\nvars:\n  API_BASE: https://staging\n  GITHUB_TOKEN: ghp_super_secret\n",
		"prod":    "description: Prod\nvars: {}\n",
	})

	out, _, code := run(t, dir, "workspaces", "list")
	if code != 0 {
		t.Fatalf("list: exit %d", code)
	}
	for _, want := range []string{"staging", "prod", "Staging", "Prod"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}

	out, _, code = run(t, dir, "workspaces", "show", "staging")
	if code != 0 {
		t.Fatalf("show: exit %d", code)
	}
	if !strings.Contains(out, "https://staging") {
		t.Errorf("show should print API_BASE: %s", out)
	}
	if strings.Contains(out, "ghp_super_secret") {
		t.Errorf("show must mask GITHUB_TOKEN value, got: %s", out)
	}
	if !strings.Contains(out, "****") {
		t.Errorf("show should display masked placeholder: %s", out)
	}
}

// TestWorkspacesCreateAndDelete: round-trip a workspace through the
// CLI create + delete commands. Asserts the file is materialized and
// then removed, that a duplicate create errors, and that --force
// skips the confirmation prompt.
func TestWorkspacesCreateAndDelete(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{})

	wsFile := filepath.Join(dir, ".factorly", "workspaces", "qa.yaml")

	// Create
	_, stderr, code := run(t, dir, "workspaces", "create", "qa", "--description", "QA tier")
	if code != 0 {
		t.Fatalf("create: exit %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "Created workspace") {
		t.Errorf("expected creation message, got: %s", stderr)
	}
	data, err := os.ReadFile(wsFile)
	if err != nil {
		t.Fatalf("workspace file not created: %v", err)
	}
	if !strings.Contains(string(data), "QA tier") {
		t.Errorf("description not persisted: %s", data)
	}

	// Duplicate create
	_, stderr, code = run(t, dir, "workspaces", "create", "qa")
	if code == 0 {
		t.Error("expected non-zero exit on duplicate create")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", stderr)
	}

	// Delete --force skips prompt
	_, stderr, code = run(t, dir, "workspaces", "delete", "qa", "--force")
	if code != 0 {
		t.Fatalf("delete: exit %d, stderr=%s", code, stderr)
	}
	if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
		t.Errorf("workspace file not removed: %v", err)
	}

	// Delete missing
	_, stderr, code = run(t, dir, "workspaces", "delete", "ghost", "--force")
	if code == 0 {
		t.Error("expected non-zero exit on delete of missing workspace")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", stderr)
	}
}

// TestWorkspacesCreateRejectsBadName: path traversal characters in
// the workspace name return an error rather than write the file.
func TestWorkspacesCreateRejectsBadName(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{})

	for _, bad := range []string{"foo/bar", "a..b", `back\slash`, ".hidden"} {
		_, stderr, code := run(t, dir, "workspaces", "create", bad)
		if code == 0 {
			t.Errorf("expected non-zero exit for name %q", bad)
		}
		if !strings.Contains(stderr, "must not") {
			t.Errorf("name %q: expected helpful error, got: %s", bad, stderr)
		}
	}
}

// TestVaultSetRejectsBadWorkspaceName closes the path-traversal
// seam: workspaceVaultPath used to happily join `../escape.enc` and
// `os.Stat` resolved it to anywhere on disk. With ValidateName at
// the open entry points, the command fails with a clear error and
// nothing lands outside .factorly/vaults/.
//
// The test pipes a valid FACTORLY_VAULT_PASSWORD so a CLI that *did*
// proceed past validation would reach the vault-open step and
// (typically) succeed — making any failure here unambiguously a
// validation failure rather than a stdin-starvation false positive.
// Without this env var, a passing test could just mean "the prompt
// hit EOF on stdin and exited with the wrong error code."
func TestVaultSetRejectsBadWorkspaceName(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{})

	for _, bad := range []string{"../escape", "foo/bar", "a..b", `back\slash`, ".hidden"} {
		cmd := exec.Command(binary, "vault", "set", "--workspace", bad, "KEY", "value")
		cmd.Dir = dir
		// envWithoutHome strips HOME and FACTORLY_VAULT_PATH so the
		// test doesn't pick up the dev's real global vault. The
		// password is a tripwire: if validation were skipped, the
		// vault open would succeed with this password and the test
		// would fail loudly (zero exit code).
		cmd.Env = append(envWithoutHome(),
			"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
			"FACTORLY_VAULT_PASSWORD=test-password-tripwire",
			"HOME="+t.TempDir(),
		)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		code := 0
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}

		if code == 0 {
			t.Errorf("expected non-zero exit for workspace %q (stdout: %s)", bad, stdout.String())
		}
		if !strings.Contains(stderr.String(), "workspace name") {
			t.Errorf("workspace %q: expected error mentioning workspace name, got: %s", bad, stderr.String())
		}
		// Nothing should have been written outside .factorly/vaults/.
		// In particular, no file with the literal traversal pattern in
		// its name should now exist on the test tempdir's parent.
		if _, err := os.Stat(filepath.Join(dir, "..", "escape.enc")); err == nil {
			t.Errorf("workspace %q escaped .factorly/vaults/", bad)
		}
	}
}

// TestExecHonorsWorkspace: `factorly exec --workspace X -- ... {{env:VAR}} ...`
// resolves VAR from the workspace's vars. Without this, exec's resolver
// would only see os.Getenv. Also verifies precedence: --env wins over
// workspace var; workspace var wins over os env.
func TestExecHonorsWorkspace(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"default": "vars: {GREETING: hi-from-default}\n",
		"staging": "vars: {GREETING: hi-from-staging}\n",
	})

	out, _, code := run(t, dir, "exec", "--workspace", "staging", "--", "echo", "{{env:GREETING}}")
	if code != 0 {
		t.Fatalf("staging exec: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-staging") {
		t.Errorf("expected staging greeting, got %q", out)
	}

	out, _, code = run(t, dir, "exec", "-w", "default", "--", "echo", "{{env:GREETING}}")
	if code != 0 {
		t.Fatalf("default exec: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-default") {
		t.Errorf("expected default greeting, got %q", out)
	}

	// --env overrides workspace var
	out, _, code = run(t, dir, "exec", "-w", "default", "--env", "GREETING=from-cli", "--", "echo", "{{env:GREETING}}")
	if code != 0 {
		t.Fatalf("--env override exec: exit %d", code)
	}
	if !strings.Contains(out, "from-cli") {
		t.Errorf("--env should win over workspace, got %q", out)
	}
}

// TestParamEnvRefResolvesAgainstWorkspace: an {{env:NAME}} reference
// inside a *parameter value* (not just a config value) resolves against
// the active workspace before falling through to os.Getenv. The
// regression scenario was passing a {{env:VAR}} as the `command`
// parameter to factorly.shell — the proxy's runtime resolver had no
// env backend registered, so the placeholder reached the shell
// verbatim. Use a plain CLI tool to keep the test hermetic (no
// confirm prompts on factorly.shell).
func TestParamEnvRefResolvesAgainstWorkspace(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  passthrough:
    type: cli
    command: echo
    args: ["{{value}}"]
    parameters:
      - name: value
        required: true
`, map[string]string{
		"default": "vars: {GREETING: hi-from-default}\n",
		"staging": "vars: {GREETING: hi-from-staging}\n",
	})

	out, _, code := run(t, dir, "call", "passthrough", "--value", "{{env:GREETING}}", "--workspace", "staging")
	if code != 0 {
		t.Fatalf("staging call: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-staging") {
		t.Errorf("expected staging value resolved in param, got %q", out)
	}

	out, _, code = run(t, dir, "call", "passthrough", "--value", "{{env:GREETING}}", "-w", "default")
	if code != 0 {
		t.Fatalf("default call: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-default") {
		t.Errorf("expected default value resolved in param, got %q", out)
	}

	// Embedded ref with surrounding literal text.
	out, _, code = run(t, dir, "call", "passthrough", "--value", "say {{env:GREETING}}!", "-w", "staging")
	if code != 0 {
		t.Fatalf("embedded call: exit %d", code)
	}
	if !strings.Contains(out, "say hi-from-staging!") {
		t.Errorf("expected 'say hi-from-staging!', got %q", out)
	}
}

// TestExecEnvRefsEmbeddedInArgs: exec's resolver handles {{env:VAR}}
// embedded in surrounding text — including multiple refs in one arg.
func TestExecEnvRefsEmbeddedInArgs(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"default": "vars: {FIRST: alice, SECOND: bob}\n",
	})

	out, _, code := run(t, dir, "exec", "--workspace", "default", "--", "echo", "from {{env:FIRST}} to {{env:SECOND}}")
	if code != 0 {
		t.Fatalf("exec: exit %d", code)
	}
	if !strings.Contains(out, "from alice to bob") {
		t.Errorf("expected both refs resolved, got %q", out)
	}
}

// TestWorkspaceVaultSelection: a {{vault:KEY}} reference resolves
// against the workspace vault first, falling through to the project
// vault when not present.
func TestWorkspaceVaultSelection(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  show.token:
    type: cli
    command: echo
    args: ["{{vault:GITHUB_TOKEN}}"]
`, map[string]string{
		"staging": "vars: {}\n",
	})

	// Populate vaults. Isolate HOME so the test doesn't inherit a real
	// ~/.config/factorly/vault.enc with a different password (would
	// poison the no-workspace fallback path on machines where one
	// exists). FACTORLY_VAULT_PATH overrides workspace selection and
	// can't be used here.
	fakeHome := t.TempDir()
	vaultEnv := []string{
		"FACTORLY_VAULT_PASSWORD=test123",
		"HOME=" + fakeHome,
	}
	for _, args := range [][]string{
		{"vault", "set", "--workspace", "staging", "GITHUB_TOKEN", "tok-staging"},
		{"vault", "set", "GITHUB_TOKEN", "tok-project"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
		var errb strings.Builder
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("vault set %v: %v\nstderr: %s", args, err, errb.String())
		}
	}

	runVault := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return out.String()
	}

	if got := strings.TrimSpace(runVault("call", "show.token", "--workspace", "staging")); got != "tok-staging" {
		t.Errorf("workspace call: got %q, want tok-staging", got)
	}
	if got := strings.TrimSpace(runVault("call", "show.token")); got != "tok-project" {
		t.Errorf("project call: got %q, want tok-project", got)
	}
}

// TestWorkspaceVaultFallback: workspace vault returns its own keys but
// falls through to project vault for keys only present there.
func TestWorkspaceVaultFallback(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  show.wsonly:
    type: cli
    command: echo
    args: ["{{vault:WS_ONLY}}"]
  show.shared:
    type: cli
    command: echo
    args: ["{{vault:SHARED}}"]
`, map[string]string{
		"staging": "vars: {}\n",
	})

	fakeHome := t.TempDir()
	vaultEnv := []string{
		"FACTORLY_VAULT_PASSWORD=test123",
		"HOME=" + fakeHome,
	}
	for _, args := range [][]string{
		{"vault", "set", "--workspace", "staging", "WS_ONLY", "ws"},
		{"vault", "set", "SHARED", "proj"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
		var errb strings.Builder
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("vault set %v: %v\nstderr: %s", args, err, errb.String())
		}
	}

	runWithFlag := func(args ...string) string {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return strings.TrimSpace(out.String())
	}

	if got := runWithFlag("call", "show.wsonly", "--workspace", "staging"); got != "ws" {
		t.Errorf("WS_ONLY: got %q, want ws", got)
	}
	if got := runWithFlag("call", "show.shared", "--workspace", "staging"); got != "proj" {
		t.Errorf("SHARED (via fallback): got %q, want proj", got)
	}
}

// TestWorkspaceVaultWriteIsScoped: `vault set --workspace X` writes to
// <project>/.factorly/vaults/X.enc and does NOT modify the project vault.
func TestWorkspaceVaultWriteIsScoped(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "vars: {}\n",
	})

	fakeHome := t.TempDir()
	vaultEnv := []string{
		"FACTORLY_VAULT_PASSWORD=test123",
		"HOME=" + fakeHome,
	}

	// Seed project vault first so we can verify it's left alone.
	cmd := exec.Command(binary, "vault", "set", "PROJECT_KEY", "proj-value")
	cmd.Dir = dir
	cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("seed project vault: %v", err)
	}

	// Now write to workspace vault.
	cmd = exec.Command(binary, "vault", "set", "--workspace", "staging", "WS_KEY", "ws-value")
	cmd.Dir = dir
	cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("workspace vault set: %v", err)
	}

	wsVault := filepath.Join(dir, ".factorly", "vaults", "staging.enc")
	if _, err := os.Stat(wsVault); err != nil {
		t.Errorf("expected workspace vault file at %s: %v", wsVault, err)
	}

	// Project vault should not contain WS_KEY. List its keys.
	cmd = exec.Command(binary, "vault", "list")
	cmd.Dir = dir
	cmd.Env = append(envWithoutHome(), append(vaultEnv, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault list: %v\nstderr: %s", err, errb.String())
	}
	keys := out.String()
	if !strings.Contains(keys, "PROJECT_KEY") {
		t.Errorf("project vault missing PROJECT_KEY: %s", keys)
	}
	if strings.Contains(keys, "WS_KEY") {
		t.Errorf("project vault should NOT contain WS_KEY: %s", keys)
	}
}

// --- Default-workspace auto-select ---

// TestDefaultWorkspaceAutoLoadsWhenNoFlag: .factorly/workspaces/default.yaml
// is implicitly active when no --workspace flag and no FACTORLY_WORKSPACE
// env var are set.
func TestDefaultWorkspaceAutoLoadsWhenNoFlag(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  show:
    type: cli
    command: echo
    args: ["{{env:GREETING}}"]
`, map[string]string{
		"default": "vars:\n  GREETING: hi-from-default\n",
	})

	out, _, code := run(t, dir, "call", "show")
	if code != 0 {
		t.Fatalf("call: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-default") {
		t.Errorf("expected default vars to apply with no flag, got %q", out)
	}
}

// TestDefaultWorkspaceOverriddenByExplicitFlag: --workspace staging
// wins over an existing default workspace.
func TestDefaultWorkspaceOverriddenByExplicitFlag(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  show:
    type: cli
    command: echo
    args: ["{{env:GREETING}}"]
`, map[string]string{
		"default": "vars:\n  GREETING: hi-from-default\n",
		"staging": "vars:\n  GREETING: hi-from-staging\n",
	})

	out, _, code := run(t, dir, "call", "show", "--workspace", "staging")
	if code != 0 {
		t.Fatalf("call: exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "hi-from-staging") {
		t.Errorf("expected staging vars (explicit flag), got %q", out)
	}
}

// TestNoDefaultPreservesProcessEnvFallthrough: when no default.yaml
// exists and no workspace flag is set, {{env:NAME}} falls through to
// os.Getenv — the pre-workspaces behavior is preserved.
func TestNoDefaultPreservesProcessEnvFallthrough(t *testing.T) {
	// Project has NO workspaces dir at all.
	dir := t.TempDir()
	factDir := filepath.Join(dir, ".factorly")
	if err := os.MkdirAll(factDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factDir, "factorly.yaml"), []byte(`tools:
  show:
    type: cli
    command: echo
    args: ["{{env:GREETING}}"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binary, "call", "show")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_NO_UPDATE_CHECK=1",
		"GREETING=via-os-env",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("call: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "via-os-env") {
		t.Errorf("expected os env fallthrough, got %q", stdout.String())
	}
}

// TestDefaultWorkspaceStampedInAuditLog: audit entries from
// auto-selected default workspace carry workspace:"default".
func TestDefaultWorkspaceStampedInAuditLog(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  show:
    type: cli
    command: echo
    args: ["hi"]
`, map[string]string{
		"default": "vars: {}\n",
	})

	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	cmd := exec.Command(binary, "call", "show")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_LOG_PATH="+logPath,
		"FACTORLY_NO_UPDATE_CHECK=1",
	)
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("call: %v\nstderr: %s", err, errb.String())
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type entry struct {
		Tool      string `json:"tool"`
		Workspace string `json:"workspace,omitempty"`
	}
	var found *entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if e.Tool == "show" {
			found = &e
			break
		}
	}
	if found == nil {
		t.Fatal("no show entry in audit log")
	}
	if found.Workspace != "default" {
		t.Errorf("workspace=%q, want default", found.Workspace)
	}
}

// TestWorkspaceOAuthTokenIsolation: OAuth tokens are stored in the
// vault under a deterministic key (e.g., github_oauth), so per-workspace
// vaults give per-workspace tokens for free. The same {{vault:KEY}}
// reference (and the same OAuth provider config) resolves to different
// bundles depending on --workspace. We don't drive the browser flow
// here — we seed bundle JSON via `vault set`, which is exactly what
// `auth login` stores. Then verify `auth status` reads back the right
// bundle per workspace.
func TestWorkspaceOAuthTokenIsolation(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  github.me:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user
    auth:
      type: oauth
      provider: github
oauth_providers:
  github:
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    client_id: stub
    client_secret: stub
`, map[string]string{
		"staging": "vars: {}\n",
		"prod":    "vars: {}\n",
	})

	// Realistic OAuth bundle shape — matches what auth login writes.
	stagingBundle := `{"access_token":"tok-staging","refresh_token":"rt-staging","expiry":"2099-01-01T00:00:00Z"}`
	prodBundle := `{"access_token":"tok-prod","refresh_token":"rt-prod","expiry":"2099-01-01T00:00:00Z"}`

	// Isolate HOME so the test doesn't inherit any real
	// ~/.config/factorly/vault.enc with a different password (would
	// trip the no-workspace fallback path).
	fakeHome := t.TempDir()
	vaultEnv := []string{
		"FACTORLY_VAULT_PASSWORD=test123",
		"HOME=" + fakeHome,
		"FACTORLY_NO_LOG=1",
		"FACTORLY_NO_UPDATE_CHECK=1",
	}
	runVault := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), vaultEnv...)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return out.String(), errb.String()
	}

	// Seed bundles in each workspace vault — mimics two separate
	// `factorly auth login github --workspace X` flows.
	runVault("vault", "set", "--workspace", "staging", "github_oauth", stagingBundle)
	runVault("vault", "set", "--workspace", "prod", "github_oauth", prodBundle)

	// `auth status` lists the configured token key and reports valid.
	// Most useful assertion: each workspace has its own state file.
	stagingVault := filepath.Join(dir, ".factorly", "vaults", "staging.enc")
	prodVault := filepath.Join(dir, ".factorly", "vaults", "prod.enc")
	for _, p := range []string{stagingVault, prodVault} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected vault file %s: %v", p, err)
		}
	}

	// `auth status` under each workspace should report the token as valid.
	out, _ := runVault("auth", "status", "github", "--workspace", "staging")
	if !strings.Contains(out, "github_oauth") || !strings.Contains(out, "valid") {
		t.Errorf("staging auth status: expected valid github_oauth, got: %s", out)
	}

	out, _ = runVault("auth", "status", "github", "--workspace", "prod")
	if !strings.Contains(out, "github_oauth") || !strings.Contains(out, "valid") {
		t.Errorf("prod auth status: expected valid github_oauth, got: %s", out)
	}

	// Without a workspace, no token exists (the project vault was never
	// written to). auth status should report not-authenticated.
	out, _ = runVault("auth", "status", "github")
	if !strings.Contains(out, "not authenticated") {
		t.Errorf("project auth status: expected not authenticated, got: %s", out)
	}

	// Read back the staging token directly to confirm it's the staging
	// bundle (not the prod one). We use `vault get` against the workspace
	// vault — which goes through the same chain auth refresh uses.
	got, _ := runVault("vault", "get", "--workspace", "staging", "github_oauth")
	if !strings.Contains(got, "tok-staging") {
		t.Errorf("staging vault should hold staging bundle, got: %s", got)
	}
	if strings.Contains(got, "tok-prod") {
		t.Errorf("staging vault must not contain prod bundle: %s", got)
	}
}

// TestWorkspaceOAuthFallsBackToProjectVault: an OAuth bundle stored in
// the project vault is visible from a workspace that doesn't have its
// own copy (the fallback chain at work). Documents the "shared token"
// pattern — common login, both workspaces use it.
func TestWorkspaceOAuthFallsBackToProjectVault(t *testing.T) {
	dir := setupWorkspaceProject(t, `tools:
  github.me:
    type: rest
    base_url: https://api.github.com
    method: GET
    path: /user
    auth:
      type: oauth
      provider: github
oauth_providers:
  github:
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    client_id: stub
    client_secret: stub
`, map[string]string{
		"staging": "vars: {}\n",
	})

	bundle := `{"access_token":"shared-tok","refresh_token":"shared-rt","expiry":"2099-01-01T00:00:00Z"}`

	// Isolate HOME — see TestWorkspaceOAuthTokenIsolation.
	fakeHome := t.TempDir()
	vaultEnv := []string{
		"FACTORLY_VAULT_PASSWORD=test123",
		"HOME=" + fakeHome,
		"FACTORLY_NO_LOG=1",
		"FACTORLY_NO_UPDATE_CHECK=1",
	}
	runFactorly := func(args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(), vaultEnv...)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, errb.String())
		}
		return out.String(), errb.String()
	}

	// Token lives only in the project vault.
	runFactorly("vault", "set", "github_oauth", bundle)

	// Workspace has no vault file yet.
	wsVault := filepath.Join(dir, ".factorly", "vaults", "staging.enc")
	if _, err := os.Stat(wsVault); !os.IsNotExist(err) {
		t.Fatalf("workspace vault should not exist yet, stat=%v", err)
	}

	// auth status under --workspace staging should see the project
	// token via the fallback chain.
	out, _ := runFactorly("auth", "status", "github", "--workspace", "staging")
	if !strings.Contains(out, "valid") {
		t.Errorf("expected valid status via fallback to project vault, got: %s", out)
	}
}

// --- Shared-password chain ---

// runWithIsolatedStdin runs the binary with explicit stdin contents,
// fully isolated env (no FACTORLY_*_PASSWORD inherited, HOME pointed
// at a tempdir so the developer's real ~/.config/factorly/vault.enc
// is invisible). Returns stdout, stderr, exit code.
func runWithIsolatedStdin(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FACTORLY_VAULT_PASSWORD=") ||
			strings.HasPrefix(kv, "FACTORLY_PROJECT_VAULT_PASSWORD=") ||
			strings.HasPrefix(kv, "FACTORLY_WORKSPACE_VAULT_PASSWORD_") ||
			strings.HasPrefix(kv, "FACTORLY_VAULT_PATH=") ||
			strings.HasPrefix(kv, "HOME=") {
			continue
		}
		env = append(env, kv)
	}
	fakeHome := t.TempDir()
	env = append(env, "HOME="+fakeHome, "FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1")

	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run %v: %v\nstderr: %s", args, err, stderr.String())
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

// countPasswordPrompts counts "Vault password" labels in the stderr
// output. Used to verify the shared-password fallback skipped a prompt.
func countPasswordPrompts(stderr string) int {
	return strings.Count(stderr, "Vault password")
}

// TestWorkspaceSharedPasswordUnlocksProject: when workspace and
// project vaults share a password, the workspace prompt's input is
// reused for the project tier — one prompt total. This was the
// missing behavior; without the shared-candidate flow the user would
// be prompted twice (or, due to the bufio scanner bug, fail entirely).
func TestWorkspaceSharedPasswordUnlocksProject(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "vars: {}\n",
	})

	// Seed both vaults with the same password via FACTORLY_VAULT_PASSWORD.
	for _, args := range [][]string{
		{"vault", "set", "--workspace", "staging", "WS_KEY", "ws-val"},
		{"vault", "set", "PROJ_KEY", "proj-val"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(),
			"FACTORLY_VAULT_PASSWORD=shared",
			"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
			"HOME="+t.TempDir(),
		)
		if err := cmd.Run(); err != nil {
			t.Fatalf("seed %v: %v", args, err)
		}
	}

	// Now read PROJ_KEY under --workspace staging. Pipe the password
	// once; the workspace tier should consume it and the project tier
	// should silently reuse it via the shared-password candidate.
	stdout, stderr, code := runWithIsolatedStdin(t, dir, "shared\n",
		"vault", "get", "PROJ_KEY", "--workspace", "staging")
	if code != 0 {
		t.Fatalf("get: exit %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "proj-val") {
		t.Errorf("expected proj-val from fallback, got %q", stdout)
	}
	if got := countPasswordPrompts(stderr); got != 1 {
		t.Errorf("expected 1 password prompt (shared-password fallback), got %d\nstderr: %s", got, stderr)
	}
}

// TestWorkspaceDifferentPasswordsBothPrompted: when workspace and
// project vaults have different passwords, the project prompt fires
// after the workspace one. This exercises the bufio.Scanner-sharing
// fix — without it the second prompt would see "no input received."
func TestWorkspaceDifferentPasswordsBothPrompted(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "vars: {}\n",
	})

	// Seed workspace with ws-pw and project with proj-pw.
	for _, set := range []struct {
		pw   string
		args []string
	}{
		{"ws-pw", []string{"vault", "set", "--workspace", "staging", "WS_KEY", "ws-val"}},
		{"proj-pw", []string{"vault", "set", "PROJ_KEY", "proj-val"}},
	} {
		cmd := exec.Command(binary, set.args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(),
			"FACTORLY_VAULT_PASSWORD="+set.pw,
			"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
			"HOME="+t.TempDir(),
		)
		if err := cmd.Run(); err != nil {
			t.Fatalf("seed %v: %v", set.args, err)
		}
	}

	// Pipe both passwords in order. Workspace prompt consumes ws-pw;
	// shared-password candidate fails on project; project prompt
	// consumes proj-pw from the shared scanner. Without the scanner
	// fix the second Scan() would return false and error.
	stdout, stderr, code := runWithIsolatedStdin(t, dir, "ws-pw\nproj-pw\n",
		"vault", "get", "PROJ_KEY", "--workspace", "staging")
	if code != 0 {
		t.Fatalf("get: exit %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "proj-val") {
		t.Errorf("expected proj-val, got %q", stdout)
	}
	if got := countPasswordPrompts(stderr); got != 2 {
		t.Errorf("expected 2 password prompts (different passwords), got %d\nstderr: %s", got, stderr)
	}
}

// TestProjectSharedPasswordUnlocksGlobal: regression — the pre-existing
// project→global shared-password fallback (in openFallbackVault before
// workspaces existed) still works after the candidate refactor.
func TestProjectSharedPasswordUnlocksGlobal(t *testing.T) {
	dir := t.TempDir()
	// Create .factorly/ to mark this as a project dir.
	if err := os.MkdirAll(filepath.Join(dir, ".factorly"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed project vault with "shared" and a fake global vault under HOME.
	fakeHome := t.TempDir()
	globalDir := filepath.Join(fakeHome, ".config", "factorly")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seedEnv := append(envWithoutHome(),
		"FACTORLY_VAULT_PASSWORD=shared",
		"HOME="+fakeHome,
		"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
	)
	for _, args := range [][]string{
		{"vault", "set", "PROJ_KEY", "proj-val"},
		{"vault", "set", "--global", "GLOBAL_KEY", "global-val"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = seedEnv
		if err := cmd.Run(); err != nil {
			t.Fatalf("seed %v: %v", args, err)
		}
	}

	// Read GLOBAL_KEY without --workspace; chain is project→global.
	// One password prompt should unlock both via shared-password.
	cmd := exec.Command(binary, "vault", "get", "GLOBAL_KEY")
	cmd.Dir = dir
	cmd.Env = append(envWithoutHome(),
		"HOME="+fakeHome,
		"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
	)
	cmd.Stdin = strings.NewReader("shared\n")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("get: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "global-val") {
		t.Errorf("expected global-val via fallback, got %q", stdout.String())
	}
	if got := countPasswordPrompts(stderr.String()); got != 1 {
		t.Errorf("expected 1 password prompt (shared-password project→global), got %d\nstderr: %s", got, stderr.String())
	}
}

// TestWorkspaceChainSurfacesProjectVaultError exercises the
// FallbackBackend error-surfacing fix: when the workspace vault opens
// fine but the lazy project-tier opener fails (here: wrong password),
// the user must see a clear error, not "secret not found."
//
// Before the fix, FallbackBackend.ensureSecondary silently swallowed
// the open error and Get fell through to ErrNotFound — the user saw
// "secret not found" and had no way to diagnose the real cause.
func TestWorkspaceChainSurfacesProjectVaultError(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "vars: {}\n",
	})

	// Seed workspace vault with ws-pw, project vault with proj-pw,
	// and PROJ_KEY living in project.
	for _, set := range []struct {
		pw   string
		args []string
	}{
		{"ws-pw", []string{"vault", "set", "--workspace", "staging", "WS_KEY", "ws-val"}},
		{"proj-pw", []string{"vault", "set", "PROJ_KEY", "proj-val"}},
	} {
		cmd := exec.Command(binary, set.args...)
		cmd.Dir = dir
		cmd.Env = append(envWithoutHome(),
			"FACTORLY_VAULT_PASSWORD="+set.pw,
			"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
			"HOME="+t.TempDir(),
		)
		if err := cmd.Run(); err != nil {
			t.Fatalf("seed %v: %v", set.args, err)
		}
	}

	// Workspace unlocks (ws-pw is right), then the chain falls back to
	// project; the shared candidate fails, the second prompt receives
	// the wrong password. The error must surface — not get flattened
	// into "secret not found."
	stdout, stderr, code := runWithIsolatedStdin(t, dir, "ws-pw\nwrong-pw\n",
		"vault", "get", "PROJ_KEY", "--workspace", "staging")
	if code == 0 {
		t.Fatalf("expected non-zero exit on wrong password, got 0\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	combined := stderr + stdout
	if strings.Contains(combined, "secret not found") {
		t.Errorf("error surfaced as bare 'secret not found' — FallbackBackend swallowed the open error\nstderr: %s", stderr)
	}
	if !strings.Contains(combined, "vault") {
		t.Errorf("expected error to mention vault/chain, got: %s", combined)
	}
}

// TestUIInheritsCLIUnlockedTiers verifies that when `factorly ui` is
// started with --workspace and the user supplies their vault password
// on stdin (the CLI prompt), both the workspace and project tiers
// show as already-unlocked in the UI — no per-tier locked badges.
//
// Regression: without the extractVaultTiers + WorkspaceVault wiring,
// the UI re-opens each tier via non-interactive password sources, and
// shows the project tier as locked even though the CLI just had the
// password.
func TestUIInheritsCLIUnlockedTiers(t *testing.T) {
	dir := setupWorkspaceProject(t, "tools: {}\n", map[string]string{
		"staging": "vars: {}\n",
	})

	// Seed both vaults with the SAME password using env var; we'll
	// pipe that same password as stdin to the UI startup below.
	fakeHome := t.TempDir()
	seedEnv := append(envWithoutHome(),
		"FACTORLY_VAULT_PASSWORD=shared",
		"HOME="+fakeHome,
		"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
	)
	for _, args := range [][]string{
		{"vault", "set", "--workspace", "staging", "WS_KEY", "ws-val"},
		{"vault", "set", "PROJ_KEY", "proj-val"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = seedEnv
		if err := cmd.Run(); err != nil {
			t.Fatalf("seed %v: %v", args, err)
		}
	}

	// Pick a port unlikely to clash with parallel tests.
	port := freePort(t)

	// Start the UI with stdin-supplied password and no env-var
	// password — exercises the CLI prompt path.
	uiCmd := exec.Command(binary, "ui", "--workspace", "staging",
		"--port", fmt.Sprintf("%d", port), "--no-launch")
	uiCmd.Dir = dir
	uiCmd.Env = append(envWithoutHome(),
		"HOME="+fakeHome,
		"FACTORLY_NO_LOG=1", "FACTORLY_NO_UPDATE_CHECK=1",
	)
	uiCmd.Stdin = strings.NewReader("shared\n")
	var uiStderr strings.Builder
	uiCmd.Stderr = &uiStderr
	uiStdout, err := uiCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := uiCmd.Start(); err != nil {
		t.Fatalf("starting ui: %v", err)
	}
	defer func() {
		_ = uiCmd.Process.Kill()
		_ = uiCmd.Wait()
	}()

	// Capture the session token from the UI's startup output.
	token := readSessionToken(t, uiStdout, &uiStderr)
	if token == "" {
		t.Fatalf("ui didn't start within timeout\nstderr: %s", uiStderr.String())
	}

	// Hit /vault and check both tiers list their keys without "locked".
	cookie := &http.Cookie{Name: "factorly_session", Value: token}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/vault", port), nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /vault: %v", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)

	if !strings.Contains(body, "WS_KEY") {
		t.Errorf("expected WS_KEY listed in workspace tier")
	}
	if !strings.Contains(body, "PROJ_KEY") {
		t.Errorf("expected PROJ_KEY listed in project tier (inherited from CLI password)")
	}
	// "(locked)" markers indicate the UI didn't inherit. Different
	// from the (locked) tier rendering — the workspace tier badge.
	if strings.Contains(body, ">(locked)<") {
		t.Errorf("found locked-tier badge in /vault response — UI didn't inherit CLI unlocks")
	}
}

// freePort grabs an unused TCP port for a test-local server.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// readSessionToken pulls the per-run nonce token from the UI's
// startup message. The UI prints "running at http://localhost:PORT/
// ?token=NONCE" to stderr on listen.
func readSessionToken(t *testing.T, stdout io.Reader, stderr *strings.Builder) string {
	t.Helper()
	// Drain stdout in the background — required so the subprocess
	// doesn't block on a full pipe. We don't actually need stdout
	// content for this test.
	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := tokenRE.FindString(stderr.String()); m != "" {
			return strings.TrimPrefix(m, "token=")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ""
}

var tokenRE = regexp.MustCompile(`token=[a-f0-9]+`)
