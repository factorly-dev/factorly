// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/proxy"
)

type historyEntry struct {
	Timestamp    string
	TimestampRel string
	Tool         string
	Interface    string
	Status       string
	DurationMs   int64
	ShadowAction string
	Output       string
	Error        string
	Params       map[string]string
	AgentID      string
	Workspace    string
	// Hash is the per-entry hash from the audit chain. Used as the
	// stable identifier in replay URLs (POST /history/{hash}/replay
	// and GET /tools/{tool}?prefill={hash}).
	Hash string
	// ReplayedFrom is the Hash of the original entry when this entry
	// itself was produced by a replay. Empty for fresh calls. Used
	// by the history UI to badge replayed rows.
	ReplayedFrom string
	// Replayable is true when the entry is eligible for the Replay
	// button: non-workflow interface (workflow runs replay as a unit
	// via a separate flow once /history coalescing lands), tool name
	// non-empty, and we have a stable Hash to address it by.
	Replayable bool
	// SourceSHA is the script hash for factorly.code / type:code calls.
	// Empty for other tools. Used by the "Save as tool" button to
	// address this specific run in the promote flow.
	SourceSHA string
	// Promotable is true when this entry is a successful factorly.code
	// call — i.e. the operator can convert this run into a named
	// type:code tool via /tools/promote.
	Promotable bool
	// WorkflowRunID is the run identifier shared by every audit
	// entry of one workflow run. Empty for non-workflow calls.
	WorkflowRunID string
	// WorkflowName is the registered name of the workflow whose run
	// this entry belongs to. Empty for non-workflow calls. Provides
	// the human label for /history's coalesced parent row.
	WorkflowName string
}

// historyGroup is a logical unit in /history's coalesced view: either
// a single standalone call or a workflow run with its child step
// entries gathered underneath. Workflow runs render as one
// expandable row showing the workflow name + step count; the steps
// appear nested when expanded.
type historyGroup struct {
	// Lead is the entry that defines the group's headline row. For
	// workflow runs, this is a synthesized parent (Tool = workflow
	// name, derived status, total duration). For standalone calls,
	// this is just the call itself.
	Lead historyEntry
	// Children are the step entries of a workflow run, newest-first.
	// Empty for standalone groups.
	Children []historyEntry
	// IsWorkflow is true for grouped workflow runs.
	IsWorkflow bool
	// StepCount is the number of step entries; >0 only for workflow
	// groups.
	StepCount int
	// FilterMatchedChildOnly is true when the active filter matched
	// only a child step (not the parent), and we're showing the
	// group anyway to preserve context. The UI uses this to hint
	// "matched step within this run."
	FilterMatchedChildOnly bool
}

// historyPageSize is the number of coalesced groups shown per page on
// /history. Load-more appends another page each click. Tuned to keep
// the initial render under a few hundred DOM nodes while still being
// useful — most operators want "last hour or so" not "last entry."
const historyPageSize = 50

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	toolFilter := r.URL.Query().Get("tool")
	statusFilter := r.URL.Query().Get("status")
	workspaceFilter := r.URL.Query().Get("workspace")

	// Read the full log (newest-first) once. Workspace dropdown needs
	// the full distinct set; pagination needs the full filtered stream
	// to slice the requested page out. The cost is one file scan per
	// request — fine for the design budget (~10MB / ~1000 entries) we
	// document for the store; if the audit log ever grows large enough
	// for this to bite, the next step is an on-disk index keyed by
	// hash, not in-memory paging tricks.
	all := readAllLogs(s.cfgPath)

	workspacesSeen := map[string]bool{}
	for _, e := range all {
		if e.Workspace != "" {
			workspacesSeen[e.Workspace] = true
		}
	}
	workspaceOptions := make([]string, 0, len(workspacesSeen))
	for k := range workspacesSeen {
		workspaceOptions = append(workspaceOptions, k)
	}
	sortStringsAsc(workspaceOptions)

	groups, nextCursor := paginateHistory(all, toolFilter, statusFilter, workspaceFilter, "", historyPageSize)

	s.render(w, "history.html", map[string]any{
		"Title":            "History",
		"Nav":              "history",
		"Groups":           groups,
		"ToolFilter":       toolFilter,
		"StatusFilter":     statusFilter,
		"WorkspaceFilter":  workspaceFilter,
		"WorkspaceOptions": workspaceOptions,
		"NextCursor":       nextCursor,
	})
}

