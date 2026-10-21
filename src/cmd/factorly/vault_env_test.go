// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"errors"
	"strings"
	"testing"
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
