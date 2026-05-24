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
	if !strings.Contains(body, "Get started") {
		t.Error("expected 'Get started' section header")
	}
	// Fresh-install tiles are ordered easiest path → bespoke →
	// support layer (vault for the credentials those tools will need).
	orderedTitles := []string{
		"Browse blueprints",
		"Import an OpenAPI spec",
		"Create a tool by hand",
		"Stash credentials in the vault",
	}
	prev := -1
	prevTitle := ""
	for _, title := range orderedTitles {
		idx := strings.Index(body, title)
		if idx < 0 {
			t.Errorf("expected tile %q in fresh-install dashboard", title)
			continue
		}
		if idx < prev {
			t.Errorf("tile order regressed: %q (at %d) should come after %q (at %d)",
				title, idx, prevTitle, prev)
		}
		prev = idx
		prevTitle = title
	}
	if strings.Contains(body, "Top tools") {
		t.Error("did not expect Top tools panel on fresh-install dashboard")
	}
}

// TestHandleDashboard_BuiltinsOnlyStillTreatedAsFreshInstall pins
// the "fresh install" definition: builtins (factorly.fetch,
// factorly.code, factorly.shell, etc.) don't count toward the
// user's tool inventory. Without this filter, every real install
// would land on the has-tools branch because builtins.Register
// populates several entries before the dashboard ever sees the
// config. The fresh-install branch would be unreachable in
// production.
func TestHandleDashboard_BuiltinsOnlyStillTreatedAsFreshInstall(t *testing.T) {
	srv, _ := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"factorly.fetch": {Type: "builtin"},
			"factorly.code":  {Type: "builtin"},
			"factorly.shell": {Type: "builtin"},
			"some.workflow":  {Type: "workflow"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Fresh-install signature: "Create a tool by hand" tile that
	// only appears in the no-user-tools branch.
	if !strings.Contains(body, "Create a tool by hand") {
		t.Error("expected fresh-install tiles when config contains only builtins + workflows")
	}
	// And the has-tools-only "Try a built-in" tile must NOT appear.
	if strings.Contains(body, "Try a built-in") {
		t.Error("did not expect has-tools 'Try a built-in' tile when there are no user-defined tools")
	}
}

// TestHandleDashboard_HasToolsNoCallsShowsBuildOutTiles covers the
// second quick-start branch: the user has tools defined but no
// calls in the audit log yet. They should see prompts to fill in
// the missing pieces (vault credentials, OAuth, workspaces, more
// blueprints) plus the "Try a built-in" / "Compose a workflow"
// nudges. None of these tiles should appear on a fresh install
// (different branch) so we leave that assertion to the
// fresh-install test.
//
// The tile order is part of the contract — see quickStartTiles'
// doc comment for the gradual-enhancement rationale — so the
// assertion below pins the sequence, not just the membership.
func TestHandleDashboard_HasToolsNoCallsShowsBuildOutTiles(t *testing.T) {
	srv, _ := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"my.tool": {Type: "cli", Command: "true"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Gradual-enhancement order: try → vault → auth → workflow →
	// workspace → more blueprints. Verify each appears AFTER its
	// predecessor by walking the indices forward.
	orderedTitles := []string{
		"Try a built-in",
		"Add credentials to the vault",
		"Connect an OAuth provider",
		"Compose a workflow",
		"Set up a workspace",
		"Explore more blueprints",
	}
	prev := -1
	prevTitle := ""
	for _, title := range orderedTitles {
		idx := strings.Index(body, title)
		if idx < 0 {
			t.Errorf("expected tile %q in has-tools dashboard", title)
			continue
		}
		if idx < prev {
			t.Errorf("tile order regressed: %q (at %d) should come after %q (at %d)",
				title, idx, prevTitle, prev)
		}
		prev = idx
		prevTitle = title
	}

	// Section header + tile targets — independent of order.
	for _, want := range []string{
		"Get started",
		`href="/vault/new"`,
		`href="/auth/new"`,
		`href="/blueprints/browse"`,
		`href="/workspaces/new"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in has-tools dashboard body", want)
		}
	}
	// Fresh-install-only copy must not appear here.
	if strings.Contains(body, "Create a tool by hand") {
		t.Error("did not expect fresh-install 'Create a tool by hand' on has-tools dashboard")
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

// --- Top-vault-keys tests ---------------------------------------------

func TestTopVaultKeys_EmptyInputReturnsNil(t *testing.T) {
	if got := topVaultKeys(nil, 10); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestTopVaultKeys_NoVaultKeysReturnsNil(t *testing.T) {
	// Entries exist but none declared vault_keys.
	entries := []logger.Entry{
		{Tool: "x", Status: "success"},
		{Tool: "y", Status: "success"},
	}
	if got := topVaultKeys(entries, 10); got != nil {
		t.Fatalf("expected nil when no entries reference vault keys, got %#v", got)
	}
}

func TestTopVaultKeys_OrdersByCountThenName(t *testing.T) {
	entries := []logger.Entry{
		{Tool: "github.list", VaultKeys: []string{"GITHUB_TOKEN"}},
		{Tool: "github.create", VaultKeys: []string{"GITHUB_TOKEN"}},
		{Tool: "github.delete", VaultKeys: []string{"GITHUB_TOKEN"}},
		{Tool: "slack.post", VaultKeys: []string{"SLACK_BOT"}},
		{Tool: "slack.read", VaultKeys: []string{"SLACK_BOT"}},
		// Ties at 1; alphabetical breaks them: ANTHROPIC_KEY before
		// OPENAI_KEY.
		{Tool: "claude.ask", VaultKeys: []string{"ANTHROPIC_KEY"}},
		{Tool: "gpt.ask", VaultKeys: []string{"OPENAI_KEY"}},
		// Empty string filtered out:
		{Tool: "junk", VaultKeys: []string{""}},
	}
	got := topVaultKeys(entries, 10)
	if len(got) != 4 {
		t.Fatalf("expected 4 keys, got %d (%#v)", len(got), got)
	}
	if got[0].Key != "GITHUB_TOKEN" || got[0].Count != 3 || got[0].Pct != 100 {
		t.Errorf("leader wrong: %#v", got[0])
	}
	if got[1].Key != "SLACK_BOT" || got[1].Count != 2 {
		t.Errorf("second wrong: %#v", got[1])
	}
	if got[2].Key != "ANTHROPIC_KEY" {
		t.Errorf("tiebreak wrong, expected ANTHROPIC_KEY before OPENAI_KEY, got %q", got[2].Key)
	}
	// 2/3 * 100 = 66 (integer div).
	if got[1].Pct != 66 {
		t.Errorf("expected SLACK_BOT.Pct=66, got %d", got[1].Pct)
	}
}

func TestTopVaultKeys_RespectsLimitN(t *testing.T) {
	entries := []logger.Entry{
		{Tool: "x", VaultKeys: []string{"A"}},
		{Tool: "x", VaultKeys: []string{"B"}},
		{Tool: "x", VaultKeys: []string{"C"}},
		{Tool: "x", VaultKeys: []string{"D"}},
	}
	got := topVaultKeys(entries, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
}

func TestHandleDashboard_RendersTopVaultKeysPanel(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{"x.tool": {Type: "cli", Command: "true"}},
	})
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-10 * time.Minute), Tool: "x.tool", Status: "success", VaultKeys: []string{"GITHUB_TOKEN"}},
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "x.tool", Status: "success", VaultKeys: []string{"GITHUB_TOKEN", "SLACK_BOT"}},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Top vault items", "GITHUB_TOKEN", "SLACK_BOT"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in dashboard body", want)
		}
	}
}

// TestHandleDashboard_OversightTilesDeepLinkToHistory pins the
// oversight tile → /history?status=X drill-down so a refactor
// can't quietly drop the affordance. Each tile must link to the
// matching status filter.
func TestHandleDashboard_OversightTilesDeepLinkToHistory(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{"x.tool": {Type: "cli", Command: "true"}},
	})
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "x.tool", Status: "success"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`href="/history?status=success"`,
		`href="/history?status=error"`,
		`href="/history?status=blocked"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected oversight tile linking to %q", want)
		}
	}
}

// TestHandleDashboard_TopToolsHasManageLink mirrors the Top vault
// items manage link — header should also have one to /tools.
func TestHandleDashboard_TopToolsHasManageLink(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{"x.tool": {Type: "cli", Command: "true"}},
	})
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "x.tool", Status: "success"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `<a href="/tools" class="text-[10px] text-indigo-600`) {
		t.Error("expected Top tools panel header to carry a 'manage →' link to /tools")
	}
}

