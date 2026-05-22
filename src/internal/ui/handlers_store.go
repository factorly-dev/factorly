// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
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

// handleStoreNew renders the dedicated Create Entry page. The form
// posts to POST /store/new which 303s back to /store on success.
// (POST /store still exists for the now-removed in-list form;
// keeping it for parity with vault and as a programmatic endpoint.)
func (s *Server) handleStoreNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "store_new.html", map[string]any{
		"Title":           "New Store Entry",
		"Nav":             "store",
		"Sections":        s.storeSections(r),
		"ActiveWorkspace": s.requestWorkspace(r),
	})
}

// handleStoreNewSubmit writes the new entry and 303s back to the
// list page. Plain navigation pattern — body-level hx-boost handles
// the swap. Reuses handleStoreSet's write path by re-dispatching;
// the only behavioral difference is the redirect-after-write.
func (s *Server) handleStoreNewSubmit(w http.ResponseWriter, r *http.Request) {
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
	writeErr := func() error {
		defer backend.Close()
		ttl, hasTTL, parseErr := parseStoreTTLValue(ttlStr)
		if parseErr != nil {
			return parseErr
		}
		if hasTTL {
			lb, ok := backend.(*store.LocalBackend)
			if !ok {
				return errors.New("TTL not supported by this backend")
			}
			return lb.SetWithTTL(key, value, ttl)
		}
		return backend.Set(key, value)
	}()
	if writeErr != nil {
		http.Error(w, writeErr.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/store", http.StatusSeeOther)
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

	// IIFE so one defer Close covers every error path AND releases
	// the bbolt lock before renderStoreKeys runs (which re-opens the
	// same file via listKeysFromOpener). Without this, the two opens
	// collide and the second one waits out the 2s lock timeout.
	writeErr := func() error {
		defer backend.Close()
		ttl, hasTTL, parseErr := parseStoreTTLValue(ttlStr)
		if parseErr != nil {
			return parseErr
		}
		if hasTTL {
			lb, ok := backend.(*store.LocalBackend)
			if !ok {
				return errors.New("TTL not supported by this backend")
			}
			return lb.SetWithTTL(key, value, ttl)
		}
		return backend.Set(key, value)
	}()
	if writeErr != nil {
		http.Error(w, writeErr.Error(), http.StatusBadRequest)
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
	// IIFE-with-defer closes the backend before renderStoreKeys
	// re-opens the same file — same lock-collision avoidance as the
	// Set handler.
	delErr := func() error {
		defer backend.Close()
		if err := backend.Delete(key); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return nil
	}()
	if delErr != nil {
		http.Error(w, "store delete: "+delErr.Error(), http.StatusInternalServerError)
		return
	}
	s.renderStoreKeys(w, r)
}

// renderStoreKeys writes just the per-tier sections markup that
// lives inside #store-keys — used as the htmx target for both Save
// and Delete so the list updates in place without a full reload.
//
// Hand-rolled HTML rather than a template partial: matches the
// pattern handlers_vault.go uses, and avoids the "render the whole
// layout into a small target" trap that produced visible nesting.
// Section structure must stay in sync with the {{range $section :=
// .Sections}} block in templates/store.html.
func (s *Server) renderStoreKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	sections := s.storeSections(r)
	for _, sec := range sections {
		scope := html.EscapeString(sec.Scope)
		fmt.Fprintf(w, `<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
			<div class="px-5 py-2 bg-gray-50 text-[10px] font-medium text-gray-600 uppercase tracking-wide border-b border-gray-200">%s <span class="text-gray-500">(%d keys)</span></div>`,
			html.EscapeString(sec.Label), len(sec.Keys))
		if len(sec.Keys) == 0 {
			fmt.Fprint(w, `<div class="px-5 py-4 text-center text-gray-600 text-xs">empty</div>`)
		} else {
			for _, k := range sec.Keys {
				ek := html.EscapeString(k)
				eq := url.QueryEscape(k)
				fmt.Fprintf(w, `<div class="px-5 py-2.5 border-b border-gray-100 last:border-b-0">
					<div class="flex items-center justify-between">
						<a href="/store/entry?scope=%s&key=%s" class="font-mono text-sm hover:text-indigo-600">%s</a>
						<div class="flex items-center gap-3">
							<a href="/store/entry?scope=%s&key=%s" class="text-indigo-600 hover:text-indigo-700 text-xs">view</a>
							<button hx-delete="/store/%s?scope=%s" hx-target="#store-keys" hx-swap="innerHTML" hx-confirm="Delete store entry '%s'?" class="text-red-600 hover:text-red-700 text-xs">delete</button>
						</div>
					</div>
				</div>`, scope, eq, ek, scope, eq, eq, scope, ek)
			}
		}
		fmt.Fprint(w, `</div>`)
	}
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

// handleStoreEntry renders the single-entry detail page: value
// (editable), TTL remaining badge, created/last-read timestamps,
// scope, and save/delete actions. Read-only when the storeOpener
// is unavailable.
//
// Uses LocalBackend.Entry(key) which is side-effect-free — opening
// the detail page does NOT bump LastReadAt. Get would refresh the
// TTL window; the detail page is a meta-view, not a use of the
// entry, so we keep the lifetime anchor in place.
func (s *Server) handleStoreEntry(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	key := r.URL.Query().Get("key")
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

	// LocalBackend exposes Entry; the narrower Backend interface does
	// not (it's the resolver-compatible shape). Per-op opens give us
	// a fresh concrete LocalBackend each time, so this assertion is
	// the natural seam.
	lb, ok := backend.(*store.LocalBackend)
	if !ok {
		http.Error(w, "entry metadata not supported by backend", http.StatusInternalServerError)
		return
	}
	info, err := lb.Entry(key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "reading entry: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rem, hasTTL := info.Remaining(time.Now())
	s.render(w, "store_entry.html", map[string]any{
		"Title":           "Store · " + key,
		"Nav":             "store",
		"Key":             key,
		"Scope":           scope,
		"ScopeLabel":      scopeLabel(scope),
		"Value":           info.Value,
		"CreatedAt":       info.CreatedAt,
		"LastReadAt":      info.LastReadAt,
		"TTL":             info.TTL,
		"HasTTL":          hasTTL,
		"RemainingHuman":  humanizeRemaining(rem, hasTTL),
		"ActiveWorkspace": s.requestWorkspace(r),
	})
}

// handleStoreEntryUpdate persists an edited value from the detail
// page. The form is a plain <form method="POST"> (no htmx attrs);
// body-level hx-boost intercepts it transparently and treats the
// POST + 303 + GET as a content swap. The handler stays simple:
// write, close, redirect.
func (s *Server) handleStoreEntryUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope := r.FormValue("scope")
	key := r.FormValue("key")
	value := r.FormValue("value")
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

	// IIFE so one deferred Close covers every write error path. The
	// handle is released before we return — important because the
	// browser's follow-up GET (via the 303 below) opens a fresh
	// handle, and bbolt won't tolerate two opens of the same file.
	writeErr := func() error {
		defer backend.Close()
		ttl, hasTTL, parseErr := parseStoreTTLValue(ttlStr)
		if parseErr != nil {
			return parseErr
		}
		if hasTTL {
			lb, ok := backend.(*store.LocalBackend)
			if !ok {
				return errors.New("TTL not supported by this backend")
			}
			return lb.SetWithTTL(key, value, ttl)
		}
		return backend.Set(key, value)
	}()
	if writeErr != nil {
		http.Error(w, writeErr.Error(), http.StatusBadRequest)
		return
	}

	target := "/store/entry?scope=" + url.QueryEscape(scope) + "&key=" + url.QueryEscape(key)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// handleStoreEntryDelete removes the entry and redirects back to
// the list page. Plain POST form on the detail page submits here;
// hx-boost makes it feel like an in-page navigation.
func (s *Server) handleStoreEntryDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope := r.FormValue("scope")
	key := r.FormValue("key")
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
	delErr := func() error {
		defer backend.Close()
		if err := backend.Delete(key); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		return nil
	}()
	if delErr != nil {
		http.Error(w, delErr.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/store", http.StatusSeeOther)
}

// scopeLabel renders a human-friendly version of an internal scope
// string. Mirrors the labels used in storeSections.
func scopeLabel(scope string) string {
	switch {
	case scope == "project":
		return "Project store"
	case scope == "global":
		return "Global store"
	case strings.HasPrefix(scope, "workspace:"):
		return "Workspace · " + strings.TrimPrefix(scope, "workspace:")
	}
	return scope
}

// humanizeRemaining renders the TTL-remaining duration as a short
// human string for the detail page badge. Never-expire entries
// return "never expires"; expired entries return "expired".
// Otherwise: "5d", "2h", "30m", etc., picking the largest unit
// that still gives a non-zero number.
func humanizeRemaining(d time.Duration, hasTTL bool) string {
	if !hasTTL {
		return "never expires"
	}
	if d <= 0 {
		return "expired"
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
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
