// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package packs

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/factorly-dev/factorly/internal/config"
)

// PackHeader is the identity portion of a pack: the pack's own metadata. Used
// by listings and previews.
type PackHeader struct {
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	License     string `json:"license,omitempty"`
	Filename    string `json:"filename,omitempty"` // basename on disk
}

// InstallResult describes what an install operation will do (in DryRun mode)
// or did (in commit mode). Fields are populated even on the dry-run path so
// the UI can render a preview.
type InstallResult struct {
	Header           PackHeader       `json:"header"`
	ToolsAdded       []string         `json:"tools_added,omitempty"`
	WorkflowsAdded   []string         `json:"workflows_added,omitempty"`
	ProvidersAdded   []string         `json:"providers_added,omitempty"`
	VaultBackends    []string         `json:"vault_backends,omitempty"`
	VaultKeysMissing []string         `json:"vault_keys_missing,omitempty"`
	Conflicts        []Conflict       `json:"conflicts,omitempty"`
	RequiresMissing  []MissingRequire `json:"requires_missing,omitempty"`
	// FilePath is set on a real install; it's the path on disk where the
	// pack was written.
	FilePath string `json:"file_path,omitempty"`
	// DryRun is true if no changes were written.
	DryRun bool `json:"dry_run,omitempty"`
	// AlreadyInstalled is true when a pack with this name is already on disk.
	// In dry-run, this is reported without an error so the UI can render a
	// "you already have this — uninstall first" preview. In commit mode the
	// caller also receives a non-nil error.
	AlreadyInstalled bool `json:"already_installed,omitempty"`
}

// Conflict is a name that the incoming pack would shadow.
type Conflict struct {
	Kind string `json:"kind"` // "tool", "oauth_provider", "vault_backend"
	Name string `json:"name"`
}

// MissingRequire is a Requires entry the recipient doesn't satisfy.
type MissingRequire struct {
	Kind string `json:"kind"` // "tool", "oauth_provider"
	Name string `json:"name"`
}

// InstallOptions controls what Install does.
type InstallOptions struct {
	// Source is the raw input — file path, URL, or github shorthand.
	Source string
	// CfgPath is the user's primary config file. Pack files are written
	// alongside it in <cfg-dir>/.factorly/packs/ (or <cfg-dir>/packs/ if
	// CfgPath is already inside a .factorly/ directory).
	CfgPath string
	// DryRun, when true, validates and reports without writing anything.
	DryRun bool
	// HTTPClient is used for remote fetches. nil → default with 10s timeout.
	HTTPClient *http.Client
	// BuiltinTools is the set of builtin tool names. Used to satisfy
	// workflow step references and Requires.tools entries. Pass nil if
	// builtins aren't available in the caller's context.
	BuiltinTools map[string]bool
}