// handleHistoryMore returns the next page of history rows as an HTMX
// fragment. The button at the bottom of /history fires hx-get with
// the current filters and the lead hash of the last visible group as
// `cursor`. Response is the rendered rows plus an OOB swap that
// replaces the load-more container (either with a fresh button
// carrying the new cursor, or with nothing when we've reached the end
// of the log).
func (s *Server) handleHistoryMore(w http.ResponseWriter, r *http.Request) {
	toolFilter := r.URL.Query().Get("tool")
	statusFilter := r.URL.Query().Get("status")
	workspaceFilter := r.URL.Query().Get("workspace")
	cursor := r.URL.Query().Get("cursor")

	all := readAllLogs(s.cfgPath)
	groups, nextCursor := paginateHistory(all, toolFilter, statusFilter, workspaceFilter, cursor, historyPageSize)

	s.renderPartial(w, "history_more", map[string]any{
		"Groups":          groups,
		"ToolFilter":      toolFilter,
		"StatusFilter":    statusFilter,
		"WorkspaceFilter": workspaceFilter,
		"NextCursor":      nextCursor,
	})
}

// groupCursor is the stable id we use to advance pagination. For
// standalone groups it's the entry's audit hash. For workflow groups
// it's the captured parent hash when we have one; otherwise we fall
// back to "wf:<runID>" so older runs (whose parent entry has scrolled
// off the visible window) still get a unique cursor. The fallback is
// safe because workflow run IDs are globally unique within the log.
func groupCursor(g historyGroup) string {
	if g.IsWorkflow {
		if g.Lead.Hash != "" {
			return g.Lead.Hash
		}
		if g.Lead.WorkflowRunID != "" {
			return "wf:" + g.Lead.WorkflowRunID
		}
	}
	return g.Lead.Hash
}

// paginateHistory groups + filters all entries, then slices out the
// requested page. cursor is the groupCursor of the last group on the
// previous page; "" returns the first page. Returns (page, nextCursor)
// where nextCursor is "" when no more pages remain.
//
// Filtering happens at the group level (same as before), but applied
// to the full entry stream rather than a 100-entry tail — so "show me
// all errors" surfaces every error in the log, not just errors that
// happen to be in the recent slice.
func paginateHistory(all []historyEntry, toolFilter, statusFilter, workspaceFilter, cursor string, pageSize int) ([]historyGroup, string) {
	groups := groupHistoryEntries(all)
	groups = filterHistoryGroups(groups, toolFilter, statusFilter, workspaceFilter)

	start := 0
	if cursor != "" {
		for i, g := range groups {
			if groupCursor(g) == cursor {
				start = i + 1
				break
			}
		}
	}
	if start >= len(groups) {
		return nil, ""
	}
	end := start + pageSize
	if end > len(groups) {
		end = len(groups)
	}
	page := groups[start:end]
	nextCursor := ""
	if end < len(groups) && len(page) > 0 {
		nextCursor = groupCursor(page[len(page)-1])
	}
	return page, nextCursor
}

