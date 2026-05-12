// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package packs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempProject creates a temp dir with a minimal factorly.yaml at the root and
// returns the path to that config file. Tests then point Install at it.
func tempProject(t *testing.T, existingCfg string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "factorly.yaml")
	if existingCfg == "" {
		existingCfg = "tools: {}\n"
	}
	if err := os.WriteFile(cfgPath, []byte(existingCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

// writePackFile writes a pack YAML to a temp location and returns its path.
func writePackFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "pack-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestResolveLocalFile(t *testing.T) {
	path := writePackFile(t, "name: foo\ntools: {}\n")
	src, err := Resolve(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src.Kind != "file" {
		t.Fatalf("kind = %q, want file", src.Kind)
	}
	if src.LocalPath == "" {
		t.Fatal("LocalPath empty")
	}
}

func TestResolveMissingFile(t *testing.T) {
	_, err := Resolve("/nonexistent/path/foo.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestResolveRawURL(t *testing.T) {
	src, err := Resolve("https://example.com/pack.yaml")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src.Kind != "url" {
		t.Fatalf("kind = %q, want url", src.Kind)
	}
	if len(src.URLs) != 1 || src.URLs[0] != "https://example.com/pack.yaml" {
		t.Fatalf("URLs = %v", src.URLs)
	}
}

func TestResolveGitHubShorthand(t *testing.T) {
	src, err := Resolve("github.com/widefido/factorly-gmail")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src.Kind != "github" {
		t.Fatalf("kind = %q, want github", src.Kind)
	}
	// Expect main x {factorly.yaml, pack.yaml} and master x {factorly.yaml, pack.yaml}
	if len(src.URLs) != 4 {
		t.Fatalf("URLs = %d, want 4: %v", len(src.URLs), src.URLs)
	}
	want := "https://raw.githubusercontent.com/widefido/factorly-gmail/main/factorly.yaml"
	if src.URLs[0] != want {
		t.Fatalf("URLs[0] = %q, want %q", src.URLs[0], want)
	}
}

func TestResolveGitHubWithRef(t *testing.T) {
	src, err := Resolve("github.com/widefido/factorly-gmail@v1.0.0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Only main… no, only the pinned ref, with two default filenames.
	if len(src.URLs) != 2 {
		t.Fatalf("URLs = %d, want 2: %v", len(src.URLs), src.URLs)
	}
	if !strings.Contains(src.URLs[0], "/v1.0.0/") {
		t.Fatalf("URLs[0] = %q, want ref segment", src.URLs[0])
	}
}

func TestResolveGitHubWithPath(t *testing.T) {
	src, err := Resolve("github.com/widefido/factorly-gmail/packs/search.yaml")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(src.URLs) != 2 {
		t.Fatalf("URLs = %d, want 2: %v", len(src.URLs), src.URLs)
	}
	if !strings.HasSuffix(src.URLs[0], "/packs/search.yaml") {
		t.Fatalf("URLs[0] = %q", src.URLs[0])
	}
}

func TestInstallLocalPackDryRun(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: gmail-toolkit
version: 1.0.0
description: Gmail integration
tools:
  gmail.search:
    type: cli
    command: echo
    description: search
  gmail.daily:
    type: workflow
    steps:
      - tool: gmail.search
`)

	res, err := Install(InstallOptions{
		Source:  pack,
		CfgPath: cfgPath,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("install dry-run: %v", err)
	}
	if !res.DryRun {
		t.Fatalf("DryRun flag not set on result")
	}
	if res.Header.Name != "gmail-toolkit" {
		t.Fatalf("Header.Name = %q", res.Header.Name)
	}
	if got, want := len(res.ToolsAdded), 1; got != want {
		t.Fatalf("ToolsAdded = %v", res.ToolsAdded)
	}
	if got, want := len(res.WorkflowsAdded), 1; got != want {
		t.Fatalf("WorkflowsAdded = %v", res.WorkflowsAdded)
	}
	// Dry-run must not write a file.
	if res.FilePath != "" {
		t.Fatalf("FilePath should be empty on dry-run, got %q", res.FilePath)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfgPath), ".factorly", "packs")); err == nil {
		t.Fatal("packs/ directory should not exist after dry-run")
	}
}

func TestInstallLocalPackCommits(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: simple
tools:
  simple.tool:
    type: cli
    command: echo
    description: simple
`)

	res, err := Install(InstallOptions{
		Source:  pack,
		CfgPath: cfgPath,
		DryRun:  false,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.FilePath == "" {
		t.Fatal("FilePath should be set on real install")
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Fatalf("pack file should exist: %v", err)
	}
}

func TestInstallConflictsBlock(t *testing.T) {
	cfgPath := tempProject(t, `
tools:
  simple.tool:
    type: cli
    command: existing
    description: existing
`)
	pack := writePackFile(t, `
name: simple
tools:
  simple.tool:
    type: cli
    command: echo
    description: new
`)
	res, err := Install(InstallOptions{
		Source:  pack,
		CfgPath: cfgPath,
		DryRun:  false,
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("Conflicts = %v", res.Conflicts)
	}
	if res.Conflicts[0].Name != "simple.tool" {
		t.Fatalf("Conflicts[0].Name = %q", res.Conflicts[0].Name)
	}
}

func TestInstallMissingRequiresFails(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: needs-ghost
requires:
  tools: [does.not.exist]
tools:
  my.tool:
    type: cli
    command: echo
    description: x
`)
	_, err := Install(InstallOptions{
		Source:       pack,
		CfgPath:      cfgPath,
		DryRun:       false,
		BuiltinTools: nil,
	})
	if err == nil {
		t.Fatal("expected requires-missing error")
	}
}

func TestInstallSatisfiedRequiresPasses(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: uses-fetch
requires:
  tools: [factorly.fetch]
tools:
  my.tool:
    type: cli
    command: echo
    description: x
`)
	_, err := Install(InstallOptions{
		Source:       pack,
		CfgPath:      cfgPath,
		DryRun:       false,
		BuiltinTools: map[string]bool{"factorly.fetch": true},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
}

func TestInstallVaultKeysReported(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: needs-vault
requires:
  vault_keys:
    - foo_client_id
    - foo_client_secret
tools:
  my.tool:
    type: cli
    command: echo
    description: x
`)
	res, err := Install(InstallOptions{
		Source:  pack,
		CfgPath: cfgPath,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("install dry-run: %v", err)
	}
	if got := len(res.VaultKeysMissing); got != 2 {
		t.Fatalf("VaultKeysMissing = %v", res.VaultKeysMissing)
	}
}

func TestInstallTwiceFails(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, "name: twice\ntools: {}\n")
	if _, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	res, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath})
	if err == nil {
		t.Fatal("expected error on second install of same pack")
	}
	if res == nil || !res.AlreadyInstalled {
		t.Fatalf("expected AlreadyInstalled flag on second install result, got %+v", res)
	}
}

func TestInstallDryRunReportsAlreadyInstalledWithoutError(t *testing.T) {
	// UI Preview hits dry-run. When the pack is already installed, we want
	// structured info (AlreadyInstalled=true) instead of an error so the UI
	// can render a preview with an Uninstall hint.
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, "name: preview-installed\ntools: {}\n")
	if _, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	res, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath, DryRun: true})
	if err != nil {
		t.Fatalf("dry-run on already-installed should not error, got %v", err)
	}
	if !res.AlreadyInstalled {
		t.Fatalf("expected AlreadyInstalled flag, got %+v", res)
	}
	if res.Header.Name != "preview-installed" {
		t.Errorf("expected header still populated for preview rendering, got %+v", res.Header)
	}
}

func TestUninstall(t *testing.T) {
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, "name: removable\ntools: {}\n")
	if _, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := Uninstall(cfgPath, "removable"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := Uninstall(cfgPath, "removable"); err == nil {
		t.Fatal("expected error uninstalling non-existent pack")
	}
}

func TestList(t *testing.T) {
	cfgPath := tempProject(t, "")
	if _, err := Install(InstallOptions{Source: writePackFile(t, "name: a\ntools: {}\n"), CfgPath: cfgPath}); err != nil {
		t.Fatalf("install a: %v", err)
	}
	if _, err := Install(InstallOptions{Source: writePackFile(t, "name: b\nversion: 2.0\ntools: {}\n"), CfgPath: cfgPath}); err != nil {
		t.Fatalf("install b: %v", err)
	}
	packs, err := List(cfgPath)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("packs = %v", packs)
	}
	if packs[0].Name != "a" || packs[1].Name != "b" {
		t.Fatalf("sort order wrong: %v", packs)
	}
	if packs[1].Version != "2.0" {
		t.Fatalf("version not read: %v", packs[1])
	}
}

func TestInstallFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("name: from-url\ntools: {}\n"))
	}))
	defer srv.Close()

	cfgPath := tempProject(t, "")
	res, err := Install(InstallOptions{
		Source:  srv.URL + "/pack.yaml",
		CfgPath: cfgPath,
	})
	if err != nil {
		t.Fatalf("install from URL: %v", err)
	}
	if res.Header.Name != "from-url" {
		t.Fatalf("Header.Name = %q", res.Header.Name)
	}
}

func TestInstallURL404Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgPath := tempProject(t, "")
	_, err := Install(InstallOptions{
		Source:  srv.URL + "/missing.yaml",
		CfgPath: cfgPath,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestParsePackEmpty(t *testing.T) {
	_, err := ParsePack(nil)
	if err == nil {
		t.Fatal("expected error for empty pack")
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"gmail-toolkit":    "gmail-toolkit",
		"My Pack 1.0":      "My-Pack-1.0",
		"  spaces  ":       "spaces",
		"weird/path\\char": "weird-path-char",
		"---only-bad---":   "only-bad",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- Additional unit coverage ---

func TestResolveMalformedGitHub(t *testing.T) {
	// "github.com/onlyowner" has no repo segment.
	if _, err := Resolve("github.com/onlyowner"); err == nil {
		t.Fatal("expected error for github.com/<owner> with no repo")
	}
}

func TestResolveEmptySource(t *testing.T) {
	if _, err := Resolve(""); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestResolveOversizedFile(t *testing.T) {
	// Build a file larger than MaxPackSize and verify Resolve rejects it
	// before any parsing happens.
	dir := t.TempDir()
	big := filepath.Join(dir, "big.yaml")
	data := make([]byte, MaxPackSize+1024)
	for i := range data {
		data[i] = 'a'
	}
	if err := os.WriteFile(big, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(big); err == nil {
		t.Fatal("expected size-limit error")
	}
}

func TestFetchHTTPNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src, err := Resolve(srv.URL + "/pack.yaml")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := Fetch(src, srv.Client()); err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestInstallOAuthProviderConflict(t *testing.T) {
	cfgPath := tempProject(t, `
oauth_providers:
  shared:
    client_id: cid
    auth_url: https://example/auth
    token_url: https://example/token
tools: {}
`)
	pack := writePackFile(t, `
name: provider-conflict
oauth_providers:
  shared:
    client_id: cid2
    auth_url: https://other/auth
    token_url: https://other/token
tools: {}
`)
	res, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath})
	if err == nil {
		t.Fatal("expected conflict error for duplicate oauth provider")
	}
	foundProvider := false
	for _, c := range res.Conflicts {
		if c.Kind == "oauth_provider" && c.Name == "shared" {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Fatalf("expected oauth_provider conflict reported, got %v", res.Conflicts)
	}
}

func TestInstallRequiresOAuthProviderSatisfied(t *testing.T) {
	// requires.oauth_providers should resolve against the merged config
	// (existing + incoming). Here the existing config has the provider.
	cfgPath := tempProject(t, `
oauth_providers:
  google:
    client_id: x
    auth_url: https://x
    token_url: https://x
tools: {}
`)
	pack := writePackFile(t, `
name: needs-google
requires:
  oauth_providers: [google]
tools:
  my.tool:
    type: cli
    command: echo
    description: x
`)
	if _, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath}); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func TestInstallRequiresOAuthProviderShippedInSamePack(t *testing.T) {
	// A pack can declare requires.oauth_providers AND ship the provider
	// itself; the merged-view check should pass.
	cfgPath := tempProject(t, "")
	pack := writePackFile(t, `
name: self-contained
requires:
  oauth_providers: [own]
oauth_providers:
  own:
    client_id: x
    auth_url: https://x
    token_url: https://x
tools:
  my.tool:
    type: cli
    command: echo
    description: x
`)
	if _, err := Install(InstallOptions{Source: pack, CfgPath: cfgPath}); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func TestParsePackInvalidYAML(t *testing.T) {
	if _, err := ParsePack([]byte("this: is: not: valid")); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestListSkipsNonYAMLFiles(t *testing.T) {
	cfgPath := tempProject(t, "")
	// Install one real pack, then drop a stray non-yaml file.
	if _, err := Install(InstallOptions{Source: writePackFile(t, "name: real\ntools: {}\n"), CfgPath: cfgPath}); err != nil {
		t.Fatalf("install: %v", err)
	}
	dir := filepath.Join(filepath.Dir(cfgPath), ".factorly", "packs")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := List(cfgPath)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "real" {
		t.Fatalf("List() = %v, want exactly one 'real' pack", list)
	}
}

func TestListUnnamedPack(t *testing.T) {
	cfgPath := tempProject(t, "")
	// Hand-write a pack file with no name field.
	dir := filepath.Join(filepath.Dir(cfgPath), ".factorly", "packs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nameless.yaml"), []byte("tools:\n  x:\n    type: cli\n    command: echo\n    description: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := List(cfgPath)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 pack, got %d", len(list))
	}
	if !strings.HasPrefix(list[0].Name, "unnamed-") {
		t.Errorf("expected synthesized name, got %q", list[0].Name)
	}
}
