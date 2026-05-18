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
	"github.com/factorly-dev/factorly/internal/configyaml"
)

type toolListItem struct {
	Name        string
	Type        string
	Description string
	Shadow      *config.ShadowConfig
	Hidden      bool
}

type toolGroup struct {
	Prefix string
	Tools  []toolListItem
}

func groupTools(items []toolListItem) []toolGroup {
	groups := make(map[string][]toolListItem)
	var order []string
	for _, item := range items {
		prefix := ""
		if i := strings.Index(item.Name, "."); i > 0 {
			prefix = item.Name[:i]
		}
		if _, exists := groups[prefix]; !exists {
			order = append(order, prefix)
		}
		groups[prefix] = append(groups[prefix], item)
	}
	// Sort: ungrouped first, then alphabetical
	sort.Slice(order, func(i, j int) bool {
		if order[i] == "" {
			return true
		}
		if order[j] == "" {
			return false
		}
		return order[i] < order[j]
	})
	result := make([]toolGroup, len(order))
	for i, prefix := range order {
		result[i] = toolGroup{Prefix: prefix, Tools: groups[prefix]}
	}
	return result
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
		"IsBuiltin":  tc.Type == "builtin",
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

// handleToolYAML renders the tool's YAML definition. ?download=1 returns
// raw application/yaml with Content-Disposition: attachment so the browser
// saves it directly. Workflows live under /workflows/{name}/yaml — this
// route 404s for type: workflow so the breadcrumb in the view stays right.
func (s *Server) handleToolYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok || tc.Type == "workflow" {
		http.NotFound(w, r)
		return
	}
	s.renderYAMLView(w, r, yamlViewArgs{
		Name:         name,
		Heading:      name,
		Subheading:   "Tool definition",
		BackHref:     "/tools/" + name,
		BackLabel:    "Back to " + name,
		DownloadName: name + ".yaml",
		Render:       func() ([]byte, error) { return configyaml.RenderTool(name, tc) },
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
			if result.Error != "" && output == "" {
				output = result.Error
			} else if result.Error != "" {
				output = output + "\n" + result.Error
			}
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
		tree := formatJSONTree(output)
		if tree != "" {
			formattedOutput = tree
		}
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
		"IsJSON":          isJSON,
		"OutputSize":      len(output),
		"Timestamp":       time.Now().Format("15:04:05"),
	})
}

// formatJSONTree renders JSON as a collapsible HTML tree using <details>/<summary>.
func formatJSONTree(s string) string {
	var data any
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return ""
	}
	var out strings.Builder
	renderJSONNode(&out, "", data, true)
	return out.String()
}

func renderJSONNode(out *strings.Builder, key string, value any, last bool) {
	comma := ","
	if last {
		comma = ""
	}
	keyHTML := ""
	if key != "" {
		keyHTML = `<span class="text-indigo-300">"` + template.HTMLEscapeString(key) + `"</span><span class="text-gray-500">: </span>`
	}

	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-gray-400">{}</span>` + comma + `</div>`)
			return
		}
		out.WriteString(`<details open class="json-node"><summary class="json-line cursor-pointer hover:bg-gray-800/50">` + keyHTML + `<span class="text-gray-400">{</span> <span class="text-gray-600 text-[10px]">` + fmt.Sprintf("%d keys", len(v)) + `</span></summary>`)
		out.WriteString(`<div class="ml-4">`)
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			renderJSONNode(out, k, v[k], i == len(keys)-1)
		}
		out.WriteString(`</div>`)
		out.WriteString(`<div class="json-line"><span class="text-gray-400">}</span>` + comma + `</div></details>`)

	case []any:
		if len(v) == 0 {
			out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-gray-400">[]</span>` + comma + `</div>`)
			return
		}
		out.WriteString(`<details open class="json-node"><summary class="json-line cursor-pointer hover:bg-gray-800/50">` + keyHTML + `<span class="text-gray-400">[</span> <span class="text-gray-600 text-[10px]">` + fmt.Sprintf("%d items", len(v)) + `</span></summary>`)
		out.WriteString(`<div class="ml-4">`)
		for i, item := range v {
			renderJSONNode(out, "", item, i == len(v)-1)
		}
		out.WriteString(`</div>`)
		out.WriteString(`<div class="json-line"><span class="text-gray-400">]</span>` + comma + `</div></details>`)

	case string:
		out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-green-300">"` + template.HTMLEscapeString(v) + `"</span>` + comma + `</div>`)

	case float64:
		out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-amber-300">` + fmt.Sprintf("%v", v) + `</span>` + comma + `</div>`)

	case bool:
		out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-amber-300">` + fmt.Sprintf("%t", v) + `</span>` + comma + `</div>`)

	case nil:
		out.WriteString(`<div class="json-line">` + keyHTML + `<span class="text-gray-500">null</span>` + comma + `</div>`)
	}
}

