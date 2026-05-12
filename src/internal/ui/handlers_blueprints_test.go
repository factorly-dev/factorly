// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/blueprints"
	"github.com/factorly-dev/factorly/internal/config"
)

// writeBlueprintOnDisk writes a YAML pack file in the test's temp dir and returns
// its absolute path. The caller passes that path to /blueprints/preview or
// /blueprints/install as the "source" field.
func writeBlueprintOnDisk(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func postJSON(t *testing.T, srv *Server, path string, body any) (*httptest.ResponseRecorder, previewResponse) {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var resp previewResponse
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	}
	return rec, resp
}

func TestBlueprintPreviewHappyPath(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, `
name: previewable
version: 0.1
description: testing preview
tools:
  preview.tool:
    type: cli
    command: echo
    description: t
`)
	rec, resp := postJSON(t, srv, "/blueprints/preview", previewRequest{Source: pack})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Result == nil || resp.Result.Header.Name != "previewable" {
		t.Fatalf("expected preview result with header, got %+v", resp.Result)
	}
	if !resp.Result.DryRun {
		t.Fatal("preview result should have DryRun=true")
	}
	if len(resp.Result.ToolsAdded) != 1 || resp.Result.ToolsAdded[0] != "preview.tool" {
		t.Errorf("expected preview.tool in ToolsAdded, got %v", resp.Result.ToolsAdded)
	}
}

func TestBlueprintPreviewMissingSource(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	rec, resp := postJSON(t, srv, "/blueprints/preview", previewRequest{Source: ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if resp.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestBlueprintPreviewBadSource(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	rec, resp := postJSON(t, srv, "/blueprints/preview", previewRequest{Source: "/no/such/file.yaml"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (errors come back in body)", rec.Code)
	}
	if resp.Error == "" {
		t.Fatal("expected error in response body")
	}
}

func TestBlueprintInstallWritesFileAndReloads(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, `
name: install-me
tools:
  installed.tool:
    type: cli
    command: echo
    description: t
`)

	rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{Source: pack})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	// Blueprint file should now exist on disk.
	blueprintFile := filepath.Join(filepath.Dir(cfgPath), ".factorly", "blueprints", "install-me.yaml")
	if _, err := os.Stat(blueprintFile); err != nil {
		t.Fatalf("expected blueprint file at %s: %v", blueprintFile, err)
	}

	// Server's live config should now include the new tool (reload happened).
	if _, ok := srv.cfg.Tools["installed.tool"]; !ok {
		t.Errorf("expected installed.tool in live config after install, got %v", srv.cfg.Tools)
	}
}

func TestBlueprintInstallWritesVaultValues(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, `
name: needs-keys
requires:
  vault_keys:
    - MY_KEY
tools:
  k.tool:
    type: cli
    command: echo
    description: t
`)

	rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{
		Source:      pack,
		VaultValues: map[string]string{"MY_KEY": "my-value"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	// Vault should now have the value.
	v, err := srv.vault.Get("MY_KEY")
	if err != nil {
		t.Fatalf("vault.Get: %v", err)
	}
	if v != "my-value" {
		t.Errorf("vault value = %q, want %q", v, "my-value")
	}
}

func TestBlueprintInstallSkipsBlankVaultValues(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, `
name: skip-blank
requires:
  vault_keys:
    - OPTIONAL_KEY
tools:
  k.tool:
    type: cli
    command: echo
    description: t
`)

	_, resp := postJSON(t, srv, "/blueprints/install", installRequest{
		Source:      pack,
		VaultValues: map[string]string{"OPTIONAL_KEY": ""},
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	// Vault should NOT have an empty entry — blanks are skipped.
	if _, err := srv.vault.Get("OPTIONAL_KEY"); err == nil {
		t.Error("expected vault.Get to fail for blank-skipped key")
	}
}

func TestBlueprintInstallReportsConflictError(t *testing.T) {
	// Pre-populate the server with a tool the pack will collide with.
	cfg := &config.Config{
		Tools: map[string]config.ToolConfig{
			"shared.tool": {Type: "cli", Command: "existing", Description: "existing"},
		},
	}
	srv, _ := testServerWithProxy(t, cfg)
	// The on-disk config also needs the tool for blueprints.Install's view.
	_ = os.WriteFile(srv.cfgPath, []byte(`
tools:
  shared.tool:
    type: cli
    command: existing
    description: existing
`), 0o644)

	pack := writeBlueprintOnDisk(t, `
name: conflicty
tools:
  shared.tool:
    type: cli
    command: new
    description: new
`)

	rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{Source: pack})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error == "" {
		t.Fatal("expected conflict error in response")
	}
	if resp.Result == nil || len(resp.Result.Conflicts) == 0 {
		t.Fatalf("expected populated Conflicts, got %+v", resp.Result)
	}
}

func TestBlueprintPreviewReportsAlreadyInstalled(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, "name: already-here\ntools: {}\n")

	// First install via the in-package API (not the handler) to bypass the
	// reload path noise.
	if _, err := blueprints.Install(blueprints.InstallOptions{
		Source:  pack,
		CfgPath: srv.cfgPath,
	}); err != nil {
		t.Fatalf("priming install: %v", err)
	}

	// Now preview the same pack — should report already-installed without an
	// error so the UI can render the preview cleanly.
	rec, resp := postJSON(t, srv, "/blueprints/preview", previewRequest{Source: pack})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("preview on already-installed should not error, got %s", resp.Error)
	}
	if resp.Result == nil || !resp.Result.AlreadyInstalled {
		t.Fatalf("expected AlreadyInstalled flag, got %+v", resp.Result)
	}
}

func TestBlueprintUninstallRemovesFileAndReloads(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)

	// Install via the handler so registry state matches an install flow.
	pack := writeBlueprintOnDisk(t, `
name: removable
tools:
  rm.tool:
    type: cli
    command: echo
    description: t
`)
	if rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{Source: pack}); rec.Code != http.StatusOK || resp.Error != "" {
		t.Fatalf("install failed: status=%d err=%q", rec.Code, resp.Error)
	}

	req := httptest.NewRequest(http.MethodDelete, "/blueprints/removable", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "/blueprints" {
		t.Errorf("expected HX-Redirect: /blueprints, got %q", rec.Header().Get("HX-Redirect"))
	}

	// Blueprint file should be gone.
	blueprintFile := filepath.Join(filepath.Dir(cfgPath), ".factorly", "blueprints", "removable.yaml")
	if _, err := os.Stat(blueprintFile); err == nil {
		t.Fatal("blueprint file should be removed")
	}

	// Live config should no longer have the tool.
	if _, ok := srv.cfg.Tools["rm.tool"]; ok {
		t.Error("rm.tool should be removed from live config after uninstall")
	}
}