// groupHistoryEntries coalesces audit entries into rendering groups.
// Entries with a non-empty WorkflowRunID belong to a workflow run
// and gather under a synthesized parent row. Entries without one
// pass through as standalone single-row groups, except for the
// "real parent" of a workflow run — the outer call that invoked the
// workflow tool, which the proxy logs *without* WorkflowRunID
// (the context value is set inside the workflow provider, after the
// proxy has already constructed its own audit entry). Those parent
// entries would otherwise show up as a duplicate row right next to
// the coalesced row; we detect them by tool name + timestamp
// adjacency and suppress.
//
// Input is newest-first (as readAllLogs returns). Output
// preserves that ordering at the group level.
//
// The synthesized parent's status reflects the run's outcome: any
// failed child → "error"; otherwise "success". Duration is the sum
// of children's durations. Timestamp uses the newest child's so the
// run sorts to its most recent activity.
func groupHistoryEntries(entries []historyEntry) []historyGroup {
	// First pass: bucket by WorkflowRunID. Empty key = standalone.
	type bucket struct {
		ids      []int // indexes into entries
		runID    string
		workflow string
	}
	buckets := map[string]*bucket{}
	order := []string{} // first-seen order (newest-first traversal)
	for i, e := range entries {
		if e.WorkflowRunID == "" {
			// Per-entry unique key so standalones stay in order.
			k := fmt.Sprintf("solo:%d", i)
			buckets[k] = &bucket{ids: []int{i}}
			order = append(order, k)
			continue
		}
		b, ok := buckets[e.WorkflowRunID]
		if !ok {
			b = &bucket{runID: e.WorkflowRunID, workflow: e.WorkflowName}
			buckets[e.WorkflowRunID] = b
			order = append(order, e.WorkflowRunID)
		}
		b.ids = append(b.ids, i)
	}

	// Detect "real parent" standalones to suppress, AND capture the
	// parent's audit hash so the synthesized lead row can carry it
	// forward — that's what makes the workflow's Replay button able
	// to re-fire the workflow as one unit (POST /history/{parent
	// hash}/replay re-invokes the workflow tool through the proxy).
	//
	// A workflow run's outer call writes its audit entry AFTER all
	// children. In newest-first traversal that places the parent
	// immediately BEFORE its run's children. Match by: tool name ==
	// workflow name, and the entry's index is exactly one less than
	// the first child's index.
	suppressed := map[int]bool{}
	parentHash := map[string]string{} // run ID → parent's audit hash
	for _, k := range order {
		b := buckets[k]
		if b.runID == "" || b.workflow == "" || len(b.ids) == 0 {
			continue
		}
		firstChildIdx := b.ids[0]
		if firstChildIdx == 0 {
			continue
		}
		candidate := entries[firstChildIdx-1]
		if candidate.WorkflowRunID == "" && candidate.Tool == b.workflow {
			suppressed[firstChildIdx-1] = true
			parentHash[b.runID] = candidate.Hash
		}
	}

	groups := make([]historyGroup, 0, len(order))
	for _, k := range order {
		b := buckets[k]
		// Standalone — single entry, no children. Skip suppressed ones.
		if b.runID == "" {
			if suppressed[b.ids[0]] {
				continue
			}
			groups = append(groups, historyGroup{Lead: entries[b.ids[0]]})
			continue
		}
		// Workflow run — synthesize a parent row from the children.
		// b.ids is in newest-first traversal order; reverse to render
		// steps in execution order (step 1, step 2, …, step N).
		children := make([]historyEntry, 0, len(b.ids))
		var totalDur int64
		runStatus := "success"
		for i := len(b.ids) - 1; i >= 0; i-- {
			ch := entries[b.ids[i]]
			children = append(children, ch)
			totalDur += ch.DurationMs
			if ch.Status != "success" {
				runStatus = "error"
			}
		}
		newest := entries[b.ids[0]]
		label := b.workflow
		if label == "" {
			label = "workflow (run " + b.runID + ")"
		}
		lead := historyEntry{
			Timestamp:    newest.Timestamp,
			TimestampRel: newest.TimestampRel,
			Tool:         label,
			Interface:    "workflow",
			Status:       runStatus,
			DurationMs:   totalDur,
			Workspace:    newest.Workspace,
			AgentID:      newest.AgentID,
			// Lead carries the suppressed parent entry's hash so
			// "Replay" on the coalesced row re-fires the whole
			// workflow via the same /history/{hash}/replay endpoint
			// any other call uses. If no real parent was found in
			// the visible window (e.g., the run pre-dates the
			// recent-entries cutoff), Replay isn't offered.
			Hash:       parentHash[b.runID],
			Replayable: parentHash[b.runID] != "",
			// Run ID is hoisted so dashboard seed rows can carry
			// data-run-id and the live-feed JS can rehydrate its
			// coalescing Map. /history doesn't need it but the
			// extra field is harmless there.
			WorkflowRunID: b.runID,
			WorkflowName:  b.workflow,
		}
		groups = append(groups, historyGroup{
			Lead:       lead,
			Children:   children,
			IsWorkflow: true,
			StepCount:  len(children),
		})
	}
	return groups
}

