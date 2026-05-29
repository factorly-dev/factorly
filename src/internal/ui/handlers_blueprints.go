// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/factorly-dev/factorly/internal/blueprints"
	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/configyaml"
)

// previewRequest is the JSON body posted to /blueprints/preview.
//
// Either Source (URL / GitHub shorthand / local path) or Content (raw YAML
// pasted in the UI) must be provided. If both are set, Content wins.
type previewRequest struct {
	Source  string `json:"source,omitempty"`
	Content string `json:"content,omitempty"`
}

// installRequest is the JSON body posted to /blueprints/install. Same
// Source/Content rules as previewRequest.
type installRequest struct {
	Source      string            `json:"source,omitempty"`
	Content     string            `json:"content,omitempty"`
	VaultValues map[string]string `json:"vault_values,omitempty"`
}

// previewResponse wraps the install result with a top-level error string so
// the UI can render structured errors (parse failures, source-not-found)
// the same way it renders structured success previews.
type previewResponse struct {
	Result *blueprints.InstallResult `json:"result,omitempty"`
	Error  string                    `json:"error,omitempty"`
}

// builtinNamesFromConfig returns the set of registered builtin tool names so
// blueprint validation can satisfy workflow-step references to e.g.
// factorly.fetch without those tools needing to also appear in the incoming
// blueprint.
func (s *Server) builtinNamesFromConfig() map[string]bool {
	out := map[string]bool{}
	for name := range s.cfg.Tools {
		if builtins.IsBuiltinTool(name) {
			out[name] = true
		}
	}
	return out
}

// handleBlueprintPreview fetches and validates a blueprint without writing
// anything. Returns a structured InstallResult so the UI modal can render a
// preview (header, tools/workflows added, providers, vault keys needed,
// conflicts, missing requires, already-installed flag).
func (s *Server) handleBlueprintPreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: err.Error()})
		return
	}
	if req.Source == "" && req.Content == "" {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: "source or content is required"})
		return
	}

	opts := blueprints.InstallOptions{
		CfgPath:      s.cfgPath,
		DryRun:       true,
		BuiltinTools: s.builtinNamesFromConfig(),
	}
	if req.Content != "" {
		opts.Content = []byte(req.Content)
	} else {
		opts.Source = req.Source
	}
	res, err := blueprints.Install(opts)
	if err != nil {
		// Even on error we may have a partial result with conflicts/missing-requires
		// populated — surface both to the UI so the modal can show the diagnosis.
		writeJSON(w, http.StatusOK, previewResponse{Result: res, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, previewResponse{Result: res})
}

// handleBlueprintInstall commits a blueprint to disk, writes any user-supplied
// vault values to the local vault, and triggers a config reload so the new
// tools are live in the proxy without requiring a process restart.
func (s *Server) handleBlueprintInstall(w http.ResponseWriter, r *http.Request) {
	var req installRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: err.Error()})
		return
	}
	if req.Source == "" && req.Content == "" {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: "source or content is required"})
		return
	}

	// Write user-supplied vault values BEFORE the install so the
	// resolveBackendRefs pass during the post-install reload finds them.
	for key, value := range req.VaultValues {
		if value == "" {
			continue
		}
		if s.vault == nil {
			writeJSON(w, http.StatusServiceUnavailable, previewResponse{Error: "vault not available"})
			return
		}
		if err := s.vault.Set(key, value); err != nil {
			writeJSON(w, http.StatusInternalServerError, previewResponse{
				Error: fmt.Sprintf("storing vault key %q: %v", key, err),
			})
			return
		}
	}

	opts := blueprints.InstallOptions{
		CfgPath:      s.cfgPath,
		DryRun:       false,
		BuiltinTools: s.builtinNamesFromConfig(),
	}
	if req.Content != "" {
		opts.Content = []byte(req.Content)
	} else {
		opts.Source = req.Source
	}
	res, err := blueprints.Install(opts)
	if err != nil {
		writeJSON(w, http.StatusOK, previewResponse{Result: res, Error: err.Error()})
		return
	}

	// Reload so the new tools become live in the proxy. We swallow the
	// reload error because the install itself succeeded — partial activation
	// is recoverable by the user clicking Reload manually. The error is
	// surfaced in the response so the UI can flag it.
	if _, rerr := s.reloadConfig(); rerr != nil {
		writeJSON(w, http.StatusOK, previewResponse{
			Result: res,
			Error:  fmt.Sprintf("blueprint written but reload failed: %v (try clicking Reload)", rerr),
		})
		return
	}

	writeJSON(w, http.StatusOK, previewResponse{Result: res})
}

