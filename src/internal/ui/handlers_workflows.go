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
	"strings"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
)

func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "workflow_new.html", map[string]any{
		"Title": "New Workflow",
		"Nav":   "workflows",
	})
}

func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	tc := config.ToolConfig{
		Type:        "workflow",
		Description: r.FormValue("description"),
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Tools[name] = tc
	s.registerTool(name, tc)

	http.Redirect(w, r, "/workflows/"+name, http.StatusFound)
}

func (s *Server) handleWorkflowsList(w http.ResponseWriter, r *http.Request) {
	type wfItem struct {
		Name        string
		Description string
		StepCount   int
	}
	var workflows []wfItem
	for name, tc := range s.cfg.Tools {
		if tc.Type == "workflow" {
			workflows = append(workflows, wfItem{
				Name:        name,
				Description: tc.Description,
				StepCount:   len(tc.Steps),
			})
		}
	}
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name < workflows[j].Name
	})

	s.render(w, "workflows.html", map[string]any{
		"Title":     "Workflows",
		"Nav":       "workflows",
		"Workflows": workflows,
	})
}

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
		"Nav":            "workflows",
		"ActiveTool":     name,
		"Name":           name,
		"Description":    tc.Description,
		"Steps":          tc.Steps,
		"Params":         tc.Parameters,
		"AvailableTools": available,
		"Tool":           tc,
	})
}

func (s *Server) handleWorkflowAddStep(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Count existing steps to determine the new index
	idx := len(tc.Steps)
	// If steps were added client-side, use query param
	if qIdx := r.URL.Query().Get("index"); qIdx != "" {
		if n, err := strconv.Atoi(qIdx); err == nil {
			idx = n
		}
	}

	var available []string
	for n, t := range s.cfg.Tools {
		if t.Type != "workflow" {
			available = append(available, n)
		}
	}
	sort.Strings(available)

	s.renderPartial(w, "workflow_step", map[string]any{
		"Index":          idx,
		"WorkflowName":   name,
		"AvailableTools": available,
	})
}

func (s *Server) handleWorkflowStepParams(w http.ResponseWriter, r *http.Request) {
	toolName := r.URL.Query().Get("tool")
	stepIdx := r.URL.Query().Get("index")
	if stepIdx == "" {
		stepIdx = "0"
	}

	tc, ok := s.cfg.Tools[toolName]
	if !ok || len(tc.Parameters) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return // empty response clears the params area
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, p := range tc.Parameters {
		fmt.Fprintf(w, `<div class="param-row flex gap-1">
			<input type="text" name="step_param_key_%s[]" value="%s" placeholder="key"
			       class="w-1/3 px-2 py-1 text-xs font-mono border border-gray-200 rounded bg-white">
			<input type="text" name="step_param_val_%s[]" value="%s" placeholder="value"
			       class="flex-1 px-2 py-1 text-xs font-mono border border-gray-200 rounded bg-white">
			<button type="button" onclick="this.closest('.param-row').remove()" class="text-gray-300 hover:text-red-500 text-xs px-1">✕</button>
		</div>`, stepIdx, template.HTMLEscapeString(p.Name), stepIdx, template.HTMLEscapeString(p.Default))
	}
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
		ifCond := r.FormValue(fmt.Sprintf("step_if_%d", i))
		reqCond := r.FormValue(fmt.Sprintf("step_require_%d", i))

		// Parse params (key[] and val[] arrays)
		keys := r.Form[fmt.Sprintf("step_param_key_%d[]", i)]
		vals := r.Form[fmt.Sprintf("step_param_val_%d[]", i)]
		var params map[string]string
		if len(keys) > 0 {
			params = make(map[string]string)
			for j, k := range keys {
				if k == "" {
					continue
				}
				v := ""
				if j < len(vals) {
					v = vals[j]
				}
				params[k] = v
			}
			if len(params) == 0 {
				params = nil
			}
		}

		steps = append(steps, config.StepConfig{
			Tool:    tool,
			Store:   store,
			If:      ifCond,
			Require: reqCond,
			Params:  params,
		})
	}

	// Parse workflow input parameters
	var wfParams []config.ParamConfig
	for i := 0; ; i++ {
		pname := r.FormValue(fmt.Sprintf("wf_param_name_%d", i))
		if pname == "" {
			break
		}
		wfParams = append(wfParams, config.ParamConfig{
			Name:        pname,
			Type:        r.FormValue(fmt.Sprintf("wf_param_type_%d", i)),
			Required:    r.FormValue(fmt.Sprintf("wf_param_required_%d", i)) == "on",
			Default:     r.FormValue(fmt.Sprintf("wf_param_default_%d", i)),
			Description: r.FormValue(fmt.Sprintf("wf_param_desc_%d", i)),
		})
	}

	tc.Description = r.FormValue("description")
	tc.Parameters = wfParams
	tc.Steps = steps
	tc.Hidden = r.FormValue("hidden") == "on"
	tc.Timeout = r.FormValue("timeout")

	if mo := r.FormValue("max_output"); mo != "" {
		if n, err := strconv.Atoi(mo); err == nil {
			tc.MaxOutput = n
		}
	} else {
		tc.MaxOutput = 0
	}
	if compress := r.FormValue("compress"); compress != "" {
		tc.Compress = splitComma(compress)
	} else {
		tc.Compress = nil
	}

	// Parse shadow/oversight
	deny := splitComma(r.FormValue("shadow_deny"))
	confirmOn := r.FormValue("shadow_confirm") == "on"
	rateLimit := r.FormValue("shadow_rate_limit")
	if len(deny) > 0 || confirmOn || rateLimit != "" {
		sc := &config.ShadowConfig{Deny: deny, RateLimit: rateLimit}
		if confirmOn {
			sc.Confirm = true
		}
		tc.Shadow = sc
	} else {
		tc.Shadow = nil
	}

	// Parse output filter
	fc := parseFilterForm(r)
	if fc != nil {
		tc.Filter = fc
	} else {
		tc.Filter = nil
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Tools[name] = tc
	s.registerTool(name, tc)

	// htmx callers get an inline status fragment so save-and-run can chain.
	// Non-htmx form posts redirect back to the edit page as before.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<span class="text-green-600 text-xs font-medium">✓ Saved</span>`)
		return
	}
	http.Redirect(w, r, "/workflows/"+name, http.StatusFound)
}

