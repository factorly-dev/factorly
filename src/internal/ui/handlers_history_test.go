// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
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

// TestIsReplayable pins the eligibility rules — only entries
// without a chain hash or without a tool name are rejected.
// Workflow steps ARE replayable: replay re-fires the step's tool
// as a standalone call (no parent workflow context, no
// workflow_run_id on the resulting audit entry).
func TestIsReplayable(t *testing.T) {
	cases := []struct {
		name string
		e    logger.Entry
		want bool
	}{
		{"normal call", logger.Entry{Hash: "abc", Tool: "x", Interface: "cli"}, true},
		{"missing hash", logger.Entry{Tool: "x", Interface: "cli"}, false},
		{"workflow step is replayable", logger.Entry{Hash: "abc", Tool: "x", Interface: "workflow"}, true},
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
		{Timestamp: time.Now().Add(-2 * time.Hour), Tool: "first", Hash: "hash-one", Status: "success"},
		{Timestamp: time.Now().Add(-1 * time.Hour), Tool: "second", Hash: "hash-two", Status: "success"},
		{Timestamp: time.Now(), Tool: "third", Hash: "hash-three", Status: "success"},
	}
	seedAuditLog(t, entries)

	got, err := findAuditEntryByHash("", "hash-two")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil || got.Tool != "second" {
		t.Fatalf("got %+v, want entry with tool=second", got)
	}

	missing, err := findAuditEntryByHash("", "hash-not-found")
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
		{Timestamp: time.Now(), Tool: "x", Hash: "hash-one", Status: "success"},
	})
	srv, _ := testServerWithProxy(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/history/hash-missing/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHistoryReplay_AcceptsWorkflowStep confirms that a workflow
// step entry can be replayed independently — the handler treats it
// like any other call (re-fires the step's tool with recorded
// params). The audit log entry produced by the replay does NOT
// carry a workflow_run_id, because the replay isn't itself running
// inside a workflow run; that's the intentional v1 semantic of
// "re-run this individual step in isolation."
//
// Negative path: when the step's tool no longer exists in the
// current config, the handler returns 400 ("no longer registered"),
// same as for any other replayed call referencing a deleted tool.
func TestHistoryReplay_AcceptsWorkflowStep(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "echo", Hash: "hash-wf-step", Status: "success", Interface: "workflow", WorkflowRunID: "abc12345"},
	})
	// Empty config — the step's tool is no longer registered. We
	// expect 400 (missing tool) rather than 400 (workflow rejected).
	srv, _ := testServerWithProxy(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/history/hash-wf-step/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (missing tool), got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no longer registered") {
		t.Errorf("expected 'no longer registered' rejection, got %q (workflow-rejection should no longer fire)", rec.Body.String())
	}
}

// TestHistoryReplay_RejectsMissingTool covers the case where the
// audit entry references a tool that's since been removed from the
// config. We can't replay against a tool that doesn't exist; the
// handler surfaces 400 rather than a confusing proxy error.
func TestHistoryReplay_RejectsMissingTool(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "deleted.tool", Hash: "hash-one", Status: "success", Interface: "cli"},
	})
	cfg := &config.Config{Tools: map[string]config.ToolConfig{
		// Only "other.tool" exists; "deleted.tool" was removed
		"other.tool": {Type: "cli"},
	}}
	srv, _ := testServerWithProxy(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "/history/hash-one/replay", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no longer registered") {
		t.Errorf("expected 'no longer registered' in error, got %s", rec.Body.String())
	}
}

