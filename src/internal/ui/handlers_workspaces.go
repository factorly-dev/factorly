// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/factorly-dev/factorly/internal/vault"
	"github.com/factorly-dev/factorly/internal/workspace"
)

// handleWorkspacesList renders the /workspaces page (list view).
func (s *Server) handleWorkspacesList(w http.ResponseWriter, r *http.Request) {
	wss, err := workspace.List(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "workspaces.html", map[string]any{
		"Title":      "Workspaces",
		"Nav":        "workspaces",
		"Workspaces": wss,
		"Active":     s.requestWorkspace(r),
	})
}

// handleWorkspaceNew renders the create form.
func (s *Server) handleWorkspaceNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "workspace_new.html", map[string]any{
		"Title": "New Workspace",
		"Nav":   "workspaces",
	})
}

// handleWorkspaceCreate writes the new workspace file.
func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.Form.Get("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	ws := &workspace.Workspace{
		Name:        name,
		Description: strings.TrimSpace(r.Form.Get("description")),
		Vars:        map[string]string{},
	}
	if err := workspace.Save(s.cfgPath, ws); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("HX-Redirect", "/workspaces/"+name)
	w.WriteHeader(http.StatusNoContent)
}

// handleWorkspaceEdit renders the edit form for a single workspace.
func (s *Server) handleWorkspaceEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ws, err := workspace.Load(s.cfgPath, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// Sort var keys for stable form rendering — Go template ranges over
	// maps in unspecified order.
	keys := make([]string, 0, len(ws.Vars))
	for k := range ws.Vars {
		keys = append(keys, k)
	}
	sortStringsAsc(keys)
	type kv struct{ Key, Value string }
	pairs := make([]kv, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, kv{Key: k, Value: ws.Vars[k]})
	}
	s.render(w, "workspace_edit.html", map[string]any{
		"Title":     "Workspace · " + name,
		"Nav":       "workspaces",
		"Workspace": ws,
		"VarPairs":  pairs,
		"Active":    s.requestWorkspace(r),
	})
}

func sortStringsAsc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// handleWorkspaceSave writes updates to an existing workspace YAML.
func (s *Server) handleWorkspaceSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ws := &workspace.Workspace{
		Name:        name,
		Description: strings.TrimSpace(r.Form.Get("description")),
		Vars:        map[string]string{},
	}
	// Vars come in as parallel arrays var_key[i] / var_value[i] or
	// indexed names var_key_0/var_value_0. We use the indexed form so
	// row deletes that leave gaps can be reindexed client-side.
	for i := 0; ; i++ {
		k := strings.TrimSpace(r.Form.Get(fmt.Sprintf("var_key_%d", i)))
		v := r.Form.Get(fmt.Sprintf("var_value_%d", i))
		if k == "" && v == "" {
			// Stop at the first empty slot — matches the tool-params
			// parsing convention used elsewhere in the UI.
			if _, exists := r.Form[fmt.Sprintf("var_key_%d", i)]; !exists {
				break
			}
			if k == "" {
				continue
			}
		}
		if k == "" {
			continue
		}
		ws.Vars[k] = v
	}
	if err := workspace.Save(s.cfgPath, ws); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("HX-Redirect", "/workspaces/"+name)
	w.WriteHeader(http.StatusNoContent)
}

// handleWorkspaceDelete removes the workspace YAML file. Does NOT touch
// the workspace's vault file — that's a separate destructive action.
func (s *Server) handleWorkspaceDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := workspace.Delete(s.cfgPath, name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Reload state if we just deleted the active workspace.
	if s.requestWorkspace(r) == name {
		s.setActiveWorkspace("")
		http.SetCookie(w, &http.Cookie{
			Name: workspaceCookieName, Value: "", Path: "/", MaxAge: -1,
		})
	}
	w.Header().Set("HX-Redirect", "/workspaces")
	w.WriteHeader(http.StatusNoContent)
}

