// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
)

type toolListItem struct {
	Name        string
	Type        string
	Description string
	Shadow      *config.ShadowConfig
}

func (s *Server) handleToolsList(w http.ResponseWriter, r *http.Request) {
	var tools []toolListItem
	for name, tc := range s.cfg.Tools {
		tools = append(tools, toolListItem{
			Name:        name,
			Type:        tc.Type,
			Description: tc.Description,
			Shadow:      tc.Shadow,
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	s.render(w, "tools.html", map[string]any{
		"Title": "Tools",
		"Nav":   "tools",
		"Tools": tools,
	})
}

func (s *Server) handleToolEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Combine tool config name with struct for template
	type toolView struct {
		Name string
		config.ToolConfig
	}

	s.render(w, "tool_edit.html", map[string]any{
		"Title":  name,
		"Nav":    "tools",
		"Tool":   toolView{Name: name, ToolConfig: tc},
		"Params": tc.Parameters,
	})
}

func (s *Server) handleToolTry(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract params from form (prefixed with "param_")
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
				<span class="text-red-600 font-medium text-sm">Blocked</span>
				<span class="text-gray-400 text-xs">%dms</span>
			</div>
			<pre class="text-red-700 text-xs whitespace-pre-wrap">%s</pre>
		</div>`, duration.Milliseconds(), execErr.Error())
		return
	}

	output := ""
	status := "success"
	if result != nil {
		output = result.Output
		if result.IsError() {
			status = "error"
		}
	}

	statusColor := "green"
	if status == "error" {
		statusColor = "amber"
	}

	fmt.Fprintf(w, `<div class="rounded-lg border border-%s-200 bg-%s-50 p-4">
		<div class="flex items-center gap-2 mb-2">
			<span class="text-%s-600 font-medium text-sm">%s</span>
			<span class="text-gray-400 text-xs">%dms</span>
		</div>
		<pre class="text-gray-800 text-xs whitespace-pre-wrap max-h-96 overflow-y-auto">%s</pre>
	</div>`, statusColor, statusColor, statusColor, status, duration.Milliseconds(), template.HTMLEscapeString(output))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls["templates/"+name]
	if !ok {
		http.Error(w, fmt.Sprintf("template %q not found", name), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