// TestHistoryDetail_ReturnsRowBodyFragment hits the new
// /history/{hash}/detail endpoint used by the dashboard's inline
// expand affordance. The response is the same per-row body that
// renders inside /history's expandable rows — params, status,
// duration, and (when eligible) the Replay / Edit & Replay buttons.
func TestHistoryDetail_ReturnsRowBodyFragment(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{
			Timestamp:  time.Now(),
			Tool:       "echo.tool",
			Hash:       "hash-detail-aaaa",
			Status:     "success",
			Interface:  "cli",
			DurationMs: 42,
			Params:     map[string]string{"text": "hello"},
			Output:     "hello\n",
		},
	})
	srv, _ := testServerWithProxy(t, &config.Config{Tools: map[string]config.ToolConfig{
		"echo.tool": {Type: "cli", Command: "echo"},
	}})

	req := httptest.NewRequest(http.MethodGet, "/history/hash-detail-aaaa/detail", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Body should NOT contain the page layout chrome — just the row
	// detail fragment (the partial). Look for distinctive bits.
	for _, want := range []string{
		"echo.tool",     // tool name appears in the Replay link href
		"42ms",          // duration
		"hello",         // params value
		"Parameters",    // section header
		"Replay",        // action button (entry is replayable)
		"Edit & Replay", // action button
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in detail fragment, body=%s", want, body)
		}
	}
	// No layout wrapper — fragment only.
	if strings.Contains(body, "<html") || strings.Contains(body, "<body") {
		t.Errorf("expected fragment, got full page; body=%s", body[:min(200, len(body))])
	}
}

