// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import "net/http"

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	s.render(w, "vault.html", map[string]any{
		"Title": "Vault",
		"Nav":   "vault",
	})
}
