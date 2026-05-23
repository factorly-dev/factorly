// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/factorly-dev/factorly/internal/vault"
)

type vaultSection struct {
	Label string
	Keys  []string
	Scope string // "project", "global", "workspace:<name>", or "default"
	// Locked is true when the section's vault file exists but no
	// non-interactive password source (env var / keyfile / cached
	// backend) was usable. The UI surfaces an inline unlock prompt.
	Locked bool
}

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	sections := s.vaultSections(r)
	s.render(w, "vault.html", map[string]any{
		"Title":    "Vault",
		"Nav":      "vault",
		"Sections": sections,
	})
}

// handleVaultNew renders the dedicated Create Secret page. Form posts
// to POST /vault/new which 303s back to /vault on success, or
// surfaces the unlock prompt full-page when the target tier is
// locked. Distinct from POST /vault (fragment swap for in-list
// edits / the "replace" button).
func (s *Server) handleVaultNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "vault_new.html", map[string]any{
		"Title":    "Create Vault Secret",
		"Nav":      "vault",
		"Sections": s.vaultSections(r),
	})
}

// handleVaultNewSubmit writes a new secret and 303s back to the
// list page. Locked vault: render the unlock prompt inline as a
// full page (preserves the key+value so the user doesn't lose
// their work to a password prompt).
func (s *Server) handleVaultNewSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := r.FormValue("key")
	value := r.FormValue("value")
	scope := r.FormValue("scope")
	if key == "" || value == "" {
		http.Error(w, "key and value required", http.StatusBadRequest)
		return
	}
	backend, err := s.resolveVaultBackend(scope)
	if err != nil {
		if isVaultLockedErr(err) {
			// Locked: render the unlock partial as a full page so the
			// user gets the layout and nav back. The partial carries
			// the key/value forward through the unlock flow.
			s.renderVaultUnlockPartial(w, vaultUnlockData{
				Scope: scope,
				Key:   key,
				Value: value,
				Op:    "set",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if backend == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}
	if err := backend.Set(key, value); err != nil {
		http.Error(w, fmt.Sprintf("vault set: %v", err), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/vault", http.StatusSeeOther)
}

// vaultSections collects every vault tier the user can read from or
// write to. Each entry shows up as both a "Store in" choice and a
// listed section on the page.
//
// Ordering: workspace (if active) → project (always, when a project
// dir exists) → global (always — fresh-install users need somewhere
// to write).
func (s *Server) vaultSections(r *http.Request) []vaultSection {
	var sections []vaultSection

	// Workspace tier — only when active.
	if name := s.requestWorkspace(r); name != "" {
		sec := vaultSection{
			Label: "Workspace · " + name,
			Scope: "workspace:" + name,
		}
		if _, statErr := os.Stat(workspaceVaultFilePath(name)); statErr == nil {
			b, openErr := s.vaultMgr.GetOrOpen("workspace:" + name)
			switch {
			case openErr == nil && b != nil:
				if keys, listErr := b.List(); listErr == nil {
					sec.Keys = keys
				}
			case isVaultLockedErr(openErr):
				sec.Locked = true
			}
		}
		// File missing → empty section, ready to upsert on first write.
		sections = append(sections, sec)
	}

	// Project tier.
	if hasProjectDir() {
		sec := vaultSection{Label: "Project vault", Scope: "project"}
		if _, statErr := os.Stat(projectVaultFilePath()); statErr == nil {
			b, openErr := s.vaultMgr.GetOrOpen("project")
			switch {
			case openErr == nil && b != nil:
				if keys, listErr := b.List(); listErr == nil {
					sec.Keys = keys
				}
			case isVaultLockedErr(openErr):
				sec.Locked = true
			}
		}
		sections = append(sections, sec)
	}

	// Global tier. Always show — fresh installs need somewhere to land.
	{
		sec := vaultSection{Label: "Global vault", Scope: "global"}
		if cached := s.vaultMgr.Cached("global"); cached != nil {
			if keys, err := cached.List(); err == nil {
				sec.Keys = keys
			}
		} else if _, statErr := os.Stat(vault.DefaultVaultPath()); statErr == nil {
			b, openErr := s.vaultMgr.GetOrOpen("global")
			switch {
			case openErr == nil && b != nil:
				if keys, listErr := b.List(); listErr == nil {
					sec.Keys = keys
				}
			case isVaultLockedErr(openErr):
				sec.Locked = true
			}
		}
		sections = append(sections, sec)
	}

	// Fallback for explicit --vault-path or other one-off setups.
	if len(sections) == 0 && s.vault != nil {
		if keys, err := s.vault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Vault", Keys: keys, Scope: "default"})
		}
	}

	return sections
}

func hasProjectDir() bool {
	info, err := os.Stat(".factorly")
	return err == nil && info.IsDir()
}

func projectVaultFilePath() string {
	return filepath.Join(".factorly", "vault.enc")
}

func workspaceVaultFilePath(name string) string {
	return filepath.Join(".factorly", "vaults", name+".enc")
}

// isVaultLockedErr matches errors that should produce an unlock
// prompt in the UI:
//   - "<tier> vault locked: password required" — no non-interactive
//     password source resolved (env var, keyfile).
//   - vault.ErrWrongPassword — a password was found but it didn't
//     decrypt the file. Same UX answer: prompt for the right one.
func isVaultLockedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, vault.ErrWrongPassword) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "vault locked") ||
		strings.Contains(msg, "decrypting vault") ||
		strings.Contains(msg, "wrong password")
}

