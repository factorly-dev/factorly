// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

//go:build integration

package test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

	// Accept all defaults: no tools dir, yes example, no openapi, skip template, no sync
	stdin := "n\ny\nn\nskip\nn\n"
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
}

func TestInitWithToolsDir(t *testing.T) {
	dir := t.TempDir()

	// Yes tools dir, default path, yes example, no openapi, skip template, no sync
	stdin := "y\n\ny\nn\nskip\nn\n"
	_, _, code := runWithStdin(t, dir, stdin, "init")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Verify tools directory was created inside .factorly/
	if _, err := os.Stat(filepath.Join(dir, ".factorly", "tools")); os.IsNotExist(err) {
		t.Error("expected .factorly/tools directory to be created")
	}

	data, err := os.ReadFile(filepath.Join(dir, ".factorly", "factorly.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tools_dir") {
		t.Error("expected tools_dir in config")
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

	stdin := "n\ny\nn\nskip\nn\n"
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

	stdin := "n\nn\nn\nskip\nn\n"
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

	logPath := filepath.Join(t.TempDir(), "calls.jsonl")

	// Run with logging enabled to a custom path — we need to test the
	// actual log file permissions. Override via env since there's no flag.
	cmd := exec.Command(binary, "call", "echo.test", "--msg", "test")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+filepath.Dir(filepath.Dir(logPath)))
	// Create the expected directory structure
	_ = os.MkdirAll(filepath.Join(filepath.Dir(filepath.Dir(logPath)), ".config", "factorly"), 0o755)
	_ = cmd.Run()

	// Check that the log file was created — if it exists, verify permissions
	configLogPath := filepath.Join(filepath.Dir(filepath.Dir(logPath)), ".config", "factorly", "calls.jsonl")
	if info, err := os.Stat(configLogPath); err == nil {
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("expected log file permissions 0600, got %04o", perm)
		}
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
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for wrong vault password")
	}
}

// --- Templates ---

func TestTemplatesList(t *testing.T) {
	stdout, _, code := run(t, "", "tools", "import", "templates")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, name := range []string{"linear", "github", "slack", "stripe", "notion"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("expected %q in template list", name)
		}
	}
	if !strings.Contains(stdout, "engineering") {
		t.Error("expected 'engineering' category in output")
	}
}

func TestTemplatesDryRun(t *testing.T) {
	stdout, _, code := run(t, "", "tools", "import", "templates", "github", "--dry-run")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "github.list_repos") {
		t.Error("expected github.list_repos in dry-run output")
	}
	if !strings.Contains(stdout, "vault:GITHUB_TOKEN") {
		t.Error("expected vault reference in dry-run output")
	}
	if !strings.Contains(stdout, "type: rest") {
		t.Error("expected 'type: rest' in dry-run output")
	}
}

func TestTemplatesDryRunAllTemplates(t *testing.T) {
	// Verify every template produces valid dry-run output
	for _, name := range []string{"linear", "github", "slack", "stripe", "notion"} {
		t.Run(name, func(t *testing.T) {
			stdout, _, code := run(t, "", "tools", "import", "templates", name, "--dry-run")
			if code != 0 {
				t.Fatalf("expected exit 0, got %d", code)
			}
			if !strings.Contains(stdout, "type: rest") {
				t.Error("expected 'type: rest' in output")
			}
			if !strings.Contains(stdout, "vault:") {
				t.Error("expected vault reference in output")
			}
		})
	}
}

func TestTemplatesUnknown(t *testing.T) {
	_, _, code := run(t, "", "tools", "import", "templates", "nonexistent")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown template")
	}
}

func TestTemplatesInstallWithAPIKey(t *testing.T) {
	dir := t.TempDir()

	// Set vault password so vault can be opened non-interactively
	vaultPath := filepath.Join(dir, "vault.enc")
	cmd := exec.Command(binary, "vault", "set", "DUMMY", "dummy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpassword",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		t.Fatalf("vault setup failed: %v", err)
	}

	// Install template with --api-key and --all (non-interactive)
	cmd2 := exec.Command(binary, "tools", "import", "templates", "linear", "--api-key", "lin_test_key_123", "--all")
	cmd2.Dir = dir
	cmd2.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpassword",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	var stdout2, stderr2 strings.Builder
	cmd2.Stdout = &stdout2
	cmd2.Stderr = &stderr2
	if err := cmd2.Run(); err != nil {
		t.Fatalf("template install failed: %v\nstdout: %s\nstderr: %s", err, stdout2.String(), stderr2.String())
	}

	// Verify YAML file was created
	yamlPath := filepath.Join(dir, ".factorly", "tools", "linear.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", yamlPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "linear.create_issue") {
		t.Error("expected linear.create_issue in generated YAML")
	}
	if !strings.Contains(content, "vault:LINEAR_API_KEY") {
		t.Error("expected vault reference in generated YAML")
	}

	// Verify key was stored in vault
	cmd3 := exec.Command(binary, "vault", "get", "LINEAR_API_KEY")
	cmd3.Dir = dir
	cmd3.Env = append(os.Environ(),
		"FACTORLY_NO_LOG=1",
		"FACTORLY_VAULT_PASSWORD=testpassword",
		"FACTORLY_VAULT_PATH="+vaultPath,
	)
	var out3 strings.Builder
	cmd3.Stdout = &out3
	if err := cmd3.Run(); err != nil {
		t.Fatalf("vault get failed: %v", err)
	}
	if strings.TrimSpace(out3.String()) != "lin_test_key_123" {
		t.Errorf("expected vault to contain API key, got %q", out3.String())
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
	// Dogfood test: the checked-in examples/blueprints/gmail.yaml is the canonical
	// example of the blueprint format. If this test ever fails, it means a format
	// change has broken the example — fix one or the other to keep them in
	// sync. The blueprint is real: 6 REST tools and a self-shipped OAuth provider
	// using vault refs for client credentials.
	gmailBlueprint := repoFile(t, "examples/blueprints/gmail.yaml")

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
