// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/factorly-dev/factorly/internal/templates"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	s.render(w, "templates.html", map[string]any{
		"Title":     "Templates",
		"Nav":       "tools",
		"Templates": templates.All(),
	})
}

func (s *Server) handleTemplateDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tmpl := templates.Get(name)
	if tmpl == nil {
		http.NotFound(w, r)
		return
	}

	s.render(w, "template_detail.html", map[string]any{
		"Title":     tmpl.DisplayName,
		"Nav":       "tools",
		"Template":  tmpl,
		"ToolNames": tmpl.ToolNames(),
		"YAML":      tmpl.YAML,
	})
}

func (s *Server) handleTemplateInstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tmpl := templates.Get(name)
	if tmpl == nil {
		http.NotFound(w, r)
		return
	}

	// Get the full YAML for all tools in the template
	toolYAML := tmpl.FilterYAML(tmpl.ToolNames())

	// Write to tools_dir if configured, otherwise to main config
	if s.toolsDir != "" {
		if err := os.MkdirAll(s.toolsDir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outPath := filepath.Join(s.toolsDir, name+".yaml")
		if err := os.WriteFile(outPath, []byte(toolYAML), 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if s.cfgPath != "" {
		// Merge into main config
		var newTools map[string]any
		if err := yaml.Unmarshal([]byte(toolYAML), &newTools); err != nil {
			http.Error(w, fmt.Sprintf("parsing template YAML: %v", err), http.StatusInternalServerError)
			return
		}

		data, err := os.ReadFile(s.cfgPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tools, ok := raw["tools"].(map[string]any)
		if !ok {
			tools = make(map[string]any)
			raw["tools"] = tools
		}
		for k, v := range newTools {
			tools[k] = v
		}
		out, err := yaml.Marshal(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(s.cfgPath, out, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/tools", http.StatusFound)
}