// unlockErrorMessage classifies an OpenWithPassword failure for the
// inline unlock form. ErrWrongPassword gets a user-friendly retry
// nudge; everything else surfaces its underlying error so the user
// can see what actually broke (corrupt file, I/O failure, etc.)
// rather than getting a misleading "incorrect password" prompt.
//
// Returns "" when err is nil so callers can ignore-check.
func unlockErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, vault.ErrWrongPassword) {
		return "Incorrect password — try again."
	}
	return "Failed to open vault: " + err.Error()
}

func (s *Server) handleVaultSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	key := r.FormValue("key")
	value := r.FormValue("value")
	scope := r.FormValue("scope")
	if key == "" || value == "" {
		http.Error(w, "key and value required", http.StatusBadRequest)
		return
	}

	backend, err := s.resolveVaultBackend(scope)
	if err != nil {
		if isVaultLockedErr(err) {
			s.renderVaultUnlockPartial(w, vaultUnlockData{
				Scope: scope,
				Key:   key,
				Value: value,
				Op:    "set",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if backend == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	if err := backend.Set(key, value); err != nil {
		http.Error(w, fmt.Sprintf("vault set: %v", err), http.StatusInternalServerError)
		return
	}

	s.renderVaultKeys(w, r)
}

func (s *Server) handleVaultDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	scope := r.URL.Query().Get("scope")

	backend, err := s.resolveVaultBackend(scope)
	if err != nil {
		if isVaultLockedErr(err) {
			s.renderVaultUnlockPartial(w, vaultUnlockData{
				Scope: scope,
				Key:   key,
				Op:    "delete",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if backend == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	if err := backend.Delete(key); err != nil {
		http.Error(w, fmt.Sprintf("vault delete: %v", err), http.StatusInternalServerError)
		return
	}

	s.renderVaultKeys(w, r)
}

// resolveVaultBackend returns a write-capable backend for the given
// scope. Project / workspace / global tiers are upserted on demand
// using the CLI's non-interactive password chain. Errors with the
// "vault locked" sentinel signal that the UI should prompt the user
// for a password; callers translate those into the unlock partial.
func (s *Server) resolveVaultBackend(scope string) (vault.Backend, error) {
	switch {
	case strings.HasPrefix(scope, "workspace:"), scope == "project", scope == "global":
		return s.vaultMgr.GetOrOpen(scope)
	}
	return s.vault, nil
}

// vaultUnlockData carries the state for an inline password prompt.
// The carry-through fields (Key, Value, Op) let the unlock handler
// complete the user's deferred operation in a single round-trip after
// they supply the password.
type vaultUnlockData struct {
	Scope string // workspace:<name> | project | global
	Key   string // for set/delete: the key being written/removed
	Value string // for set: the value being stored
	Op    string // "set", "delete", or "" for plain unlock-to-list
	Error string // shown when a previous unlock attempt failed
}

// renderVaultUnlockPartial emits the inline password form. htmx swaps
// it in place of the result area; the form posts back to /vault/unlock
// which completes the deferred operation on success.
func (s *Server) renderVaultUnlockPartial(w http.ResponseWriter, d vaultUnlockData) {
	s.renderPartial(w, "vault_unlock", map[string]any{
		"Scope": d.Scope,
		"Label": vaultScopeLabel(d.Scope),
		"Key":   d.Key,
		"Value": d.Value,
		"Op":    d.Op,
		"Error": d.Error,
	})
}

func vaultScopeLabel(scope string) string {
	switch {
	case strings.HasPrefix(scope, "workspace:"):
		return "Workspace · " + strings.TrimPrefix(scope, "workspace:")
	case scope == "project":
		return "Project vault"
	case scope == "global":
		return "Global vault"
	}
	return scope
}

// handleVaultUnlock takes a password + the carry-through fields and
// either lists the unlocked tier (op == "" or "list") or completes
// the user's original set/delete.
func (s *Server) handleVaultUnlock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope := r.FormValue("scope")
	password := r.FormValue("password")
	op := r.FormValue("op")
	key := r.FormValue("key")
	value := r.FormValue("value")

	if scope == "" || password == "" {
		s.renderVaultUnlockPartial(w, vaultUnlockData{
			Scope: scope, Op: op, Key: key, Value: value,
			Error: "password is required",
		})
		return
	}

	pw := vault.SecretFromString(password)
	defer pw.Zero()
	backend, err := s.vaultMgr.OpenWithPassword(scope, pw)
	if err != nil {
		s.renderVaultUnlockPartial(w, vaultUnlockData{
			Scope: scope, Op: op, Key: key, Value: value,
			Error: unlockErrorMessage(err),
		})
		return
	}
	s.vaultMgr.Put(scope, backend)

	// Complete the deferred operation.
	switch op {
	case "set":
		if key == "" || value == "" {
			s.renderVaultKeys(w, r)
			return
		}
		if err := backend.Set(key, value); err != nil {
			http.Error(w, fmt.Sprintf("vault set: %v", err), http.StatusInternalServerError)
			return
		}
	case "delete":
		if key == "" {
			s.renderVaultKeys(w, r)
			return
		}
		if err := backend.Delete(key); err != nil {
			http.Error(w, fmt.Sprintf("vault delete: %v", err), http.StatusInternalServerError)
			return
		}
	}

	s.renderVaultKeys(w, r)
}

func (s *Server) renderVaultKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	sections := s.vaultSections(r)

	if len(sections) == 0 {
		fmt.Fprint(w, `<div class="px-5 py-8 text-center text-gray-600 text-sm">No secrets stored.</div>`)
		return
	}

	for _, sec := range sections {
		header := fmt.Sprintf("%s <span class=\"text-gray-500\">(%d keys)</span>", html.EscapeString(sec.Label), len(sec.Keys))
		if sec.Locked {
			header = fmt.Sprintf("%s <span class=\"text-amber-500\">(locked)</span>", html.EscapeString(sec.Label))
		}
		fmt.Fprintf(w, `<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
			<div class="px-5 py-2 bg-gray-50 text-[10px] font-medium text-gray-500 uppercase tracking-wide border-b border-gray-200">%s</div>`, header)
		if sec.Locked {
			fmt.Fprintf(w, `<div class="px-5 py-3">
				<button type="button" hx-post="/vault/unlock-form" hx-target="#vault-keys" hx-swap="innerHTML" hx-vals='{"scope":"%s"}'
				        class="text-xs text-indigo-600 hover:text-indigo-800">Unlock to view secrets</button>
			</div>`, html.EscapeString(sec.Scope))
		} else if len(sec.Keys) == 0 {
			fmt.Fprint(w, `<div class="px-5 py-4 text-center text-gray-600 text-xs">empty</div>`)
		} else {
			for _, key := range sec.Keys {
				esc := html.EscapeString(key)
				fmt.Fprintf(w, `<div class="px-5 py-2.5 flex items-center justify-between border-b border-gray-100 last:border-b-0">
					<span class="font-mono text-sm">%s</span>
					<div class="flex items-center gap-3">
						<span class="text-gray-500 text-sm">••••••••</span>
						<button hx-delete="/vault/%s?scope=%s"
								hx-target="#vault-keys"
								hx-swap="innerHTML"
								hx-confirm="Delete secret &#39;%s&#39;?"
								class="text-red-600 hover:text-red-700 text-xs">delete</button>
					</div>
				</div>`, esc, esc, html.EscapeString(sec.Scope), esc)
			}
		}
		fmt.Fprint(w, `</div>`)
	}
}

// handleVaultUnlockForm renders just the unlock prompt for the given
// scope. Used by the "Unlock to view secrets" button on locked
// sections — produces the same partial the set/delete handlers
// return when they hit a locked tier.
func (s *Server) handleVaultUnlockForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scope := r.FormValue("scope")
	if scope == "" {
		http.Error(w, "scope required", http.StatusBadRequest)
		return
	}
	s.renderVaultUnlockPartial(w, vaultUnlockData{Scope: scope, Op: "list"})
}

// lockedTier describes a vault tier that exists on disk but hasn't
// been unlocked yet. Used by the startup unlock modal.
type lockedTier struct {
	Scope string // workspace:<name> | project | global
	Label string // user-facing label
}

// lockedTiers walks the vault tiers the UI knows about and returns
// only those that have a file on disk but no successful open. Tiers
// without a vault file are intentionally omitted — there's nothing
// to "unlock" until the user creates one.
func (s *Server) lockedTiers(r *http.Request) []lockedTier {
	var out []lockedTier
	for _, sec := range s.vaultSections(r) {
		if sec.Locked {
			out = append(out, lockedTier{Scope: sec.Scope, Label: sec.Label})
		}
	}
	return out
}

// unlockModalDismissedCookieName tracks whether the user has dismissed
// the startup unlock modal for this session. Re-shown on /vault visits
// per the design.
const unlockModalDismissedCookieName = "factorly_unlock_dismissed"

// handleVaultUnlockModal returns the modal HTML when any tier is
// locked AND the user hasn't dismissed it. Returns an empty body
// otherwise so the htmx swap does nothing.
//
// The modal endpoint also reads ?force=1 — set by the /vault page so
// dismissal isn't honored once the user is actively trying to manage
// secrets.
func (s *Server) handleVaultUnlockModal(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1"
	if !force {
		if c, err := r.Cookie(unlockModalDismissedCookieName); err == nil && c.Value == "1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	tiers := s.lockedTiers(r)
	if len(tiers) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.renderPartial(w, "vault_unlock_modal", map[string]any{
		"Tiers": tiers,
	})
}

// handleVaultUnlockAll takes one password and tries each locked tier.
// Returns a result partial summarizing per-tier outcomes. Successful
// opens are cached so subsequent vault reads/writes don't re-prompt.
func (s *Server) handleVaultUnlockAll(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	type tierResult struct {
		Label   string
		Scope   string
		Success bool
		Error   string
	}

	var results []tierResult
	pw := vault.SecretFromString(password)
	defer pw.Zero()
	for _, t := range s.lockedTiers(r) {
		backend, err := s.vaultMgr.OpenWithPassword(t.Scope, pw)
		if err != nil {
			results = append(results, tierResult{
				Label: t.Label, Scope: t.Scope, Success: false,
				Error: unlockErrorMessage(err),
			})
			continue
		}
		s.vaultMgr.Put(t.Scope, backend)
		results = append(results, tierResult{Label: t.Label, Scope: t.Scope, Success: true})
	}

	allSucceeded := true
	for _, r := range results {
		if !r.Success {
			allSucceeded = false
			break
		}
	}

	s.renderPartial(w, "vault_unlock_result", map[string]any{
		"Results":      results,
		"AllSucceeded": allSucceeded,
	})
}

// handleVaultUnlockDismiss sets the dismissal cookie so the modal
// doesn't re-appear on subsequent page loads (except /vault which
// passes ?force=1).
func (s *Server) handleVaultUnlockDismiss(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     unlockModalDismissedCookieName,
		Value:    "1",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