// filterHistoryGroups applies the tool / status / workspace filters
// at the group level. A workflow group passes if its lead matches OR
// if any of its child step entries matches; in the child-match-only
// case the group is flagged so the UI can show "matched step within
// this run."
func filterHistoryGroups(groups []historyGroup, toolFilter, statusFilter, workspaceFilter string) []historyGroup {
	if toolFilter == "" && statusFilter == "" && workspaceFilter == "" {
		return groups
	}
	matches := func(e historyEntry) bool {
		if toolFilter != "" && !strings.Contains(e.Tool, toolFilter) {
			return false
		}
		if statusFilter != "" && e.Status != statusFilter {
			return false
		}
		if workspaceFilter != "" && e.Workspace != workspaceFilter {
			return false
		}
		return true
	}
	out := make([]historyGroup, 0, len(groups))
	for _, g := range groups {
		if matches(g.Lead) {
			out = append(out, g)
			continue
		}
		if g.IsWorkflow {
			for _, ch := range g.Children {
				if matches(ch) {
					g.FilterMatchedChildOnly = true
					out = append(out, g)
					break
				}
			}
		}
	}
	return out
}

// readAllLogs reads the full audit log newest-first. /history's
// pagination requires the full filtered stream (so "show me all
// errors" surfaces every match, not just matches in the recent
// slice). Cost: one full-file scan per request. Within the
// store/audit design budget of ~10MB; revisit if logs grow huge.
func readAllLogs(cfgPath string) []historyEntry {
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		return readLogsFromPath(p, -1)
	}
	return readLogsFromPath(logger.ProjectLogPath(cfgPath), -1)
}

