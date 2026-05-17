// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package shadow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRateStorePathEmpty(t *testing.T) {
	got := ProjectRateStorePath("")
	want := DefaultRateStorePath()
	if got != want {
		t.Errorf("empty cfgPath: got %q, want %q", got, want)
	}
}

func TestProjectRateStorePathGlobalConfigYieldsGlobalState(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cfg := filepath.Join(home, ".config", "factorly", "factorly.yaml")
	got := ProjectRateStorePath(cfg)
	want := DefaultRateStorePath()
	if got != want {
		t.Errorf("global cfg: got %q, want %q", got, want)
	}
}

func TestProjectRateStorePathTopLevelProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "factorly.yaml")
	got := ProjectRateStorePath(cfg)
	want := filepath.Join(dir, ".factorly", "ratelimit.json")
	if got != want {
		t.Errorf("top-level cfg: got %q, want %q", got, want)
	}
}

func TestProjectRateStorePathDotFactorlyDirConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".factorly", "factorly.yaml")
	got := ProjectRateStorePath(cfg)
	want := filepath.Join(dir, ".factorly", "ratelimit.json")
	if got != want {
		t.Errorf(".factorly cfg: got %q, want %q", got, want)
	}
}
