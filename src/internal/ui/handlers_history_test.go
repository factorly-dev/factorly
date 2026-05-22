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

// seedAuditLog writes entries as JSONL to a temp file and points
// FACTORLY_LOG_PATH at it so the history reader/finder picks it up
// without needing the proxy to be wired through an actual logger.
// Returns the file path; cleanup is automatic via t.Cleanup.
func seedAuditLog(t *testing.T, entries []logger.Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(&e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
	_ = f.Close()
	t.Setenv("FACTORLY_LOG_PATH", path)
	return path
}

// TestIsReplayable pins the three exclusion rules — entries without
// a chain hash, workflow steps, and entries with empty tool names
// are not eligible. Everything else is.
func TestIsReplayable(t *testing.T) {
	cases := []struct {
		name string
		e    logger.Entry
		want bool
	}{
		{"normal call", logger.Entry{Hash: "abc", Tool: "x", Interface: "cli"}, true},
		{"missing hash", logger.Entry{Tool: "x", Interface: "cli"}, false},
		{"workflow step", logger.Entry{Hash: "abc", Tool: "x", Interface: "workflow"}, false},
		{"empty tool", logger.Entry{Hash: "abc", Interface: "cli"}, false},
		{"mcp call ok", logger.Entry{Hash: "abc", Tool: "y", Interface: "mcp"}, true},
		{"ui call ok", logger.Entry{Hash: "abc", Tool: "z", Interface: "ui"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isReplayable(c.e); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestFindAuditEntryByHash exercises the scanner. Confirms hits land
// the right entry, misses return (nil, nil), and the function copes
// with multi-entry files.
func TestFindAuditEntryByHash(t *testing.T) {
	entries := []logger.Entry{
		{Timestamp: time.Now().Add(-2 * time.Hour), Tool: "first", Hash: "h1", Status: "success"},
		{Timestamp: time.Now().Add(-1 * time.Hour), Tool: "second", Hash: "h2", Status: "success"},
		{Timestamp: time.Now(), Tool: "third", Hash: "h3", Status: "success"},
	}
	seedAuditLog(t, entries)

	got, err := findAuditEntryByHash("", "h2")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.Tool != "second" {
		t.Fatalf("got %+v, want entry with tool=second", got)
	}

	missing, err := findAuditEntryByHash("", "h-nope")
	if err != nil {
		t.Fatalf("err on miss: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing hash, got %+v", missing)
	}

	if _, err := findAuditEntryByHash("", ""); err == nil {
		t.Error("empty hash should error")
	}
}

// TestHistoryReplay_NotFound returns 404 when the hash doesn't
// correspond to any entry in the audit log.
func TestHistoryReplay_NotFound(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "x", Hash: "h1", Status: "success"},
	})
	srv, _ := testServerWithProxy(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/history/h-missing/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHistoryReplay_RejectsWorkflow surfaces a clear 400 when the
// targeted entry is a workflow step (workflows replay as a unit via
// a separate flow once /history coalescing lands).
func TestHistoryReplay_RejectsWorkflow(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "some-workflow.step", Hash: "h-wf", Status: "success", Interface: "workflow"},
	})
	srv, _ := testServerWithProxy(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/history/h-wf/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHistoryReplay_RejectsMissingTool covers the case where the
// audit entry references a tool that's since been removed from the
// config. We can't replay against a tool that doesn't exist; the
// handler surfaces 400 rather than a confusing proxy error.
func TestHistoryReplay_RejectsMissingTool(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "deleted.tool", Hash: "h1", Status: "success", Interface: "cli"},
	})
	cfg := &config.Config{Tools: map[string]config.ToolConfig{
		// Only "other.tool" exists; "deleted.tool" was removed
		"other.tool": {Type: "cli"},
	}}
	srv, _ := testServerWithProxy(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/history/h1/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no longer registered") {
		t.Errorf("expected 'no longer registered' in error, got %s", rec.Body.String())
	}
}

// TestApplyPrefill confirms the per-param-default override used by
// the Edit & Replay flow. Params declared on the tool but absent in
// src keep their declared default; src values for params the tool
// doesn't declare are ignored (no field to put them in).
func TestApplyPrefill(t *testing.T) {
	declared := []config.ParamConfig{
		{Name: "url", Default: "https://example.com"},
		{Name: "method", Default: "GET"},
		{Name: "retries", Default: "3"},
	}
	src := map[string]string{
		"url":     "https://overridden.test",
		"method":  "POST",
		"unknown": "ignored",
	}
	got := applyPrefill(declared, src)

	if got[0].Default != "https://overridden.test" {
		t.Errorf("url default = %q, want overridden", got[0].Default)
	}
	if got[1].Default != "POST" {
		t.Errorf("method default = %q, want POST", got[1].Default)
	}
	if got[2].Default != "3" {
		t.Errorf("retries default = %q, want 3 (unchanged)", got[2].Default)
	}

	// Empty src returns the declared slice as-is.
	if same := applyPrefill(declared, nil); &same[0] != &declared[0] {
		// Acceptable for the implementation to return either the
		// same backing slice OR a new one — both are fine. We just
		// confirm values are unchanged.
		if same[0].Default != "https://example.com" {
			t.Errorf("empty src changed url default: %q", same[0].Default)
		}
	}
}