// readLogsFromPath returns audit entries newest-first. max < 0 means
// "no limit, return everything." max >= 0 keeps only the last `max`
// lines from the file.
func readLogsFromPath(path string, max int) []historyEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// 1MB max line: factorly.code entries embed the full script as a
	// params value, which can exceed the 256KB default scanner limit
	// and would silently skip the affected entries (hiding them from
	// /history). Matches internal/promote's scanner buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	start := 0
	if max >= 0 && len(lines) > max {
		start = len(lines) - max
	}
	lines = lines[start:]

	// Parse in reverse (most recent first)
	entries := make([]historyEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var raw logger.Entry
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		entries = append(entries, historyEntry{
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

	return entries
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		return t.Format("Jan 2")
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// isReplayable decides whether a `Replay` button should appear on
// the history row for raw. v1 rules:
//   - The entry must carry a Hash (the audit chain identifier we
//     use in the replay URL). Old entries from before the chain was
//     added don't have one.
//   - The Tool must still exist in the current config; replaying
//     a tool that's been deleted produces a confusing failure. We
//     can't check the live config from here (this helper runs in
//     the log reader), so we defer that check to
//     handleHistoryReplay and surface a 400 there. The button still
//     shows for entries whose tools were since removed; that's
//     better than silently hiding a row the operator might want to
//     re-fire.
//
// Workflow-step entries (Interface == "workflow") ARE replayable.
// Replay re-fires that step's tool with its recorded params as a
// plain standalone call — no parent workflow context, no
// workflow_run_id on the resulting audit entry. That's the right
// semantics for "re-run this specific step in isolation" without
// having to rerun the whole workflow.
func isReplayable(raw logger.Entry) bool {
	if raw.Hash == "" {
		return false
	}
	if raw.Tool == "" {
		return false
	}
	return true
}

// findAuditEntryByHash resolves the active audit-log path and
// delegates to logger.FindByHash. Thin wrapper so UI callers don't
// duplicate the cfgPath → log path resolution rule used elsewhere
// in this file (env-var override; otherwise project log path).
func findAuditEntryByHash(cfgPath, hash string) (*logger.Entry, error) {
	path := logger.ProjectLogPath(cfgPath)
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		path = p
	}
	return logger.FindByHash(path, hash)
}

// handleHistoryDetail returns the per-entry detail body (params,
// status, error, output, plus Replay / Edit-and-Replay buttons) as
// an HTML fragment. Reuses the same `history_entry_body` partial
// that /history's row bodies render with, so the dashboard's
// inline-expand affordance shows identical content to clicking the
// matching row on /history.
//
// Lookup is by audit hash (exact or ≥4-char prefix). Returns 404
// if no matching entry exists in the current log; the dashboard JS
// surfaces that as a small inline error and leaves the row
// expanded-empty so the user can collapse and try again.
func (s *Server) handleHistoryDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	raw, err := findAuditEntryByHash(s.cfgPath, hash)
	if err != nil {
		http.Error(w, "looking up entry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if raw == nil {
		http.Error(w, "audit entry not found", http.StatusNotFound)
		return
	}
	entry := historyEntry{
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
		Replayable:    isReplayable(*raw),
		SourceSHA:     raw.SourceSHA,
		Promotable:    raw.Tool == "factorly.code" && raw.Status == "success" && raw.SourceSHA != "",
		WorkflowRunID: raw.WorkflowRunID,
		WorkflowName:  raw.WorkflowName,
	}
	s.renderPartial(w, "history_entry_body", entry)
}

// handleHistoryReplay re-runs a historical call. Looks up the entry
// by Hash, validates eligibility (must be replayable, tool must
// still exist), then fires through the same proxy as any other UI
// call — vault refs re-resolve, shadow rules re-apply, audit log
// gets a fresh entry whose ReplayedFrom links back to the original.
//
// Limitation worth knowing: vault-resolved param values land in the
// audit log already-resolved (the proxy resolves before logging),
// so a replay sends those literal values. If the user originally
// passed `--token {{vault:KEY}}` and the vault has since rotated,
// the replay will send the stale resolved value. Workaround for
// now: use "Edit & Replay" (the prefill flow) and restore the
// `{{vault:...}}` template manually. A future logger-shape change
// can carry the pre-resolution template for full replay fidelity.
func (s *Server) handleHistoryReplay(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	entry, err := findAuditEntryByHash(s.cfgPath, hash)
	if err != nil {
		http.Error(w, "looking up entry: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if entry == nil {
		http.Error(w, "audit entry not found", http.StatusNotFound)
		return
	}
	if !isReplayable(*entry) {
		http.Error(w, "entry is not replayable (missing chain hash or tool name)", http.StatusBadRequest)
		return
	}
	if _, ok := s.cfg.Tools[entry.Tool]; !ok {
		http.Error(w, "tool "+entry.Tool+" is no longer registered", http.StatusBadRequest)
		return
	}

	// Use OriginalParams if validation modified them (so the replay
	// re-runs the user's intent, not the post-coerce form). Otherwise
	// Params is the only thing we logged.
	params := entry.Params
	if len(entry.OriginalParams) > 0 {
		params = entry.OriginalParams
	}

	ctx := context.WithValue(r.Context(), proxy.ReplayedFromKey, hash)
	if _, execErr := s.proxy.ExecuteWithContext(ctx, entry.Tool, params, "ui"); execErr != nil {
		// Shadow blocked / vault locked / etc. — surface as a 4xx so
		// the browser shows the message. The audit log already records
		// the blocked attempt for visibility on /history reload.
		http.Error(w, "replay blocked: "+execErr.Error(), http.StatusBadRequest)
		return
	}

	// Land back on history. The new entry will be at the top.
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}
