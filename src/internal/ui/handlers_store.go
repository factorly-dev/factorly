// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/store"
	"github.com/factorly-dev/factorly/internal/workspace"
)

// storeSection mirrors vaultSection. No Locked flag — store has no
// password concept; a missing store file means "no entries yet."
type storeSection struct {
	Label string
	Keys  []string
	Scope string // "project", "global", or "workspace:<name>"
}

func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	sections := s.storeSections(r)
	s.render(w, "store.html", map[string]any{
		"Title":           "Store",
		"Nav":             "store",
		"Sections":        sections,
		"ActiveWorkspace": s.requestWorkspace(r),
	})
}

// storeSections gathers the per-tier views the /store page renders.
// Ordering matches the vault page: workspace (when active) →
// project (when a project dir exists) → global (always). Each tier
// the user can write to shows up as both a "Save in" choice and a
// listed section.
//
// Per-tier opens: each section opens its bbolt file, lists keys,
// closes. bbolt holds an exclusive file lock for the lifetime of an
// open handle — caching would block concurrent factorly processes
// (CLI in another terminal, MCP server). Costs a sub-ms open per
// section per page render; not worth caching.
//
// Unlike vault, store has no locked concept. A missing file is
// just "empty" — the file will be created on first write.
func (s *Server) storeSections(r *http.Request) []storeSection {
	var sections []storeSection

	// Workspace tier — only when active.
	if name := s.requestWorkspace(r); name != "" && workspace.ValidateName(name) == nil {
		sec := storeSection{
			Label: "Workspace · " + name,
			Scope: "workspace:" + name,
		}
		if _, statErr := os.Stat(workspaceStoreFilePath(name)); statErr == nil {
			sec.Keys = listKeysFromOpener(s.storeOpener, "workspace:"+name)
		}
		sections = append(sections, sec)
	}

	// Project tier.
	if hasProjectDir() {
		sec := storeSection{Label: "Project store", Scope: "project"}
		if _, statErr := os.Stat(projectStoreFilePath()); statErr == nil {
			sec.Keys = listKeysFromOpener(s.storeOpener, "project")
		}
		sections = append(sections, sec)
	}

	// Global tier — always offered so fresh-install users have
	// somewhere to write.
	{
		sec := storeSection{Label: "Global store", Scope: "global"}
		if home, err := os.UserHomeDir(); err == nil {
			globalPath := filepath.Join(home, ".config", "factorly", "store.db")
			if _, statErr := os.Stat(globalPath); statErr == nil {
				sec.Keys = listKeysFromOpener(s.storeOpener, "global")
			}
		}
		sections = append(sections, sec)
	}

	return sections
}

// listKeysFromOpener opens the scope, lists keys, closes. Returns
// nil on any error or when the opener is unavailable — the caller
// renders an "empty section" in that case rather than surfacing the
// error in the page (a fresh project has no store file yet, which
// is normal, not exceptional).
func listKeysFromOpener(opener StoreOpener, scope string) []string {
	if opener == nil {
		return nil
	}
	b, err := opener(scope)
	if err != nil || b == nil {
		return nil
	}
	defer b.Close()
	keys, err := b.List()
	if err != nil {
		return nil
	}
	return keys
}

// projectStoreFilePath / workspaceStoreFilePath duplicate the path
// builders from cmd/factorly's store_tier.go because the UI package
// can't import cmd/factorly. Both layers must stay in sync — Step 6
// (audit-log refactor) is a good time to move the path builders into
// internal/store as a single source of truth.
func projectStoreFilePath() string {
	return filepath.Join(".factorly", "store.db")
}

func workspaceStoreFilePath(name string) string {
	if workspace.ValidateName(name) != nil {
		return ""
	}
	return filepath.Join(".factorly", "workspaces", name, "store.db")
}

// handleStoreSet writes a new entry. The form posts key + value +
// optional ttl + scope; we route to the right Manager scope and
// call SetWithTTL (or Set, when no ttl given).
func (s *Server) handleStoreSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	value := r.FormValue("value")
	scope := r.FormValue("scope")
	ttlStr := strings.TrimSpace(r.FormValue("ttl"))

	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if !validStoreScope(scope) {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}
	if s.storeOpener == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}

	backend, err := s.storeOpener(scope)
	if err != nil {
		http.Error(w, "opening store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if backend == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}
	defer backend.Close()

	ttl, hasTTL, parseErr := parseStoreTTLValue(ttlStr)
	if parseErr != nil {
		http.Error(w, parseErr.Error(), http.StatusBadRequest)
		return
	}
	if hasTTL {
		if lb, ok := backend.(*store.LocalBackend); ok {
			if err := lb.SetWithTTL(key, value, ttl); err != nil {
				http.Error(w, "store set: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "TTL not supported by this backend", http.StatusInternalServerError)
			return
		}
	} else if err := backend.Set(key, value); err != nil {
		http.Error(w, "store set: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderStoreKeys(w, r)
}

// handleStoreDelete removes an entry. Idempotent — deleting a missing
// key produces no error, matching the LocalBackend contract.
func (s *Server) handleStoreDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	scope := r.URL.Query().Get("scope")
	if key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if !validStoreScope(scope) {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}
	if s.storeOpener == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}
	backend, err := s.storeOpener(scope)
	if err != nil {
		http.Error(w, "opening store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if backend == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}
	defer backend.Close()
	if err := backend.Delete(key); err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "store delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderStoreKeys(w, r)
}

// renderStoreKeys renders just the #store-keys block — used as the
// htmx target for both set and delete so the page updates in place
// without a full reload.
func (s *Server) renderStoreKeys(w http.ResponseWriter, r *http.Request) {
	s.renderPartialInTemplate(w, "store.html", "store-keys-fragment", map[string]any{
		"Sections": s.storeSections(r),
	})
}

// renderPartialInTemplate is a thin wrapper that uses the named
// template fragment from a page template file. The store template
// defines a "store-keys-fragment" block specifically for this
// htmx-target use case; the rest of the page wraps that fragment
// for the full-page render path.
//
// Currently the template re-renders the full key list inside a
// container div. The :before-swap htmx behavior plus the
// hx-target="#store-keys" attribute means we can return the full
// sections markup and htmx swaps just the inner content.
func (s *Server) renderPartialInTemplate(w http.ResponseWriter, page, _ string, data map[string]any) {
	// For now, render the whole page and let htmx pick out the right
	// fragment via hx-select. If we need a more granular partial
	// pathway later, we can declare a named subtemplate. The page
	// template's content block is short enough that this is cheap.
	s.render(w, page, data)
}

// validStoreScope gates writes to the three known tiers. Anything
// else is operator error (typo) or attack (someone trying to write
// to an arbitrary scope name).
func validStoreScope(scope string) bool {
	switch {
	case scope == "project":
		return true
	case scope == "global":
		return true
	case strings.HasPrefix(scope, "workspace:"):
		return workspace.ValidateName(strings.TrimPrefix(scope, "workspace:")) == nil
	}
	return false
}

// parseStoreTTLValue is a local copy of the CLI's TTL parser. (Step 6
// will dedupe; for now the duplication keeps the UI package
// independent of cmd/factorly.)
func parseStoreTTLValue(s string) (time.Duration, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, nil
	}
	if s == "0" {
		return 0, true, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0, false, fmt.Errorf("invalid ttl %q: %w", s, err)
		}
		return days * 24, true, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false, fmt.Errorf("invalid ttl %q: %w", s, err)
	}
	return d, true, nil
}