// handleBlueprintsList renders the /blueprints page showing installed
// blueprints with uninstall actions.
func (s *Server) handleBlueprintsList(w http.ResponseWriter, r *http.Request) {
	list, err := blueprints.List(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "blueprints.html", map[string]any{
		"Title":      "Blueprints",
		"Nav":        "blueprints",
		"Blueprints": list,
	})
}

// handleBlueprintsBrowse renders the /blueprints/browse catalog page. Lists
// every bundled blueprint as a card with category + auth-type chips. Cards
// that are already installed get an "Installed" badge so users don't try to
// install them twice.
func (s *Server) handleBlueprintsBrowse(w http.ResponseWriter, r *http.Request) {
	installedNames := map[string]bool{}
	if list, err := blueprints.List(s.cfgPath); err == nil {
		for _, bp := range list {
			installedNames[bp.Name] = true
		}
	}

	type cardData struct {
		*blueprints.BundledBlueprint
		Installed bool
	}
	bundled := blueprints.Bundled()
	cards := make([]cardData, 0, len(bundled))
	for _, bp := range bundled {
		cards = append(cards, cardData{
			BundledBlueprint: bp,
			Installed:        installedNames[bp.Header.Name],
		})
	}
	s.render(w, "blueprints_browse.html", map[string]any{
		"Title": "Browse Blueprints",
		"Nav":   "blueprints",
		"Cards": cards,
	})
}

// handleBlueprintBrowseDetail renders the per-blueprint detail page from the
// bundled catalog. Shows the auth guide, the list of tools the blueprint
// would add, and a one-click Install button.
func (s *Server) handleBlueprintBrowseDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bp := blueprints.BundledByName(name)
	if bp == nil {
		http.NotFound(w, r)
		return
	}

	// Parse the YAML so the page can list tool names + descriptions without
	// duplicating that data in the header.
	parsed, err := blueprints.ParseBlueprint([]byte(bp.YAML))
	if err != nil {
		http.Error(w, fmt.Sprintf("parsing bundled blueprint: %v", err), http.StatusInternalServerError)
		return
	}

	// Is it already installed?
	installed := false
	if list, err := blueprints.List(s.cfgPath); err == nil {
		for _, b := range list {
			if b.Name == bp.Header.Name {
				installed = true
				break
			}
		}
	}

	// Sort tool names for stable display.
	toolNames := make([]string, 0, len(parsed.Tools))
	for n := range parsed.Tools {
		toolNames = append(toolNames, n)
	}
	sort.Strings(toolNames)

	type toolRow struct {
		Name        string
		Description string
	}
	tools := make([]toolRow, 0, len(toolNames))
	for _, n := range toolNames {
		tools = append(tools, toolRow{Name: n, Description: parsed.Tools[n].Description})
	}

	// Vault keys the user needs to set, if any.
	var vaultKeys []string
	if parsed.Requires != nil {
		vaultKeys = parsed.Requires.VaultKeys
	}

	s.render(w, "blueprint_browse_detail.html", map[string]any{
		"Title":     bp.Header.DisplayName,
		"Nav":       "blueprints",
		"BP":        bp,
		"Tools":     tools,
		"VaultKeys": vaultKeys,
		"Installed": installed,
	})
}

