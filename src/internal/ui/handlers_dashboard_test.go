// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
)

// --- Pure helper tests -------------------------------------------------

func TestTopTools_EmptyInputReturnsNil(t *testing.T) {
	if got := topTools(nil, 5); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
	if got := topTools([]logger.Entry{}, 5); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestTopTools_OrdersByCountThenName(t *testing.T) {
	entries := []logger.Entry{
		{Tool: "alpha"}, {Tool: "alpha"}, {Tool: "alpha"},
		{Tool: "beta"}, {Tool: "beta"},
		{Tool: "charlie"}, {Tool: "charlie"},
		{Tool: "delta"},
	}
	got := topTools(entries, 5)
	if len(got) != 4 {
		t.Fatalf("expected 4 rows, got %d (%#v)", len(got), got)
	}
	if got[0].Tool != "alpha" || got[0].Count != 3 || got[0].Pct != 100 {
		t.Errorf("leader wrong: %#v", got[0])
	}
	// beta and charlie both count 2; alphabetical tiebreak puts beta first.
	if got[1].Tool != "beta" || got[2].Tool != "charlie" {
		t.Errorf("tiebreak wrong: %v %v", got[1].Tool, got[2].Tool)
	}
	// 2/3 * 100 = 66 (integer div), 1/3 * 100 = 33.
	if got[1].Pct != 66 {
		t.Errorf("expected beta.Pct=66, got %d", got[1].Pct)
	}
	if got[3].Pct != 33 {
		t.Errorf("expected delta.Pct=33, got %d", got[3].Pct)
	}
}

func TestTopTools_RespectsLimitN(t *testing.T) {
	entries := []logger.Entry{
		{Tool: "a"}, {Tool: "b"}, {Tool: "c"}, {Tool: "d"}, {Tool: "e"},
	}
	got := topTools(entries, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
}

func TestTopTools_NaNGuard(t *testing.T) {
	// All same tool: counts non-empty, leader == sole entry. Single-row
	// Pct must be 100, not NaN, not a divide-by-zero.
	got := topTools([]logger.Entry{{Tool: "x"}}, 5)
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].Pct != 100 {
		t.Errorf("expected Pct=100, got %d", got[0].Pct)
	}
}

func TestOversightBreakdown_CountsStatuses(t *testing.T) {
	entries := []logger.Entry{
		{Status: "success"}, {Status: "success"}, {Status: "success"},
		{Status: "error"},
		{Status: "blocked"}, {Status: "blocked"},
		{Status: "weird-unknown-status"}, // counted toward Total only
	}
	got := oversightBreakdown(entries)
	if got.Total != 7 {
		t.Errorf("Total: want 7, got %d", got.Total)
	}
	if got.Success != 3 {
		t.Errorf("Success: want 3, got %d", got.Success)
	}
	if got.Error != 1 {
		t.Errorf("Error: want 1, got %d", got.Error)
	}
	if got.Blocked != 2 {
		t.Errorf("Blocked: want 2, got %d", got.Blocked)
	}
}

func TestOversightBreakdown_EmptyZeroValue(t *testing.T) {
	// Critical: the template divides by Total to render percentages
	// and skips the block when Total is 0. This test pins the
	// zero-Total contract so a future refactor can't introduce NaN
	// rendering by accident.
	got := oversightBreakdown(nil)
	if got.Total != 0 || got.Success != 0 || got.Error != 0 || got.Blocked != 0 {
		t.Fatalf("expected zero value for empty input, got %#v", got)
	}
}

func TestFilterAfter_WindowsByTimestamp(t *testing.T) {
	base := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	entries := []logger.Entry{
		{Timestamp: base.Add(-3 * time.Hour), Tool: "old"},
		{Timestamp: base.Add(-30 * time.Minute), Tool: "recent"},
		{Timestamp: base, Tool: "now"},
	}
	cutoff := base.Add(-1 * time.Hour)
	got := filterAfter(entries, cutoff)
	if len(got) != 2 {
		t.Fatalf("want 2 entries within window, got %d (%#v)", len(got), got)
	}
	if got[0].Tool != "recent" || got[1].Tool != "now" {
		t.Errorf("filterAfter wrong order/contents: %#v", got)
	}
}

// --- Handler render tests ----------------------------------------------

func TestHandleDashboard_FreshInstallShowsQuickStart(t *testing.T) {
	srv, _ := testServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Fresh-install quick-start tiles must show; rollup panels must
	// not (because HasAnyCalls is false).
	for _, want := range []string{"Get started", "Browse blueprints", "Import an OpenAPI spec", "Create a tool by hand"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in fresh-install dashboard body", want)
		}
	}
	if strings.Contains(body, "Top tools") {
		t.Error("did not expect Top tools panel on fresh-install dashboard")
	}
}

func TestHandleDashboard_ActiveUseShowsRollups(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"a.tool": {Type: "cli", Command: "true"},
		},
	})

	// Write an audit log with two recent calls so HasAnyCalls is true.
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-10 * time.Minute), Tool: "a.tool", Status: "success"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "a.tool", Status: "error"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Top tools", "Oversight", "a.tool"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in active-use dashboard body", want)
		}
	}
	if strings.Contains(body, "Get started") {
		t.Error("did not expect quick-start tiles when audit log is non-empty")
	}
}

func TestHandleDashboard_StatusStripCountsTools(t *testing.T) {
	srv, _ := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"a.cli":      {Type: "cli", Command: "true"},
			"b.rest":     {Type: "rest", Method: "GET", BaseURL: "https://x"},
			"wf.example": {Type: "workflow"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// ToolsTotal excludes workflow entries, so the count pill should
	// read 2 and a separate "workflow 1" chip should also appear.
	if !strings.Contains(body, "cli 1") {
		t.Error("expected 'cli 1' pill in status strip")
	}
	if !strings.Contains(body, "rest 1") {
		t.Error("expected 'rest 1' pill in status strip")
	}
	if !strings.Contains(body, "workflow 1") {
		t.Error("expected 'workflow 1' pill in status strip")
	}
}

// --- Helpers -----------------------------------------------------------

// writeDashboardLog writes one JSONL line per entry to path. The
// dashboard handler reads via FACTORLY_LOG_PATH when set, so tests
// don't need the full logger pipeline.
func writeDashboardLog(t *testing.T, path string, entries []logger.Entry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := range entries {
		if err := enc.Encode(entries[i]); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
}
