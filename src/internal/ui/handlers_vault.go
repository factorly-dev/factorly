// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
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
			b, openErr := s.openWorkspaceVaultForRead(name)
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
			b, openErr := s.openProjectVaultForRead()
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
		switch {
		case s.globalVault != nil:
			if keys, err := s.globalVault.List(); err == nil {
				sec.Keys = keys
			}
		default:
			if _, statErr := os.Stat(vault.DefaultVaultPath()); statErr == nil {
				b, openErr := s.openGlobalVaultForRead()
				switch {
				case openErr == nil && b != nil:
					if keys, listErr := b.List(); listErr == nil {
						sec.Keys = keys
					}
				case isVaultLockedErr(openErr):
					sec.Locked = true
				}
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
//   - "decrypting vault (wrong password?)" — a password was found but
//     it didn't decrypt the file. Same UX answer: prompt for the
//     right one.
func isVaultLockedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "vault locked") ||
		strings.Contains(msg, "decrypting vault") ||
		strings.Contains(msg, "wrong password")
}

// openWorkspaceVaultForRead returns a cached backend if present,
// otherwise tries the non-interactive opener. Caches successful opens.
func (s *Server) openWorkspaceVaultForRead(name string) (vault.Backend, error) {
	if b := s.cachedWorkspaceVault(name); b != nil {
		return b, nil
	}
	if s.WorkspaceVaultUpsertOpener == nil {
		return nil, nil
	}
	b, err := s.WorkspaceVaultUpsertOpener(name)
	if err == nil && b != nil {
		s.cacheWorkspaceVault(name, b)
	}
	return b, err
}

// openProjectVaultForRead returns the startup-opened project vault
// if present, otherwise tries the non-interactive opener.
func (s *Server) openProjectVaultForRead() (vault.Backend, error) {
	if s.projectVault != nil {
		return s.projectVault, nil
	}
	if s.ProjectVaultOpener == nil {
		return nil, nil
	}
	b, err := s.ProjectVaultOpener()
	if err == nil {
		s.projectVault = b
	}
	return b, err
}

// openGlobalVaultForRead — same pattern for the global tier.
func (s *Server) openGlobalVaultForRead() (vault.Backend, error) {
	if s.globalVault != nil {
		return s.globalVault, nil
	}
	if s.GlobalVaultOpener == nil {
		return nil, nil
	}
	b, err := s.GlobalVaultOpener()
	if err == nil {
		s.globalVault = b
	}
	return b, err
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
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		if s.WorkspaceVaultUpsertOpener == nil {
			return nil, fmt.Errorf("workspace vault not configured")
		}
		b, err := s.WorkspaceVaultUpsertOpener(name)
		if err != nil {
			return nil, err
		}
		return b, nil

	case scope == "project":
		if s.projectVault != nil {
			return s.projectVault, nil
		}
		if s.ProjectVaultOpener == nil {
			return nil, fmt.Errorf("project vault not configured")
		}
		b, err := s.ProjectVaultOpener()
		if err != nil {
			return nil, err
		}
		s.projectVault = b
		return b, nil

	case scope == "global":
		if s.globalVault != nil {
			return s.globalVault, nil
		}
		if s.GlobalVaultOpener == nil {
			return nil, fmt.Errorf("global vault not configured")
		}
		b, err := s.GlobalVaultOpener()
		if err != nil {
			return nil, err
		}
		s.globalVault = b
		return b, nil
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

	backend, err := s.openVaultWithPassword(scope, []byte(password))
	if err != nil {
		s.renderVaultUnlockPartial(w, vaultUnlockData{
			Scope: scope, Op: op, Key: key, Value: value,
			Error: "incorrect password",
		})
		return
	}
	s.cacheUnlockedBackend(scope, backend)

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

// openVaultWithPassword opens the given vault tier using an explicit
// password supplied through the unlock form.
func (s *Server) openVaultWithPassword(scope string, password []byte) (vault.Backend, error) {
	switch {
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		if s.WorkspacePasswordOpener == nil {
			return nil, fmt.Errorf("workspace password unlock not configured")
		}
		return s.WorkspacePasswordOpener(name, password)
	case scope == "project":
		if s.ProjectVaultPasswordOpener == nil {
			return nil, fmt.Errorf("project password unlock not configured")
		}
		return s.ProjectVaultPasswordOpener(password)
	case scope == "global":
		if s.GlobalVaultPasswordOpener == nil {
			return nil, fmt.Errorf("global password unlock not configured")
		}
		return s.GlobalVaultPasswordOpener(password)
	}
	return nil, fmt.Errorf("unknown scope %q", scope)
}

// cacheUnlockedBackend stores the opened backend so subsequent reads
// and writes don't re-prompt for the password.
func (s *Server) cacheUnlockedBackend(scope string, b vault.Backend) {
	switch {
	case strings.HasPrefix(scope, "workspace:"):
		name := strings.TrimPrefix(scope, "workspace:")
		s.cacheWorkspaceVault(name, b)
	case scope == "project":
		s.projectVault = b
	case scope == "global":
		s.globalVault = b
	}
}

func (s *Server) renderVaultKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	sections := s.vaultSections(r)

	if len(sections) == 0 {
		fmt.Fprint(w, `<div class="px-5 py-8 text-center text-gray-400 text-sm">No secrets stored.</div>`)
		return
	}

	for _, sec := range sections {
		header := fmt.Sprintf("%s <span class=\"text-gray-300\">(%d keys)</span>", html.EscapeString(sec.Label), len(sec.Keys))
		if sec.Locked {
			header = fmt.Sprintf("%s <span class=\"text-amber-500\">(locked)</span>", html.EscapeString(sec.Label))
		}
		fmt.Fprintf(w, `<div class="border-b border-gray-200 last:border-b-0">
			<div class="px-5 py-2 bg-gray-50 text-[10px] font-medium text-gray-500 uppercase tracking-wide">%s</div>`, header)
		if sec.Locked {
			fmt.Fprintf(w, `<div class="px-5 py-3">
				<button type="button" hx-post="/vault/unlock-form" hx-target="#vault-keys" hx-swap="innerHTML" hx-vals='{"scope":"%s"}'
				        class="text-xs text-indigo-600 hover:text-indigo-800">Unlock to view secrets</button>
			</div>`, html.EscapeString(sec.Scope))
		} else if len(sec.Keys) == 0 {
			fmt.Fprint(w, `<div class="px-5 py-4 text-center text-gray-300 text-xs">empty</div>`)
		} else {
			for _, key := range sec.Keys {
				esc := html.EscapeString(key)
				fmt.Fprintf(w, `<div class="px-5 py-2.5 flex items-center justify-between">
					<span class="font-mono text-sm">%s</span>
					<div class="flex items-center gap-3">
						<span class="text-gray-300 text-sm">••••••••</span>
						<button hx-delete="/vault/%s?scope=%s"
								hx-target="#vault-keys"
								hx-swap="innerHTML"
								hx-confirm="Delete secret &#39;%s&#39;?"
								class="text-red-400 hover:text-red-600 text-xs">delete</button>
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
	pw := []byte(password)
	for _, t := range s.lockedTiers(r) {
		backend, err := s.openVaultWithPassword(t.Scope, pw)
		if err != nil {
			results = append(results, tierResult{
				Label: t.Label, Scope: t.Scope, Success: false,
				Error: "incorrect password",
			})
			continue
		}
		s.cacheUnlockedBackend(t.Scope, backend)
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
