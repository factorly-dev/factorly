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

// --- Browse / catalog handler tests ---

func TestBlueprintsBrowseRenders(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/blueprints/browse", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Header + at least one bundled card.
	if !strings.Contains(body, "Browse Blueprints") {
		t.Errorf("expected 'Browse Blueprints' header, got %s", body[:300])
	}
	// Linear is one of the bundled blueprints — its display name must
	// appear on the catalog page.
	if !strings.Contains(body, "Linear") {
		t.Error("expected Linear card on catalog page")
	}
}

func TestBlueprintBrowseDetailRenders(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/blueprints/browse/linear", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Linear") {
		t.Error("expected 'Linear' on detail page")
	}
	if !strings.Contains(body, "linear.list_issues") {
		t.Error("expected tool name from bundled YAML on detail page")
	}
	if !strings.Contains(body, "LINEAR_API_KEY") {
		t.Error("expected vault key from bundled YAML on detail page")
	}
}

func TestBlueprintBrowseDetailUnknown404s(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/blueprints/browse/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestBlueprintBrowseInstallWritesAndReloads(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)

	// Use a bundled blueprint that doesn't require vault keys to keep the
	// test focused. Git/make/npm are AuthType=none.
	req := httptest.NewRequest(http.MethodPost, "/blueprints/browse/git/install", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	// File should land on disk.
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfgPath), ".factorly", "blueprints", "git.yaml")); err != nil {
		t.Fatalf("expected git.yaml in .factorly/blueprints/: %v", err)
	}
}

func TestBlueprintBrowseInstallStoresVaultValues(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)

	form := strings.NewReader("vault_LINEAR_API_KEY=test-key-value")
	req := httptest.NewRequest(http.MethodPost, "/blueprints/browse/linear/install", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	// Whether the install succeeded or not, the vault key should have been
	// written first (the handler writes secrets before attempting install).
	v, err := srv.vault.Get("LINEAR_API_KEY")
	if err != nil {
		t.Fatalf("vault.Get: %v", err)
	}
	if v != "test-key-value" {
		t.Errorf("vault value = %q, want test-key-value", v)
	}
}

func TestBlueprintBrowseInstallUnknown404s(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/blueprints/browse/nonexistent/install", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestBlueprintsBrowseMarksInstalled(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	// Install one bundled blueprint first.
	if _, err := blueprints.Install(blueprints.InstallOptions{
		Content: []byte(blueprints.BundledByName("git").YAML),
		CfgPath: srv.cfgPath,
	}); err != nil {
		t.Fatalf("priming install: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blueprints/browse", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()

	// The Installed badge should appear at least once in the rendered page.
	if !strings.Contains(body, "Installed") {
		t.Error("expected 'Installed' badge on at least one card")
	}
}

// --- Homepage icon rendering ---

func TestBlueprintsListShowsExternalLinkIconWhenHomepageSet(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, `
name: with-homepage
version: 0.1
description: has a homepage
homepage: https://example.com/has-homepage
tools: {}
`)
	if _, err := blueprints.Install(blueprints.InstallOptions{Source: pack, CfgPath: srv.cfgPath}); err != nil {
		t.Fatalf("priming: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blueprints", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()

	// The homepage URL should appear in an anchor.
	if !strings.Contains(body, "https://example.com/has-homepage") {
		t.Errorf("expected homepage URL in body")
	}
	// The Lucide external-link icon's distinctive path data ("M15 3h6v6")
	// should render via {{icon "external-link"}}. If the link still uses the
	// raw ↗ glyph or the icon is missing, this assertion catches it.
	if !strings.Contains(body, "M15 3h6v6") {
		t.Errorf("expected Lucide external-link icon path in body next to homepage")
	}
	if !strings.Contains(body, `aria-label="Open homepage"`) {
		t.Errorf("expected aria-label on the homepage anchor for accessibility")
	}
	// Make sure the bare ↗ character isn't lurking — we want the icon, not
	// the unicode arrow.
	if strings.Contains(body, "↗") {
		t.Errorf("found legacy ↗ glyph; should be replaced by the external-link icon")
	}
}

func TestBlueprintsListNoHomepageNoIcon(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	pack := writeBlueprintOnDisk(t, "name: no-homepage\nversion: 0.1\ntools: {}\n")
	if _, err := blueprints.Install(blueprints.InstallOptions{Source: pack, CfgPath: srv.cfgPath}); err != nil {
		t.Fatalf("priming: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/blueprints", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()

	// With no Homepage set the template skips the link entirely — no icon,
	// no aria-label, no stray anchor pointing at an empty href.
	if strings.Contains(body, `aria-label="Open homepage"`) {
		t.Errorf("homepage anchor rendered for a blueprint with no Homepage field")
	}
}
