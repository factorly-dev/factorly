// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"html"
	"net/http"
)

type vaultSection struct {
	Label string
	Keys  []string
	Scope string // "project" or "global"
}

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	var sections []vaultSection

	if s.projectVault != nil {
		if keys, err := s.projectVault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Project vault", Keys: keys, Scope: "project"})
		}
	}
	if s.globalVault != nil {
		if keys, err := s.globalVault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Global vault", Keys: keys, Scope: "global"})
		}
	}

	// Fallback: if no separate vaults but we have a combined one
	if len(sections) == 0 && s.vault != nil {
		if keys, err := s.vault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Vault", Keys: keys, Scope: "default"})
		}
	}

	s.render(w, "vault.html", map[string]any{
		"Title":    "Vault",
		"Nav":      "vault",
		"Sections": sections,
	})
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

	backend := s.resolveVaultBackend(scope)
	if backend == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	if err := backend.Set(key, value); err != nil {
		http.Error(w, fmt.Sprintf("vault set: %v", err), http.StatusInternalServerError)
		return
	}

	s.renderVaultKeys(w)
}

func (s *Server) handleVaultDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	scope := r.URL.Query().Get("scope")

	backend := s.resolveVaultBackend(scope)
	if backend == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	if err := backend.Delete(key); err != nil {
		http.Error(w, fmt.Sprintf("vault delete: %v", err), http.StatusInternalServerError)
		return
	}

	s.renderVaultKeys(w)
}

func (s *Server) resolveVaultBackend(scope string) interface {
	Set(key, value string) error
	Delete(key string) error
	List() ([]string, error)
} {
	switch scope {
	case "project":
		if s.projectVault != nil {
			return s.projectVault
		}
	case "global":
		if s.globalVault != nil {
			return s.globalVault
		}
	}
	return s.vault
}

func (s *Server) renderVaultKeys(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var sections []vaultSection
	if s.projectVault != nil {
		if keys, err := s.projectVault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Project vault", Keys: keys, Scope: "project"})
		}
	}
	if s.globalVault != nil {
		if keys, err := s.globalVault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Global vault", Keys: keys, Scope: "global"})
		}
	}
	if len(sections) == 0 && s.vault != nil {
		if keys, err := s.vault.List(); err == nil {
			sections = append(sections, vaultSection{Label: "Vault", Keys: keys, Scope: "default"})
		}
	}

	if len(sections) == 0 {
		fmt.Fprint(w, `<div class="px-5 py-8 text-center text-gray-400 text-sm">No secrets stored.</div>`)
		return
	}

	for _, sec := range sections {
		fmt.Fprintf(w, `<div class="border-b border-gray-200 last:border-b-0">
			<div class="px-5 py-2 bg-gray-50 text-[10px] font-medium text-gray-500 uppercase tracking-wide">%s <span class="text-gray-300">(%d keys)</span></div>`, sec.Label, len(sec.Keys))
		if len(sec.Keys) == 0 {
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
				</div>`, esc, esc, sec.Scope, esc)
			}
		}
		fmt.Fprint(w, `</div>`)
	}
}
