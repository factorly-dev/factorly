// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package projectpath

import (
	"os"
	"path/filepath"
	"testing"
)

const globalFallback = "/etc/factorly/state.json"

func TestResolveEmpty(t *testing.T) {
	got := Resolve("", "audit.jsonl", globalFallback)
	if got != globalFallback {
		t.Errorf("empty cfgPath: got %q, want %q", got, globalFallback)
	}
}

func TestResolveGlobalConfigUsesGlobalFallback(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cfg := filepath.Join(home, ".config", "factorly", "factorly.yaml")
	got := Resolve(cfg, "audit.jsonl", globalFallback)
	if got != globalFallback {
		t.Errorf("global cfg: got %q, want %q", got, globalFallback)
	}
}

func TestResolveTopLevelProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "factorly.yaml")
	got := Resolve(cfg, "audit.jsonl", globalFallback)
	want := filepath.Join(dir, ".factorly", "audit.jsonl")
	if got != want {
		t.Errorf("top-level cfg: got %q, want %q", got, want)
	}
}

func TestResolveDotFactorlyDirConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".factorly", "factorly.yaml")
	got := Resolve(cfg, "audit.jsonl", globalFallback)
	want := filepath.Join(dir, ".factorly", "audit.jsonl")
	if got != want {
		t.Errorf(".factorly cfg: got %q, want %q", got, want)
	}
}

func TestResolveSubdirBasename(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".factorly", "factorly.yaml")
	got := Resolve(cfg, "runs", globalFallback)
	want := filepath.Join(dir, ".factorly", "runs")
	if got != want {
		t.Errorf("subdir basename: got %q, want %q", got, want)
	}
}