// TestHandleDashboard_TopToolsRowsLinkToToolPage pins the deep-link
// from each top-tools bar row to its /tools/{name} page. Without
// this, the affordance is invisible to a refactor that re-shapes
// the row markup.
func TestHandleDashboard_TopToolsRowsLinkToToolPage(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"alpha.tool": {Type: "cli", Command: "true"},
			"beta.tool":  {Type: "cli", Command: "true"},
		},
	})
	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-10 * time.Minute), Tool: "alpha.tool", Status: "success"},
		{Timestamp: time.Now().Add(-9 * time.Minute), Tool: "alpha.tool", Status: "success"},
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "beta.tool", Status: "success"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`href="/tools/alpha.tool"`, `href="/tools/beta.tool"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in top-tools rollup, body did not contain it", want)
		}
	}
}

func TestHandleDashboard_NoStatusStrip(t *testing.T) {
	// Status strip was removed; the markers that used to identify it
	// must not appear anywhere on the page.
	srv, _ := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{"x.tool": {Type: "cli", Command: "true"}},
	})
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, banned := range []string{"BlueprintsInstalled", "of 0 available", "tiers opened", "VaultTiersOpened"} {
		if strings.Contains(body, banned) {
			t.Errorf("status-strip remnant %q still rendered", banned)
		}
	}
}

// --- Helpers -----------------------------------------------------------

// --- Feed-seed tests --------------------------------------------------

func TestFeedSeed_EmptyInputReturnsNil(t *testing.T) {
	if got := feedSeed(nil, 30); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
	if got := feedSeed([]logger.Entry{}, 30); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestFeedSeed_CoalescesWorkflowRun(t *testing.T) {
	// Order matches what the audit log stores: oldest-first. The
	// parent's audit entry is written AFTER its children because the
	// proxy logs the outer call's result once all steps return.
	base := time.Now().Add(-30 * time.Minute)
	entries := []logger.Entry{
		{Timestamp: base, Tool: "step.one", Status: "success", WorkflowRunID: "run-1", WorkflowName: "wf.demo"},
		{Timestamp: base.Add(1 * time.Second), Tool: "step.two", Status: "success", WorkflowRunID: "run-1", WorkflowName: "wf.demo"},
		// "Real parent" entry — same tool name as the workflow,
		// immediately before its first child in newest-first order
		// (i.e. immediately after in oldest-first). groupHistoryEntries
		// detects this and suppresses the duplicate row, hoisting the
		// parent's hash onto the synthesized lead.
		{Timestamp: base.Add(2 * time.Second), Tool: "wf.demo", Status: "success", Hash: "parent-hash-aaaa"},
	}
	got := feedSeed(entries, 30)
	if len(got) != 1 {
		t.Fatalf("expected 1 group (one coalesced workflow), got %d (%#v)", len(got), got)
	}
	g := got[0]
	if !g.IsWorkflow {
		t.Errorf("expected IsWorkflow=true on the workflow group")
	}
	if g.StepCount != 2 {
		t.Errorf("StepCount: want 2, got %d", g.StepCount)
	}
	if g.Lead.Tool != "wf.demo" {
		t.Errorf("Lead.Tool: want wf.demo, got %q", g.Lead.Tool)
	}
	if g.Lead.Hash != "parent-hash-aaaa" {
		t.Errorf("Lead.Hash should be hoisted from suppressed parent, got %q", g.Lead.Hash)
	}
	// Children render in execution order (oldest first).
	if len(g.Children) != 2 || g.Children[0].Tool != "step.one" || g.Children[1].Tool != "step.two" {
		t.Errorf("children out of order: %#v", g.Children)
	}
}

func TestFeedSeed_CapsAtMaxRows(t *testing.T) {
	base := time.Now().Add(-1 * time.Hour)
	var entries []logger.Entry
	for i := 0; i < 50; i++ {
		entries = append(entries, logger.Entry{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Tool:      "x.tool",
			Status:    "success",
		})
	}
	got := feedSeed(entries, 30)
	if len(got) != 30 {
		t.Fatalf("expected 30 groups (cap), got %d", len(got))
	}
}

func TestFeedSeed_NewestFirst(t *testing.T) {
	base := time.Now().Add(-1 * time.Hour)
	entries := []logger.Entry{
		{Timestamp: base, Tool: "oldest", Status: "success"},
		{Timestamp: base.Add(5 * time.Minute), Tool: "middle", Status: "success"},
		{Timestamp: base.Add(10 * time.Minute), Tool: "newest", Status: "success"},
	}
	got := feedSeed(entries, 30)
	if len(got) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(got))
	}
	if got[0].Lead.Tool != "newest" {
		t.Errorf("first group should be newest, got %q", got[0].Lead.Tool)
	}
	if got[2].Lead.Tool != "oldest" {
		t.Errorf("last group should be oldest, got %q", got[2].Lead.Tool)
	}
}

func TestHandleDashboard_ActiveUseSeedsWorkflowRowWithRunID(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{
			"wf.demo": {Type: "workflow"},
		},
	})

	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	now := time.Now()
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: now.Add(-5 * time.Minute), Tool: "step.a", Status: "success", WorkflowRunID: "run-xyz", WorkflowName: "wf.demo"},
		{Timestamp: now.Add(-4 * time.Minute), Tool: "step.b", Status: "success", WorkflowRunID: "run-xyz", WorkflowName: "wf.demo"},
		{Timestamp: now.Add(-3 * time.Minute), Tool: "wf.demo", Status: "success", Hash: "parent-hash-xyz"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Debug if anything fails below: dump the relevant region.
	defer func() {
		if t.Failed() {
			start := strings.Index(body, "dashboard-feed")
			if start >= 0 {
				end := start + 2000
				if end > len(body) {
					end = len(body)
				}
				t.Logf("feed region: %s", body[start:end])
			}
		}
	}()
	// Seed row carries data-run-id so the JS can rehydrate its map
	// and merge subsequent workflow_step events into the existing
	// parent rather than synthesizing a duplicate.
	if !strings.Contains(body, `data-run-id="run-xyz"`) {
		t.Error("expected seed workflow row to carry data-run-id")
	}
	// data-feed-row="seed" marks all server-rendered rows so the JS
	// rehydration query selector can pick them up.
	if !strings.Contains(body, `data-feed-row="seed"`) {
		t.Error("expected seed rows to carry data-feed-row marker")
	}
	// The empty-state placeholder must NOT render when we have a seed.
	// (Note: "Waiting for the next call…" also appears as a literal
	// string inside the inline <script> for clearDashboardFeed; we
	// check the id-bearing div specifically, not the raw substring.)
	if strings.Contains(body, `id="dashboard-feed-empty"`) {
		t.Error("did not expect dashboard-feed-empty div when feed is seeded")
	}
}

// TestHandleDashboard_SeedRowsCarryDetailHxGet verifies that
// server-rendered standalone seed rows expose the htmx attributes
// needed to lazy-load /history/{hash}/detail on first click.
// Without this, clicking a row would expand to "loading…" forever.
func TestHandleDashboard_SeedRowsCarryDetailHxGet(t *testing.T) {
	srv, cfgPath := testServer(t, &config.Config{
		Tools: map[string]config.ToolConfig{"x.tool": {Type: "cli", Command: "true"}},
	})

	logPath := filepath.Join(filepath.Dir(cfgPath), "audit.jsonl")
	writeDashboardLog(t, logPath, []logger.Entry{
		{Timestamp: time.Now().Add(-5 * time.Minute), Tool: "x.tool", Hash: "seed-hash-abcd", Status: "success", Interface: "cli"},
	})
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/history/seed-hash-abcd/detail"`) {
		t.Error("expected seed standalone row to carry hx-get on its summary")
	}
	if !strings.Contains(body, `hx-trigger="click once"`) {
		t.Error("expected hx-trigger=\"click once\" so detail loads on first expand")
	}
	if !strings.Contains(body, `class="feed-row-body`) {
		t.Error("expected feed-row-body container for the lazy-loaded detail")
	}
}

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
