// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	toolFilter := r.URL.Query().Get("tool")
	statusFilter := r.URL.Query().Get("status")
	workspaceFilter := r.URL.Query().Get("workspace")

	entries := readRecentLogs(s.cfgPath, 100)

	// Collect distinct workspaces present in the (unfiltered) log so the
	// dropdown shows everything the user has run, not just what matches
	// the current filter.
	workspacesSeen := map[string]bool{}
	for _, e := range entries {
		if e.Workspace != "" {
			workspacesSeen[e.Workspace] = true
		}
	}
	workspaceOptions := make([]string, 0, len(workspacesSeen))
	for k := range workspacesSeen {
		workspaceOptions = append(workspaceOptions, k)
	}
	sortStringsAsc(workspaceOptions)

	// Apply filters
	if toolFilter != "" || statusFilter != "" || workspaceFilter != "" {
		var filtered []historyEntry
		for _, e := range entries {
			if toolFilter != "" && !strings.Contains(e.Tool, toolFilter) {
				continue
			}
			if statusFilter != "" && e.Status != statusFilter {
				continue
			}
			if workspaceFilter != "" && e.Workspace != workspaceFilter {
				continue
			}
			filtered = append(filtered, e)
		}
		entries = filtered
	}

	s.render(w, "history.html", map[string]any{
		"Title":            "History",
		"Nav":              "history",
		"Entries":          entries,
		"ToolFilter":       toolFilter,
		"StatusFilter":     statusFilter,
		"WorkspaceFilter":  workspaceFilter,
		"WorkspaceOptions": workspaceOptions,
	})
}

func readRecentLogs(cfgPath string, max int) []historyEntry {
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		return readLogsFromPath(p, max)
	}
	return readLogsFromPath(logger.ProjectLogPath(cfgPath), max)
}

func readLogsFromPath(path string, max int) []historyEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Read all lines (we'll take the last `max`)
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

	// Take last `max` lines
	start := 0
	if len(lines) > max {
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
			Timestamp:    raw.Timestamp.Format("2006-01-02 15:04:05"),
			TimestampRel: relativeTime(raw.Timestamp),
			Tool:         raw.Tool,
			Interface:    raw.Interface,
			Status:       raw.Status,
			DurationMs:   raw.DurationMs,
			ShadowAction: raw.ShadowAction,
			Output:       truncate(raw.Output, 200),
			Error:        raw.Error,
			Params:       raw.Params,
			AgentID:      raw.AgentID,
			Workspace:    raw.Workspace,
			Hash:         raw.Hash,
			ReplayedFrom: raw.ReplayedFrom,
			Replayable:   isReplayable(raw),
			SourceSHA:    raw.SourceSHA,
			Promotable:   raw.Tool == "factorly.code" && raw.Status == "success" && raw.SourceSHA != "",
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

// isReplayable decides whether a `Replay` button should appear on the
// history row for raw. v1 rules:
//   - The entry must carry a Hash (the audit chain identifier we use
//     in the replay URL). Old entries from before the chain was added
//     don't have one.
//   - The Interface must NOT be "workflow" — workflow step calls
//     aren't independently replayable; the full run is replayed via a
//     separate (future) flow tied to the dashboard coalescing work.
//   - The Tool must still exist in the current config; replaying a
//     tool that's been deleted produces a confusing failure. We can't
//     check the live config from here (this helper runs in the log
//     reader), so we defer that check to handleHistoryReplay and
//     surface a 404 there. The button still shows for entries whose
//     tools were since removed; that's better than silently hiding a
//     row the operator might want to re-fire as a workflow input.
func isReplayable(raw logger.Entry) bool {
	if raw.Hash == "" {
		return false
	}
	if raw.Interface == "workflow" {
		return false
	}
	if raw.Tool == "" {
		return false
	}
	return true
}

// findAuditEntryByHash scans the audit log for the entry whose Hash
// matches. Linear sequential scan from oldest to newest — cheap on
// any reasonable log; future perf work (sidecar index) is tracked
// separately. Returns (nil, nil) when the hash isn't present so the
// caller can 404 cleanly.
func findAuditEntryByHash(cfgPath, hash string) (*logger.Entry, error) {
	if hash == "" {
		return nil, errors.New("hash is required")
	}
	path := logger.ProjectLogPath(cfgPath)
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		path = p
	}
	f, err := os.Open(path) // #nosec G304 -- audit log path is operator-supplied
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// 1MB max line — matches the readLogsFromPath setting; factorly.code
	// entries embed their full script body and exceed the default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e logger.Entry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		if e.Hash == hash {
			return &e, nil
		}
	}
	return nil, scanner.Err()
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
		http.Error(w, "entry is not replayable (workflow steps and entries without a chain hash are excluded)", http.StatusBadRequest)
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
