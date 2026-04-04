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
	os.Exit(m.Run())
}

func run(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
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
    args: ["-s", "{url}"]
  file.read:
    type: cli
    description: "Read a file"
    command: cat
    args: ["{path}"]
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
    args: ["{msg}"]
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
    args: ["{first}", "{second}"]
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
    args: ["{msg}"]
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
    args: ["{msg}"]
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

// --- Tool Directory ---

func TestToolsDirLoading(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"factorly.yaml": `
tools_dir: ./tools
tools:
  web.fetch:
    type: cli
    command: curl
    args: ["-s", "{url}"]
`,
		"tools/echo.yaml": `
echo.test:
  type: cli
  command: echo
  args: ["{msg}"]
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
  args: ["{msg}"]
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
  args: ["{msg}"]
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

	stdout, _, code := run(t, "", "import", "openapi", specPath)
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

	_, stderr, code := run(t, "", "import", "openapi", specPath, "--out", outPath)
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

	stdout, _, code := run(t, "", "import", "openapi", specPath, "--prefix", "myapi")
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
    args: ["{msg}"]
`,
	})

	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Import OpenAPI spec into tools dir
	outPath := filepath.Join(toolsDir, "petstore.yaml")
	_, _, code := run(t, dir, "import", "openapi", specPath, "--out", outPath)
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
	stdout, _, code = run(t, dir, "call", "echo.test", "--msg", "pipeline works")
	if code != 0 {
		t.Fatal("call failed")
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
    path: /items/{id}
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

// --- Missing config ---

func TestMissingConfig(t *testing.T) {
	dir := t.TempDir() // empty directory, no factorly.yaml

	_, _, code := run(t, dir, "tools")
	if code == 0 {
		t.Fatal("expected non-zero exit for missing config")
	}
}

// helpers

func findPetstoreSpec(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../examples/petstore.yaml",
		"examples/petstore.yaml",
		"src/examples/petstore.yaml",
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
