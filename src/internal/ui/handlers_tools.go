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
	"strconv"
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
		if tc.Type == "workflow" {
			continue
		}
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

func (s *Server) handleToolTryPanel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	fmt.Fprint(w, `<div class="bg-white rounded-lg border border-gray-200 p-5 sticky top-6">
		<div class="flex items-center justify-between mb-4">
			<h2 class="text-xs font-medium text-gray-500 uppercase tracking-wide">Try it</h2>`)
	fmt.Fprintf(w, `<span class="text-[10px] text-gray-400 font-mono">%s</span>
		</div>`, template.HTMLEscapeString(name))

	fmt.Fprintf(w, `<form hx-post="/tools/%s/try" hx-target="#try-result" hx-swap="innerHTML" hx-indicator="#try-spinner">`, template.HTMLEscapeString(name))

	if len(tc.Parameters) > 0 {
		fmt.Fprint(w, `<div class="space-y-3 mb-4">`)
		for _, p := range tc.Parameters {
			fmt.Fprintf(w, `<div>
				<label class="block text-xs text-gray-600 mb-1 font-medium">%s`, template.HTMLEscapeString(p.Name))
			if p.Required {
				fmt.Fprint(w, ` <span class="text-red-400">*</span>`)
			}
			if p.Type != "" {
				fmt.Fprintf(w, ` <span class="text-gray-300 font-normal">%s</span>`, template.HTMLEscapeString(p.Type))
			}
			fmt.Fprint(w, `</label>`)
			if p.Description != "" {
				fmt.Fprintf(w, `<p class="text-[10px] text-gray-400 mb-1">%s</p>`, template.HTMLEscapeString(p.Description))
			}
			fmt.Fprintf(w, `<input type="text" name="param_%s" value="%s"
				class="w-full px-3 py-1.5 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200"`,
				template.HTMLEscapeString(p.Name), template.HTMLEscapeString(p.Default))
			if p.Required {
				fmt.Fprint(w, ` required`)
			}
			fmt.Fprint(w, `></div>`)
		}
		fmt.Fprint(w, `</div>`)
	} else {
		fmt.Fprint(w, `<p class="text-xs text-gray-400 mb-4">No parameters.</p>`)
	}

	fmt.Fprint(w, `<button type="submit"
		class="w-full px-4 py-2.5 bg-indigo-600 text-white text-sm font-medium rounded-lg hover:bg-indigo-700 transition-colors shadow-sm inline-flex items-center justify-center gap-2">Send</button>
		<span id="try-spinner" class="htmx-indicator block mt-2 text-center text-xs text-gray-400">
			<span class="inline-block animate-pulse">●</span> running...
		</span>
	</form>
	<div id="try-result" class="mt-4"></div>
	</div>`)
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
				<input type="text" name="path" placeholder="/v1/resource"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Auth</label>
				<select name="auth_type" class="w-full px-3 py-2 border border-gray-200 rounded text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 mb-2"
				        onchange="toggleNewAuthFields(this.value)">
					<option value="">None</option>
					<option value="bearer">Bearer token</option>
					<option value="header">Custom header</option>
					<option value="oauth">OAuth</option>
				</select>
				<div id="new-auth-fields"></div>
			</div>
		</div>
		<script>
		function toggleNewAuthFields(type) {
			const c = document.getElementById('new-auth-fields');
			switch(type) {
				case 'bearer':
					c.innerHTML = '<input type="text" name="auth_token" placeholder="token or {{vault:KEY}}" class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">';
					break;
				case 'header':
					c.innerHTML = '<div class="flex gap-2"><input type="text" name="auth_header" placeholder="Header name" class="w-1/3 px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200"><input type="text" name="auth_value" placeholder="value" class="flex-1 px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200"></div>';
					break;
				case 'oauth':
					c.innerHTML = '<input type="text" name="auth_provider" placeholder="provider name" class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">';
					break;
				default:
					c.innerHTML = '';
			}
		}
		</script>`)
	case "mcp":
		fmt.Fprint(w, `<div class="space-y-4 pt-4 border-t border-gray-100">
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Command</label>
				<input type="text" name="command" placeholder="npx -y @modelcontextprotocol/server-github"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Args</label>
				<input type="text" name="args" placeholder=""
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
				<p class="text-xs text-gray-400 mt-1">Space-separated arguments for the MCP server command.</p>
			</div>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">URL (HTTP transport)</label>
				<input type="text" name="url" placeholder="http://localhost:8080/mcp (optional, for HTTP transport)"
					   class="w-full px-3 py-2 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">
				<p class="text-xs text-gray-400 mt-1">Leave empty for stdio transport (command-based).</p>
			</div>
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
	case "mcp":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		}
		tc.URL = r.FormValue("url")
	}

	// Parse auth (for REST tools)
	if authType := r.FormValue("auth_type"); authType != "" {
		auth := &config.AuthConfig{Type: authType}
		switch authType {
		case "bearer":
			auth.Token = r.FormValue("auth_token")
		case "header":
			auth.Header = r.FormValue("auth_header")
			auth.Value = r.FormValue("auth_value")
		case "oauth":
			auth.Provider = r.FormValue("auth_provider")
		}
		tc.Auth = auth
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config and registry
	s.cfg.Tools[name] = tc
	s.registerTool(name, tc)

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
	tc.Stdin = r.FormValue("stdin")
	tc.Timeout = r.FormValue("timeout")
	tc.Body = r.FormValue("body")

	if mo := r.FormValue("max_output"); mo != "" {
		if n, err := strconv.Atoi(mo); err == nil {
			tc.MaxOutput = n
		}
	}
	if compress := r.FormValue("compress"); compress != "" {
		tc.Compress = splitComma(compress)
	} else {
		tc.Compress = nil
	}

	switch tc.Type {
	case "cli":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		} else {
			tc.Args = nil
		}
	case "rest":
		tc.BaseURL = r.FormValue("base_url")
		tc.Method = r.FormValue("method")
		tc.Path = r.FormValue("path")
	case "mcp":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		} else {
			tc.Args = nil
		}
	}

	// Parse parameters
	var params []config.ParamConfig
	for i := 0; ; i++ {
		pname := r.FormValue(fmt.Sprintf("param_name_%d", i))
		if pname == "" {
			break
		}
		params = append(params, config.ParamConfig{
			Name:        pname,
			Type:        r.FormValue(fmt.Sprintf("param_type_%d", i)),
			Required:    r.FormValue(fmt.Sprintf("param_required_%d", i)) == "on",
			Default:     r.FormValue(fmt.Sprintf("param_default_%d", i)),
			Description: r.FormValue(fmt.Sprintf("param_desc_%d", i)),
		})
	}
	tc.Parameters = params

	// Parse shadow/oversight
	deny := splitComma(r.FormValue("shadow_deny"))
	confirmOn := r.FormValue("shadow_confirm") == "on"
	rateLimit := r.FormValue("shadow_rate_limit")
	if len(deny) > 0 || confirmOn || rateLimit != "" {
		sc := &config.ShadowConfig{
			Deny:      deny,
			RateLimit: rateLimit,
		}
		if confirmOn {
			sc.Confirm = true
		}
		tc.Shadow = sc
	} else {
		tc.Shadow = nil
	}

	// Parse auth
	authType := r.FormValue("auth_type")
	if authType != "" {
		auth := &config.AuthConfig{Type: authType}
		switch authType {
		case "bearer":
			auth.Token = r.FormValue("auth_token")
		case "header":
			auth.Header = r.FormValue("auth_header")
			auth.Value = r.FormValue("auth_value")
		case "oauth":
			auth.Provider = r.FormValue("auth_provider")
		}
		tc.Auth = auth
	} else {
		tc.Auth = nil
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.cfg.Tools[name] = tc
	s.registerTool(name, tc)

	// Return inline confirmation for htmx
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<span class="text-green-600 text-xs font-medium">✓ Saved</span>`)
}

func (s *Server) handleToolRename(w http.ResponseWriter, r *http.Request) {
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

	// Update description
	if desc := r.FormValue("description"); desc != "" || r.Form.Has("description") {
		tc.Description = r.FormValue("description")
	}

	// Handle rename
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

	// Redirect to the (possibly new) tool page
	w.Header().Set("HX-Redirect", "/tools/"+newName)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleToolDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := DeleteTool(s.cfgPath, s.toolsDir, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delete(s.cfg.Tools, name)
	s.unregisterTool(name)

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

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	t, ok := s.tmpls["templates/"+name]
	if !ok {
		http.Error(w, fmt.Sprintf("template %q not found", name), http.StatusInternalServerError)
		return
	}

	if m, ok := data.(map[string]any); ok {
		// Inject sidebar tools for tools-related pages
		if m["Nav"] == "tools" {
			if _, has := m["SidebarTools"]; !has {
				m["SidebarTools"] = s.getSidebarTools()
			}
		}
		// Inject sidebar workflows for workflow pages
		if m["Nav"] == "workflows" {
			if _, has := m["SidebarWorkflows"]; !has {
				m["SidebarWorkflows"] = s.getSidebarWorkflows()
			}
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
		if tc.Type == "workflow" {
			continue
		}
		tools = append(tools, toolListItem{
			Name: name,
			Type: tc.Type,
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func (s *Server) getSidebarWorkflows() []toolListItem {
	var workflows []toolListItem
	for name, tc := range s.cfg.Tools {
		if tc.Type != "workflow" {
			continue
		}
		workflows = append(workflows, toolListItem{
			Name: name,
			Type: tc.Type,
		})
	}
	sort.Slice(workflows, func(i, j int) bool { return workflows[i].Name < workflows[j].Name })
	return workflows
}
