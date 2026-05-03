// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"net/http"
)

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	var keys []string
	available := s.vault != nil

	if available {
		var err error
		keys, err = s.vault.List()
		if err != nil {
			available = false
		}
	}

	s.render(w, "vault.html", map[string]any{
		"Title":     "Vault",
		"Nav":       "vault",
		"Available": available,
		"Keys":      keys,
	})
}

func (s *Server) handleVaultSet(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	key := r.FormValue("key")
	value := r.FormValue("value")
	if key == "" || value == "" {
		http.Error(w, "key and value required", http.StatusBadRequest)
		return
	}

	if err := s.vault.Set(key, value); err != nil {
		http.Error(w, fmt.Sprintf("vault set: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated key list as HTML partial for htmx swap
	s.renderVaultKeys(w)
}

func (s *Server) handleVaultDelete(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		http.Error(w, "vault not available", http.StatusServiceUnavailable)
		return
	}

	key := r.PathValue("key")
	if err := s.vault.Delete(key); err != nil {
		http.Error(w, fmt.Sprintf("vault delete: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated key list
	s.renderVaultKeys(w)
}

func (s *Server) renderVaultKeys(w http.ResponseWriter) {
	keys, err := s.vault.List()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		fmt.Fprintf(w, `<div class="px-5 py-8 text-center text-red-400 text-sm">Error listing keys: %s</div>`, err.Error())
		return
	}
	if len(keys) == 0 {
		fmt.Fprint(w, `<div class="px-5 py-8 text-center text-gray-400 text-sm">No secrets stored.</div>`)
		return
	}

	fmt.Fprint(w, `<div class="divide-y divide-gray-100">`)
	for _, key := range keys {
		fmt.Fprintf(w, `<div class="px-5 py-3 flex items-center justify-between">
			<span class="font-mono text-sm">%s</span>
			<div class="flex items-center gap-3">
				<span class="text-gray-300 text-sm">••••••••</span>
				<button hx-delete="/vault/%s"
						hx-target="#vault-keys"
						hx-swap="innerHTML"
						hx-confirm="Delete secret '%s'?"
						class="text-red-400 hover:text-red-600 text-xs">delete</button>
			</div>
		</div>`, key, key, key)
	}
	fmt.Fprint(w, `</div>`)
}
