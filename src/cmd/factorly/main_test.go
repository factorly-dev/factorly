// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package main

import "testing"

// TestSummaryLine pins the contract for the `factorly tools` column
// formatter: first non-empty line of the description, trimmed. The
// long agent-facing prose used by builtins (factorly.store.save,
// factorly.code, etc.) gets collapsed to its leading sentence so
// the tabular listing stays readable.
func TestSummaryLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single line", "Fetch a URL", "Fetch a URL"},
		{"multi line picks first", "Save a value.\n\nUse this for cross-run state.", "Save a value."},
		{"leading blank skipped", "\n\nSave a value.\nmore detail", "Save a value."},
		{"trim spaces", "   Save a value.   \nmore", "Save a value."},
		{"only whitespace", "   \n   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summaryLine(tc.in); got != tc.want {
				t.Errorf("summaryLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