func TestBlueprintUninstallNotFound(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodDelete, "/blueprints/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestBlueprintsListRoute(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/blueprints", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	// Empty-state copy should appear.
	if !strings.Contains(rec.Body.String(), "No blueprints installed") {
		t.Errorf("expected empty-state copy, got %s", rec.Body.String())
	}
}

func TestBlueprintsListShowsInstalled(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, "name: visible\nversion: 1.2.3\ndescription: shows up\ntools: {}\n")
	if _, err := blueprints.Install(blueprints.InstallOptions{Source: pack, CfgPath: srv.cfgPath}); err != nil {
		t.Fatalf("priming: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blueprints", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "visible") {
		t.Errorf("expected pack name in body, got %s", body)
	}
	if !strings.Contains(body, "1.2.3") {
		t.Errorf("expected version in body, got %s", body)
	}
	if !strings.Contains(body, "shows up") {
		t.Errorf("expected description in body, got %s", body)
	}
}

func TestBuiltinNamesFromConfig(t *testing.T) {
	srv, _ := testServerWithProxy(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"factorly.fetch": {Type: "cli", Command: "echo"},
			"factorly.shell": {Type: "cli", Command: "sh"},
			"my.custom":      {Type: "cli", Command: "echo"},
		},
	})
	names := srv.builtinNamesFromConfig()
	if !names["factorly.fetch"] || !names["factorly.shell"] {
		t.Errorf("expected factorly.* tools to be flagged as builtins, got %v", names)
	}
	if names["my.custom"] {
		t.Errorf("my.custom should NOT be flagged as a builtin, got %v", names)
	}
}

// --- Inline content (paste-YAML) handler tests ---

func TestBlueprintPreviewAcceptsInlineContent(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	rec, resp := postJSON(t, srv, "/blueprints/preview", previewRequest{
		Content: "name: pasted\ntools:\n  p.tool:\n    type: cli\n    command: echo\n    description: x\n",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Result == nil || resp.Result.Header.Name != "pasted" {
		t.Fatalf("expected pasted result, got %+v", resp.Result)
	}
	if !resp.Result.DryRun {
		t.Error("preview should be a dry-run")
	}
}

func TestBlueprintInstallAcceptsInlineContent(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{
		Content: "name: pasted-install\ntools:\n  pi.tool:\n    type: cli\n    command: echo\n    description: x\n",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	blueprintFile := filepath.Join(filepath.Dir(cfgPath), ".factorly", "blueprints", "pasted-install.yaml")
	if _, err := os.Stat(blueprintFile); err != nil {
		t.Fatalf("expected blueprint file at %s: %v", blueprintFile, err)
	}
	if _, ok := srv.cfg.Tools["pi.tool"]; !ok {
		t.Errorf("expected pi.tool registered after install, got %v", srv.cfg.Tools)
	}
}

func TestBlueprintInstallRequiresSourceOrContent(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	rec, resp := postJSON(t, srv, "/blueprints/install", installRequest{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if resp.Error == "" {
		t.Fatal("expected error in response")
	}
}
