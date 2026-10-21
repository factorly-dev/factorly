// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// vaultEnvSpec is one parsed --vault entry. Exactly one of Pattern or
// Key is set: a glob pattern expands against the vault's key list, a
// plain key exports under EnvName.
type vaultEnvSpec struct {
	EnvName string // env var to set (empty for glob specs — each match uses its own name)
	Key     string // vault key to read
	Pattern string // glob pattern (path.Match syntax) when the spec is a wildcard
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseVaultEnvSpecs parses --vault flag values. Accepted forms:
//
//	KEY            export vault key KEY as env var KEY
//	ENV=KEY        export vault key KEY as env var ENV
//	PREFIX_*       export every vault key matching the glob under its own name
//
// Env names must be valid identifiers; a vault key that isn't one has
// to be aliased with the ENV=KEY form.
func parseVaultEnvSpecs(raw []string) ([]vaultEnvSpec, error) {
	specs := make([]vaultEnvSpec, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			return nil, fmt.Errorf("invalid --vault value: empty")
		}
		if envName, key, ok := strings.Cut(r, "="); ok {
			if !envNamePattern.MatchString(envName) {
				return nil, fmt.Errorf("invalid --vault %q: %q is not a valid environment variable name", r, envName)
			}
			if key == "" || strings.ContainsAny(key, "*?[") {
				return nil, fmt.Errorf("invalid --vault %q: ENV=KEY form needs a single vault key", r)
			}
			specs = append(specs, vaultEnvSpec{EnvName: envName, Key: key})
			continue
		}
		if strings.ContainsAny(r, "*?[") {
			if _, err := path.Match(r, ""); err != nil {
				return nil, fmt.Errorf("invalid --vault pattern %q: %w", r, err)
			}
			specs = append(specs, vaultEnvSpec{Pattern: r})
			continue
		}
		if !envNamePattern.MatchString(r) {
			return nil, fmt.Errorf("invalid --vault %q: not a valid environment variable name (use ENV=%s to alias it)", r, r)
		}
		specs = append(specs, vaultEnvSpec{EnvName: r, Key: r})
	}
	return specs, nil
}

// vaultEnvRefs turns parsed specs into env entries whose values are
// {{vault:KEY}} references, ready to drop into a tool's Env map so the
// normal resolver path handles them. Glob specs are expanded against
// listKeys, which is only called when a glob is present — a plain key
// never forces the vault open early. Keys matched by a glob that are
// not valid env names are skipped with a warning via warnf.
func vaultEnvRefs(specs []vaultEnvSpec, listKeys func() ([]string, error), warnf func(string, ...any)) (map[string]string, error) {
	out := make(map[string]string, len(specs))
	var keys []string
	listed := false
	for _, s := range specs {
		if s.Pattern == "" {
			out[s.EnvName] = "{{vault:" + s.Key + "}}"
			continue
		}
		if !listed {
			var err error
			keys, err = listKeys()
			if err != nil {
				return nil, fmt.Errorf("listing vault keys for --vault %q: %w", s.Pattern, err)
			}
			sort.Strings(keys)
			listed = true
		}
		matched := 0
		for _, k := range keys {
			if ok, _ := path.Match(s.Pattern, k); !ok {
				continue
			}
			matched++
			if !envNamePattern.MatchString(k) {
				warnf("--vault %s: skipping key %q (not a valid environment variable name)", s.Pattern, k)
				continue
			}
			out[k] = "{{vault:" + k + "}}"
		}
		if matched == 0 {
			return nil, fmt.Errorf("--vault %q matched no vault keys", s.Pattern)
		}
	}
	return out, nil
}

// listCachedVaultKeys lists keys from the active vault chain (the same
// backend initResolver registers), opening it if needed.
func listCachedVaultKeys() ([]string, error) {
	backend, err := getCachedVault()
	if err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, fmt.Errorf("no vault available")
	}
	return backend.List()
}

// mergeVaultEnv folds --vault entries into an existing env map. Explicit
// --env entries win on conflict — they express more specific intent.
func mergeVaultEnv(env map[string]string, vaultEnv map[string]string) map[string]string {
	if len(vaultEnv) == 0 {
		return env
	}
	if env == nil {
		env = make(map[string]string, len(vaultEnv))
	}
	for k, v := range vaultEnv {
		if _, exists := env[k]; !exists {
			env[k] = v
		}
	}
	return env
}