// TestHistoryDetail_PrefixHashMatchesUniqueEntry covers the same
// ≥4-char prefix matching `findAuditEntryByHash` supports for the
// replay endpoint. Lets the dashboard avoid passing 64-char hashes
// in attributes when a short prefix is enough.
func TestHistoryDetail_PrefixHashMatchesUniqueEntry(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "x.tool", Hash: "abcd1234567890", Status: "success", Interface: "cli"},
	})
	srv, _ := testServerWithProxy(t, &config.Config{Tools: map[string]config.ToolConfig{
		"x.tool": {Type: "cli", Command: "true"},
	}})

	req := httptest.NewRequest(http.MethodGet, "/history/abcd/detail", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("prefix lookup expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHistoryDetail_NotFound returns 404 when no entry matches the
// hash — the dashboard's expanded body shows whatever the server
// returns, so a non-empty error string is what the user sees.
func TestHistoryDetail_NotFound(t *testing.T) {
	seedAuditLog(t, []logger.Entry{
		{Timestamp: time.Now(), Tool: "x", Hash: "hash-only-one", Status: "success"},
	})
	srv, _ := testServerWithProxy(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/history/hash-missing-xx/detail", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestGroupHistoryEntries_StandaloneAndWorkflow exercises the
// coalescing function. Entries without a WorkflowRunID pass through
// as singleton groups; entries sharing a WorkflowRunID gather under
// a synthesized parent that reports the workflow name and step
// count.
func TestGroupHistoryEntries_StandaloneAndWorkflow(t *testing.T) {
	entries := []historyEntry{
		// newest-first ordering (mimics readAllLogs output)
		{Tool: "standalone1", Status: "success", DurationMs: 50},
		{Tool: "github.create_issue", Status: "success", DurationMs: 200, WorkflowRunID: "abc123", WorkflowName: "daily-prep"},
		{Tool: "slack.send", Status: "success", DurationMs: 100, WorkflowRunID: "abc123", WorkflowName: "daily-prep"},
		{Tool: "standalone2", Status: "error", DurationMs: 30},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// First group: standalone1
	if groups[0].IsWorkflow || groups[0].Lead.Tool != "standalone1" {
		t.Errorf("group 0 = %+v, want standalone standalone1", groups[0].Lead)
	}
	// Second group: workflow run (gathered both children)
	wf := groups[1]
	if !wf.IsWorkflow {
		t.Error("group 1 should be a workflow run")
	}
	if wf.Lead.Tool != "daily-prep" {
		t.Errorf("lead.Tool = %q, want daily-prep", wf.Lead.Tool)
	}
	if wf.StepCount != 2 {
		t.Errorf("step count = %d, want 2", wf.StepCount)
	}
	if wf.Lead.DurationMs != 300 {
		t.Errorf("lead duration = %d, want sum 300", wf.Lead.DurationMs)
	}
	if wf.Lead.Status != "success" {
		t.Errorf("lead status = %q, want success (all children succeeded)", wf.Lead.Status)
	}
	// Third group: standalone2
	if groups[2].IsWorkflow || groups[2].Lead.Tool != "standalone2" {
		t.Errorf("group 2 = %+v, want standalone standalone2", groups[2].Lead)
	}
}

// TestGroupHistoryEntries_WorkflowErrorStatus confirms the lead's
// status flips to "error" if any child failed.
func TestGroupHistoryEntries_WorkflowErrorStatus(t *testing.T) {
	entries := []historyEntry{
		{Tool: "step1", Status: "success", DurationMs: 100, WorkflowRunID: "r1", WorkflowName: "wf"},
		{Tool: "step2", Status: "error", DurationMs: 50, WorkflowRunID: "r1", WorkflowName: "wf"},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 1 || !groups[0].IsWorkflow {
		t.Fatalf("expected one workflow group, got %+v", groups)
	}
	if groups[0].Lead.Status != "error" {
		t.Errorf("status = %q, want error (one child failed)", groups[0].Lead.Status)
	}
}

// TestGroupHistoryEntries_MissingWorkflowName falls back to a
// generic label when the workflow name is empty (older audit
// entries from before WorkflowName was stamped, or partial-state
// scenarios).
func TestGroupHistoryEntries_MissingWorkflowName(t *testing.T) {
	entries := []historyEntry{
		{Tool: "step1", Status: "success", WorkflowRunID: "deadbeef"},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 1 || !groups[0].IsWorkflow {
		t.Fatalf("expected one workflow group, got %+v", groups)
	}
	if !strings.Contains(groups[0].Lead.Tool, "deadbeef") {
		t.Errorf("lead tool = %q, expected to contain run id deadbeef", groups[0].Lead.Tool)
	}
}

// TestGroupHistoryEntries_ChildrenInStepOrder confirms that
// children render in execution order (step 1, step 2, …) even
// though the input is in newest-first traversal order (step N
// first). Critical for the /history UX: an operator reading a
// workflow's expanded steps top-to-bottom should see them as the
// workflow executed them, not reversed.
func TestGroupHistoryEntries_ChildrenInStepOrder(t *testing.T) {
	// Newest-first input — step 3 appears first, step 1 last.
	entries := []historyEntry{
		{Tool: "step3", Status: "success", WorkflowRunID: "r1", WorkflowName: "wf"},
		{Tool: "step2", Status: "success", WorkflowRunID: "r1", WorkflowName: "wf"},
		{Tool: "step1", Status: "success", WorkflowRunID: "r1", WorkflowName: "wf"},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 1 || !groups[0].IsWorkflow {
		t.Fatalf("expected 1 workflow group, got %+v", groups)
	}
	got := []string{}
	for _, ch := range groups[0].Children {
		got = append(got, ch.Tool)
	}
	want := []string{"step1", "step2", "step3"}
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("child %d = %q, want %q (chronological order)", i, got[i], w)
		}
	}
}

// TestGroupHistoryEntries_LeadHashFromSuppressedParent confirms
// the synthesized workflow lead inherits the suppressed parent
// entry's audit chain hash so the Replay button on the coalesced
// row can re-fire the entire workflow via the same
// /history/{hash}/replay endpoint used by standalone calls.
func TestGroupHistoryEntries_LeadHashFromSuppressedParent(t *testing.T) {
	entries := []historyEntry{
		{Tool: "wf", Hash: "parent-hash", Status: "success", Interface: "cli"},
		{Tool: "step1", Status: "success", WorkflowRunID: "r1", WorkflowName: "wf"},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 1 || !groups[0].IsWorkflow {
		t.Fatalf("expected 1 workflow group, got %+v", groups)
	}
	if groups[0].Lead.Hash != "parent-hash" {
		t.Errorf("Lead.Hash = %q, want parent-hash", groups[0].Lead.Hash)
	}
	if !groups[0].Lead.Replayable {
		t.Error("Lead.Replayable = false, want true (hash present)")
	}
}

// TestGroupHistoryEntries_LeadNoHashWhenParentMissing exercises the
// case where the visible window starts after the workflow run
// began — there's no suppressed parent in the entries slice, so
// the lead can't be replayed (no hash to address). Replay button
// should not appear on the lead in that case.
func TestGroupHistoryEntries_LeadNoHashWhenParentMissing(t *testing.T) {
	// Only the step entries — no preceding parent call.
	entries := []historyEntry{
		{Tool: "step1", Status: "success", WorkflowRunID: "r1", WorkflowName: "wf"},
	}
	groups := groupHistoryEntries(entries)
	if len(groups) != 1 || !groups[0].IsWorkflow {
		t.Fatalf("expected 1 workflow group, got %+v", groups)
	}
	if groups[0].Lead.Hash != "" {
		t.Errorf("Lead.Hash = %q, want empty (no parent to source it from)", groups[0].Lead.Hash)
	}
	if groups[0].Lead.Replayable {
		t.Error("Lead.Replayable = true, want false (no hash)")
	}
}

// TestGroupHistoryEntries_SuppressesRealParent confirms that the
// outer workflow call (logged by the proxy without WorkflowRunID,
// since context-stamping happens inside the workflow provider
// after the proxy's entry is constructed) gets suppressed when it
// appears as the immediate predecessor of its run's children in
// newest-first order — preventing a duplicate row in /history.
func TestGroupHistoryEntries_SuppressesRealParent(t *testing.T) {
	// Newest-first ordering. The real "echo-twice" parent call
	// appears just before its first child step.
	entries := []historyEntry{
		{Tool: "echo-twice", Status: "success", DurationMs: 5},                                              // real parent (logged later, appears first)
		{Tool: "echo", Status: "success", DurationMs: 200, WorkflowRunID: "r1", WorkflowName: "echo-twice"}, // child 1
		{Tool: "echo", Status: "success", DurationMs: 100, WorkflowRunID: "r1", WorkflowName: "echo-twice"}, // child 2
		{Tool: "other-standalone", Status: "success", DurationMs: 50},                                       // unrelated
	}
	groups := groupHistoryEntries(entries)
	// Expect 2 groups: the synthesized workflow run + the unrelated
	// standalone. NOT 3 — the "echo-twice" real parent should be
	// suppressed.
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (real parent suppressed), got %d: %+v", len(groups), groups)
	}
	if !groups[0].IsWorkflow || groups[0].Lead.Tool != "echo-twice" {
		t.Errorf("group 0 should be the workflow run, got %+v", groups[0].Lead)
	}
	if groups[1].IsWorkflow || groups[1].Lead.Tool != "other-standalone" {
		t.Errorf("group 1 should be the unrelated standalone, got %+v", groups[1].Lead)
	}
}

// TestFilterHistoryGroups_ChildMatchKeepsParent confirms that a
// filter matching only a step entry inside a workflow still surfaces
// the entire group (with the FilterMatchedChildOnly flag set so the
// UI can hint at it).
func TestFilterHistoryGroups_ChildMatchKeepsParent(t *testing.T) {
	groups := []historyGroup{
		{
			Lead:       historyEntry{Tool: "daily-prep", Status: "success"},
			Children:   []historyEntry{{Tool: "github.create_issue", Status: "success"}},
			IsWorkflow: true,
			StepCount:  1,
		},
		{Lead: historyEntry{Tool: "echo", Status: "success"}},
	}
	// Filter on a tool name that only the child has.
	filtered := filterHistoryGroups(groups, "github.create_issue", "", "")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 group surfaced, got %d", len(filtered))
	}
	if !filtered[0].IsWorkflow {
		t.Errorf("expected the workflow group, got %+v", filtered[0])
	}
	if !filtered[0].FilterMatchedChildOnly {
		t.Error("expected FilterMatchedChildOnly to be true")
	}
}

// TestFilterHistoryGroups_NoFilter returns all groups unchanged.
func TestFilterHistoryGroups_NoFilter(t *testing.T) {
	groups := []historyGroup{
		{Lead: historyEntry{Tool: "a"}},
		{Lead: historyEntry{Tool: "b"}},
	}
	out := filterHistoryGroups(groups, "", "", "")
	if len(out) != 2 {
		t.Errorf("expected 2 groups, got %d", len(out))
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

// makeEntries returns N historyEntries newest-first with synthetic
// hashes "h0", "h1", … so tests can assert cursor advancement.
func makeEntries(n int) []historyEntry {
	out := make([]historyEntry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, historyEntry{
			Tool:   "echo",
			Status: "success",
			Hash:   fmt.Sprintf("h%d", i),
		})
	}
	return out
}

// TestPaginateHistory_FirstPage returns the first pageSize groups and
// a non-empty cursor when more remain.
func TestPaginateHistory_FirstPage(t *testing.T) {
	entries := makeEntries(120)
	page, next := paginateHistory(entries, "", "", "", "", 50)
	if len(page) != 50 {
		t.Fatalf("page size = %d, want 50", len(page))
	}
	if next != "h49" {
		t.Errorf("next cursor = %q, want h49", next)
	}
	if page[0].Lead.Hash != "h0" {
		t.Errorf("first entry hash = %q, want h0", page[0].Lead.Hash)
	}
}

// TestPaginateHistory_CursorAdvances second page starts after the
// cursor and returns the next slice.
func TestPaginateHistory_CursorAdvances(t *testing.T) {
	entries := makeEntries(120)
	page, next := paginateHistory(entries, "", "", "", "h49", 50)
	if len(page) != 50 {
		t.Fatalf("page size = %d, want 50", len(page))
	}
	if page[0].Lead.Hash != "h50" {
		t.Errorf("first entry of second page = %q, want h50", page[0].Lead.Hash)
	}
	if next != "h99" {
		t.Errorf("next cursor = %q, want h99", next)
	}
}

// TestPaginateHistory_FinalPage the last page has fewer than pageSize
// entries and the next cursor is empty.
func TestPaginateHistory_FinalPage(t *testing.T) {
	entries := makeEntries(120)
	page, next := paginateHistory(entries, "", "", "", "h99", 50)
	if len(page) != 20 {
		t.Fatalf("final page size = %d, want 20", len(page))
	}
	if next != "" {
		t.Errorf("next cursor = %q, want empty (end of log)", next)
	}
}

// TestPaginateHistory_PastEnd a cursor pointing at the last group
// returns an empty page and no next cursor (terminator condition).
func TestPaginateHistory_PastEnd(t *testing.T) {
	entries := makeEntries(10)
	page, next := paginateHistory(entries, "", "", "", "h9", 50)
	if len(page) != 0 {
		t.Errorf("page past last cursor should be empty, got %d entries", len(page))
	}
	if next != "" {
		t.Errorf("next cursor past end = %q, want empty", next)
	}
}

// TestPaginateHistory_FiltersBeforeWindowing filtering is applied to
// the full stream, not after windowing — so a small page of "errors"
// is reachable even when most of the log is successes.
func TestPaginateHistory_FiltersBeforeWindowing(t *testing.T) {
	// 100 successes + 3 errors scattered. The 3 errors should all
	// fit on one page of 50 with no Load-more.
	entries := makeEntries(100)
	entries[10].Status = "error"
	entries[50].Status = "error"
	entries[90].Status = "error"
	page, next := paginateHistory(entries, "", "error", "", "", 50)
	if len(page) != 3 {
		t.Errorf("filtered page = %d, want 3 errors", len(page))
	}
	if next != "" {
		t.Errorf("filtered single-page cursor = %q, want empty", next)
	}
}

// TestPaginateHistory_WorkflowGroupNotSplit a workflow run is one
// group in the page (parent + children together), regardless of how
// many child entries it contains.
func TestPaginateHistory_WorkflowGroupNotSplit(t *testing.T) {
	// Newest-first: parent solo, then 5 workflow child entries.
	// (groupHistoryEntries detects parent-adjacent-to-children and
	// suppresses the standalone parent, synthesizing a workflow lead.)
	entries := []historyEntry{
		{Tool: "daily-prep", Status: "success", Hash: "parent"},
		{Tool: "step5", Status: "success", Hash: "c5", WorkflowRunID: "run-1", WorkflowName: "daily-prep"},
		{Tool: "step4", Status: "success", Hash: "c4", WorkflowRunID: "run-1", WorkflowName: "daily-prep"},
		{Tool: "step3", Status: "success", Hash: "c3", WorkflowRunID: "run-1", WorkflowName: "daily-prep"},
		{Tool: "step2", Status: "success", Hash: "c2", WorkflowRunID: "run-1", WorkflowName: "daily-prep"},
		{Tool: "step1", Status: "success", Hash: "c1", WorkflowRunID: "run-1", WorkflowName: "daily-prep"},
	}
	page, next := paginateHistory(entries, "", "", "", "", 50)
	if len(page) != 1 {
		t.Fatalf("expected 1 coalesced group, got %d", len(page))
	}
	if !page[0].IsWorkflow {
		t.Errorf("group should be workflow")
	}
	if page[0].StepCount != 5 {
		t.Errorf("StepCount = %d, want 5", page[0].StepCount)
	}
	if next != "" {
		t.Errorf("single-group page cursor = %q, want empty", next)
	}
}

// TestGroupCursor workflow lead with parent hash uses the hash; without
// one it falls back to wf:<runID>. Solo groups use the entry hash.
func TestGroupCursor(t *testing.T) {
	cases := []struct {
		name string
		g    historyGroup
		want string
	}{
		{"solo", historyGroup{Lead: historyEntry{Hash: "abc"}}, "abc"},
		{
			"workflow with parent hash",
			historyGroup{
				IsWorkflow: true,
				Lead:       historyEntry{Hash: "parent-hash", WorkflowRunID: "run-1"},
			},
			"parent-hash",
		},
		{
			"workflow without parent hash falls back to runID",
			historyGroup{
				IsWorkflow: true,
				Lead:       historyEntry{WorkflowRunID: "run-2"},
			},
			"wf:run-2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := groupCursor(c.g); got != c.want {
				t.Errorf("groupCursor = %q, want %q", got, c.want)
			}
		})
	}
}

// TestHandleHistoryMore the endpoint returns the next page and an OOB
// chunk that replaces #history-more with a fresh button (or empty
// when the cursor reached the end).
func TestHandleHistoryMore(t *testing.T) {
	// Seed enough entries to require two pages at size 50.
	raws := make([]logger.Entry, 0, 120)
	base := time.Now()
	for i := 0; i < 120; i++ {
		raws = append(raws, logger.Entry{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Tool:      "echo",
			Status:    "success",
			Hash:      fmt.Sprintf("h%d", i),
			Interface: "cli",
		})
	}
	seedAuditLog(t, raws)

	s, _ := testServer(t, nil)

	// First page: no cursor.
	req := httptest.NewRequest(http.MethodGet, "/history/more", nil)
	rec := httptest.NewRecorder()
	s.handleHistoryMore(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	// Should include rows (echo tool appears in each) and an OOB
	// container with a fresh load-more button (more remain).
	if !strings.Contains(body, "echo") {
		t.Errorf("response missing row content; body=%s", body)
	}
	if !strings.Contains(body, `id="history-more"`) {
		t.Errorf("response missing OOB #history-more container; body=%s", body)
	}
	if !strings.Contains(body, "Load more") {
		t.Errorf("first-page response should still offer Load more; body=%s", body)
	}

	// Cursor at the very last hash: zero rows back and no fresh
	// button (OOB container is empty).
	req = httptest.NewRequest(http.MethodGet, "/history/more?cursor=h0", nil)
	rec = httptest.NewRecorder()
	s.handleHistoryMore(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("end-of-log status = %d, want 200", rec.Code)
	}
	tailBody := rec.Body.String()
	// h0 is the OLDEST entry (smallest timestamp). Newest-first
	// parse puts h119 first and h0 last, so cursor=h0 means "past
	// the end" and should yield no button.
	if strings.Contains(tailBody, "Load more") {
		t.Errorf("end-of-log response should NOT contain a Load more button; body=%s", tailBody)
	}
}
