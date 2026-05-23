// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/blueprints"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/workspace"
)

// dashboardData is the render payload for /dashboard.
//
// The page is always populated. Status is built from current install
// state and never empty. QuickStart is only set when HasAnyCalls is
// false (fresh install). TopTools and Oversight are computed over
// the last 24h of the audit log; both helpers return zero values on
// empty input so the template can render "0 calls" without NaN.
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
	HasAnyCalls bool
	Status      statusStrip
	QuickStart  []ctaTile
	TopTools    []toolRollup
	Oversight   oversightCounts
	FeedSeed    []historyGroup
	WindowLabel string
}

// statusStrip is the always-visible "what you have" row at the top
// of the dashboard. Counts are derived from the live config + on-disk
// state, not the audit log.
type statusStrip struct {
	ToolsByType         []typeCount
	ToolsTotal          int
	BlueprintsInstalled int
	BlueprintsAvailable int
	Workspaces          int
	VaultTiersOpened    int
	VaultTiersTotal     int
	StoreTiersPresent   int
	StoreTiersTotal     int
}

// typeCount is one tool-type pill in the status strip.
type typeCount struct {
	Type  string // "cli", "rest", "mcp", "code", "builtin", "workflow"
	Count int
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
		HasAnyCalls: len(entries) > 0,
		Status:      s.buildStatusStrip(r),
		TopTools:    topTools(inWindow, 10),
		Oversight:   oversightBreakdown(inWindow),
		FeedSeed:    feedSeed(inWindow, dashboardFeedMaxRows),
		WindowLabel: "last 24h",
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

// buildStatusStrip captures "what is in this install right now."
// Independent of audit log; renders the same on a fresh install
// (no calls yet) as on a busy one.
func (s *Server) buildStatusStrip(r *http.Request) statusStrip {
	strip := statusStrip{}

	// Tool counts by type. Workflows live in the same map (type ==
	// "workflow") but we surface them as a separate pill so the
	// "real tools" count isn't inflated by workflow definitions.
	typeCounts := map[string]int{}
	if s.cfg != nil {
		for _, tc := range s.cfg.Tools {
			typeCounts[tc.Type]++
			if tc.Type != "workflow" {
				strip.ToolsTotal++
			}
		}
	}
	for _, t := range []string{"builtin", "cli", "rest", "mcp", "code", "workflow"} {
		if c := typeCounts[t]; c > 0 {
			strip.ToolsByType = append(strip.ToolsByType, typeCount{Type: t, Count: c})
		}
	}

	// Blueprint counts.
	if installed, err := blueprints.List(s.cfgPath); err == nil {
		strip.BlueprintsInstalled = len(installed)
	}
	strip.BlueprintsAvailable = len(blueprints.Bundled())

	// Workspace count.
	if wss, err := workspace.List(s.cfgPath); err == nil {
		strip.Workspaces = len(wss)
	}

	// Vault tiers — count how many of the three (workspace / project
	// / global) the Manager already has opened. We don't try to
	// distinguish "exists but locked" from "doesn't exist" here; the
	// strip is a quick at-a-glance signal and the /vault page is the
	// source of truth.
	strip.VaultTiersOpened, strip.VaultTiersTotal = s.countVaultTiers(r)

	// Store tiers — count how many of the three on-disk store files
	// exist. Same shorthand as above: a present file is "there",
	// absent is "not yet."
	strip.StoreTiersPresent, strip.StoreTiersTotal = countStoreTiers(s.requestWorkspace(r))

	return strip
}

// countVaultTiers reports (opened, total) across the three potential
// vault tiers (workspace / project / global). "Opened" comes from
// the shared vault.Manager cache, the same source the /vault page
// uses to decide whether to show "Unlock" buttons.
func (s *Server) countVaultTiers(r *http.Request) (opened, total int) {
	if s.vaultMgr == nil {
		return 0, 3
	}
	total = 3
	if s.vaultMgr.Cached("") != nil {
		opened++
	}
	if ws := s.requestWorkspace(r); ws != "" {
		if s.vaultMgr.Cached("workspace:"+ws) != nil {
			opened++
		}
	}
	// Global tier is folded into the project chain in the current
	// model (one Backend opens both project + global tiers), so we
	// don't count it separately. Surface as total=2 to be honest
	// about what we can observe.
	total = 2
	return opened, total
}

// countStoreTiers returns (present, total) where present is the
// number of on-disk store.db files visible (workspace, project,
// global). Path layout mirrors handlers_store.go.
func countStoreTiers(activeWorkspace string) (present, total int) {
	total = 3
	if activeWorkspace != "" {
		if _, err := os.Stat(workspaceStoreFilePath(activeWorkspace)); err == nil {
			present++
		}
	}
	if hasProjectDir() {
		if _, err := os.Stat(projectStoreFilePath()); err == nil {
			present++
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".config", "factorly", "store.db")); err == nil {
			present++
		}
	}
	return present, total
}

// quickStartTiles picks the empty-state CTAs to show. Branches on
// "do you have any tools defined yet?" so a fresh install gets the
// browse/import/create trio, while someone with tools but no calls
// gets "try one out" prompts.
func (s *Server) quickStartTiles() []ctaTile {
	hasTools := false
	if s.cfg != nil {
		for _, tc := range s.cfg.Tools {
			if tc.Type != "workflow" {
				hasTools = true
				break
			}
		}
	}
	if !hasTools {
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
		}
	}
	return []ctaTile{
		{
			Title:       "Try a built-in",
			Body:        "Browse your tool list and click \"Try It\" on any row to fire a real call.",
			Href:        "/tools",
			ButtonLabel: "Open tools",
		},
		{
			Title:       "Compose a workflow",
			Body:        "Chain tools into a multi-step pipeline with conditionals and state.",
			Href:        "/workflows/new",
			ButtonLabel: "New workflow",
		},
	}
}
