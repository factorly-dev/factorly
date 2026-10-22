// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/factorly-dev/factorly/internal/vault"
)

func TestParseVaultEnvSpecsForms(t *testing.T) {
	specs, err := parseVaultEnvSpecs([]string{"GITHUB_TOKEN", "GH_TOKEN=github-token", "STRIPE_*"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
	if specs[0].EnvName != "GITHUB_TOKEN" || specs[0].Key != "GITHUB_TOKEN" {
		t.Errorf("bare form parsed wrong: %+v", specs[0])
	}
	if specs[1].EnvName != "GH_TOKEN" || specs[1].Key != "github-token" {
		t.Errorf("alias form parsed wrong: %+v", specs[1])
	}
	if specs[2].Pattern != "STRIPE_*" || specs[2].EnvName != "" {
		t.Errorf("glob form parsed wrong: %+v", specs[2])
	}
}

func TestParseVaultEnvSpecsRejects(t *testing.T) {
	cases := map[string]string{
		"":             "empty",
		"github-token": "not a valid environment variable name",
		"1ABC":         "not a valid environment variable name",
		"bad-name=KEY": "not a valid environment variable name",
		"ENV=":         "single vault key",
		"ENV=STRIPE_*": "single vault key",
		"[":            "invalid --vault pattern",
	}
	for in, want := range cases {
		_, err := parseVaultEnvSpecs([]string{in})
		if err == nil {
			t.Errorf("%q: expected error", in)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %q does not mention %q", in, err.Error(), want)
		}
	}
}

func TestVaultEnvRefsPlainKeysNeverList(t *testing.T) {
	specs, _ := parseVaultEnvSpecs([]string{"A", "B=C"})
	listed := false
	refs, err := vaultEnvRefs(specs, func() ([]string, error) { listed = true; return nil, nil }, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if listed {
		t.Error("plain keys must not list the vault")
	}
	if refs["A"] != "{{vault:A}}" || refs["B"] != "{{vault:C}}" {
		t.Errorf("unexpected refs: %v", refs)
	}
}

func TestVaultEnvRefsGlob(t *testing.T) {
	specs, _ := parseVaultEnvSpecs([]string{"STRIPE_*"})
	var warnings []string
	list := func() ([]string, error) {
		return []string{"STRIPE_KEY", "STRIPE_WEBHOOK", "stripe-legacy", "GITHUB_TOKEN"}, nil
	}
	refs, err := vaultEnvRefs(specs, list, func(f string, a ...any) { warnings = append(warnings, f) })
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs["STRIPE_KEY"] == "" || refs["STRIPE_WEBHOOK"] == "" {
		t.Errorf("unexpected refs: %v", refs)
	}
	if _, ok := refs["GITHUB_TOKEN"]; ok {
		t.Error("glob must not match unrelated keys")
	}
	if len(warnings) != 0 {
		t.Errorf("stripe-legacy does not match STRIPE_* so no warning expected, got %v", warnings)
	}

	// Case where a matching key is not a valid env name → warned and skipped
	specs, _ = parseVaultEnvSpecs([]string{"stripe*"})
	warnings = nil
	refs, err = vaultEnvRefs(specs, list, func(f string, a ...any) { warnings = append(warnings, f) })
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 || len(warnings) != 1 {
		t.Errorf("expected 0 refs and 1 warning, got %v / %v", refs, warnings)
	}
}

func TestVaultEnvRefsGlobNoMatch(t *testing.T) {
	specs, _ := parseVaultEnvSpecs([]string{"NOPE_*"})
	_, err := vaultEnvRefs(specs, func() ([]string, error) { return []string{"A"}, nil }, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "matched no vault keys") {
		t.Errorf("expected no-match error, got %v", err)
	}
}

func TestVaultEnvRefsListError(t *testing.T) {
	specs, _ := parseVaultEnvSpecs([]string{"*"})
	_, err := vaultEnvRefs(specs, func() ([]string, error) { return nil, errors.New("locked") }, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("expected list error surfaced, got %v", err)
	}
}

func TestMergeVaultEnvExplicitWins(t *testing.T) {
	env := map[string]string{"TOKEN": "explicit"}
	out := mergeVaultEnv(env, map[string]string{"TOKEN": "{{vault:TOKEN}}", "OTHER": "{{vault:OTHER}}"})
	if out["TOKEN"] != "explicit" {
		t.Errorf("--env must win over --vault, got %q", out["TOKEN"])
	}
	if out["OTHER"] != "{{vault:OTHER}}" {
		t.Errorf("missing merged key: %v", out)
	}
	if got := mergeVaultEnv(nil, map[string]string{"A": "x"}); got["A"] != "x" {
		t.Errorf("nil env should be created: %v", got)
	}
}

func TestParseEnvFlags(t *testing.T) {
	explicit, forward, err := parseEnvFlags([]string{"A=1", "DISPLAY", "B=x=y", "SSH_AUTH_SOCK"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit["A"] != "1" || explicit["B"] != "x=y" {
		t.Errorf("explicit parsed wrong: %v", explicit)
	}
	if len(forward) != 2 || forward[0] != "DISPLAY" || forward[1] != "SSH_AUTH_SOCK" {
		t.Errorf("forward parsed wrong: %v", forward)
	}
	for _, bad := range []string{"=x", "not-a-name", "1ABC", ""} {
		if _, _, err := parseEnvFlags([]string{bad}); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	explicit, forward, err = parseEnvFlags(nil)
	if err != nil || explicit != nil || forward != nil {
		t.Errorf("nil input should yield nil/nil/nil, got %v %v %v", explicit, forward, err)
	}
}

func TestForwardEnvUsesResolverOverlayAndSkipsUnset(t *testing.T) {
	t.Setenv("FWD_FROM_OS", "os-value")
	t.Setenv("FWD_OVERLAID", "os-value")
	resolver := vault.NewResolver()
	resolver.Register("env", vault.EnvBackendWithOverrides{Overrides: map[string]string{"FWD_OVERLAID": "workspace-value"}})

	var skipped []string
	env := map[string]string{"FWD_FROM_OS": "{{vault:FWD_FROM_OS}}"} // simulates a --vault entry for the same name
	env = forwardEnv(env, []string{"FWD_FROM_OS", "FWD_OVERLAID", "FWD_UNSET_XYZ"}, resolver, func(f string, a ...any) { skipped = append(skipped, f) })

	if env["FWD_FROM_OS"] != "os-value" {
		t.Errorf("bare --env should win over a --vault entry, got %q", env["FWD_FROM_OS"])
	}
	if env["FWD_OVERLAID"] != "workspace-value" {
		t.Errorf("workspace overlay should win over os env, got %q", env["FWD_OVERLAID"])
	}
	if _, ok := env["FWD_UNSET_XYZ"]; ok {
		t.Error("unset variable must not be forwarded")
	}
	if len(skipped) != 1 {
		t.Errorf("expected one skip log, got %v", skipped)
	}
	if got := forwardEnv(nil, []string{"FWD_FROM_OS"}, resolver, func(string, ...any) {}); got["FWD_FROM_OS"] != "os-value" {
		t.Errorf("nil env should be created: %v", got)
	}
}

func TestEscapeResolvedEnv(t *testing.T) {
	env := escapeResolvedEnv(map[string]string{
		"PLAIN": "value",
		"TMPL":  "{{vault:X}} and {{env:Y}}",
	})
	if env["PLAIN"] != "value" {
		t.Errorf("plain value changed: %q", env["PLAIN"])
	}
	if env["TMPL"] != `\{{vault:X}} and \{{env:Y}}` {
		t.Errorf("braces not escaped: %q", env["TMPL"])
	}
	// One resolver pass restores the literal braces without resolving.
	r := vault.NewResolver()
	r.Register("env", vault.EnvBackend{})
	got, err := r.Resolve(env["TMPL"])
	if err != nil || got != "{{vault:X}} and {{env:Y}}" {
		t.Errorf("round trip through Resolve = %q, %v", got, err)
	}
	if escapeResolvedEnv(nil) != nil {
		t.Error("nil map should stay nil")
	}
}
