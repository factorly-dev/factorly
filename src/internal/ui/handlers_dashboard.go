// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
)

// dashboardData is the render payload for /dashboard.
//
// QuickStart is only set when HasAnyCalls is false (fresh install).
// TopTools, TopVaultKeys, and Oversight are computed over the last
// 24h of the audit log; helpers return zero values on empty input
// so the template can render "no calls in last 24h" without NaN.
//
// FeedSeed is the initial server-rendered set of feed rows so the
// "Live activity" panel doesn't look dead when the page loads with
// existing audit data. It uses the same coalescing rules as the live
// JS so a workflow run that finished an hour ago renders as one
// expandable row with its steps nested, exactly the way an incoming
// run would render. The JS rehydrates its run-id Map from the seed
// rows on init so subsequent workflow_step events merge into the
// right parent rather than creating duplicates.
type dashboardData struct {
	HasAnyCalls   bool
	QuickStart    []ctaTile
	TopTools      []toolRollup
	TopVaultKeys  []vaultRollup
	Oversight     oversightCounts
	FeedSeed      []historyGroup
	WindowLabel   string
}

// ctaTile is one quick-start card shown when the user has no audit
// log entries yet. Tiles are static markup; the handler picks which
// set to show based on install state.
type ctaTile struct {
	Title       string
	Body        string
	Href        string
	ButtonLabel string
}

// toolRollup is one row in the top-tools bar list.
type toolRollup struct {
	Tool  string
	Count int
	// Pct is Count/MaxCount * 100, rounded. Drives the bar width
	// (0–100) in the template. The leader is always 100; others
	// scale relative to it.
	Pct int
}

// vaultRollup is one row in the top-vault-keys bar list. Count is
// the number of calls in the window whose tool declared this key
// in its config.vault_keys list — a "this key was needed by my
// activity" signal rather than a strict "this call resolved it"
// signal (the proxy resolves vault refs before logging, so the
// audit entries have already-resolved param values; the declared
// list on the entry is the closest stable proxy we have).
type vaultRollup struct {
	Key   string
	Count int
	Pct   int
}

// oversightCounts breaks the window's calls down by status. We don't
// compute percentages in Go — the template divides Success / Total
// (etc.) and guards against div-by-zero when Total is 0.
type oversightCounts struct {
	Success int
	Blocked int
	Error   int
	Total   int
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	entries := readRecentEntries(s.cfgPath, 1000)
	cutoff := time.Now().Add(-24 * time.Hour)
	inWindow := filterAfter(entries, cutoff)

	data := dashboardData{
		HasAnyCalls:  len(entries) > 0,
		TopTools:     topTools(inWindow, 10),
		TopVaultKeys: topVaultKeys(inWindow, 10),
		Oversight:    oversightBreakdown(inWindow),
		FeedSeed:     feedSeed(inWindow, dashboardFeedMaxRows),
		WindowLabel:  "last 24h",
	}
	if !data.HasAnyCalls {
		data.QuickStart = s.quickStartTiles()
	}

	s.render(w, "dashboard.html", map[string]any{
		"Title": "Dashboard",
		"Nav":   "dashboard",
		"Data":  data,
	})
}

// dashboardFeedMaxRows is the visible cap on the live feed, applied
// both to the initial seed (server-rendered from the 24h window) and
// to incoming SSE events (the JS trims oldest rows once we exceed it).
// Keep both sides in sync — JS uses the same constant.
const dashboardFeedMaxRows = 30

// feedSeed builds the initial set of feed rows from `windowed` (the
// last-24h slice in oldest-first order). Coalesces workflow runs by
// run_id using the same logic /history uses, so a run that finished
// before the page loaded renders as one expandable parent row with
// its steps nested — identical to the live JS coalescing path.
//
// Returns at most maxRows groups, newest-first. If `windowed` holds
// more than maxRows after grouping, the oldest groups are dropped so
// the freshest activity always shows.
func feedSeed(windowed []logger.Entry, maxRows int) []historyGroup {
	if len(windowed) == 0 || maxRows <= 0 {
		return nil
	}
	// groupHistoryEntries expects newest-first input. windowed is
	// oldest-first (from the JSONL); reverse it into a fresh slice.
	reversed := make([]logger.Entry, len(windowed))
	for i, e := range windowed {
		reversed[len(windowed)-1-i] = e
	}
	enriched := make([]historyEntry, 0, len(reversed))
	for _, raw := range reversed {
		enriched = append(enriched, historyEntry{
			Timestamp:     raw.Timestamp.Format("2006-01-02 15:04:05"),
			TimestampRel:  relativeTime(raw.Timestamp),
			Tool:          raw.Tool,
			Interface:     raw.Interface,
			Status:        raw.Status,
			DurationMs:    raw.DurationMs,
			ShadowAction:  raw.ShadowAction,
			Output:        truncate(raw.Output, 200),
			Error:         raw.Error,
			Params:        raw.Params,
			AgentID:       raw.AgentID,
			Workspace:     raw.Workspace,
			Hash:          raw.Hash,
			ReplayedFrom:  raw.ReplayedFrom,
			Replayable:    isReplayable(raw),
			SourceSHA:     raw.SourceSHA,
			Promotable:    raw.Tool == "factorly.code" && raw.Status == "success" && raw.SourceSHA != "",
			WorkflowRunID: raw.WorkflowRunID,
			WorkflowName:  raw.WorkflowName,
		})
	}
	groups := groupHistoryEntries(enriched)
	if len(groups) > maxRows {
		groups = groups[:maxRows]
	}
	return groups
}