// handleBlueprintBrowseInstall installs a bundled blueprint by name. The
// catalog detail page POSTs here; this is just sugar over the main
// /blueprints/install endpoint that doesn't require the client to ship the
// full YAML body back to us.
func (s *Server) handleBlueprintBrowseInstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	bp := blueprints.BundledByName(name)
	if bp == nil {
		http.NotFound(w, r)
		return
	}

	// Optional vault values supplied as form fields (one per missing key).
	// The form posts URL-encoded values; ParseForm handles both GET and POST.
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for key, vals := range r.Form {
		if !strings.HasPrefix(key, "vault_") {
			continue
		}
		realKey := strings.TrimPrefix(key, "vault_")
		val := strings.TrimSpace(vals[0])
		if val == "" || s.vault == nil {
			continue
		}
		if err := s.vault.Set(realKey, val); err != nil {
			http.Error(w, fmt.Sprintf("storing vault key %q: %v", realKey, err), http.StatusInternalServerError)
			return
		}
	}

	res, err := blueprints.Install(blueprints.InstallOptions{
		Content:      []byte(bp.YAML),
		CfgPath:      s.cfgPath,
		BuiltinTools: s.builtinNamesFromConfig(),
	})
	if err != nil {
		// Re-render the detail page with the error inline.
		http.Error(w, fmt.Sprintf("%s\n\n%s", err.Error(), summarizeResult(res)), http.StatusBadRequest)
		return
	}
	if _, rerr := s.reloadConfig(); rerr != nil {
		http.Error(w, fmt.Sprintf("installed but reload failed: %v (click Reload to refresh)", rerr), http.StatusInternalServerError)
		return
	}
	// On success, redirect back to /blueprints so the user sees the
	// freshly-installed entry in their list.
	toast(w, toastSuccess, "Installed blueprint "+name)
	w.Header().Set("HX-Redirect", "/blueprints")
	http.Redirect(w, r, "/blueprints", http.StatusSeeOther)
}

// summarizeResult turns an InstallResult into a short error-message tail so
// the user sees conflicts / missing requires inline with the failure on the
// detail page.
func summarizeResult(res *blueprints.InstallResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Conflicts {
		parts = append(parts, fmt.Sprintf("%s %q already defined", c.Kind, c.Name))
	}
	for _, m := range res.RequiresMissing {
		parts = append(parts, fmt.Sprintf("missing %s %q", m.Kind, m.Name))
	}
	if res.AlreadyInstalled {
		parts = append(parts, "already installed")
	}
	return strings.Join(parts, "; ")
}

// handleBlueprintUninstall removes a blueprint and reloads.
func (s *Server) handleBlueprintUninstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "blueprint name required", http.StatusBadRequest)
		return
	}
	if err := blueprints.Uninstall(s.cfgPath, name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.reloadConfig(); err != nil {
		// Best-effort: blueprint file is gone, but reload failed. Hint at
		// manual remedy.
		http.Error(w, fmt.Sprintf("uninstalled but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	toast(w, toastInfo, "Uninstalled blueprint "+name)
	w.Header().Set("HX-Redirect", "/blueprints")
	w.WriteHeader(http.StatusOK)
}

// --- helpers ---

func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("empty request body")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parsing body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// handleBlueprintYAML renders the installed blueprint file from disk.
// 404s if the named blueprint isn't installed.
func (s *Server) handleBlueprintYAML(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Existence check up front so the 404 path doesn't get swallowed by
	// renderYAMLView's 500 fallback.
	if _, err := configyaml.RenderBlueprint(s.cfgPath, name); err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderYAMLView(w, r, yamlViewArgs{
		Name:         name,
		Heading:      name,
		Subheading:   "Installed blueprint",
		BackHref:     "/blueprints",
		BackLabel:    "Back to blueprints",
		DownloadName: name + ".yaml",
		Render:       func() ([]byte, error) { return configyaml.RenderBlueprint(s.cfgPath, name) },
	})
}
