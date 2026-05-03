// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
)

func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok || tc.Type != "workflow" {
		http.NotFound(w, r)
		return
	}

	// List all non-workflow tools for step dropdown
	var available []string
	for n, t := range s.cfg.Tools {
		if t.Type != "workflow" {
			available = append(available, n)
		}
	}
	sort.Strings(available)

	s.render(w, "workflow_edit.html", map[string]any{
		"Title":          name,
		"Nav":            "tools",
		"Name":           name,
		"Description":    tc.Description,
		"Steps":          tc.Steps,
		"Params":         tc.Parameters,
		"AvailableTools": available,
	})
}

func (s *Server) handleWorkflowSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Rebuild steps from form
	var steps []config.StepConfig
	for i := 0; ; i++ {
		tool := r.FormValue(fmt.Sprintf("step_tool_%d", i))
		if tool == "" {
			break
		}
		store := r.FormValue(fmt.Sprintf("step_store_%d", i))
		steps = append(steps, config.StepConfig{
			Tool:  tool,
			Store: store,
		})
	}

	tc.Steps = steps
	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Tools[name] = tc

	http.Redirect(w, r, "/workflows/"+name, http.StatusFound)
}

func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	params := make(map[string]string)
	for key, vals := range r.Form {
		if len(key) > 6 && key[:6] == "param_" {
			params[key[6:]] = vals[0]
		}
	}

	start := time.Now()
	ctx := context.Background()
	result, execErr := s.proxy.ExecuteWithContext(ctx, name, params, "ui")
	duration := time.Since(start)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if execErr != nil {
		fmt.Fprintf(w, `<div class="rounded-lg border border-red-200 bg-red-50 p-4">
			<div class="flex items-center gap-2 mb-2">
				<span class="text-red-600 font-medium text-sm">Failed</span>
				<span class="text-gray-400 text-xs">%dms</span>
			</div>
			<pre class="text-red-700 text-xs whitespace-pre-wrap">%s</pre>
		</div>`, duration.Milliseconds(), template.HTMLEscapeString(execErr.Error()))
		return
	}

	// Parse workflow JSON output for nice rendering
	output := ""
	if result != nil {
		output = result.Output
	}

	var wfResult struct {
		Status string `json:"status"`
		Steps  []struct {
			Tool       string `json:"tool"`
			Status     string `json:"status"`
			DurationMs int    `json:"duration_ms"`
			Error      string `json:"error,omitempty"`
		} `json:"steps"`
		Result string `json:"result"`
		Error  string `json:"error,omitempty"`
	}

	if err := json.Unmarshal([]byte(output), &wfResult); err != nil {
		// Couldn't parse as workflow JSON — show raw
		fmt.Fprintf(w, `<pre class="bg-gray-900 text-green-300 text-xs p-4 rounded-lg max-h-96 overflow-y-auto">%s</pre>`,
			template.HTMLEscapeString(output))
		return
	}

	// Render structured workflow result
	statusColor := "green"
	if wfResult.Status == "failed" {
		statusColor = "red"
	}

	fmt.Fprintf(w, `<div class="rounded-lg border border-%s-200 bg-%s-50 p-4">`, statusColor, statusColor)
	fmt.Fprintf(w, `<div class="flex items-center gap-2 mb-3">
		<span class="text-%s-600 font-medium text-sm">%s</span>
		<span class="text-gray-400 text-xs">%dms</span>
	</div>`, statusColor, wfResult.Status, duration.Milliseconds())

	// Step list
	fmt.Fprint(w, `<div class="space-y-1 mb-3">`)
	for _, step := range wfResult.Steps {
		icon := "✓"
		color := "green"
		if step.Status == "failed" {
			icon = "✗"
			color = "red"
		} else if step.Status == "skipped" {
			icon = "—"
			color = "gray"
		}
		dur := ""
		if step.DurationMs > 0 {
			dur = strconv.Itoa(step.DurationMs) + "ms"
		}
		fmt.Fprintf(w, `<div class="flex items-center gap-2 text-xs">
			<span class="text-%s-500">%s</span>
			<span class="font-mono">%s</span>
			<span class="text-gray-400">%s</span>
			<span class="text-gray-400">%s</span>
		</div>`, color, icon, step.Tool, step.Status, dur)
		if step.Error != "" {
			fmt.Fprintf(w, `<div class="text-red-600 text-xs ml-6">%s</div>`, template.HTMLEscapeString(step.Error))
		}
	}
	fmt.Fprint(w, `</div>`)

	// Result
	if wfResult.Result != "" {
		fmt.Fprintf(w, `<pre class="text-gray-700 text-xs whitespace-pre-wrap mt-2 pt-2 border-t border-%s-200">%s</pre>`,
			statusColor, template.HTMLEscapeString(wfResult.Result))
	}
	if wfResult.Error != "" {
		fmt.Fprintf(w, `<pre class="text-red-700 text-xs whitespace-pre-wrap mt-2">%s</pre>`,
			template.HTMLEscapeString(wfResult.Error))
	}

	fmt.Fprint(w, `</div>`)
}
