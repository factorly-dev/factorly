// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package blueprints

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"sync"
)

// bundledFS embeds every YAML file under bundled/ into the binary so the
// catalog UI can browse them without network access. The build will fail if
// bundled/ disappears, which is what we want — these blueprints are part of
// the product surface.
//
//go:embed bundled/*.yaml
var bundledFS embed.FS

// BundledBlueprint pairs the parsed header metadata with the raw YAML bytes.
// Catalog pages use Header for display; the install path uses YAML as the
// Content argument to Install().
type BundledBlueprint struct {
	Header BlueprintHeader
	YAML   string
}

var (
	bundledOnce  sync.Once
	bundledList  []*BundledBlueprint
	bundledByKey map[string]*BundledBlueprint
	bundledErr   error
)

// loadBundled parses every file in bundled/ exactly once. Files that fail to
// parse are skipped — they're authored in-repo, so a parse error is a build
// problem, not a runtime problem, and surfacing it here would break the
// catalog page for the user. We do record the first error for tests/debug.
func loadBundled() {
	bundledOnce.Do(func() {
		entries, err := bundledFS.ReadDir("bundled")
		if err != nil {
			bundledErr = fmt.Errorf("blueprints: reading bundled FS: %w", err)
			return
		}

		bundledByKey = make(map[string]*BundledBlueprint, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := path.Ext(e.Name())
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			data, err := bundledFS.ReadFile("bundled/" + e.Name())
			if err != nil {
				if bundledErr == nil {
					bundledErr = fmt.Errorf("blueprints: reading bundled/%s: %w", e.Name(), err)
				}
				continue
			}
			bp, err := ParseBlueprint(data)
			if err != nil {
				if bundledErr == nil {
					bundledErr = fmt.Errorf("blueprints: parsing bundled/%s: %w", e.Name(), err)
				}
				continue
			}
			// Bundled blueprints must declare a name — they're authored
			// in-repo, so this is a build-time invariant, not a recovery
			// case. Skip files missing it so the catalog stays usable.
			if bp.Name == "" {
				if bundledErr == nil {
					bundledErr = fmt.Errorf("blueprints: bundled/%s missing name", e.Name())
				}
				continue
			}
			b := &BundledBlueprint{
				Header: BlueprintHeader{
					Name:        bp.Name,
					DisplayName: bp.DisplayName,
					Version:     bp.Version,
					Description: bp.Description,
					Category:    bp.Category,
					Author:      bp.Author,
					Homepage:    bp.Homepage,
					License:     bp.License,
					AuthType:    bp.AuthType,
					AuthGuide:   bp.AuthGuide,
					Filename:    e.Name(),
				},
				YAML: string(data),
			}
			bundledList = append(bundledList, b)
			bundledByKey[bp.Name] = b
		}
		sort.Slice(bundledList, func(i, j int) bool {
			return bundledList[i].Header.Name < bundledList[j].Header.Name
		})
	})
}

// Bundled returns all blueprints baked into the binary, sorted by name.
func Bundled() []*BundledBlueprint {
	loadBundled()
	return bundledList
}

// BundledByName returns the bundled blueprint with the given name, or nil if
// none matches.
func BundledByName(name string) *BundledBlueprint {
	loadBundled()
	return bundledByKey[name]
}

// BundledLoadError exposes the first parse/read error encountered when the
// bundled set was loaded. Useful in tests; non-nil here is a build problem.
func BundledLoadError() error {
	loadBundled()
	return bundledErr
}