// readRecentEntries reads up to `max` newest audit entries as raw
// logger.Entry values. /history's readRecentLogs is similar but
// returns the UI's enriched historyEntry shape (formatted timestamps,
// derived booleans) — for rollup math we want the raw time.Time so
// we can window by absolute timestamp.
func readRecentEntries(cfgPath string, max int) []logger.Entry {
	path := os.Getenv("FACTORLY_LOG_PATH")
	if path == "" {
		path = logger.ProjectLogPath(cfgPath)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	start := 0
	if len(lines) > max {
		start = len(lines) - max
	}
	lines = lines[start:]

	entries := make([]logger.Entry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e logger.Entry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// filterAfter returns the entries with Timestamp >= t. Input order
// (oldest-first from the JSONL) is preserved. The audit log is
// strictly append-only, so we could binary-search; linear is fine
// at our scale (1000-entry window).
func filterAfter(entries []logger.Entry, t time.Time) []logger.Entry {
	out := make([]logger.Entry, 0, len(entries))
	for _, e := range entries {
		if !e.Timestamp.Before(t) {
			out = append(out, e)
		}
	}
	return out
}

// topTools returns the top-n tools by call count over `entries`.
// Ties broken alphabetically. Pct is the percentage of the leader's
// count (so the leader's bar is always 100). Returns empty slice on
// empty input — no NaN risk.
func topTools(entries []logger.Entry, n int) []toolRollup {
	if len(entries) == 0 || n <= 0 {
		return nil
	}
	counts := map[string]int{}
	for _, e := range entries {
		if e.Tool == "" {
			continue
		}
		counts[e.Tool]++
	}
	rows := make([]toolRollup, 0, len(counts))
	for tool, count := range counts {
		rows = append(rows, toolRollup{Tool: tool, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Tool < rows[j].Tool
	})
	if len(rows) > n {
		rows = rows[:n]
	}
	leader := rows[0].Count
	for i := range rows {
		// Leader pegs at 100; integer division floors others. leader
		// is >0 here because rows is non-empty and a row only enters
		// counts via a non-zero count.
		rows[i].Pct = rows[i].Count * 100 / leader
	}
	return rows
}

// oversightBreakdown counts the status mix in `entries`. Unknown /
// missing statuses contribute to Total but to none of the buckets;
// rare and safe (template never divides without a Total > 0 guard).
func oversightBreakdown(entries []logger.Entry) oversightCounts {
	var c oversightCounts
	for _, e := range entries {
		c.Total++
		switch e.Status {
		case "success":
			c.Success++
		case "blocked":
			c.Blocked++
		case "error":
			c.Error++
		}
	}
	return c
}

// topVaultKeys returns the top-n vault keys by reference count over
// `entries`. Each entry contributes one count for every key in its
// logger.Entry.VaultKeys slice (which mirrors the tool's declared
// `vault_keys` config at log time). Sorted by count desc, ties
// broken alphabetically. Pct mirrors topTools — leader pegs at 100,
// integer division floors the rest. Returns nil on empty input.
//
// Caveat: the proxy resolves vault refs in params before logging,
// so audit entries never contain literal `{{vault:KEY}}` strings.
// This rollup counts which keys the tools that ran *declared* they
// needed, not which keys actually got resolved on each call. Same
// signal in practice but worth knowing if you go looking for a
// stricter "resolved" semantic later.
func topVaultKeys(entries []logger.Entry, n int) []vaultRollup {
	if len(entries) == 0 || n <= 0 {
		return nil
	}
	counts := map[string]int{}
	for _, e := range entries {
		for _, k := range e.VaultKeys {
			if k == "" {
				continue
			}
			counts[k]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	rows := make([]vaultRollup, 0, len(counts))
	for k, c := range counts {
		rows = append(rows, vaultRollup{Key: k, Count: c})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Key < rows[j].Key
	})
	if len(rows) > n {
		rows = rows[:n]
	}
	leader := rows[0].Count
	for i := range rows {
		rows[i].Pct = rows[i].Count * 100 / leader
	}
	return rows
}

// quickStartTiles picks the empty-state CTAs to show. Branches on
// "do you have any tools defined yet?" so a fresh install sees
// install-the-tools prompts (browse / import / create / where do
// my credentials go), while someone with tools but no calls sees
// fill-in-the-blanks prompts in a gradual-enhancement order.
//
// Ordering within each branch follows the path a new user would
// reasonably take, not the order the panels happen to render. The
// idea is that scrolling the list top-to-bottom is itself a
// "what to do next" tutorial.
//
// Tiles are unconditional within each branch — we don't try to
// detect "does any tool actually need vault credentials?" yet.
// The cost of an irrelevant tile is mild visual noise; the cost
// of hiding a relevant one is the user not discovering the feature.
func (s *Server) quickStartTiles() []ctaTile {
	// "Fresh install" means "no user-defined tools yet" — builtins
	// (factorly.fetch, factorly.code, factorly.store.*, etc.) and
	// workflow definitions don't count. The user thinks of builtins
	// as "factorly's stuff," not as tools they added; without the
	// filter, every real install would skip the fresh-install branch
	// because builtins.Register populates ~11 tools before we ever
	// see the config.
	hasUserTools := false
	if s.cfg != nil {
		for _, tc := range s.cfg.Tools {
			if tc.Type == "workflow" || tc.Type == "builtin" {
				continue
			}
			hasUserTools = true
			break
		}
	}
	if !hasUserTools {
		// Fresh install arc: easiest path to a working setup (blueprints)
		// → adapt an existing API (OpenAPI import) → write your own
		// (hand-built tool) → support layer (vault for the credentials
		// any of the above will need).
		return []ctaTile{
			{
				Title:       "Browse blueprints",
				Body:        "Install a ready-made toolchain (GitHub, Slack, Trello, Linear, ...) in one click.",
				Href:        "/blueprints/browse",
				ButtonLabel: "Browse",
			},
			{
				Title:       "Import an OpenAPI spec",
				Body:        "Point at any spec URL or file; we generate one tool per operation.",
				Href:        "/tools/import",
				ButtonLabel: "Import",
			},
			{
				Title:       "Create a tool by hand",
				Body:        "CLI command, REST endpoint, MCP server, or a Go script — your call.",
				Href:        "/tools/new",
				ButtonLabel: "Create",
			},
			{
				Title:       "Stash credentials in the vault",
				Body:        "Encrypted key/value store for API tokens, passwords, anything tools reference via {{vault:KEY}}.",
				Href:        "/vault/new",
				ButtonLabel: "Add a secret",
			},
		}
	}
	// Has-tools-no-calls arc: fire something immediately (try it) →
	// fix the most common reason a call fails (missing credentials)
	// → harder credentials (OAuth) → start composing what you have
	// (workflows) → scale to multiple environments (workspaces) →
	// power-user expansion (more blueprints).
	return []ctaTile{
		{
			Title:       "Try a built-in",
			Body:        "Browse your tool list and click \"Try It\" on any row to fire a real call.",
			Href:        "/tools",
			ButtonLabel: "Open tools",
		},
		{
			Title:       "Add credentials to the vault",
			Body:        "Your tools probably reference {{vault:KEY}} for API tokens. Add them here so calls don't 401.",
			Href:        "/vault/new",
			ButtonLabel: "Add a secret",
		},
		{
			Title:       "Connect an OAuth provider",
			Body:        "For tools that auth through GitHub / Google / etc. — register the provider, then log in with one click.",
			Href:        "/auth/new",
			ButtonLabel: "Set up auth",
		},
		{
			Title:       "Compose a workflow",
			Body:        "Chain tools into a multi-step pipeline with conditionals and state.",
			Href:        "/workflows/new",
			ButtonLabel: "New workflow",
		},
		{
			Title:       "Set up a workspace",
			Body:        "Named env + vault overlays so you can swap between staging / prod / per-customer setups without editing YAML.",
			Href:        "/workspaces/new",
			ButtonLabel: "New workspace",
		},
		{
			Title:       "Explore more blueprints",
			Body:        "Stack another ready-made toolchain on top — Slack, Trello, Linear, and more.",
			Href:        "/blueprints/browse",
			ButtonLabel: "Browse",
		},
	}
}
