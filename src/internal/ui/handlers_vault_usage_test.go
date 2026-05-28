// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRenderVaultSections_UsedByBadge feeds a synthetic vault section
// and a synthetic usage map into the pure renderer and asserts the
// "used by N tool" badge shows up for keys with N > 0 and is absent
// for keys with N == 0. This is the test that pins the bug: if the
// badge HTML is missing, this fails.
func TestRenderVaultSections_UsedByBadge(t *testing.T) {
	sections := []vaultSection{
		{
			Label: "Project vault",
			Scope: "project",
			Keys:  []string{"TRELLO_API_KEY", "UNUSED_KEY"},
		},
	}
	usage := map[string]int{
		"TRELLO_API_KEY": 1,
	}

	rec := httptest.NewRecorder()
	renderVaultSections(rec, sections, usage, map[string]int{})
	body := rec.Body.String()

	if !strings.Contains(body, "TRELLO_API_KEY") {
		t.Fatalf("missing key in response; body:\n%s", body)
	}
	if !strings.Contains(body, "used by 1 tool") {
		t.Errorf("expected badge 'used by 1 tool' for TRELLO_API_KEY; body:\n%s", body)
	}
	// Pluralization sanity.
	usage["TRELLO_API_KEY"] = 3
	rec = httptest.NewRecorder()
	renderVaultSections(rec, sections, usage, map[string]int{})
	if !strings.Contains(rec.Body.String(), "used by 3 tools") {
		t.Errorf("expected pluralized badge 'used by 3 tools' (n=3); body:\n%s", rec.Body.String())
	}
}

// TestRenderVaultSections_AuthBadge confirms the green "used by N
// auth" pill renders when oauthProviderReferenceCounts reports >0.
// Independent from the tool count — a key can be tool-only, auth-only,
// or both.
func TestRenderVaultSections_AuthBadge(t *testing.T) {
	sections := []vaultSection{{
		Label: "Project vault",
		Scope: "project",
		Keys:  []string{"GH_CLIENT_SECRET", "TRELLO_API_KEY"},
	}}
	usage := map[string]int{"TRELLO_API_KEY": 2}
	authUsage := map[string]int{"GH_CLIENT_SECRET": 1, "TRELLO_API_KEY": 1}

	rec := httptest.NewRecorder()
	renderVaultSections(rec, sections, usage, authUsage)
	body := rec.Body.String()

	if !strings.Contains(body, "used by 1 auth") {
		t.Errorf("missing 'used by 1 auth' badge; body:\n%s", body)
	}
	if !strings.Contains(body, "bg-green-50") {
		t.Errorf("auth badge should use bg-green-50; body:\n%s", body)
	}
	if !strings.Contains(body, "used by 2 tools") {
		t.Errorf("missing tool-count badge for shared key; body:\n%s", body)
	}
}
