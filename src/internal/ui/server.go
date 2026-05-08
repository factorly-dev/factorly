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
	"time"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/oauth"
	"github.com/factorly-dev/factorly/internal/output"
	"github.com/factorly-dev/factorly/internal/provider"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
	"github.com/factorly-dev/factorly/internal/templates"
	"github.com/factorly-dev/factorly/internal/vault"
)

// Server serves the Factorly web UI.
type Server struct {
	cfg           *config.Config
	cfgPath       string
	toolsDir      string
	registry      *registry.Registry
	proxy         *proxy.Proxy
	vault         vault.Backend
	projectVault  vault.Backend
	globalVault   vault.Backend
	tmpls         map[string]*template.Template
	mux           *http.ServeMux
	mcpHandler    http.Handler
	activity      *ActivityBroadcaster
	confirmBroker *ConfirmBroker
}

// Options configures the UI server.
type Options struct {
	Config        *config.Config
	CfgPath       string
	ToolsDir      string
	Registry      *registry.Registry
	Proxy         *proxy.Proxy
	Vault         vault.Backend
	ProjectVault  vault.Backend
	GlobalVault   vault.Backend
	Activity      *ActivityBroadcaster
	ConfirmBroker *ConfirmBroker
}

// New creates a UI server.
func New(opts Options) (*Server, error) {
	// Parse each page template individually with the shared layout.
	// This avoids "content" redefinition conflicts between pages.
	pages := []string{
		"templates/tools.html",
		"templates/tool_edit.html",
		"templates/tool_new.html",
		"templates/import.html",
		"templates/templates.html",
		"templates/template_detail.html",
		"templates/workflows.html",
		"templates/workflow_new.html",
		"templates/workflow_edit.html",
		"templates/history.html",
		"templates/auth.html",
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
		cfg:           opts.Config,
		cfgPath:       opts.CfgPath,
		toolsDir:      opts.ToolsDir,
		registry:      opts.Registry,
		proxy:         opts.Proxy,
		vault:         opts.Vault,
		projectVault:  opts.ProjectVault,
		globalVault:   opts.GlobalVault,
		activity:      opts.Activity,
		confirmBroker: opts.ConfirmBroker,
		tmpls:         tmpls,
		mux:           http.NewServeMux(),
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
	s.mux.HandleFunc("POST /tools/{name}/rename", s.handleToolRename)
	s.mux.HandleFunc("GET /tools/{name}/try-panel", s.handleToolTryPanel)
	s.mux.HandleFunc("POST /tools/{name}/try", s.handleToolTry)
	s.mux.HandleFunc("DELETE /tools/{name}", s.handleToolDelete)

	// Import
	s.mux.HandleFunc("GET /tools/import", s.handleImport)
	s.mux.HandleFunc("POST /tools/import/preview", s.handleImportPreview)
	s.mux.HandleFunc("POST /tools/import/confirm", s.handleImportConfirm)

	// Templates
	s.mux.HandleFunc("GET /templates", s.handleTemplatesList)
	s.mux.HandleFunc("GET /templates/{name}", s.handleTemplateDetail)
	s.mux.HandleFunc("POST /templates/{name}/install", s.handleTemplateInstall)

	// Workflows
	s.mux.HandleFunc("GET /workflows", s.handleWorkflowsList)
	s.mux.HandleFunc("GET /workflows/new", s.handleWorkflowNew)
	s.mux.HandleFunc("POST /workflows/_new", s.handleWorkflowCreate)
	s.mux.HandleFunc("GET /workflows/{name}", s.handleWorkflowEdit)
	s.mux.HandleFunc("POST /workflows/{name}/rename", s.handleWorkflowRename)
	s.mux.HandleFunc("POST /workflows/{name}/save", s.handleWorkflowSave)
	s.mux.HandleFunc("POST /workflows/{name}/run", s.handleWorkflowRun)
	s.mux.HandleFunc("DELETE /workflows/{name}", s.handleWorkflowDelete)

	// Activity (live feed)
	s.mux.HandleFunc("GET /activity", s.handleActivity)
	s.mux.HandleFunc("GET /activity/stream", s.handleActivityStream)

	// Confirm (shadow confirm prompts routed to browser)
	s.mux.HandleFunc("GET /confirm/pending", s.handleConfirmPending)
	s.mux.HandleFunc("GET /confirm/stream", s.handleConfirmSSE)
	s.mux.HandleFunc("POST /confirm/{id}/{action}", s.handleConfirmRespond)

	// History
	s.mux.HandleFunc("GET /history", s.handleHistory)

	// Auth
	s.mux.HandleFunc("GET /auth", s.handleAuth)
	s.mux.HandleFunc("POST /auth/_new", s.handleAuthCreate)
	s.mux.HandleFunc("POST /auth/{provider}", s.handleAuthUpdate)
	s.mux.HandleFunc("POST /auth/{provider}/refresh", s.handleAuthRefresh)
	s.mux.HandleFunc("DELETE /auth/{provider}", s.handleAuthDelete)
	s.mux.HandleFunc("DELETE /auth/{provider}/token", s.handleAuthLogout)

	// Vault
	s.mux.HandleFunc("GET /vault", s.handleVault)
	s.mux.HandleFunc("POST /vault", s.handleVaultSet)
	s.mux.HandleFunc("DELETE /vault/{key}", s.handleVaultDelete)
}

// MountMCP sets the MCP handler. The StreamableHTTPServer is its own
// root handler (it internally routes /mcp), so we intercept requests
// before the UI mux and delegate /mcp paths to it directly.
func (s *Server) MountMCP(handler http.Handler) {
	s.mcpHandler = handler
}

// Handler returns the HTTP handler for the UI (for wrapping with middleware).
// If MCP is mounted, /mcp requests are delegated to the StreamableHTTPServer
// which acts as its own root handler (it internally routes /mcp).
func (s *Server) Handler() http.Handler {
	if s.mcpHandler == nil {
		return s.mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/mcp") {
			s.mcpHandler.ServeHTTP(w, r)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

// Start begins serving the UI.
func (s *Server) Start(addr string) error {
	fmt.Fprintf(os.Stderr, "Factorly UI running at http://%s\n", addr)
	return http.ListenAndServe(addr, s.mux)
}

// registerTool adds or updates a tool in the live registry so it can be
// executed via Try It and MCP without restarting the server.
func (s *Server) registerTool(name string, tc config.ToolConfig) {
	if s.registry == nil {
		return
	}
	params := make([]registry.Parameter, len(tc.Parameters))
	for i, p := range tc.Parameters {
		params[i] = registry.Parameter{
			Name:        p.Name,
			Description: p.Description,
			Required:    p.Required,
			Type:        p.Type,
			Default:     p.Default,
			Min:         p.Min,
			Max:         p.Max,
			MinLength:   p.MinLength,
			MaxLength:   p.MaxLength,
			Pattern:     p.Pattern,
			Enum:        p.Enum,
		}
	}
	tool := &registry.Tool{
		Name:        name,
		Type:        tc.Type,
		Description: tc.Description,
		Hidden:      tc.Hidden,
		Parameters:  params,
		ProviderKey: tc.Type,
		MaxOutput:   tc.MaxOutput,
		Compress:    tc.Compress,
		Filter:      output.CompileFilter(tc.Filter),
	}
	if tc.Shadow != nil {
		var overrides []string
		overrides = append(overrides, tc.Shadow.AllowPatterns...)
		overrides = append(overrides, tc.Shadow.AllowPaths...)
		overrides = append(overrides, tc.Shadow.AllowURLs...)
		if len(overrides) > 0 {
			tool.AllowOverrides = overrides
		}
	}
	s.registry.Register(tool)

	// Update shadow policy if tool has oversight rules
	s.updateShadowRule(name, tc)

	// Register with the appropriate provider and track vault keys for audit
	vaultKeys := s.registerProvider(name, tc)
	if len(vaultKeys) > 0 {
		tool.VaultKeys = dedup(vaultKeys)
	}
}

// registerProvider adds the tool definition to the appropriate provider.
// Returns vault keys that were accessed during resolution.
func (s *Server) registerProvider(name string, tc config.ToolConfig) []string {
	if s.proxy == nil {
		return nil
	}
	var vaultKeys []string
	switch tc.Type {
	case "cli":
		s.registerCLIProvider(name, tc, &vaultKeys)
	case "rest":
		s.registerRESTProvider(name, tc, &vaultKeys)
	case "workflow":
		s.registerWorkflowProvider(name, tc)
	}
	return vaultKeys
}

func (s *Server) registerCLIProvider(name string, tc config.ToolConfig, vaultKeys *[]string) {
	prov := s.proxy.Provider("cli")
	if prov == nil {
		cp := provider.NewCLI(map[string]provider.CLIToolDef{})
		_ = cp.Setup()
		s.proxy.RegisterProvider("cli", cp)
		prov = cp
	}
	cp, ok := prov.(*provider.CLIProvider)
	if !ok {
		return
	}
	def := provider.CLIToolDef{
		Command:     s.resolveRefT(tc.Command, vaultKeys),
		Args:        s.resolveRefsTracked(tc.Args, vaultKeys),
		Stdin:       s.resolveRefT(tc.Stdin, vaultKeys),
		Interactive: tc.Interactive,
		Env:         s.resolveRefMapTracked(tc.Env, vaultKeys),
		EnvStrict:   tc.EnvIsolation == "strict",
	}
	if tc.Timeout != "" {
		if d, err := time.ParseDuration(tc.Timeout); err == nil {
			def.Timeout = d
		}
	}
	cp.AddTool(name, def)
}

func (s *Server) registerRESTProvider(name string, tc config.ToolConfig, vaultKeys *[]string) {
	prov := s.proxy.Provider("rest")
	if prov == nil {
		rp := provider.NewREST(map[string]provider.RESTToolDef{}, nil)
		_ = rp.Setup()
		s.proxy.RegisterProvider("rest", rp)
		prov = rp
	}
	rp, ok := prov.(*provider.RESTProvider)
	if !ok {
		return
	}
	def := provider.RESTToolDef{
		Method:  tc.Method,
		BaseURL: s.resolveRefT(tc.BaseURL, vaultKeys),
		Path:    s.resolveRefT(tc.Path, vaultKeys),
		Body:    tc.Body,
		Headers: s.resolveRefMapTracked(tc.Headers, vaultKeys),
	}
	if tc.Auth != nil {
		def.Auth = &provider.AuthDef{
			Type:   tc.Auth.Type,
			Token:  s.resolveRefT(tc.Auth.Token, vaultKeys),
			Header: tc.Auth.Header,
			Value:  s.resolveRefT(tc.Auth.Value, vaultKeys),
		}
		if tc.Auth.Type == "oauth" && s.cfg != nil {
			oauthCfg := s.cfg.ResolveOAuthProvider(tc.Auth)
			if oauthCfg != nil {
				def.Auth.OAuthProvider = &oauth.ProviderConfig{
					ClientID:     s.resolveRefT(oauthCfg.ClientID, vaultKeys),
					ClientSecret: s.resolveRefT(oauthCfg.ClientSecret, vaultKeys),
					AuthURL:      oauthCfg.AuthURL,
					TokenURL:     oauthCfg.TokenURL,
					Scopes:       oauthCfg.Scopes,
				}
				def.Auth.TokenKey = config.OAuthTokenKey(tc.Auth)
			}
		}
	}
	for _, p := range tc.Parameters {
		def.Params = append(def.Params, provider.RESTParamDef{
			Name:     p.Name,
			In:       p.In,
			Required: p.Required,
			Type:     p.Type,
		})
	}
	if tc.Timeout != "" {
		if d, err := time.ParseDuration(tc.Timeout); err == nil {
			def.Timeout = d
		}
	}
	rp.AddTool(name, def)
}

func (s *Server) registerWorkflowProvider(name string, tc config.ToolConfig) {
	prov := s.proxy.Provider("workflow")
	if prov == nil {
		return
	}
	wp, ok := prov.(*provider.WorkflowProvider)
	if !ok {
		return
	}
	steps := make([]provider.WorkflowStep, len(tc.Steps))
	for i, st := range tc.Steps {
		ws := provider.WorkflowStep{
			Tool:    st.Tool,
			Params:  st.Params,
			Store:   st.Store,
			If:      st.If,
			Require: st.Require,
		}
		for _, sc := range st.Switch {
			ws.Switch = append(ws.Switch, provider.WorkflowSwitchCase{
				Condition: sc.Condition,
				Tool:      sc.Tool,
				Params:    sc.Params,
				Store:     sc.Store,
			})
		}
		steps[i] = ws
	}
	wp.RegisterWorkflow(name, steps)
}

// unregisterTool removes a tool from the live registry and provider.
func (s *Server) unregisterTool(name string) {
	if s.registry == nil {
		return
	}
	// Remove from provider
	if s.proxy != nil {
		if tc, ok := s.cfg.Tools[name]; ok {
			if prov := s.proxy.Provider(tc.Type); prov != nil {
				switch p := prov.(type) {
				case *provider.CLIProvider:
					p.RemoveTool(name)
				case *provider.RESTProvider:
					p.RemoveTool(name)
				case *provider.WorkflowProvider:
					p.RemoveWorkflow(name)
				}
			}
		}
	}
	// Remove shadow rule
	if s.proxy != nil {
		if policy := s.proxy.Shadow(); policy != nil {
			policy.RemoveRule(name)
		}
	}
	s.registry.Unregister(name)
}

// updateShadowRule syncs shadow/oversight config to the live shadow policy.
func (s *Server) updateShadowRule(name string, tc config.ToolConfig) {
	if s.proxy == nil {
		return
	}
	policy := s.proxy.Shadow()
	if policy == nil {
		return
	}

	if tc.Shadow == nil {
		policy.RemoveRule(name)
		return
	}

	confirmList, confirmAll := tc.Shadow.ConfirmList()
	rule := &shadow.Rule{
		Deny:       tc.Shadow.Deny,
		Confirm:    confirmList,
		ConfirmAll: confirmAll,
		LogParams:  tc.Shadow.LogParams,
	}
	if tc.Shadow.RateLimit != "" {
		rl, _ := shadow.ParseRateLimit(tc.Shadow.RateLimit)
		rule.RateLimit = rl
	}
	policy.SetRule(name, rule)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inc": func(i int) int { return i + 1 },
		"joinList": func(items []string) string {
			return strings.Join(items, ", ")
		},
		"icon": func(name string) template.HTML {
			icons := map[string]string{
				"play":     `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="6 3 20 12 6 21 6 3"/></svg>`,
				"send":     `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/></svg>`,
				"plus":     `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="M12 5v14"/></svg>`,
				"trash":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/></svg>`,
				"check":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`,
				"x":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`,
				"shield":   `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg>`,
				"terminal": `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/></svg>`,
				"globe":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>`,
				"workflow": `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="8" height="8" rx="2"/><rect x="13" y="13" width="8" height="8" rx="2"/><path d="M7 11v4a2 2 0 0 0 2 2h4"/></svg>`,
				"clock":    `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`,
				"lock":     `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>`,
				"package":  `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m7.5 4.27 9 5.15"/><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/></svg>`,
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
