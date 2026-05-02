// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"

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
		"templates/templates.html",
		"templates/template_detail.html",
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
	s.mux.HandleFunc("GET /tools/{name}", s.handleToolEdit)
	s.mux.HandleFunc("POST /tools/{name}/try", s.handleToolTry)

	// Templates
	s.mux.HandleFunc("GET /templates", s.handleTemplatesList)
	s.mux.HandleFunc("GET /templates/{name}", s.handleTemplateDetail)

	// Vault (placeholder)
	s.mux.HandleFunc("GET /vault", s.handleVault)
}

// Start begins serving the UI.
func (s *Server) Start(addr string) error {
	fmt.Fprintf(os.Stderr, "Factorly UI running at http://%s\n", addr)
	return http.ListenAndServe(addr, s.mux)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inc": func(i int) int { return i + 1 },
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
