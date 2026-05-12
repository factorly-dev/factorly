// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/factorly-dev/factorly/internal/builtins"
	"github.com/factorly-dev/factorly/internal/packs"
)

// previewRequest is the JSON body posted to /packs/preview.
type previewRequest struct {
	Source string `json:"source"`
}

// installRequest is the JSON body posted to /packs/install.
type installRequest struct {
	Source      string            `json:"source"`
	VaultValues map[string]string `json:"vault_values,omitempty"`
}

// previewResponse wraps the install result with a top-level error string so
// the UI can render structured errors (parse failures, source-not-found)
// the same way it renders structured success previews.
type previewResponse struct {
	Result *packs.InstallResult `json:"result,omitempty"`
	Error  string               `json:"error,omitempty"`
}

// builtinNamesFromConfig returns the set of registered builtin tool names so
// pack validation can satisfy workflow-step references to e.g. factorly.fetch
// without those tools needing to also appear in the incoming pack.
func (s *Server) builtinNamesFromConfig() map[string]bool {
	out := map[string]bool{}
	for name := range s.cfg.Tools {
		if builtins.IsBuiltinTool(name) {
			out[name] = true
		}
	}
	return out
}

// handlePackPreview fetches and validates a pack without writing anything.
// Returns a structured InstallResult so the UI modal can render a preview
// (header, tools/workflows added, providers, vault keys needed, conflicts,
// missing requires, already-installed flag).
func (s *Server) handlePackPreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: err.Error()})
		return
	}
	if req.Source == "" {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: "source is required"})
		return
	}

	res, err := packs.Install(packs.InstallOptions{
		Source:       req.Source,
		CfgPath:      s.cfgPath,
		DryRun:       true,
		BuiltinTools: s.builtinNamesFromConfig(),
	})
	if err != nil {
		// Even on error we may have a partial result with conflicts/missing-requires
		// populated — surface both to the UI so the modal can show the diagnosis.
		writeJSON(w, http.StatusOK, previewResponse{Result: res, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, previewResponse{Result: res})
}

// handlePackInstall commits a pack to disk, writes any user-supplied vault
// values to the local vault, and triggers a config reload so the new tools
// are live in the proxy without requiring a process restart.
func (s *Server) handlePackInstall(w http.ResponseWriter, r *http.Request) {
	var req installRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: err.Error()})
		return
	}
	if req.Source == "" {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: "source is required"})
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

	res, err := packs.Install(packs.InstallOptions{
		Source:       req.Source,
		CfgPath:      s.cfgPath,
		DryRun:       false,
		BuiltinTools: s.builtinNamesFromConfig(),
	})
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
			Error:  fmt.Sprintf("pack written but reload failed: %v (try clicking Reload)", rerr),
		})
		return
	}

	writeJSON(w, http.StatusOK, previewResponse{Result: res})
}

// handlePacksList renders the /packs page showing installed packs with
// uninstall actions.
func (s *Server) handlePacksList(w http.ResponseWriter, r *http.Request) {
	list, err := packs.List(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "packs.html", map[string]any{
		"Title": "Packs",
		"Nav":   "packs",
		"Packs": list,
	})
}

// handlePackUninstall removes a pack and reloads.
func (s *Server) handlePackUninstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "pack name required", http.StatusBadRequest)
		return
	}
	if err := packs.Uninstall(s.cfgPath, name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.reloadConfig(); err != nil {
		// Best-effort: pack file is gone, but reload failed. Hint at manual
		// remedy.
		http.Error(w, fmt.Sprintf("uninstalled but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Redirect", "/packs")
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