// handleWorkspaceSwitch flips the active workspace for this UI session.
// Body: form-encoded `name`. On success: cookie set + HX-Redirect to
// the page the user came from. When the workspace's vault is locked
// (file exists but no usable password resolution), returns the unlock
// partial with a 200 status so htmx swaps it in place of the dropdown.
func (s *Server) handleWorkspaceSwitch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.Form.Get("name")
	// Empty name is valid — it clears the cookie and reverts to the
	// server-startup workspace (or no overlay if none was set).
	if name != "" {
		if !workspace.Exists(s.cfgPath, name) {
			http.Error(w, fmt.Sprintf("workspace %q not found", name), http.StatusNotFound)
			return
		}
	}

	// Try to open the workspace vault chain. The Manager handles
	// cache-or-open in one call. If we need to ask the user for a
	// password, render the unlock partial.
	//
	// A "no chain opener configured" error means the Manager was set
	// up without an opener (test default; production always wires
	// one). Treat that as "skip vault opening" — config reload still
	// runs, and the test's switch flow can succeed without unlocking
	// a real vault.
	if name != "" {
		if _, err := s.vaultMgr.GetOrOpen("workspace:" + name); err != nil && !strings.Contains(err.Error(), "no chain opener configured") {
			if isVaultLocked(err) {
				s.renderUnlockPartial(w, name, "")
				return
			}
			http.Error(w, "opening workspace vault: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Reload config under the new workspace.
	if _, err := s.reloadConfigWithWorkspace(name); err != nil {
		http.Error(w, "reloading config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update server state + cookie.
	s.setActiveWorkspace(name)
	cookie := &http.Cookie{
		Name:     workspaceCookieName,
		Value:    name,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	if name == "" {
		// Empty name clears the cookie.
		cookie.MaxAge = -1
	}
	http.SetCookie(w, cookie)

	// Refresh the current page so the new workspace context is applied
	// everywhere (pill, audit log entries, tool resolution). HX-Refresh
	// is htmx's "reload current page" header — works regardless of
	// hx-swap/hx-target on the originating form, unlike HX-Redirect
	// which can collide with a pending swap of a 204 empty body.
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// handleWorkspaceUnlock is the second half of a switch that needs a
// password. The unlock form posts here with name + password; we try
// to open the vault and on success complete the switch flow.
func (s *Server) handleWorkspaceUnlock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.Form.Get("name")
	password := r.Form.Get("password")
	if name == "" || password == "" {
		s.renderUnlockPartial(w, name, "name and password are required")
		return
	}

	pw := vault.SecretFromString(password)
	defer pw.Zero()
	backend, err := s.vaultMgr.OpenWithPassword("workspace:"+name, pw)
	if err != nil {
		s.renderUnlockPartial(w, name, "incorrect password")
		return
	}
	s.vaultMgr.Put("workspace:"+name, backend)

	if _, err := s.reloadConfigWithWorkspace(name); err != nil {
		s.renderUnlockPartial(w, name, "reload failed: "+err.Error())
		return
	}
	s.setActiveWorkspace(name)
	http.SetCookie(w, &http.Cookie{
		Name:     workspaceCookieName,
		Value:    name,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// renderUnlockPartial returns the inline password-prompt HTML used by
// both /workspaces/switch (when a vault needs unlocking) and
// /workspaces/unlock (when the password was wrong).
func (s *Server) renderUnlockPartial(w http.ResponseWriter, name, errMsg string) {
	s.renderPartial(w, "workspace_unlock", map[string]any{
		"Name":  name,
		"Error": errMsg,
	})
}

// handleWorkspaceSwitcher renders the dropdown panel that lists
// available workspaces. Loaded into the top-nav pill via htmx.
func (s *Server) handleWorkspaceSwitcher(w http.ResponseWriter, r *http.Request) {
	wss, err := workspace.List(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	active := s.requestWorkspace(r)
	s.renderPartial(w, "workspace_switcher", map[string]any{
		"Workspaces": wss,
		"Active":     active,
	})
}

// isVaultLocked detects errors from the workspace vault open path
// that mean the user needs to supply a password. The CLI's open
// helpers signal this via a sentinel "workspace vault locked"
// message when no non-interactive password source resolved; we also
// catch wrong-password decryption failures so the UI re-prompts.
func isVaultLocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "workspace vault locked"):
		return true
	case strings.Contains(msg, "decrypt") || strings.Contains(msg, "invalid password"):
		return true
	}
	return false
}
