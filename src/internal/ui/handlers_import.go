// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"html/template"
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div class="text-red-600 text-sm">Please provide a spec URL or path.</div>`)
		return
	}

	tools, err := openapi.Generate(specURL, openapi.GenerateOpts{Prefix: prefix})
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<div class="text-red-600 text-sm">Error: %s</div>`, template.HTMLEscapeString(err.Error()))
		return
	}

	if len(tools) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div class="text-amber-600 text-sm">No endpoints found in the spec.</div>`)
		return
	}

	// Sort tool names for display
	var names []string
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	// Render preview
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
		<div class="px-4 py-2.5 bg-gray-50 border-b border-gray-200 flex items-center justify-between">
			<label class="flex items-center gap-2 cursor-pointer">
				<input type="checkbox" checked onchange="this.closest('.rounded-lg').querySelectorAll('input[name=tools]').forEach(c => c.checked = this.checked)"
				       class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-200">
				<span class="text-xs font-medium text-gray-500 uppercase tracking-wide">%d tools found</span>
			</label>
		</div>
		<div class="px-4 py-2 border-b border-gray-100">
			<input type="text" placeholder="Filter tools..." oninput="this.closest('.rounded-lg').querySelectorAll('.import-row').forEach(r => r.style.display = r.textContent.toLowerCase().includes(this.value.toLowerCase()) ? '' : 'none')"
			       class="w-full px-2 py-1 text-xs border border-gray-200 rounded focus:outline-none focus:ring-1 focus:ring-indigo-200">
		</div>
		<form method="POST" action="/tools/import/confirm">
			<input type="hidden" name="spec_url" value="%s">
			<input type="hidden" name="prefix" value="%s">
			<div class="max-h-96 overflow-y-auto">`, len(tools), template.HTMLEscapeString(specURL), template.HTMLEscapeString(prefix))

	for _, name := range names {
		tc := tools[name]
		method := tc.Method
		if method == "" {
			method = "GET"
		}
		methodColor := "gray"
		switch method {
		case "GET":
			methodColor = "green"
		case "POST":
			methodColor = "blue"
		case "PUT":
			methodColor = "orange"
		case "PATCH":
			methodColor = "yellow"
		case "DELETE":
			methodColor = "red"
		}

		fmt.Fprintf(w, `<label class="import-row px-4 py-2 border-b border-gray-100 flex items-center gap-3 hover:bg-gray-50 cursor-pointer">
			<input type="checkbox" name="tools" value="%s" checked class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-200">
			<span class="px-1.5 py-0.5 text-[10px] font-bold rounded bg-%s-100 text-%s-700">%s</span>
			<span class="font-mono text-xs text-gray-800 flex-1 truncate">%s</span>
			<span class="text-[10px] text-gray-400 truncate max-w-[200px]">%s</span>
		</label>`,
			template.HTMLEscapeString(name),
			methodColor, methodColor, method,
			template.HTMLEscapeString(name),
			template.HTMLEscapeString(tc.Description))
	}

	fmt.Fprint(w, `</div>
		<div class="px-4 py-3 border-t border-gray-200 flex items-center gap-3">
			<button type="submit" class="px-4 py-1.5 bg-indigo-600 text-white text-sm rounded hover:bg-indigo-700">Import Selected</button>
			<span class="text-xs text-gray-400">Uncheck tools you don't want to import.</span>
		</div>
		</form>
	</div>`)
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
	tools, err := openapi.Generate(specURL, openapi.GenerateOpts{Prefix: prefix})
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

func writeFileCreate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
