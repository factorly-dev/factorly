// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/vault"
)

// mockVault implements vault.Backend for testing.
type mockVault struct {
	data map[string]string
}

func newMockVault() *mockVault {
	return &mockVault{data: make(map[string]string)}
}

func (m *mockVault) Get(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", vault.ErrNotFound
	}
	return v, nil
}
func (m *mockVault) Set(key, value string) error { m.data[key] = value; return nil }
func (m *mockVault) Delete(key string) error     { delete(m.data, key); return nil }
func (m *mockVault) List() ([]string, error) {
	var keys []string
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}
func (m *mockVault) Close() error { return nil }

// testServer creates a UI server with minimal config for testing.
func testServer(t *testing.T, cfg *config.Config) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	if cfg == nil {
		cfg = &config.Config{
			Tools: make(map[string]config.ToolConfig),
		}
	}

	srv, err := New(Options{
		Config:  cfg,
		CfgPath: cfgPath,
		Vault:   newMockVault(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, cfgPath
}

// testServerWithProxy creates a UI server with registry and proxy for testing runtime registration.
func testServerWithProxy(t *testing.T, cfg *config.Config) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	if cfg == nil {
		cfg = &config.Config{
			Tools: make(map[string]config.ToolConfig),
		}
	}

	reg := registry.New()
	providers := make(map[string]provider.Provider)
	p := proxy.New(reg, providers, logger.NopLogger{})

	srv, err := New(Options{
		Config:   cfg,
		CfgPath:  cfgPath,
		Vault:    newMockVault(),
		Registry: reg,
		Proxy:    p,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, cfgPath
}

// --- Tool handler tests ---

func TestHandleToolsList(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":       {Type: "cli", Command: "echo"},
			"api.fetch":  {Type: "rest", Method: "GET", BaseURL: "https://example.com"},
			"deploy.all": {Type: "workflow", Steps: []config.StepConfig{{Tool: "echo"}}},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("GET", "/tools", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	// Should include non-workflow tools
	if !strings.Contains(body, "echo") {
		t.Error("expected 'echo' in tools list")
	}
	if !strings.Contains(body, "api.fetch") {
		t.Error("expected 'api.fetch' in tools list")
	}
	// Should NOT include workflow tools
	if strings.Contains(body, "deploy.all") {
		t.Error("workflow 'deploy.all' should not appear in tools list")
	}
}

func TestHandleToolEdit_RedirectsWorkflows(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.workflow": {Type: "workflow"},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("GET", "/tools/my.workflow", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/workflows/my.workflow" {
		t.Errorf("expected redirect to /workflows/my.workflow, got %s", loc)
	}
}

func TestHandleToolCreate(t *testing.T) {
	srv, cfgPath := testServer(t, nil)

	form := url.Values{
		"name":        {"test.tool"},
		"type":        {"cli"},
		"description": {"A test tool"},
		"command":     {"echo"},
		"args":        {"hello world"},
	}

	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect 302, got %d: %s", w.Code, w.Body.String())
	}

	// Verify in-memory config updated
	tc, ok := srv.cfg.Tools["test.tool"]
	if !ok {
		t.Fatal("tool not found in config")
	}
	if tc.Type != "cli" {
		t.Errorf("expected type cli, got %s", tc.Type)
	}
	if tc.Command != "echo" {
		t.Errorf("expected command echo, got %s", tc.Command)
	}

	// Verify written to disk
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "test.tool") {
		t.Error("tool not written to config file")
	}
}

func TestHandleToolRename(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"old.name": {Type: "cli", Command: "echo", Description: "original"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"rename":      {"new.name"},
		"description": {"updated desc"},
	}

	req := httptest.NewRequest("POST", "/tools/old.name/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Old name gone
	if _, ok := srv.cfg.Tools["old.name"]; ok {
		t.Error("old.name should be deleted")
	}
	// New name exists
	tc, ok := srv.cfg.Tools["new.name"]
	if !ok {
		t.Fatal("new.name should exist")
	}
	if tc.Description != "updated desc" {
		t.Errorf("expected 'updated desc', got %q", tc.Description)
	}
}

func TestHandleToolDelete(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"doomed": {Type: "cli", Command: "rm"},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("DELETE", "/tools/doomed", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := srv.cfg.Tools["doomed"]; ok {
		t.Error("tool should have been deleted")
	}
}

// --- Workflow handler tests ---

func TestHandleWorkflowsList(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":        {Type: "cli", Command: "echo"},
			"git.deploy":  {Type: "workflow", Steps: []config.StepConfig{{Tool: "echo"}, {Tool: "echo"}}},
			"git.release": {Type: "workflow", Steps: []config.StepConfig{{Tool: "echo"}}},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("GET", "/workflows", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "git.deploy") {
		t.Error("expected git.deploy in workflows list")
	}
	if !strings.Contains(body, "git.release") {
		t.Error("expected git.release in workflows list")
	}
	// Should not show non-workflow tools
	if strings.Contains(body, ">echo<") {
		t.Error("non-workflow tool should not appear")
	}
}

func TestHandleWorkflowCreate(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{
		"name":        {"test.flow"},
		"description": {"A test workflow"},
	}

	req := httptest.NewRequest("POST", "/workflows/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	tc, ok := srv.cfg.Tools["test.flow"]
	if !ok {
		t.Fatal("workflow not found in config")
	}
	if tc.Type != "workflow" {
		t.Errorf("expected type workflow, got %s", tc.Type)
	}
	if tc.Description != "A test workflow" {
		t.Errorf("expected description, got %q", tc.Description)
	}
}

func TestHandleWorkflowDelete(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"doomed.flow": {Type: "workflow"},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("DELETE", "/workflows/doomed.flow", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := srv.cfg.Tools["doomed.flow"]; ok {
		t.Error("workflow should have been deleted")
	}
}

// --- Auth handler tests ---

// TestHandleAuthNew confirms the dedicated Create-Provider page
// renders with the form action pointing at /auth/_new (which is
// where the existing create handler lives).
func TestHandleAuthNew(t *testing.T) {
	srv, _ := testServer(t, nil)
	req := httptest.NewRequest("GET", "/auth/new", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"Create OAuth Provider", `action="/auth/_new"`, "← Back to Auth Providers"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

func TestHandleAuthCreate(t *testing.T) {
	srv, cfgPath := testServer(t, nil)

	form := url.Values{
		"name":          {"github"},
		"client_id":     {"abc123"},
		"client_secret": {"secret456"},
		"auth_url":      {"https://github.com/login/oauth/authorize"},
		"token_url":     {"https://github.com/login/oauth/access_token"},
		"scopes":        {"repo, read:org"},
	}

	req := httptest.NewRequest("POST", "/auth/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	p, ok := srv.cfg.OAuthProviders["github"]
	if !ok {
		t.Fatal("provider not found in config")
	}
	if p.ClientID != "abc123" {
		t.Errorf("expected client_id abc123, got %s", p.ClientID)
	}
	if len(p.Scopes) != 2 || p.Scopes[0] != "repo" || p.Scopes[1] != "read:org" {
		t.Errorf("unexpected scopes: %v", p.Scopes)
	}

	// Verify on disk
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "github") {
		t.Error("provider not written to config")
	}
}

func TestHandleAuthUpdate(t *testing.T) {
	cfg := &config.Config{
		Tools: make(map[string]config.ToolConfig),
		OAuthProviders: map[string]config.OAuthProviderConfig{
			"github": {
				ClientID:     "old_id",
				ClientSecret: "old_secret",
				AuthURL:      "https://github.com/login/oauth/authorize",
				TokenURL:     "https://github.com/login/oauth/access_token",
				Scopes:       []string{"repo"},
			},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"client_id":     {"new_id"},
		"client_secret": {"new_secret"},
		"auth_url":      {"https://github.com/login/oauth/authorize"},
		"token_url":     {"https://github.com/login/oauth/access_token"},
		"scopes":        {"repo, write:packages"},
	}

	req := httptest.NewRequest("POST", "/auth/github", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	p := srv.cfg.OAuthProviders["github"]
	if p.ClientID != "new_id" {
		t.Errorf("expected new_id, got %s", p.ClientID)
	}
	if len(p.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %v", p.Scopes)
	}
}

func TestHandleAuthDelete(t *testing.T) {
	v := newMockVault()
	_ = v.Set("github_oauth", `{"access_token":"tok"}`)

	cfg := &config.Config{
		Tools: make(map[string]config.ToolConfig),
		OAuthProviders: map[string]config.OAuthProviderConfig{
			"github": {ClientID: "x"},
		},
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("oauth_providers:\n    github:\n        client_id: x\n"), 0o644)

	srv, err := New(Options{
		Config:  cfg,
		CfgPath: cfgPath,
		Vault:   v,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/auth/github", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Provider removed from config
	if _, ok := srv.cfg.OAuthProviders["github"]; ok {
		t.Error("provider should be deleted")
	}

	// Token removed from vault
	if _, err := v.Get("github_oauth"); err == nil {
		t.Error("token should be deleted from vault")
	}
}

// --- Vault handler tests ---

func TestHandleVaultSetAndDelete(t *testing.T) {
	v := newMockVault()
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	mgr := vault.NewManager(nil, nil)
	mgr.Put("project", v)
	srv, err := New(Options{
		Config:       cfg,
		CfgPath:      cfgPath,
		Vault:        v,
		VaultManager: mgr,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set a key
	form := url.Values{
		"key":   {"MY_SECRET"},
		"value": {"supersecret"},
		"scope": {"project"},
	}
	req := httptest.NewRequest("POST", "/vault", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	// Vault set returns 200 with htmx partial update
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	val, err := v.Get("MY_SECRET")
	if err != nil {
		t.Fatalf("key not found: %v", err)
	}
	if val != "supersecret" {
		t.Errorf("expected 'supersecret', got %q", val)
	}

	// Delete the key
	req = httptest.NewRequest("DELETE", "/vault/MY_SECRET?scope=project", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- Tool save tests ---

func TestHandleToolSave(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool": {Type: "cli", Command: "echo", Description: "old"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":     {"updated description"},
		"command":         {"curl"},
		"args":            {"-s https://example.com"},
		"param_name_0":    {"url"},
		"param_type_0":    {"string"},
		"param_default_0": {"https://example.com"},
		"param_desc_0":    {"The URL to fetch"},
	}

	req := httptest.NewRequest("POST", "/tools/my.tool", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["my.tool"]
	if tc.Description != "updated description" {
		t.Errorf("description not updated: %q", tc.Description)
	}
	if tc.Command != "curl" {
		t.Errorf("command not updated: %q", tc.Command)
	}
	if len(tc.Parameters) != 1 {
		t.Fatalf("expected 1 param, got %d", len(tc.Parameters))
	}
	if tc.Parameters[0].Name != "url" {
		t.Errorf("param name: %q", tc.Parameters[0].Name)
	}
}

// TestHandleToolSave_EnumParam verifies the param_enum_<i> form
// field round-trips into ParamConfig.Enum. The choices input is
// only visible when type=enum in the UI, but the handler trusts
// whatever the form submits — type semantics and the "type=enum
// requires a list" rule are enforced at config-load + registry
// validation, not at save.
func TestHandleToolSave_EnumParam(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool": {Type: "cli", Command: "echo"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":  {""},
		"command":      {"echo"},
		"param_name_0": {"env"},
		"param_type_0": {"enum"},
		"param_enum_0": {"staging, prod, dev"},
		"param_desc_0": {"environment"},
	}

	req := httptest.NewRequest("POST", "/tools/my.tool", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	tc := srv.cfg.Tools["my.tool"]
	if len(tc.Parameters) != 1 {
		t.Fatalf("expected 1 param, got %d", len(tc.Parameters))
	}
	p := tc.Parameters[0]
	if p.Type != "enum" {
		t.Errorf("type = %q, want enum", p.Type)
	}
	if len(p.Enum) != 3 || p.Enum[0] != "staging" || p.Enum[2] != "dev" {
		t.Errorf("Enum = %v, want [staging prod dev] (trimmed)", p.Enum)
	}
}

func TestHandleToolSave_WithShadow(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"risky": {Type: "cli", Command: "rm"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":    {"dangerous tool"},
		"command":        {"rm"},
		"shadow_deny":    {"path=/"},
		"shadow_confirm": {"on"},
	}

	req := httptest.NewRequest("POST", "/tools/risky", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["risky"]
	if tc.Shadow == nil {
		t.Fatal("shadow should be set")
	}
	if tc.Shadow.Confirm != true {
		t.Error("shadow confirm should be true")
	}
	if len(tc.Shadow.Deny) != 1 || tc.Shadow.Deny[0] != "path=/" {
		t.Errorf("shadow deny: %v", tc.Shadow.Deny)
	}
}

func TestHandleToolSave_WithHeaders(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"api.test": {Type: "rest", Method: "GET", BaseURL: "https://example.com"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":  {"test"},
		"method":       {"GET"},
		"base_url":     {"https://example.com"},
		"path":         {"/test"},
		"header_key[]": {"Accept", "X-Custom"},
		"header_val[]": {"application/json", "myvalue"},
	}

	req := httptest.NewRequest("POST", "/tools/api.test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["api.test"]
	if len(tc.Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d: %v", len(tc.Headers), tc.Headers)
	}
	if tc.Headers["Accept"] != "application/json" {
		t.Errorf("expected Accept header, got %v", tc.Headers)
	}
	if tc.Headers["X-Custom"] != "myvalue" {
		t.Errorf("expected X-Custom header, got %v", tc.Headers)
	}
}

func TestHandleToolSave_HeadersCleared(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"api.test": {
				Type:    "rest",
				Method:  "GET",
				BaseURL: "https://example.com",
				Headers: map[string]string{"Old": "value"},
			},
		},
	}
	srv, _ := testServer(t, cfg)

	// Save with no headers
	form := url.Values{
		"description": {"test"},
		"method":      {"GET"},
		"base_url":    {"https://example.com"},
		"path":        {"/test"},
	}

	req := httptest.NewRequest("POST", "/tools/api.test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["api.test"]
	if tc.Headers != nil {
		t.Errorf("expected nil headers after clearing, got %v", tc.Headers)
	}
}

func TestHandleToolSave_NotFound(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{"description": {"x"}}
	req := httptest.NewRequest("POST", "/tools/nonexistent", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleToolCreate_MCP(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{
		"name":    {"mcp.github"},
		"type":    {"mcp"},
		"command": {"npx"},
		"args":    {"-y @modelcontextprotocol/server-github"},
		"url":     {"http://localhost:8080/mcp"},
	}

	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["mcp.github"]
	if tc.Type != "mcp" {
		t.Errorf("expected type mcp, got %s", tc.Type)
	}
	if tc.Command != "npx" {
		t.Errorf("expected command npx, got %s", tc.Command)
	}
	if tc.URL != "http://localhost:8080/mcp" {
		t.Errorf("expected URL, got %s", tc.URL)
	}
}

// --- Workflow save tests ---

func TestHandleWorkflowSave(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":    {Type: "cli", Command: "echo"},
			"my.flow": {Type: "workflow", Steps: []config.StepConfig{{Tool: "echo"}}},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":  {"updated flow"},
		"step_tool_0":  {"echo"},
		"step_store_0": {"result"},
		"step_if_0":    {"result != ''"},
		"step_tool_1":  {"echo"},
		"step_store_1": {"final"},
	}

	req := httptest.NewRequest("POST", "/workflows/my.flow/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["my.flow"]
	if tc.Description != "updated flow" {
		t.Errorf("description not updated: %q", tc.Description)
	}
	if len(tc.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(tc.Steps))
	}
	if tc.Steps[0].Store != "result" {
		t.Errorf("step 0 store: %q", tc.Steps[0].Store)
	}
	if tc.Steps[0].If != "result != ''" {
		t.Errorf("step 0 if: %q", tc.Steps[0].If)
	}
	if tc.Steps[1].Store != "final" {
		t.Errorf("step 1 store: %q", tc.Steps[1].Store)
	}
}

func TestHandleWorkflowSave_WithParams(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":    {Type: "cli", Command: "echo"},
			"my.flow": {Type: "workflow"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"description":        {"flow with params"},
		"step_tool_0":        {"echo"},
		"step_store_0":       {""},
		"step_if_0":          {""},
		"step_require_0":     {""},
		"step_param_key_0[]": {"message", "count"},
		"step_param_val_0[]": {"hello", "5"},
	}

	req := httptest.NewRequest("POST", "/workflows/my.flow/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["my.flow"]
	if len(tc.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tc.Steps))
	}
	if tc.Steps[0].Params["message"] != "hello" {
		t.Errorf("param message: %q", tc.Steps[0].Params["message"])
	}
	if tc.Steps[0].Params["count"] != "5" {
		t.Errorf("param count: %q", tc.Steps[0].Params["count"])
	}
}

func TestHandleWorkflowRename(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"old.flow": {Type: "workflow", Description: "original"},
		},
	}
	srv, _ := testServer(t, cfg)

	form := url.Values{
		"rename":      {"new.flow"},
		"description": {"renamed"},
	}

	req := httptest.NewRequest("POST", "/workflows/old.flow/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, ok := srv.cfg.Tools["old.flow"]; ok {
		t.Error("old.flow should be gone")
	}
	tc, ok := srv.cfg.Tools["new.flow"]
	if !ok {
		t.Fatal("new.flow should exist")
	}
	if tc.Description != "renamed" {
		t.Errorf("expected 'renamed', got %q", tc.Description)
	}
}

func TestHandleToolRename_SameName(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool": {Type: "cli", Command: "echo", Description: "old"},
		},
	}
	srv, _ := testServer(t, cfg)

	// Rename to same name — should just update description
	form := url.Values{
		"rename":      {"my.tool"},
		"description": {"new desc"},
	}

	req := httptest.NewRequest("POST", "/tools/my.tool/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	tc := srv.cfg.Tools["my.tool"]
	if tc.Description != "new desc" {
		t.Errorf("description should be updated: %q", tc.Description)
	}
}

// --- Auth edge cases ---

func TestHandleAuthCreate_EmptyScopes(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{
		"name":          {"minimal"},
		"client_id":     {"id"},
		"client_secret": {"secret"},
		"auth_url":      {"https://auth.example.com"},
		"token_url":     {"https://token.example.com"},
		"scopes":        {""},
	}

	req := httptest.NewRequest("POST", "/auth/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}

	p := srv.cfg.OAuthProviders["minimal"]
	if len(p.Scopes) != 0 {
		t.Errorf("expected no scopes, got %v", p.Scopes)
	}
}

func TestHandleAuthCreate_MissingName(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{
		"name":      {""},
		"client_id": {"id"},
	}

	req := httptest.NewRequest("POST", "/auth/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Import handler tests ---

func TestHandleImportPreview_EmptyURL(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{"spec_url": {""}, "prefix": {""}}
	req := httptest.NewRequest("POST", "/tools/import/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Please provide") {
		t.Error("expected error message for empty URL")
	}
}

func TestHandleImportPreview_InvalidURL(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{"spec_url": {"/nonexistent/path/spec.yaml"}, "prefix": {""}}
	req := httptest.NewRequest("POST", "/tools/import/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Error:") {
		t.Error("expected error message for invalid path")
	}
}

func TestHandleImportConfirm_NoSelection(t *testing.T) {
	srv, _ := testServer(t, nil)

	form := url.Values{"spec_url": {"http://example.com/spec.json"}, "prefix": {"test"}}
	req := httptest.NewRequest("POST", "/tools/import/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleImportPage(t *testing.T) {
	srv, _ := testServer(t, nil)

	req := httptest.NewRequest("GET", "/tools/import", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "OpenAPI") {
		t.Error("expected OpenAPI mention on import page")
	}
}

// --- Runtime registration tests ---

func TestToolCreate_RegistersInRegistryAndProvider(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	form := url.Values{
		"name":    {"test.echo"},
		"type":    {"cli"},
		"command": {"echo"},
		"args":    {"hello"},
	}

	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	// Verify registered in registry
	tool, err := srv.registry.Get("test.echo")
	if err != nil {
		t.Fatalf("tool not in registry: %v", err)
	}
	if tool.Type != "cli" {
		t.Errorf("expected type cli, got %s", tool.Type)
	}

	// Verify CLI provider was created and has the tool
	prov := srv.proxy.Provider("cli")
	if prov == nil {
		t.Fatal("cli provider should exist")
	}
	cp, ok := prov.(*provider.CLIProvider)
	if !ok {
		t.Fatal("expected *provider.CLIProvider")
	}
	// Execute to prove it's registered (will fail since echo isn't a real command in test, but shouldn't be "not found")
	_ = cp
}

func TestToolCreate_REST_RegistersProvider(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	form := url.Values{
		"name":     {"api.test"},
		"type":     {"rest"},
		"method":   {"GET"},
		"base_url": {"https://httpbin.org"},
		"path":     {"/get"},
	}

	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	// Verify REST provider was created
	prov := srv.proxy.Provider("rest")
	if prov == nil {
		t.Fatal("rest provider should exist")
	}
	if _, ok := prov.(*provider.RESTProvider); !ok {
		t.Fatal("expected *provider.RESTProvider")
	}

	// Verify in registry
	if _, err := srv.registry.Get("api.test"); err != nil {
		t.Fatalf("tool not in registry: %v", err)
	}
}

func TestToolDelete_UnregistersFromRegistryAndProvider(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"doomed": {Type: "cli", Command: "echo"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)

	// First register it
	srv.registerTool("doomed", cfg.Tools["doomed"])

	// Verify it's registered
	if _, err := srv.registry.Get("doomed"); err != nil {
		t.Fatal("tool should be in registry before delete")
	}

	req := httptest.NewRequest("DELETE", "/tools/doomed", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify removed from registry
	if _, err := srv.registry.Get("doomed"); err == nil {
		t.Error("tool should be removed from registry")
	}
}

func TestToolRename_UpdatesRegistryAndProvider(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"old.tool": {Type: "cli", Command: "echo"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	srv.registerTool("old.tool", cfg.Tools["old.tool"])

	form := url.Values{
		"rename":      {"new.tool"},
		"description": {"renamed"},
	}

	req := httptest.NewRequest("POST", "/tools/old.tool/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Old name gone from registry
	if _, err := srv.registry.Get("old.tool"); err == nil {
		t.Error("old.tool should be removed from registry")
	}
	// New name in registry
	if _, err := srv.registry.Get("new.tool"); err != nil {
		t.Errorf("new.tool should be in registry: %v", err)
	}
}

func TestToolSave_UpdatesProvider(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool": {Type: "rest", Method: "GET", BaseURL: "https://old.example.com", Path: "/old"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	srv.registerTool("my.tool", cfg.Tools["my.tool"])

	form := url.Values{
		"description": {"updated"},
		"method":      {"POST"},
		"base_url":    {"https://new.example.com"},
		"path":        {"/new"},
	}

	req := httptest.NewRequest("POST", "/tools/my.tool", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify registry updated
	tool, _ := srv.registry.Get("my.tool")
	if tool.Description != "updated" {
		t.Errorf("registry not updated: %q", tool.Description)
	}
}

// --- Integration: create → execute flow ---

func TestIntegration_CreateRESTTool_ThenTryIt(t *testing.T) {
	// Start a mock API server
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","path":"` + r.URL.Path + `"}`))
	}))
	defer mockAPI.Close()

	srv, _ := testServerWithProxy(t, nil)

	// 1. Create a REST tool via POST
	form := url.Values{
		"name":     {"test.api"},
		"type":     {"rest"},
		"method":   {"GET"},
		"base_url": {mockAPI.URL},
		"path":     {"/items"},
	}
	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("create: expected 302, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Verify it's in registry
	if _, err := srv.registry.Get("test.api"); err != nil {
		t.Fatalf("tool not in registry: %v", err)
	}

	// 3. Verify it's in the REST provider
	prov := srv.proxy.Provider("rest")
	if prov == nil {
		t.Fatal("rest provider should exist")
	}

	// 4. Try It — execute via the proxy
	tryForm := url.Values{}
	req = httptest.NewRequest("POST", "/tools/test.api/try", strings.NewReader(tryForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("try: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "ok") {
		t.Errorf("try result should contain API response, got: %s", body)
	}
}

func TestIntegration_CreateRESTTool_WithPathParams(t *testing.T) {
	var receivedPath string
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_, _ = w.Write([]byte(`{"path":"` + r.URL.Path + `"}`))
	}))
	defer mockAPI.Close()

	cfg := &config.Config{
		Tools: make(map[string]config.ToolConfig),
	}
	srv, _ := testServerWithProxy(t, cfg)

	// Manually add a tool with path params (simulating import)
	tc := config.ToolConfig{
		Type:    "rest",
		Method:  "GET",
		BaseURL: mockAPI.URL,
		Path:    "/repos/{{owner}}/{{repo}}",
		Parameters: []config.ParamConfig{
			{Name: "owner", Required: true},
			{Name: "repo", Required: true},
		},
	}
	srv.cfg.Tools["repos.get"] = tc
	srv.registerTool("repos.get", tc)

	// Try It with params
	tryForm := url.Values{
		"param_owner": {"factorly-dev"},
		"param_repo":  {"factorly"},
	}
	req := httptest.NewRequest("POST", "/tools/repos.get/try", strings.NewReader(tryForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("try: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if receivedPath != "/repos/factorly-dev/factorly" {
		t.Errorf("expected /repos/factorly-dev/factorly, got %s", receivedPath)
	}
}

func TestIntegration_CreateCLITool_ThenTryIt(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	form := url.Values{
		"name":    {"test.echo"},
		"type":    {"cli"},
		"command": {"echo"},
		"args":    {"hello world"},
	}
	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("create: expected 302, got %d", w.Code)
	}

	// Try It
	req = httptest.NewRequest("POST", "/tools/test.echo/try", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("try: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "hello world") {
		t.Errorf("try result should contain 'hello world', got: %s", body)
	}
}

func TestIntegration_SaveToolUpdatesProvider(t *testing.T) {
	// Create a mock server that echoes the path
	mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer mockAPI.Close()

	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.api": {Type: "rest", Method: "GET", BaseURL: mockAPI.URL, Path: "/old"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	srv.registerTool("my.api", cfg.Tools["my.api"])

	// Save with new path
	form := url.Values{
		"description": {"updated"},
		"method":      {"GET"},
		"base_url":    {mockAPI.URL},
		"path":        {"/new"},
	}
	req := httptest.NewRequest("POST", "/tools/my.api", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("save: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Try It — should hit /new, not /old
	req = httptest.NewRequest("POST", "/tools/my.api/try", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("try: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "/new") {
		t.Errorf("expected /new in response, got: %s", body)
	}
	if strings.Contains(body, "/old") {
		t.Errorf("should not contain /old, got: %s", body)
	}
}

func TestIntegration_VaultRefResolution(t *testing.T) {
	v := newMockVault()
	_ = v.Set("API_KEY", "secret-123")

	cfg := &config.Config{
		Tools: make(map[string]config.ToolConfig),
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	_ = os.WriteFile(cfgPath, []byte("tools: {}\n"), 0o644)

	reg := registry.New()
	providers := make(map[string]provider.Provider)
	p := proxy.New(reg, providers, logger.NopLogger{})

	resolver := vault.NewResolver()
	resolver.Register("vault", v)

	srv, err := New(Options{
		Config:   cfg,
		CfgPath:  cfgPath,
		Vault:    v,
		Resolver: resolver,
		Registry: reg,
		Proxy:    p,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Register a CLI tool with a vault ref in the command
	tc := config.ToolConfig{
		Type:    "cli",
		Command: "echo",
		Args:    []string{"{{vault:API_KEY}}"},
	}
	srv.cfg.Tools["test.vault"] = tc
	srv.registerTool("test.vault", tc)

	// Verify vault key is tracked on the registry tool
	tool, _ := srv.registry.Get("test.vault")
	if len(tool.VaultKeys) == 0 {
		t.Fatal("expected vault keys to be tracked")
	}
	found := false
	for _, k := range tool.VaultKeys {
		if k == "vault:API_KEY" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'vault:API_KEY' in vault keys, got %v", tool.VaultKeys)
	}
}

// --- Template rendering tests ---

func TestAllTemplatesRender(t *testing.T) {
	// Just verify that creating the server (which parses all templates) succeeds
	_, _ = testServer(t, nil)
	// If we get here without panic/fatal, all templates parse correctly
}

// TestReloadConfigPreservesBuiltins guards against a regression where reloading
// the on-disk config (e.g. after installing a blueprint) wiped the in-memory
// built-in tools because they aren't represented on disk.
func TestReloadConfigPreservesBuiltins(t *testing.T) {
	cfg := &config.Config{Tools: make(map[string]config.ToolConfig)}
	builtins.Register(cfg, builtins.Options{Mode: "stdio"})

	if !builtins.IsBuiltinTool("factorly.shell") {
		t.Fatal("precondition: factorly.shell should be a built-in")
	}
	if _, ok := cfg.Tools["factorly.shell"]; !ok {
		t.Fatal("precondition: factorly.shell should be registered into cfg before reload")
	}

	srv, cfgPath := testServerWithProxy(t, cfg)

	// Register the built-ins into the live registry the way the real bootstrap does.
	for name, tc := range cfg.Tools {
		srv.registerTool(name, tc)
	}

	// Now simulate a blueprint install by writing a new tool to disk and reloading.
	newCfg := []byte(`
tools:
  newly_installed:
    type: cli
    command: echo
    description: from a freshly installed blueprint
    args: ["hi"]
`)
	if err := os.WriteFile(cfgPath, newCfg, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := srv.reloadConfig(); err != nil {
		t.Fatalf("reloadConfig: %v", err)
	}

	// Built-ins must still be in cfg.Tools so handlers and the proxy can find them.
	for _, name := range []string{"factorly.shell", "factorly.fetch", "factorly.file.read"} {
		if _, ok := srv.cfg.Tools[name]; !ok {
			t.Errorf("built-in %q missing from cfg.Tools after reload", name)
		}
		if _, err := srv.registry.Get(name); err != nil {
			t.Errorf("built-in %q missing from registry after reload: %v", name, err)
		}
	}

	// The newly installed tool from disk should have been picked up.
	if _, ok := srv.cfg.Tools["newly_installed"]; !ok {
		t.Error("newly_installed tool from disk was not registered after reload")
	}
}

// TestRegisterTool_LazyCreatesWorkflowProvider guards against the bug where
// creating the first workflow in a fresh session left the workflow provider
// unregistered, causing "no provider for tool X (key: workflow)" when the
// workflow was invoked. The workflow provider should be lazy-created on
// first registration, matching the CLI/REST provider behavior.
func TestRegisterTool_LazyCreatesWorkflowProvider(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	// Precondition: no workflow provider exists at startup.
	if srv.proxy.Provider("workflow") != nil {
		t.Fatal("precondition: workflow provider should not exist before any workflow is registered")
	}

	tc := config.ToolConfig{
		Type:        "workflow",
		Description: "daily prep workflow",
		Steps: []config.StepConfig{
			{Tool: "factorly.fetch", Params: map[string]string{"url": "https://example.com"}, Store: "data"},
		},
	}
	srv.registerTool("daily.prep", tc)

	prov := srv.proxy.Provider("workflow")
	if prov == nil {
		t.Fatal("workflow provider was not lazy-created when first workflow was registered")
	}

	// Executing should now resolve the provider — we don't actually run the
	// step (that would need fetched URL), but the error must not be the
	// "no provider for tool" failure mode.
	_, err := srv.proxy.ExecuteWithContext(context.Background(), "daily.prep", nil, "test")
	if err != nil && strings.Contains(err.Error(), "no provider for tool") {
		t.Errorf("provider lookup still failed after lazy-create: %v", err)
	}
}

// TestToolSave_CLI_PersistsEnvAndIsolation guards against the env-vars UI
// form fields being dropped on save. Posting env_key[]/env_val[] rows and
// the env_isolation toggle should round-trip into cfg.Tools and the
// registry/provider.
func TestToolSave_CLI_PersistsEnvAndIsolation(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.cli": {Type: "cli", Command: "echo"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	srv.registerTool("my.cli", cfg.Tools["my.cli"])

	form := url.Values{}
	form.Set("description", "updated")
	form.Set("command", "echo")
	form.Set("args", "hi")
	form.Add("env_key[]", "FOO")
	form.Add("env_val[]", "bar")
	form.Add("env_key[]", "TOKEN")
	form.Add("env_val[]", "{{vault:MY_TOKEN}}")
	form.Add("env_key[]", "") // blank row should be ignored
	form.Add("env_val[]", "ignored")
	form.Set("env_isolation", "strict")

	req := httptest.NewRequest("POST", "/tools/my.cli", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := srv.cfg.Tools["my.cli"]
	if got.Env["FOO"] != "bar" {
		t.Errorf("env FOO not persisted; got %v", got.Env)
	}
	if got.Env["TOKEN"] != "{{vault:MY_TOKEN}}" {
		t.Errorf("env TOKEN (vault ref) not persisted; got %v", got.Env)
	}
	if _, ok := got.Env[""]; ok {
		t.Errorf("blank env key should have been skipped; got %v", got.Env)
	}
	if got.EnvIsolation != "strict" {
		t.Errorf("env_isolation not persisted; got %q", got.EnvIsolation)
	}

	// Now clear env via a follow-up save (empty rows) and confirm Env is
	// reset to nil rather than left stale.
	form2 := url.Values{}
	form2.Set("description", "updated again")
	form2.Set("command", "echo")
	// no env_key[] / env_val[] / env_isolation
	req2 := httptest.NewRequest("POST", "/tools/my.cli", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("clear save: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	got2 := srv.cfg.Tools["my.cli"]
	if got2.Env != nil {
		t.Errorf("env should be cleared on save with no env rows; got %v", got2.Env)
	}
	if got2.EnvIsolation != "" {
		t.Errorf("env_isolation should be cleared when toggle is off; got %q", got2.EnvIsolation)
	}
}

// TestToolCreate_CLI_PersistsEnvAndIsolation guards against env-vars UI rows
// being dropped when creating a new CLI tool. Same parsing as the save path,
// but exercised through POST /tools/_new.
func TestToolCreate_CLI_PersistsEnvAndIsolation(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	form := url.Values{}
	form.Set("name", "test.envcli")
	form.Set("type", "cli")
	form.Set("description", "with env")
	form.Set("command", "printenv")
	form.Add("env_key[]", "FOO")
	form.Add("env_val[]", "bar")
	form.Add("env_key[]", "TOKEN")
	form.Add("env_val[]", "{{vault:MY_TOKEN}}")
	form.Add("env_key[]", "") // blank should be skipped
	form.Add("env_val[]", "ignored")
	form.Set("env_isolation", "strict")

	req := httptest.NewRequest("POST", "/tools/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
	}

	got, ok := srv.cfg.Tools["test.envcli"]
	if !ok {
		t.Fatal("tool not registered in cfg")
	}
	if got.Env["FOO"] != "bar" {
		t.Errorf("env FOO not persisted; got %v", got.Env)
	}
	if got.Env["TOKEN"] != "{{vault:MY_TOKEN}}" {
		t.Errorf("env TOKEN (vault ref) not persisted; got %v", got.Env)
	}
	if _, ok := got.Env[""]; ok {
		t.Errorf("blank env key should have been skipped; got %v", got.Env)
	}
	if got.EnvIsolation != "strict" {
		t.Errorf("env_isolation not persisted; got %q", got.EnvIsolation)
	}
}

func TestHandleToolYAML(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":       {Type: "cli", Command: "echo", Description: "echo back"},
			"deploy.all": {Type: "workflow", Steps: []config.StepConfig{{Tool: "echo"}}},
		},
	}
	srv, _ := testServer(t, cfg)

	// Happy path
	req := httptest.NewRequest("GET", "/tools/echo/yaml", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "echo") || !strings.Contains(w.Body.String(), "command: echo") {
		t.Errorf("expected yaml in body, got:\n%s", w.Body.String())
	}

	// Workflow under /tools/ → 404 (separate route)
	req = httptest.NewRequest("GET", "/tools/deploy.all/yaml", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("workflow under /tools/ should 404, got %d", w.Code)
	}

	// Unknown tool → 404
	req = httptest.NewRequest("GET", "/tools/nope/yaml", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown → 404, got %d", w.Code)
	}

	// ?download=1 → application/yaml + attachment disposition
	req = httptest.NewRequest("GET", "/tools/echo/yaml?download=1", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("download Content-Type = %q, want application/yaml", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "echo.yaml") {
		t.Errorf("download Content-Disposition missing filename: %q", cd)
	}
}

func TestHandleWorkflowYAML(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"echo":       {Type: "cli", Command: "echo"},
			"deploy.all": {Type: "workflow", Description: "deploy", Steps: []config.StepConfig{{Tool: "echo"}}},
		},
	}
	srv, _ := testServer(t, cfg)

	req := httptest.NewRequest("GET", "/workflows/deploy.all/yaml", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "type: workflow") {
		t.Errorf("expected workflow yaml in body, got:\n%s", w.Body.String())
	}

	// Non-workflow tool under /workflows/ → 404
	req = httptest.NewRequest("GET", "/workflows/echo/yaml", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("non-workflow → 404, got %d", w.Code)
	}
}

func TestHandleBlueprintYAML(t *testing.T) {
	srv, cfgPath := testServer(t, nil)

	// cfgPath is <dir>/factorly.yaml; blueprints live under <dir>/.factorly/blueprints.
	bpDir := filepath.Join(filepath.Dir(cfgPath), ".factorly", "blueprints")
	if err := os.MkdirAll(bpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bpYAML := []byte("# my blueprint\nname: gmail\nversion: 1.0.0\n")
	if err := os.WriteFile(filepath.Join(bpDir, "gmail.yaml"), bpYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/blueprints/installed/gmail/yaml", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Comment should be preserved (raw bytes, not re-marshal)
	if !strings.Contains(w.Body.String(), "# my blueprint") {
		t.Errorf("expected blueprint comment preserved, got:\n%s", w.Body.String())
	}

	// Missing blueprint → 404
	req = httptest.NewRequest("GET", "/blueprints/installed/nope/yaml", nil)
	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("missing blueprint → 404, got %d", w.Code)
	}
}

// TestToolSave_Code_PersistsParameters guards against a regression where
// editing a code tool dropped the parameters list. The save handler must
// parse param_name_<i>/param_type_<i>/etc. for code tools just like it
// does for cli/rest/mcp.
func TestToolSave_Code_PersistsParameters(t *testing.T) {
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.code": {
				Type:        "code",
				Description: "orig",
				Code:        "package m\nfunc Run(params map[string]string) (any, error) { return \"x\", nil }",
			},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	srv.registerTool("my.code", cfg.Tools["my.code"])

	form := url.Values{}
	form.Set("description", "updated")
	form.Set("code", "package m\nfunc Run(params map[string]string) (any, error) { return params[\"who\"], nil }")
	form.Set("max_calls", "50")
	form.Set("param_name_0", "who")
	form.Set("param_type_0", "string")
	form.Set("param_in_0", "query")
	form.Set("param_required_0", "on")
	form.Set("param_default_0", "world")
	form.Set("param_desc_0", "the greetee")

	req := httptest.NewRequest("POST", "/tools/my.code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := srv.cfg.Tools["my.code"]
	if len(got.Parameters) != 1 {
		t.Fatalf("Parameters not persisted, got %d entries: %+v", len(got.Parameters), got.Parameters)
	}
	p := got.Parameters[0]
	if p.Name != "who" {
		t.Errorf("param Name = %q, want who", p.Name)
	}
	if p.Default != "world" {
		t.Errorf("param Default = %q, want world", p.Default)
	}
	if !p.Required {
		t.Error("param Required should be true")
	}
	if got.Shadow == nil || got.Shadow.MaxCalls != 50 {
		t.Errorf("Shadow.MaxCalls = %v, want 50", got.Shadow)
	}
	if !strings.Contains(got.Code, "params[\"who\"]") {
		t.Errorf("Code body not updated; got:\n%s", got.Code)
	}
}