func (s *Server) handleToolFormPartial(w http.ResponseWriter, r *http.Request) {
	toolType := r.URL.Query().Get("type")
	partialName := "tool_form_cli"
	switch toolType {
	case "rest":
		partialName = "tool_form_rest"
	case "mcp":
		partialName = "tool_form_mcp"
	case "code":
		partialName = "tool_form_code"
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
		// Env vars (key/value rows) + isolation toggle
		envKeys := r.Form["env_key[]"]
		envVals := r.Form["env_val[]"]
		env := make(map[string]string)
		for i, k := range envKeys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			v := ""
			if i < len(envVals) {
				v = envVals[i]
			}
			env[k] = v
		}
		if len(env) > 0 {
			tc.Env = env
		}
		if r.FormValue("env_isolation") == "strict" {
			tc.EnvIsolation = "strict"
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
	case "code":
		tc.Code = r.FormValue("code")
		if mc := r.FormValue("max_calls"); mc != "" {
			if n, err := strconv.Atoi(mc); err == nil {
				if tc.Shadow == nil {
					tc.Shadow = &config.ShadowConfig{}
				}
				tc.Shadow.MaxCalls = n
			}
		}
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

	// Built-in tools: only allow shadow/oversight edits
	if tc.Type == "builtin" {
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
		s.cfg.Tools[name] = tc
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<span class="text-green-600 text-xs font-medium">✓ Saved</span>`)
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
		// Env vars (key/value rows) + isolation toggle
		envKeys := r.Form["env_key[]"]
		envVals := r.Form["env_val[]"]
		env := make(map[string]string)
		for i, k := range envKeys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			v := ""
			if i < len(envVals) {
				v = envVals[i]
			}
			env[k] = v
		}
		if len(env) > 0 {
			tc.Env = env
		} else {
			tc.Env = nil
		}
		if r.FormValue("env_isolation") == "strict" {
			tc.EnvIsolation = "strict"
		} else {
			tc.EnvIsolation = ""
		}
	case "rest":
		tc.BaseURL = r.FormValue("base_url")
		tc.Method = r.FormValue("method")
		tc.Path = r.FormValue("path")
		// Parse headers
		headerKeys := r.Form["header_key[]"]
		headerVals := r.Form["header_val[]"]
		headers := make(map[string]string)
		for i, k := range headerKeys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			v := ""
			if i < len(headerVals) {
				v = headerVals[i]
			}
			headers[k] = v
		}
		if len(headers) > 0 {
			tc.Headers = headers
		} else {
			tc.Headers = nil
		}
	case "mcp":
		tc.Command = r.FormValue("command")
		if args := r.FormValue("args"); args != "" {
			tc.Args = splitArgs(args)
		} else {
			tc.Args = nil
		}
	case "code":
		tc.Code = r.FormValue("code")
		newMaxCalls := 0
		if mc := r.FormValue("max_calls"); mc != "" {
			if n, err := strconv.Atoi(mc); err == nil {
				newMaxCalls = n
			}
		}
		if newMaxCalls > 0 {
			if tc.Shadow == nil {
				tc.Shadow = &config.ShadowConfig{}
			}
			tc.Shadow.MaxCalls = newMaxCalls
		} else if tc.Shadow != nil {
			tc.Shadow.MaxCalls = 0
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
	// Preserve fields the type-specific switch above (e.g., code's
	// max_calls) may have stashed in Shadow before this generic parser
	// rebuilds it.
	var preservedMaxCalls int
	var preservedAllowPatterns, preservedAllowPaths, preservedAllowURLs []string
	if tc.Shadow != nil {
		preservedMaxCalls = tc.Shadow.MaxCalls
		preservedAllowPatterns = tc.Shadow.AllowPatterns
		preservedAllowPaths = tc.Shadow.AllowPaths
		preservedAllowURLs = tc.Shadow.AllowURLs
	}
	if len(deny) > 0 || confirmOn || rateLimit != "" || preservedMaxCalls > 0 ||
		len(preservedAllowPatterns) > 0 || len(preservedAllowPaths) > 0 || len(preservedAllowURLs) > 0 {
		sc := &config.ShadowConfig{
			Deny:          deny,
			RateLimit:     rateLimit,
			MaxCalls:      preservedMaxCalls,
			AllowPatterns: preservedAllowPatterns,
			AllowPaths:    preservedAllowPaths,
			AllowURLs:     preservedAllowURLs,
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

func (s *Server) handleToolDuplicate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tc, ok := s.cfg.Tools[name]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Generate a unique copy name
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
		// Ensure ActivePrefix is always set (templates need it)
		if _, has := m["ActivePrefix"]; !has {
			m["ActivePrefix"] = ""
		}
		// Inject the active workspace name so layout.html can render
		// the pill on every page.
		if _, has := m["ActiveWorkspace"]; !has {
			m["ActiveWorkspace"] = s.requestWorkspaceFromState()
		}
		// Inject sidebar tools for tools-related pages
		if m["Nav"] == "tools" {
			if _, has := m["SidebarToolGroups"]; !has {
				m["SidebarToolGroups"] = groupTools(s.getSidebarTools())
			}
			if active, ok := m["ActiveTool"].(string); ok {
				if i := strings.Index(active, "."); i > 0 {
					m["ActivePrefix"] = active[:i]
				}
			}
		}
		// Inject sidebar workflows for workflow pages
		if m["Nav"] == "workflows" {
			if _, has := m["SidebarWorkflowGroups"]; !has {
				m["SidebarWorkflowGroups"] = groupTools(s.getSidebarWorkflows())
			}
			if active, ok := m["ActiveTool"].(string); ok {
				if i := strings.Index(active, "."); i > 0 {
					m["ActivePrefix"] = active[:i]
				}
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
