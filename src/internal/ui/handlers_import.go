// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/openapi"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	s.render(w, "import.html", map[string]any{
		"Title": "Import Tools",
		"Nav":   "tools",
	})
}

func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	specURL := strings.TrimSpace(r.FormValue("spec_url"))
	prefix := strings.TrimSpace(r.FormValue("prefix"))

	if specURL == "" {
		s.renderPartial(w, "import_error", map[string]any{"Message": "Please provide a spec URL or path."})
		return
	}

	tools, err := openapi.Generate(specURL, openapi.GenerateOpts{
		Prefix:  prefix,
		BaseDir: s.projectDir(),
	})
	if err != nil {
		s.renderPartial(w, "import_error", map[string]any{"Message": "Error: " + err.Error()})
		return
	}

	if len(tools) == 0 {
		s.renderPartial(w, "import_warning", map[string]any{"Message": "No endpoints found in the spec."})
		return
	}

	// Build sorted tool list with method colors
	type toolPreview struct {
		Name        string
		Method      string
		MethodColor string
		Description string
	}
	var names []string
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	var previews []toolPreview
	for _, name := range names {
		tc := tools[name]
		method := tc.Method
		if method == "" {
			method = "GET"
		}
		color := "gray"
		switch method {
		case "GET":
			color = "green"
		case "POST":
			color = "blue"
		case "PUT":
			color = "orange"
		case "PATCH":
			color = "yellow"
		case "DELETE":
			color = "red"
		}
		previews = append(previews, toolPreview{
			Name:        name,
			Method:      method,
			MethodColor: color,
			Description: tc.Description,
		})
	}

	s.renderPartial(w, "import_preview", map[string]any{
		"Count":   len(tools),
		"SpecURL": specURL,
		"Prefix":  prefix,
		"Tools":   previews,
	})
}

func (s *Server) handleImportConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	specURL := strings.TrimSpace(r.FormValue("spec_url"))
	prefix := strings.TrimSpace(r.FormValue("prefix"))
	selectedTools := r.Form["tools"]

	if len(selectedTools) == 0 {
		http.Error(w, "no tools selected", http.StatusBadRequest)
		return
	}

	// Re-generate from spec
	tools, err := openapi.Generate(specURL, openapi.GenerateOpts{
		Prefix:  prefix,
		BaseDir: s.projectDir(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter to selected
	selected := make(map[string]bool, len(selectedTools))
	for _, name := range selectedTools {
		selected[name] = true
	}

	// Save selected tools
	if s.toolsDir != "" {
		// Write as a single file in tools_dir
		filtered := make(map[string]config.ToolConfig)
		for name, tc := range tools {
			if selected[name] {
				filtered[name] = tc
			}
		}
		out, err := yaml.Marshal(filtered)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		filename := prefix
		if filename == "" {
			filename = "imported"
		}
		safe, err := safePath(filename)
		if err != nil {
			http.Error(w, "invalid prefix: "+err.Error(), http.StatusBadRequest)
			return
		}
		path := filepath.Join(s.toolsDir, safe+".yaml")
		if err := writeFileCreate(path, out); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Update in-memory config and registry
		for name, tc := range filtered {
			s.cfg.Tools[name] = tc
			s.registerTool(name, tc)
		}
	} else {
		// Save each tool to main config
		for name, tc := range tools {
			if !selected[name] {
				continue
			}
			if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.cfg.Tools[name] = tc
			s.registerTool(name, tc)
		}
	}

	http.Redirect(w, r, "/tools", http.StatusFound)
}

// projectDir returns the directory containing the config file, used as the
// sandbox root for local file operations like spec import.
func (s *Server) projectDir() string {
	if s.cfgPath != "" {
		return filepath.Dir(s.cfgPath)
	}
	return "."
}

func writeFileCreate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
