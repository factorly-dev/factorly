// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Package projectpath resolves where project-scoped state files live.
// Audit logs, rate-limit buckets, workflow run state — all follow the
// same rule: if the active config is under a project's .factorly/
// dir (or alongside it), keep state next to it so it travels with the
// repo; otherwise fall back to ~/.config/factorly/.
//
// The logger and shadow packages reimplement this logic for the file
// paths they own; this package is the home for new state files added
// after the third one.
package projectpath

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns the directory or file path that pairs with the
// given config. Behaviour:
//
//   - cfgPath == "" or under ~/.config/factorly/ → globalPath
//   - cfgPath in a .factorly/ project dir → <project>/.factorly/<basename>
//   - cfgPath at a project root → <project>/.factorly/<basename>
//
// basename is the final path component (a filename or a subdir name).
// globalPath is the fallback used when no project applies.
func Resolve(cfgPath, basename, globalPath string) string {
	if cfgPath == "" {
		return globalPath
	}
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return globalPath
	}
	if home, err := os.UserHomeDir(); err == nil {
		globalDir, err := filepath.Abs(filepath.Join(home, ".config", "factorly"))
		if err == nil {
			rel, err := filepath.Rel(globalDir, abs)
			if err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
				return globalPath
			}
		}
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) == ".factorly" {
		return filepath.Join(dir, basename)
	}
	return filepath.Join(dir, ".factorly", basename)
}
