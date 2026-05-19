// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/oauth"
	"github.com/factorly-dev/factorly/internal/output"
	"github.com/factorly-dev/factorly/internal/projectpath"
	"github.com/factorly-dev/factorly/internal/provider"
	codeprov "github.com/factorly-dev/factorly/internal/provider/code"
	"github.com/factorly-dev/factorly/internal/proxy"
	"github.com/factorly-dev/factorly/internal/registry"
	"github.com/factorly-dev/factorly/internal/shadow"
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
	resolver      *vault.Resolver
	projectVault  vault.Backend
	globalVault   vault.Backend
	tmpls         map[string]*template.Template
	mux           *http.ServeMux
	mcpHandler    http.Handler
	activity      *ActivityBroadcaster
	confirmBroker *ConfirmBroker
	// OnReload is invoked at the end of reloadConfig on success. Lets
	// callers (e.g., the MCP server bridge) react to config changes.
	OnReload func()

	// workspaceMu guards activeWorkspace + vaultBackends. Switching
	// workspace mid-session reloads config and reopens the vault chain;
	// vaultBackends caches each workspace's opened chain so toggling
	// back and forth doesn't re-prompt for passwords.
	workspaceMu     sync.Mutex
	activeWorkspace string
	vaultBackends   map[string]vault.Backend
	// WorkspaceVaultOpener opens the vault chain for the given workspace
	// name. Empty name → the no-workspace chain (project → global). The
	// caller (cmd/factorly/ui_cmd.go) injects this closure so the UI
	// package doesn't have to know about vault password resolution.
	// Returns (nil, nil) when the workspace has no vault file and no
	// chain should be cached.
	WorkspaceVaultOpener func(name string) (vault.Backend, error)
	// WorkspacePasswordOpener opens a workspace vault using an explicit
	// password (bypassing env-var/keyfile/prompt). Used by the inline
	// unlock dialog when WorkspaceVaultOpener failed due to a missing
	// or wrong password.
	WorkspacePasswordOpener func(name string, password []byte) (vault.Backend, error)
}

// Config returns the currently-loaded config. Pointer is shared with the
// running server, so callers must treat it as read-only.
func (s *Server) Config() *config.Config { return s.cfg }

// CfgPath returns the path to the loaded config file (or the canonical
// path the server writes back to).
func (s *Server) CfgPath() string { return s.cfgPath }

// workspaceCookieName is the cookie that persists the user's chosen
// workspace across page loads. Plain HTTP cookie (no encryption needed
// — the name is not a secret; the workspace's vault is what holds
// secrets).
const workspaceCookieName = "factorly_workspace"

