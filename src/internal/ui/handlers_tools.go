// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
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

	// Redirect workflows to their dedicated editor
	if tc.Type == "workflow" {
		http.Redirect(w, r, "/workflows/"+name, http.StatusFound)
		return
	}

	// Combine tool config name with struct for template
	type toolView struct {
		Name string
		config.ToolConfig
	}

	s.render(w, "tool_edit.html", map[string]any{
		"Title":      name,
		"Nav":        "tools",
		"ActiveTool": name,
		"Tool":       toolView{Name: name, ToolConfig: tc},
		"Params":     tc.Parameters,
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
		renderBlockedResponse(w, name, duration, execErr)
		return
	}

	output := ""
	status := "success"
	statusIcon := "✓"
	if result != nil {
		output = result.Output
		if result.IsError() {
			status = "error"
			statusIcon = "✗"
		}
	}

	renderSuccessResponse(w, name, status, statusIcon, duration, output)
}

func renderBlockedResponse(w http.ResponseWriter, tool string, dur time.Duration, err error) {
	fmt.Fprintf(w, `
<div class="rounded-lg overflow-hidden border border-red-200">
  <div class="bg-red-50 px-4 py-2.5 flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-red-100 text-red-600 text-xs font-bold">✗</span>
      <span class="text-red-700 font-medium text-sm">blocked</span>
      <span class="text-red-400 text-xs font-mono">%s</span>
    </div>
    <span class="text-gray-400 text-xs font-mono">%dms</span>
  </div>
  <div class="bg-gray-900 p-4">
    <pre class="text-red-300 text-xs font-mono whitespace-pre-wrap">%s</pre>
  </div>
</div>`, tool, dur.Milliseconds(), template.HTMLEscapeString(err.Error()))
}

func renderSuccessResponse(w http.ResponseWriter, tool, status, icon string, dur time.Duration, output string) {
	statusColor := "green"
	statusBg := "green"
	textColor := "green-300"
	if status == "error" {
		statusColor = "amber"
		statusBg = "amber"
		textColor = "amber-300"
	}

	// Detect if output is JSON for formatting
	isJSON := len(output) > 0 && (output[0] == '{' || output[0] == '[')
	formattedOutput := template.HTMLEscapeString(output)
	if isJSON {
		// Pretty-print JSON before highlighting
		var prettyBuf bytes.Buffer
		if json.Indent(&prettyBuf, []byte(output), "", "  ") == nil {
			formattedOutput = formatJSONHTML(prettyBuf.String())
		} else {
			formattedOutput = formatJSONHTML(output)
		}
	}

	tabID := fmt.Sprintf("tab-%d", time.Now().UnixNano())

	fmt.Fprintf(w, `
<div class="rounded-lg overflow-hidden border border-%s-200">
  <!-- Status bar -->
  <div class="bg-%s-50 px-4 py-2.5 flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-%s-100 text-%s-600 text-xs font-bold">%s</span>
      <span class="text-%s-700 font-medium text-sm">%s</span>
      <span class="text-%s-400 text-xs font-mono">%s</span>
    </div>
    <span class="text-gray-400 text-xs font-mono">%dms</span>
  </div>

  <!-- Tabs -->
  <div class="bg-gray-800 border-b border-gray-700 px-4 flex gap-1">
    <button onclick="switchTab('%s', 'formatted')" class="tab-btn px-3 py-1.5 text-xs text-gray-300 hover:text-white border-b-2 border-indigo-400 font-medium" data-tab="formatted">Response</button>
    <button onclick="switchTab('%s', 'raw')" class="tab-btn px-3 py-1.5 text-xs text-gray-500 hover:text-white border-b-2 border-transparent" data-tab="raw">Raw</button>
  </div>

  <!-- Response body -->
  <div class="bg-gray-900 p-4 max-h-[500px] overflow-y-auto">
    <div id="%s-formatted">
      <pre class="text-%s text-xs font-mono whitespace-pre-wrap leading-relaxed">%s</pre>
    </div>
    <div id="%s-raw" class="hidden">
      <pre class="text-gray-300 text-xs font-mono whitespace-pre-wrap leading-relaxed">%s</pre>
    </div>
  </div>
</div>

<script>
function switchTab(prefix, tab) {
  document.getElementById(prefix + '-formatted').classList.toggle('hidden', tab !== 'formatted');
  document.getElementById(prefix + '-raw').classList.toggle('hidden', tab !== 'raw');
  const parent = document.getElementById(prefix + '-formatted').closest('.rounded-lg');
  parent.querySelectorAll('.tab-btn').forEach(btn => {
    const isActive = btn.dataset.tab === tab;
    btn.classList.toggle('border-indigo-400', isActive);
    btn.classList.toggle('text-gray-300', isActive);
    btn.classList.toggle('font-medium', isActive);
    btn.classList.toggle('border-transparent', !isActive);
    btn.classList.toggle('text-gray-500', !isActive);
  });
}
</script>`,
		statusColor,
		statusBg,
		statusColor, statusColor, icon,
		statusColor, status,
		statusColor, tool,
		dur.Milliseconds(),
		tabID, tabID,
		tabID, textColor, formattedOutput,
		tabID, template.HTMLEscapeString(output),
	)
}

