// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/logger"
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
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	toolFilter := r.URL.Query().Get("tool")
	statusFilter := r.URL.Query().Get("status")

	entries := readRecentLogs(100)

	// Apply filters
	if toolFilter != "" || statusFilter != "" {
		var filtered []historyEntry
		for _, e := range entries {
			if toolFilter != "" && !strings.Contains(e.Tool, toolFilter) {
				continue
			}
			if statusFilter != "" && e.Status != statusFilter {
				continue
			}
			filtered = append(filtered, e)
		}
		entries = filtered
	}

	s.render(w, "history.html", map[string]any{
		"Title":        "History",
		"Nav":          "history",
		"Entries":      entries,
		"ToolFilter":   toolFilter,
		"StatusFilter": statusFilter,
	})
}

func readRecentLogs(max int) []historyEntry {
	path := logger.DefaultLogPath()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Read all lines (we'll take the last `max`)
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
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