func (s *Server) handleWorkflowRename(w http.ResponseWriter, r *http.Request) {
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

	if r.Form.Has("description") {
		tc.Description = r.FormValue("description")
	}

	newName := strings.TrimSpace(r.FormValue("rename"))
	if newName == "" {
		newName = name
	}

	if newName != name {
		if err := DeleteTool(s.cfgPath, s.toolsDir, name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		delete(s.cfg.Tools, name)
		s.unregisterTool(name)
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, newName, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Tools[newName] = tc
	s.registerTool(newName, tc)

	w.Header().Set("HX-Redirect", "/workflows/"+newName)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleWorkflowDuplicate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok || tc.Type != "workflow" {
		http.NotFound(w, r)
		return
	}

	newName := name + ".copy"
	for i := 2; ; i++ {
		if _, exists := s.cfg.Tools[newName]; !exists {
			break
		}
		newName = fmt.Sprintf("%s.copy%d", name, i)
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, newName, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Tools[newName] = tc
	s.registerTool(newName, tc)

	w.Header().Set("HX-Redirect", "/workflows/"+newName)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := DeleteTool(s.cfgPath, s.toolsDir, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delete(s.cfg.Tools, name)
	s.unregisterTool(name)

	w.Header().Set("HX-Redirect", "/workflows")
	w.WriteHeader(http.StatusOK)
}

// handleWorkflowRunPanel renders just the run-form partial. The workflow
// edit page calls this after a save so the run form's inputs reflect the
// freshly-saved parameter list before chaining a Save-and-Run.
func (s *Server) handleWorkflowRunPanel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderPartial(w, "workflow_run_panel", map[string]any{
		"Name":   name,
		"Params": tc.Parameters,
	})
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

	// Render structured workflow result as mini-pipeline
	statusIcon := "✓"
	statusColor := "green"
	if wfResult.Status == "failed" {
		statusIcon = "✗"
		statusColor = "red"
	}

	fmt.Fprintf(w, `
<div class="rounded-lg overflow-hidden border border-%s-200">
  <div class="bg-%s-50 px-4 py-2.5 flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-%s-100 text-%s-600 text-xs font-bold">%s</span>
      <span class="text-%s-700 font-medium text-sm">%s</span>
    </div>
    <span class="text-gray-400 text-xs font-mono">%dms</span>
  </div>
  <div class="bg-white px-4 py-3 space-y-1.5">`,
		statusColor, statusColor, statusColor, statusColor, statusIcon, statusColor, wfResult.Status, duration.Milliseconds())

	for _, step := range wfResult.Steps {
		icon := "✓"
		iconBg := "bg-green-100 text-green-600"
		if step.Status == "failed" {
			icon = "✗"
			iconBg = "bg-red-100 text-red-600"
		} else if step.Status == "skipped" {
			icon = "—"
			iconBg = "bg-gray-100 text-gray-400"
		}
		dur := ""
		if step.DurationMs > 0 {
			dur = strconv.Itoa(step.DurationMs) + "ms"
		}
		fmt.Fprintf(w, `
    <div class="flex items-center gap-2.5">
      <span class="inline-flex items-center justify-center w-4 h-4 rounded-full %s text-[9px] font-bold shrink-0">%s</span>
      <span class="font-mono text-xs text-gray-700 flex-1">%s</span>
      <span class="text-[10px] text-gray-400 font-mono">%s</span>
    </div>`, iconBg, icon, template.HTMLEscapeString(step.Tool), dur)
		if step.Error != "" {
			fmt.Fprintf(w, `
    <div class="ml-6 text-[11px] text-red-600 font-mono">%s</div>`, template.HTMLEscapeString(step.Error))
		}
	}

	fmt.Fprint(w, `
  </div>`)

	// Result on dark background
	if wfResult.Result != "" {
		fmt.Fprintf(w, `
  <div class="bg-gray-900 px-4 py-3 border-t border-gray-200">
    <pre class="text-green-300 text-xs font-mono whitespace-pre-wrap max-h-48 overflow-y-auto">%s</pre>
  </div>`, template.HTMLEscapeString(wfResult.Result))
	}
	if wfResult.Error != "" && wfResult.Result == "" {
		fmt.Fprintf(w, `
  <div class="bg-gray-900 px-4 py-3 border-t border-gray-200">
    <pre class="text-red-300 text-xs font-mono whitespace-pre-wrap">%s</pre>
  </div>`, template.HTMLEscapeString(wfResult.Error))
	}

	fmt.Fprint(w, `
</div>`)
}
