// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
	"github.com/factorly-dev/factorly/internal/promote"
	codeprov "github.com/factorly-dev/factorly/internal/provider/code"
)

// handlePromoteForm renders the "Save as tool" form for a specific
// factorly.code audit-log entry, addressed by source_sha prefix. The
// form is the UI counterpart of the CLI's `factorly tools promote`
// command — both consume internal/promote to recover the script and
// shape the tool config.
func (s *Server) handlePromoteForm(w http.ResponseWriter, r *http.Request) {
	sha := r.URL.Query().Get("from-sha")
	if sha == "" {
		http.Error(w, "from-sha is required", http.StatusBadRequest)
		return
	}

	logPath := s.promoteLogPath()
	res, err := promote.FromLog(logPath, sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Compile-validate up front so a broken script never gets past
	// the form. If the source is invalid we still render the form
	// but disable submit and show the compile error.
	compileErr := codeprov.Validate(res.Source)

	s.render(w, "tool_promote.html", map[string]any{
		"Title":      "Save as tool",
		"Nav":        "history",
		"FromSHA":    res.SHA,
		"ShortSHA":   shortPromoteSHA(res.SHA),
		"Source":     res.Source,
		"Parameters": res.Parameters,
		"Timestamp":  res.OriginalRun.Timestamp.Format("2006-01-02 15:04:05"),
		"CompileErr": errOrEmpty(compileErr),
		"AutoDesc":   fmt.Sprintf("Promoted from factorly.code run on %s (sha %s)", res.OriginalRun.Timestamp.Format("2006-01-02"), shortPromoteSHA(res.SHA)),
	})
}

// handlePromoteSubmit creates the tool from the submitted form. On
// success, redirects to /tools/<name> so the operator can refine the
// description and parameter docs.
func (s *Server) handlePromoteSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sha := r.FormValue("from-sha")
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	confirm := r.FormValue("confirm") == "on"
	overwrite := r.FormValue("overwrite") == "on"

	if sha == "" {
		http.Error(w, "from-sha is required", http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	logPath := s.promoteLogPath()
	res, err := promote.FromLog(logPath, sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := codeprov.Validate(res.Source); err != nil {
		http.Error(w, "script does not compile: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Conflict check: refuse silent overwrite unless explicitly asked.
	if _, exists := s.cfg.Tools[name]; exists && !overwrite {
		http.Error(w, fmt.Sprintf("tool %q already exists (check 'overwrite' to replace)", name), http.StatusConflict)
		return
	}

	if description == "" {
		description = fmt.Sprintf("Promoted from factorly.code run on %s (sha %s)", res.OriginalRun.Timestamp.Format("2006-01-02"), shortPromoteSHA(res.SHA))
	}

	tc := config.ToolConfig{
		Type:        "code",
		Description: description,
		Code:        res.Source,
		Parameters:  res.Parameters,
	}
	if confirm {
		tc.Shadow = &config.ShadowConfig{Confirm: true}
	}

	if err := SaveTool(s.cfgPath, s.toolsDir, name, tc); err != nil {
		http.Error(w, "saving tool: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config so the new tool is callable immediately. Mirrors
	// what /tools/_new does after a successful save.
	if _, err := s.reloadConfig(); err != nil {
		http.Error(w, "tool saved but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// htmx clients see HX-Redirect; plain form posts get a 303 redirect.
	w.Header().Set("HX-Redirect", "/tools/"+name)
	http.Redirect(w, r, "/tools/"+name, http.StatusSeeOther)
}

// promoteLogPath returns the audit log path the server should consult
// for promote recovery. Honors FACTORLY_LOG_PATH (test/dev override)
// before falling back to the project-scoped default.
func (s *Server) promoteLogPath() string {
	if p := os.Getenv("FACTORLY_LOG_PATH"); p != "" {
		return p
	}
	return logger.ProjectLogPath(s.cfgPath)
}

func shortPromoteSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