// requestWorkspace returns the workspace name for this request:
// cookie > server-startup workspace. Empty string when neither is set.
func (s *Server) requestWorkspace(r *http.Request) string {
	if c, err := r.Cookie(workspaceCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return s.requestWorkspaceFromState()
}

// requestWorkspaceFromState returns the server-side active workspace,
// for callers without an *http.Request (e.g., reloadConfig invoked
// from a pack install handler that didn't carry the cookie through).
func (s *Server) requestWorkspaceFromState() string {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	return s.activeWorkspace
}

// setActiveWorkspace updates the server-side active workspace. Called
// from the switch endpoint after a successful reload.
func (s *Server) setActiveWorkspace(name string) {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	s.activeWorkspace = name
}

// cachedWorkspaceVault returns a previously-opened vault chain for the
// given workspace, or nil if not cached.
func (s *Server) cachedWorkspaceVault(name string) vault.Backend {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	return s.vaultBackends[name]
}

// cacheWorkspaceVault stashes an opened vault chain so subsequent
// switches back to the same workspace don't re-prompt.
func (s *Server) cacheWorkspaceVault(name string, b vault.Backend) {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	s.vaultBackends[name] = b
}

// ActiveVault returns the vault chain bound to the currently-active
// workspace (whichever was last cached during a switch). Falls back to
// the startup vault when no workspace is active. Used by the OAuth
// token store so refreshes always target the correct tier.
func (s *Server) ActiveVault() vault.Backend {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	if b, ok := s.vaultBackends[s.activeWorkspace]; ok && b != nil {
		return b
	}
	// Empty-workspace key was seeded with the startup vault in New().
	return s.vaultBackends[""]
}

// Options configures the UI server.
type Options struct {
	Config        *config.Config
	CfgPath       string
	ToolsDir      string
	Registry      *registry.Registry
	Proxy         *proxy.Proxy
	Vault         vault.Backend
	Resolver      *vault.Resolver
	ProjectVault  vault.Backend
	GlobalVault   vault.Backend
	Activity      *ActivityBroadcaster
	ConfirmBroker *ConfirmBroker
	// ActiveWorkspace is the workspace the server was started with
	// (whatever --workspace or FACTORLY_WORKSPACE or default auto-load
	// resolved to). The factorly_workspace cookie overrides it per
	// request once the user switches via the UI.
	ActiveWorkspace string
	// WorkspaceVaultOpener opens the vault chain for a named workspace.
	// See Server.WorkspaceVaultOpener for semantics.
	WorkspaceVaultOpener func(name string) (vault.Backend, error)
	// WorkspacePasswordOpener opens a named workspace vault with an
	// explicit password. See Server.WorkspacePasswordOpener.
	WorkspacePasswordOpener func(name string, password []byte) (vault.Backend, error)
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
		"templates/workflows.html",
		"templates/workflow_new.html",
		"templates/workflow_edit.html",
		"templates/history.html",
		"templates/auth.html",
		"templates/vault.html",
		"templates/blueprints.html",
		"templates/blueprints_browse.html",
		"templates/blueprint_browse_detail.html",
		"templates/yaml_view.html",
		"templates/workspaces.html",
		"templates/workspace_new.html",
		"templates/workspace_edit.html",
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
		cfg:                     opts.Config,
		cfgPath:                 opts.CfgPath,
		toolsDir:                opts.ToolsDir,
		registry:                opts.Registry,
		proxy:                   opts.Proxy,
		vault:                   opts.Vault,
		resolver:                opts.Resolver,
		projectVault:            opts.ProjectVault,
		globalVault:             opts.GlobalVault,
		activity:                opts.Activity,
		confirmBroker:           opts.ConfirmBroker,
		tmpls:                   tmpls,
		mux:                     http.NewServeMux(),
		activeWorkspace:         opts.ActiveWorkspace,
		vaultBackends:           make(map[string]vault.Backend),
		WorkspaceVaultOpener:    opts.WorkspaceVaultOpener,
		WorkspacePasswordOpener: opts.WorkspacePasswordOpener,
	}
	// Seed the cache with the startup workspace's already-opened vault so
	// the user doesn't get re-prompted when they explicitly "switch" to
	// the active workspace (e.g., back-and-forth toggling).
	if opts.Vault != nil {
		s.vaultBackends[opts.ActiveWorkspace] = opts.Vault
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	// Static assets (embedded under static/ subdir)
	staticSub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))
	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/logo.png", http.StatusMovedPermanently)
	})

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
	s.mux.HandleFunc("POST /tools/{name}/duplicate", s.handleToolDuplicate)
	s.mux.HandleFunc("POST /tools/{name}/rename", s.handleToolRename)
	s.mux.HandleFunc("GET /tools/{name}/try-panel", s.handleToolTryPanel)
	s.mux.HandleFunc("POST /tools/{name}/try", s.handleToolTry)
	s.mux.HandleFunc("GET /tools/{name}/yaml", s.handleToolYAML)
	s.mux.HandleFunc("DELETE /tools/{name}", s.handleToolDelete)

	// Import
	s.mux.HandleFunc("GET /tools/import", s.handleImport)
	s.mux.HandleFunc("POST /tools/import/preview", s.handleImportPreview)
	s.mux.HandleFunc("POST /tools/import/confirm", s.handleImportConfirm)

	// Workflows
	s.mux.HandleFunc("GET /workflows", s.handleWorkflowsList)
	s.mux.HandleFunc("GET /workflows/new", s.handleWorkflowNew)
	s.mux.HandleFunc("POST /workflows/_new", s.handleWorkflowCreate)
	s.mux.HandleFunc("GET /workflows/{name}", s.handleWorkflowEdit)
	s.mux.HandleFunc("POST /workflows/{name}/rename", s.handleWorkflowRename)
	s.mux.HandleFunc("GET /workflows/{name}/add-step", s.handleWorkflowAddStep)
	s.mux.HandleFunc("GET /workflows/{name}/step-params", s.handleWorkflowStepParams)
	s.mux.HandleFunc("POST /workflows/{name}/save", s.handleWorkflowSave)
	s.mux.HandleFunc("GET /workflows/{name}/run-panel", s.handleWorkflowRunPanel)
	s.mux.HandleFunc("POST /workflows/{name}/run", s.handleWorkflowRun)
	s.mux.HandleFunc("GET /workflows/{name}/yaml", s.handleWorkflowYAML)
	s.mux.HandleFunc("POST /workflows/{name}/duplicate", s.handleWorkflowDuplicate)
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

	// Reload
	s.mux.HandleFunc("POST /reload", s.handleReload)

	// Workspaces (CRUD + switching)
	s.mux.HandleFunc("GET /workspaces", s.handleWorkspacesList)
	s.mux.HandleFunc("GET /workspaces/new", s.handleWorkspaceNew)
	s.mux.HandleFunc("POST /workspaces/_new", s.handleWorkspaceCreate)
	s.mux.HandleFunc("GET /workspaces/switcher", s.handleWorkspaceSwitcher)
	s.mux.HandleFunc("POST /workspaces/switch", s.handleWorkspaceSwitch)
	s.mux.HandleFunc("POST /workspaces/unlock", s.handleWorkspaceUnlock)
	s.mux.HandleFunc("GET /workspaces/{name}", s.handleWorkspaceEdit)
	s.mux.HandleFunc("POST /workspaces/{name}", s.handleWorkspaceSave)
	s.mux.HandleFunc("DELETE /workspaces/{name}", s.handleWorkspaceDelete)

	// Blueprints
	s.mux.HandleFunc("GET /blueprints", s.handleBlueprintsList)
	s.mux.HandleFunc("GET /blueprints/browse", s.handleBlueprintsBrowse)
	s.mux.HandleFunc("GET /blueprints/browse/{name}", s.handleBlueprintBrowseDetail)
	s.mux.HandleFunc("POST /blueprints/browse/{name}/install", s.handleBlueprintBrowseInstall)
	s.mux.HandleFunc("POST /blueprints/preview", s.handleBlueprintPreview)
	s.mux.HandleFunc("POST /blueprints/install", s.handleBlueprintInstall)
	s.mux.HandleFunc("GET /blueprints/installed/{name}/yaml", s.handleBlueprintYAML)
	s.mux.HandleFunc("DELETE /blueprints/{name}", s.handleBlueprintUninstall)
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

