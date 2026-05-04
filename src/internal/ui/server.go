// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/templates"
	"github.com/factorly-dev/factorly/internal/vault"
)

// Server serves the Factorly web UI.
type Server struct {
	cfg      *config.Config
	cfgPath  string
	toolsDir string
	registry *registry.Registry
	proxy    *proxy.Proxy
	vault    vault.Backend
	tmpls    map[string]*template.Template
	mux      *http.ServeMux
}

// Options configures the UI server.
type Options struct {
	Config   *config.Config
	CfgPath  string
	ToolsDir string
	Registry *registry.Registry
	Proxy    *proxy.Proxy
	Vault    vault.Backend
}

// New creates a UI server.
func New(opts Options) (*Server, error) {
	// Parse each page template individually with the shared layout.
	// This avoids "content" redefinition conflicts between pages.
	pages := []string{
		"templates/tools.html",
		"templates/tool_edit.html",
		"templates/tool_new.html",
		"templates/templates.html",
		"templates/template_detail.html",
		"templates/workflow_edit.html",
		"templates/history.html",
		"templates/vault.html",
	}
	tmpls := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.New("").Funcs(templateFuncs()).ParseFS(templatesFS,
			"templates/layout.html",
			"templates/partials/*.html",
			page,
		)
		if err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", page, err)
		}
		tmpls[page] = t
	}

	s := &Server{
		cfg:      opts.Config,
		cfgPath:  opts.CfgPath,
		toolsDir: opts.ToolsDir,
		registry: opts.Registry,
		proxy:    opts.Proxy,
		vault:    opts.Vault,
		tmpls:    tmpls,
		mux:      http.NewServeMux(),
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	// Static assets (embedded under static/ subdir)
	staticSub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))

	// Tools
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tools", http.StatusFound)
	})
	s.mux.HandleFunc("GET /tools", s.handleToolsList)
	s.mux.HandleFunc("GET /tools/new", s.handleToolNew)
	s.mux.HandleFunc("GET /tools/_form", s.handleToolFormPartial)
	s.mux.HandleFunc("GET /tools/{name}", s.handleToolEdit)
	s.mux.HandleFunc("POST /tools/_new", s.handleToolCreate)
	s.mux.HandleFunc("POST /tools/{name}", s.handleToolSave)
	s.mux.HandleFunc("POST /tools/{name}/try", s.handleToolTry)
	s.mux.HandleFunc("DELETE /tools/{name}", s.handleToolDelete)

	// Templates
	s.mux.HandleFunc("GET /templates", s.handleTemplatesList)
	s.mux.HandleFunc("GET /templates/{name}", s.handleTemplateDetail)
	s.mux.HandleFunc("POST /templates/{name}/install", s.handleTemplateInstall)

	// Workflows
	s.mux.HandleFunc("GET /workflows/{name}", s.handleWorkflowEdit)
	s.mux.HandleFunc("POST /workflows/{name}/save", s.handleWorkflowSave)
	s.mux.HandleFunc("POST /workflows/{name}/run", s.handleWorkflowRun)

	// History
	s.mux.HandleFunc("GET /history", s.handleHistory)

	// Vault
	s.mux.HandleFunc("GET /vault", s.handleVault)
	s.mux.HandleFunc("POST /vault", s.handleVaultSet)
	s.mux.HandleFunc("DELETE /vault/{key}", s.handleVaultDelete)
}

// Start begins serving the UI.
func (s *Server) Start(addr string) error {
	fmt.Fprintf(os.Stderr, "Factorly UI running at http://%s\n", addr)
	return http.ListenAndServe(addr, s.mux)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inc": func(i int) int { return i + 1 },
		"icon": func(name string) template.HTML {
			icons := map[string]string{
				"play":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="6 3 20 12 6 21 6 3"/></svg>`,
				"send":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/></svg>`,
				"plus":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="M12 5v14"/></svg>`,
				"trash":       `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/></svg>`,
				"check":       `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`,
				"x":           `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`,
				"shield":      `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg>`,
				"terminal":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/></svg>`,
				"globe":       `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>`,
				"workflow":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="8" height="8" rx="2"/><rect x="13" y="13" width="8" height="8" rx="2"/><path d="M7 11v4a2 2 0 0 0 2 2h4"/></svg>`,
				"clock":       `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`,
				"lock":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>`,
			}
			if svg, ok := icons[name]; ok {
				return template.HTML(svg)
			}
			return ""
		},
		"joinArgs": func(args []string) string {
			var parts []string
			for _, a := range args {
				if strings.Contains(a, " ") {
					parts = append(parts, `"`+a+`"`)
				} else {
					parts = append(parts, a)
				}
			}
			return strings.Join(parts, " ")
		},
		"shadowSummary": func(sc *config.ShadowConfig) string {
			if sc == nil {
				return "none"
			}
			parts := []string{}
			if len(sc.Deny) > 0 {
				parts = append(parts, fmt.Sprintf("deny:%d", len(sc.Deny)))
			}
			confirmList, confirmAll := sc.ConfirmList()
			if confirmAll {
				parts = append(parts, "confirm:all")
			} else if len(confirmList) > 0 {
				parts = append(parts, fmt.Sprintf("confirm:%d", len(confirmList)))
			}
			if sc.RateLimit != "" {
				parts = append(parts, sc.RateLimit)
			}
			if len(parts) == 0 {
				return "none"
			}
			result := ""
			for i, p := range parts {
				if i > 0 {
					result += ", "
				}
				result += p
			}
			return result
		},
		"templateCategories": func() []string {
			seen := map[string]bool{}
			var cats []string
			for _, t := range templates.All() {
				if !seen[t.Category] {
					seen[t.Category] = true
					cats = append(cats, t.Category)
				}
			}
			return cats
		},
	}
}