// Install fetches, validates, and writes a pack. In dry-run mode it stops
// short of writing and returns a populated InstallResult so the caller (UI)
// can render a preview before committing.
//
// Vault key collection is the caller's responsibility: the returned
// InstallResult.VaultKeysMissing tells the caller which keys need values.
// The pack file is written even if vault keys aren't yet set — the tools in
// the pack will fail at execution time until the keys are provided. This
// matches existing behavior for inline vault refs.
func Install(opts InstallOptions) (*InstallResult, error) {
	if opts.CfgPath == "" {
		return nil, errors.New("packs: CfgPath required")
	}
	src, err := Resolve(opts.Source)
	if err != nil {
		return nil, err
	}
	data, err := Fetch(src, opts.HTTPClient)
	if err != nil {
		return nil, err
	}

	pack, err := ParsePack(data)
	if err != nil {
		return nil, fmt.Errorf("packs: parsing %s: %w", src.DisplayName, err)
	}

	// Compute the install file name. Prefer the pack's declared name; fall
	// back to a name derived from the source.
	installName := sanitizeName(pack.Name)
	if installName == "" {
		installName = sanitizeName(src.DisplayName)
	}
	if installName == "" {
		return nil, errors.New("packs: cannot derive an install name from the pack")
	}

	// Build the merged view of (existing project config) + (incoming pack)
	// so we can validate references and surface conflicts.
	existing, err := config.Load(opts.CfgPath)
	if err != nil {
		// A missing or empty project config is fine — install proceeds with
		// just the incoming pack as the visible state.
		existing = &config.Config{Tools: map[string]config.ToolConfig{}}
	}

	result := &InstallResult{
		Header: PackHeader{
			Name:        pack.Name,
			Version:     pack.Version,
			Description: pack.Description,
			Author:      pack.Author,
			Homepage:    pack.Homepage,
			License:     pack.License,
			Filename:    installName + ".yaml",
		},
		DryRun: opts.DryRun,
	}

	// Check for a same-named pack already on disk before walking conflicts.
	// Without this, re-installing the exact same pack produces a generic
	// "conflict with N definitions" error (because the first install's tools
	// are now in the merged config). Surface the more actionable signal.
	//
	// In dry-run mode we report this structurally on the result without an
	// error, so a UI preview can render "already installed" alongside the
	// other preview fields. In commit mode we also return an error so the
	// CLI fails fast.
	dir, err := packsDir(opts.CfgPath)
	if err != nil {
		return result, err
	}
	dst := filepath.Join(dir, installName+".yaml")
	if _, err := os.Stat(dst); err == nil {
		result.AlreadyInstalled = true
		if opts.DryRun {
			return result, nil
		}
		return result, fmt.Errorf("packs: pack %q is already installed (uninstall first or pick a different source)", installName)
	}

	// Detect conflicts. Don't fail yet — let the caller decide based on the
	// returned result whether to proceed. We do fail before writing if any
	// conflicts exist (no --force in v1).
	for name, tool := range pack.Tools {
		if _, exists := existing.Tools[name]; exists {
			result.Conflicts = append(result.Conflicts, Conflict{Kind: "tool", Name: name})
		}
		if tool.Type == "workflow" {
			result.WorkflowsAdded = append(result.WorkflowsAdded, name)
		} else {
			result.ToolsAdded = append(result.ToolsAdded, name)
		}
	}
	for name := range pack.OAuthProviders {
		if _, exists := existing.OAuthProviders[name]; exists {
			result.Conflicts = append(result.Conflicts, Conflict{Kind: "oauth_provider", Name: name})
		}
		result.ProvidersAdded = append(result.ProvidersAdded, name)
	}
	for name := range pack.VaultBackends {
		if _, exists := existing.VaultBackends[name]; exists {
			result.Conflicts = append(result.Conflicts, Conflict{Kind: "vault_backend", Name: name})
		}
		result.VaultBackends = append(result.VaultBackends, name)
	}

	sort.Strings(result.ToolsAdded)
	sort.Strings(result.WorkflowsAdded)
	sort.Strings(result.ProvidersAdded)
	sort.Strings(result.VaultBackends)

	// Validate references against the merged config.
	merged := mergeForValidation(existing, pack)
	if err := config.ValidateReferences(merged, opts.BuiltinTools); err != nil {
		// Translate workflow-ref / Requires errors into structured missing-require
		// entries when possible, so the UI can render them cleanly.
		if reqs := parseMissingRequires(err.Error()); len(reqs) > 0 {
			result.RequiresMissing = reqs
		} else {
			return result, err
		}
	}

	// Vault keys: report which Requires.vault_keys aren't present in env or
	// the user's vault. We can't check the vault from here without locking
	// it, so we conservatively list every key the pack declares and let the
	// caller intersect with the vault's actual state.
	if pack.Requires != nil {
		result.VaultKeysMissing = append(result.VaultKeysMissing, pack.Requires.VaultKeys...)
	}

	if opts.DryRun {
		return result, nil
	}

	if len(result.Conflicts) > 0 {
		return result, fmt.Errorf("packs: install would conflict with %d existing definition(s); resolve before installing", len(result.Conflicts))
	}
	if len(result.RequiresMissing) > 0 {
		return result, formatMissingRequiresError(result.RequiresMissing)
	}

	// Write the pack file. dir/dst computed earlier for the already-installed
	// check.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result, fmt.Errorf("packs: mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return result, fmt.Errorf("packs: write %s: %w", dst, err)
	}
	result.FilePath = dst
	return result, nil
}

