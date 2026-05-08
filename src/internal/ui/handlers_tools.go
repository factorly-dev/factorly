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
	Hidden      bool
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
			Hidden:      tc.Hidden,
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
	s.renderPartial(w, "try_panel", map[string]any{
		"Name":   name,
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
		s.renderBlockedResponse(w, name, duration, execErr)
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

	s.renderSuccessResponse(w, name, status, statusIcon, duration, output)
}

func (s *Server) renderBlockedResponse(w http.ResponseWriter, tool string, dur time.Duration, err error) {
	s.renderPartial(w, "try_blocked", map[string]any{
		"Tool":       tool,
		"DurationMs": dur.Milliseconds(),
		"Error":      err.Error(),
	})
}

func (s *Server) renderSuccessResponse(w http.ResponseWriter, tool, status, icon string, dur time.Duration, output string) {
	statusColor := "green"
	textColor := "green-300"
	if status == "error" {
		statusColor = "amber"
		textColor = "amber-300"
	}

	isJSON := len(output) > 0 && (output[0] == '{' || output[0] == '[')
	formattedOutput := template.HTMLEscapeString(output)
	if isJSON {
		var prettyBuf bytes.Buffer
		if json.Indent(&prettyBuf, []byte(output), "", "  ") == nil {
			formattedOutput = formatJSONHTML(prettyBuf.String())
		}
		// If json.Indent fails, keep the HTMLEscapeString'd version —
		// formatJSONHTML is only safe on valid JSON.
	}

	tabID := fmt.Sprintf("tab-%d", time.Now().UnixNano())

	s.renderPartial(w, "try_success", map[string]any{
		"Tool":            tool,
		"Status":          status,
		"StatusColor":     statusColor,
		"Icon":            icon,
		"DurationMs":      dur.Milliseconds(),
		"TabID":           tabID,
		"TextColor":       textColor,
		"FormattedOutput": template.HTML(formattedOutput),
		"RawOutput":       output,
	})
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
			// HTML-escape any unexpected characters as a safety net.
			switch ch {
			case '<':
				out.WriteString("&lt;")
			case '>':
				out.WriteString("&gt;")
			case '&':
				out.WriteString("&amp;")
			default:
				out.WriteByte(ch)
			}
			i++
		}
	}
	return out.String()
}

func (s *Server) handleToolFormPartial(w http.ResponseWriter, r *http.Request) {
	toolType := r.URL.Query().Get("type")
	partialName := "tool_form_cli"
	switch toolType {
	case "rest":
		partialName = "tool_form_rest"
	case "mcp":
		partialName = "tool_form_mcp"
	}
	s.renderPartial(w, partialName, nil)
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
	tc.Hidden = r.FormValue("hidden") == "on"
	tc.Stdin = r.FormValue("stdin")
	tc.Timeout = r.FormValue("timeout")
	tc.Body = r.FormValue("body")
	tc.BodyType = r.FormValue("body_type")

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
			In:          r.FormValue(fmt.Sprintf("param_in_%d", i)),
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

// renderPartial renders a named partial template (from templates/partials/).
// Uses any available page template since partials are parsed with all of them.
func (s *Server) renderPartial(w http.ResponseWriter, partialName string, data any) {
	// Use the first available template (all include partials)
	var t *template.Template
	for _, tmpl := range s.tmpls {
		t = tmpl
		break
	}
	if t == nil {
		http.Error(w, "no templates available", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, partialName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// getSidebarItems returns sidebar items filtered by type.
// If onlyType is set, only tools of that type are returned.
// If excludeType is set, tools of that type are excluded.
func (s *Server) getSidebarItems(onlyType, excludeType string) []toolListItem {
	var items []toolListItem
	for name, tc := range s.cfg.Tools {
		if onlyType != "" && tc.Type != onlyType {
			continue
		}
		if excludeType != "" && tc.Type == excludeType {
			continue
		}
		items = append(items, toolListItem{
			Name:   name,
			Type:   tc.Type,
			Hidden: tc.Hidden,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *Server) getSidebarTools() []toolListItem {
	return s.getSidebarItems("", "workflow")
}

func (s *Server) getSidebarWorkflows() []toolListItem {
	return s.getSidebarItems("workflow", "")
}