// yamlViewArgs is the input contract for renderYAMLView. Render is called
// after the args are valid; it returns the raw YAML bytes.
type yamlViewArgs struct {
	Name         string
	Heading      string
	Subheading   string
	BackHref     string
	BackLabel    string
	DownloadName string
	Render       func() ([]byte, error)
}

// renderYAMLView is the shared handler for "View YAML" pages.
// ?download=1 emits raw application/yaml with an attachment disposition;
// otherwise renders the HTML wrapper page with copy + download affordances.
func (s *Server) renderYAMLView(w http.ResponseWriter, r *http.Request, args yamlViewArgs) {
	data, err := args.Render()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", args.DownloadName))
		_, _ = w.Write(data)
		return
	}
	downloadHref := r.URL.Path + "?download=1"
	s.render(w, "yaml_view.html", map[string]any{
		"Title":        args.Heading + " — YAML",
		"Nav":          "",
		"Heading":      args.Heading,
		"Subheading":   args.Subheading,
		"BackHref":     args.BackHref,
		"BackLabel":    args.BackLabel,
		"DownloadHref": downloadHref,
		"YAML":         string(data),
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
	case "code":
		s.registerCodeProvider(name, tc)
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
	var vaultRefs []string
	collectRef := func(v string) {
		if vault.HasVaultRefs(v) {
			vaultRefs = append(vaultRefs, v)
		}
	}
	collectRef(tc.BaseURL)
	collectRef(tc.Path)
	if tc.Auth != nil {
		collectRef(tc.Auth.Token)
		collectRef(tc.Auth.Value)
	}
	def := provider.RESTToolDef{
		Method:   tc.Method,
		BaseURL:  s.resolveRefT(tc.BaseURL, vaultKeys),
		Path:     s.resolveRefT(tc.Path, vaultKeys),
		Body:     tc.Body,
		BodyType: tc.BodyType,
		Headers:  s.resolveRefMapTracked(tc.Headers, vaultKeys),
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
				collectRef(oauthCfg.ClientID)
				collectRef(oauthCfg.ClientSecret)
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
	if len(vaultRefs) > 0 && s.resolver != nil {
		refs := vaultRefs // capture for closure
		res := s.resolver // capture for closure
		def.RedactSecrets = func(str string) string {
			return res.Redact(str, refs)
		}
	}
	rp.AddTool(name, def)
}

func (s *Server) registerWorkflowProvider(name string, tc config.ToolConfig) {
	prov := s.proxy.Provider("workflow")
	if prov == nil {
		// First workflow added in this UI session — bring up the provider.
		// The proxy itself is the WorkflowExecutor (steps call back through it).
		wp := provider.NewWorkflowProvider(s.proxy, false)
		wp.SetRunsDir(projectpath.Resolve(s.cfgPath, "runs", ""))
		s.proxy.RegisterProvider("workflow", wp)
		prov = wp
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

// registerCodeProvider lazy-creates the code provider on first use and
// compiles the script. A compile/lookup failure is logged but doesn't
// fail the registration of the surrounding tool — the user can fix the
// code through the UI and re-save.
func (s *Server) registerCodeProvider(name string, tc config.ToolConfig) {
	prov := s.proxy.Provider("code")
	if prov == nil {
		cp := codeprov.NewProvider(s.proxy, false)
		s.proxy.RegisterProvider("code", cp)
		prov = cp
	}
	cp, ok := prov.(*codeprov.Provider)
	if !ok {
		return
	}
	maxCalls := 0
	if tc.Shadow != nil {
		maxCalls = tc.Shadow.MaxCalls
	}
	if err := cp.RegisterCode(name, tc.Code, maxCalls); err != nil {
		fmt.Fprintf(os.Stderr, "warning: code tool %q failed to compile: %v\n", name, err)
	}
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
				case *codeprov.Provider:
					p.RemoveCode(name)
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

// ReloadStats describes what changed during a config reload.
type ReloadStats struct {
	Added   int
	Updated int
	Removed int
}

// reloadConfig re-reads config from disk and applies deltas to the live
// registry without restarting. Both handleReload and the pack install/uninstall
// handlers call this so changes go live immediately. Reloads always run
// under the currently-active workspace.
func (s *Server) reloadConfig() (ReloadStats, error) {
	return s.reloadConfigWithWorkspace(s.requestWorkspaceFromState())
}

// reloadConfigWithWorkspace is the workspace-aware reload. Callers that
// know which workspace should be active (e.g., the switch endpoint)
// pass it explicitly; everyone else uses reloadConfig.
func (s *Server) reloadConfigWithWorkspace(workspaceName string) (ReloadStats, error) {
	newCfg, err := config.Load(s.cfgPath, config.WithWorkspace(workspaceName))
	if err != nil {
		return ReloadStats{}, err
	}

	var stats ReloadStats

	// Find removed tools (in old but not new). Built-ins live in-memory only;
	// they aren't on disk, so skip them or reload would wipe them out.
	for name := range s.cfg.Tools {
		if builtins.IsBuiltinTool(name) {
			continue
		}
		if _, exists := newCfg.Tools[name]; !exists {
			s.unregisterTool(name)
			stats.Removed++
		}
	}

	// Find added and changed tools
	for name, newTC := range newCfg.Tools {
		if builtins.IsBuiltinTool(name) {
			continue
		}
		oldTC, exists := s.cfg.Tools[name]
		if !exists {
			stats.Added++
		} else if !reflect.DeepEqual(oldTC, newTC) {
			s.unregisterTool(name)
			stats.Updated++
		} else {
			continue // unchanged
		}
		s.registerTool(name, newTC)
	}

	// Replace tools with new set, but preserve the in-memory built-ins that
	// aren't on disk.
	for name, tc := range s.cfg.Tools {
		if builtins.IsBuiltinTool(name) {
			if newCfg.Tools == nil {
				newCfg.Tools = make(map[string]config.ToolConfig)
			}
			newCfg.Tools[name] = tc
		}
	}
	s.cfg.Tools = newCfg.Tools
	s.cfg.OAuthProviders = newCfg.OAuthProviders

	if s.OnReload != nil {
		s.OnReload()
	}
	return stats, nil
}

// handleReload re-reads config from disk and applies deltas to the live
// registry, providers, and shadow policy without restarting.
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	stats, err := s.reloadConfig()
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<span class="text-red-600 text-xs">✗ %s</span>`, template.HTMLEscapeString(err.Error()))
		return
	}

	// Tell browser to refresh the page so sidebar/content updates
	w.Header().Set("HX-Refresh", "true")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="text-green-600 text-xs">✓ Reloaded (%d added, %d updated, %d removed)</span>`,
		stats.Added, stats.Updated, stats.Removed)
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"inc":          func(i int) int { return i + 1 },
		"markdown":     renderMarkdown,
		"markdownLead": markdownLead,
		"joinList": func(items []string) string {
			return strings.Join(items, ", ")
		},
		"icon": func(name string) template.HTML {
			icons := map[string]string{
				"play":          `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="6 3 20 12 6 21 6 3"/></svg>`,
				"send":          `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/></svg>`,
				"plus":          `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="M12 5v14"/></svg>`,
				"trash":         `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/></svg>`,
				"check":         `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`,
				"x":             `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`,
				"shield":        `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/></svg>`,
				"terminal":      `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/></svg>`,
				"globe":         `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>`,
				"workflow":      `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="8" height="8" rx="2"/><rect x="13" y="13" width="8" height="8" rx="2"/><path d="M7 11v4a2 2 0 0 0 2 2h4"/></svg>`,
				"clock":         `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`,
				"lock":          `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>`,
				"package":       `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m7.5 4.27 9 5.15"/><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/></svg>`,
				"external-link": `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></svg>`,
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
	}
}