// Uninstall removes a previously-installed pack by its install name (basename
// without extension).
func Uninstall(cfgPath, name string) error {
	dir, err := packsDir(cfgPath)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("packs: %q is not installed", name)
		}
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("packs: remove %s: %w", path, err)
	}
	return nil
}

// List returns the headers of all installed packs.
func List(cfgPath string) ([]PackHeader, error) {
	dir, err := packsDir(cfgPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("packs: reading %s: %w", dir, err)
	}
	var out []PackHeader
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".yaml" && filepath.Ext(name) != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		pack, err := ParsePack(data)
		if err != nil {
			// Surface an unidentified entry for the user to investigate.
			out = append(out, PackHeader{Filename: name})
			continue
		}
		h := PackHeader{
			Name:        pack.Name,
			Version:     pack.Version,
			Description: pack.Description,
			Author:      pack.Author,
			Homepage:    pack.Homepage,
			License:     pack.License,
			Filename:    name,
		}
		if h.Name == "" {
			h.Name = "unnamed-" + strings.TrimSuffix(name, filepath.Ext(name))
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParsePack unmarshals YAML pack bytes into a Config. Exposed for callers
// (preview handlers, tests) that have already fetched the bytes.
func ParsePack(data []byte) (*config.Config, error) {
	if len(data) == 0 {
		return nil, errors.New("packs: empty pack")
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]config.ToolConfig{}
	}
	return &cfg, nil
}

// packsDir resolves the directory where pack files are written. If CfgPath
// is inside a .factorly/ directory, packs go to <cfg-dir>/packs/. Otherwise
// they go to <cfg-dir>/.factorly/packs/ (creating .factorly/ as needed).
func packsDir(cfgPath string) (string, error) {
	cfgPath = filepath.Clean(cfgPath)
	cfgDir := filepath.Dir(cfgPath)
	if filepath.Base(cfgDir) == ".factorly" {
		return filepath.Join(cfgDir, "packs"), nil
	}
	return filepath.Join(cfgDir, ".factorly", "packs"), nil
}

// mergeForValidation produces a Config that combines existing + pack for
// purposes of reference validation. It does not mutate either input.
func mergeForValidation(existing, pack *config.Config) *config.Config {
	merged := &config.Config{
		Tools:          map[string]config.ToolConfig{},
		OAuthProviders: map[string]config.OAuthProviderConfig{},
		Requires:       pack.Requires,
	}
	for k, v := range existing.Tools {
		merged.Tools[k] = v
	}
	for k, v := range pack.Tools {
		merged.Tools[k] = v
	}
	for k, v := range existing.OAuthProviders {
		merged.OAuthProviders[k] = v
	}
	for k, v := range pack.OAuthProviders {
		merged.OAuthProviders[k] = v
	}
	return merged
}

// sanitizeName produces a filesystem-safe install name from a pack name or
// source string.
var nameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	s = nameSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-._")
	return s
}

// parseMissingRequires turns a ValidateReferences error string into structured
// MissingRequire entries when possible. If the format doesn't match, returns
// nil and the caller surfaces the raw error.
var (
	reMissingTool  = regexp.MustCompile(`tool "([^"]+)"`)
	reMissingOAuth = regexp.MustCompile(`oauth provider "([^"]+)"`)
)

func parseMissingRequires(msg string) []MissingRequire {
	var out []MissingRequire
	// Only treat as structured if the message looks like our Requires error.
	if !strings.Contains(msg, "requires tool") && !strings.Contains(msg, "requires oauth provider") {
		return nil
	}
	if m := reMissingTool.FindStringSubmatch(msg); m != nil && strings.Contains(msg, "requires tool") {
		out = append(out, MissingRequire{Kind: "tool", Name: m[1]})
	}
	if m := reMissingOAuth.FindStringSubmatch(msg); m != nil {
		out = append(out, MissingRequire{Kind: "oauth_provider", Name: m[1]})
	}
	return out
}

func formatMissingRequiresError(reqs []MissingRequire) error {
	var parts []string
	for _, r := range reqs {
		parts = append(parts, fmt.Sprintf("%s %q", r.Kind, r.Name))
	}
	return fmt.Errorf("packs: pack requires %s which is not installed", strings.Join(parts, ", "))
}
