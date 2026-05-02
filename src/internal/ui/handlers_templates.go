// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"

	"github.com/factorly-dev/factorly/internal/templates"
)

func (s *Server) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	s.render(w, "templates.html", map[string]any{
		"Title":     "Templates",
		"Nav":       "templates",
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
		"Nav":       "templates",
		"Template":  tmpl,
		"ToolNames": tmpl.ToolNames(),
		"YAML":      tmpl.YAML,
	})
}