// formatJSONHTML returns syntax-highlighted HTML for JSON content.
func formatJSONHTML(s string) string {
	var out strings.Builder
	inString := false
	isKey := true
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == '"' && !inString:
			// Start of string
			inString = true
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(s) {
				j++ // include closing quote
			}
			str := template.HTMLEscapeString(s[i:j])
			if isKey {
				out.WriteString(`<span class="text-indigo-300">` + str + `</span>`)
			} else {
				out.WriteString(`<span class="text-green-300">` + str + `</span>`)
			}
			i = j
			isKey = false
			continue
		case ch == ':' && !inString:
			out.WriteString(`<span class="text-gray-500">:</span>`)
			isKey = false
			i++
			continue
		case ch == ',' && !inString:
			out.WriteString(`<span class="text-gray-500">,</span>`)
			isKey = true
			i++
			continue
		case ch == '{' || ch == '}' || ch == '[' || ch == ']':
			out.WriteString(`<span class="text-gray-400">` + string(ch) + `</span>`)
			if ch == '{' {
				isKey = true
			}
			i++
			continue
		case (ch >= '0' && ch <= '9') || ch == '-':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == '-' || s[j] == 'e' || s[j] == 'E' || s[j] == '+') {
				j++
			}
			out.WriteString(`<span class="text-amber-300">` + s[i:j] + `</span>`)
			i = j
			continue
		case ch == 't' && i+4 <= len(s) && s[i:i+4] == "true":
			out.WriteString(`<span class="text-amber-300">true</span>`)
			i += 4
			continue
		case ch == 'f' && i+5 <= len(s) && s[i:i+5] == "false":
			out.WriteString(`<span class="text-amber-300">false</span>`)
			i += 5
			continue
		case ch == 'n' && i+4 <= len(s) && s[i:i+4] == "null":
			out.WriteString(`<span class="text-gray-500">null</span>`)
			i += 4
			continue
		default:
			out.WriteByte(ch)
			i++
		}
	}
	return out.String()
}

func (s *Server) handleToolFormPartial(w http.ResponseWriter, r *http.Request) {
	toolType := r.URL.Query().Get("type")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	switch toolType {
	case "rest":
		fmt.Fprint(w, `<div class="space-y-4 pt-4 border-t border-gray-100">
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Method</label>
				<select name="method" class="w-full px-3 py-2 border border-gray-200 rounded text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200">
					<option value="GET">GET</option>
					<option value="POST">POST</option>
					<option value="PUT">PUT</option>
					<option value="PATCH">PATCH</option>
					<option value="DELETE">DELETE</option>
				</select>
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Base URL</label>
				<input type="text" name="base_url" placeholder="https://api.example.com"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Path</label>
				<input type="text" name="path" placeholder="/v1/{{resource}}"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
			</div>
		</div>`)
	case "workflow":
		fmt.Fprint(w, `<div class="pt-4 border-t border-gray-100">
			<p class="text-sm text-gray-500">Workflow steps can be configured after creation.</p>
		</div>`)
	default: // cli
		fmt.Fprint(w, `<div class="space-y-4 pt-4 border-t border-gray-100">
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Command</label>
				<input type="text" name="command" placeholder="curl"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Args</label>
				<input type="text" name="args" placeholder="-s {{url}}"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
				<p class="text-xs text-gray-400 mt-1">Space-separated. Use {{param}} for placeholders.</p>
			</div>
		</div>`)
	}
}

func (s *Server) handleToolNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "tool_new.html", map[string]any{
		"Title": "New Tool",
		"Nav":   "tools",
	})
}

func (s *Server) handleToolCreate(w http.ResponseWriter, r *http.Request) {
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
		Type:        r.FormValue("type"),
		Description: r.FormValue("description"),
	}

	switch tc.Type {
	case "cli":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		}
	case "rest":
		tc.BaseURL = r.FormValue("base_url")
		tc.Method = r.FormValue("method")
		tc.Path = r.FormValue("path")
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config
	s.cfg.Tools[name] = tc

	http.Redirect(w, r, "/tools/"+name, http.StatusFound)
}

func (s *Server) handleToolSave(w http.ResponseWriter, r *http.Request) {
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

	tc.Description = r.FormValue("description")
	switch tc.Type {
	case "cli":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		} else {
			tc.Args = nil
		}
		if stdin := r.FormValue("stdin"); stdin != "" {
			tc.Stdin = stdin
		}
	case "rest":
		tc.BaseURL = r.FormValue("base_url")
		tc.Method = r.FormValue("method")
		tc.Path = r.FormValue("path")
		if body := r.FormValue("body"); body != "" {
			tc.Body = body
		}
	case "mcp":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		} else {
			tc.Args = nil
		}
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.cfg.Tools[name] = tc
	http.Redirect(w, r, "/tools/"+name, http.StatusFound)
}

func (s *Server) handleToolDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := DeleteTool(s.cfgPath, s.toolsDir, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delete(s.cfg.Tools, name)

	// Return empty response for htmx (redirects via HX-Redirect header)
	w.Header().Set("HX-Redirect", "/tools")
	w.WriteHeader(http.StatusOK)
}

// splitArgs splits a space-separated string respecting quoted segments.
func splitArgs(s string) []string {
	var args []string
	var current []byte
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current = append(current, c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' {
			if len(current) > 0 {
				args = append(args, string(current))
				current = current[:0]
			}
		} else {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		args = append(args, string(current))
	}
	return args
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls["templates/"+name]
	if !ok {
		http.Error(w, fmt.Sprintf("template %q not found", name), http.StatusInternalServerError)
		return
	}

	// Inject sidebar tools for tools-related pages
	if m, ok := data.(map[string]any); ok && m["Nav"] == "tools" {
		if _, hasSidebar := m["SidebarTools"]; !hasSidebar {
			m["SidebarTools"] = s.getSidebarTools()
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) getSidebarTools() []toolListItem {
	var tools []toolListItem
	for name, tc := range s.cfg.Tools {
		tools = append(tools, toolListItem{
			Name: name,
			Type: tc.Type,
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}
