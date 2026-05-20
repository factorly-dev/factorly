// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/config"
	"github.com/factorly-dev/factorly/internal/logger"
)

// seedPromotableLog drops a single factorly.code entry into a fresh
// audit log file under the given cfgPath. Returns the SHA so the
// test can address the entry. Helper keeps each test compact.
func seedPromotableLog(t *testing.T, cfgPath, src string, params map[string]string) string {
	t.Helper()

	// The promote handler reads FACTORLY_LOG_PATH first; point it at
	// a tmpdir file so the test doesn't write to the global log path.
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("FACTORLY_LOG_PATH", logPath)

	// Serialize the inner params as the factorly.code builtin would.
	innerJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct, predictable SHA; the promote layer trusts the
	// entry's source_sha rather than recomputing it, so any 64-hex
	// value works for tests.
	sha := strings.Repeat("a", 60) + "beef"

	entry := logger.Entry{
		Timestamp: time.Now(),
		Tool:      "factorly.code",
		Status:    "success",
		SourceSHA: sha,
		Params: map[string]string{
			"code":   src,
			"params": string(innerJSON),
		},
	}
	b, _ := json.Marshal(entry)
	if err := os.WriteFile(logPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = cfgPath // unused but kept for symmetry with other test helpers
	return sha
}

func TestPromoteFormHappyPath(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	src := "package main\nfunc Run(p map[string]string) (any, error) { return p[\"who\"], nil }"
	sha := seedPromotableLog(t, cfgPath, src, map[string]string{"who": "world"})

	req := httptest.NewRequest(http.MethodGet, "/tools/promote?from-sha="+sha[:8], nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Save as tool", "who", "world", sha[:8]} {
		if !strings.Contains(body, want) {
			t.Errorf("form missing %q", want)
		}
	}
}

func TestPromoteFormRequiresSHA(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/tools/promote", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPromoteFormNotFound(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	// Set the env var so the handler doesn't read a real log file.
	t.Setenv("FACTORLY_LOG_PATH", filepath.Join(t.TempDir(), "empty.jsonl"))
	req := httptest.NewRequest(http.MethodGet, "/tools/promote?from-sha=nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPromoteFormSurfacesCompileError(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	// Missing closing paren — yaegi rejects at compile time.
	badSrc := "package main\nfunc Run(p map[string]string) (any, error { return nil, nil }"
	sha := seedPromotableLog(t, cfgPath, badSrc, nil)

	req := httptest.NewRequest(http.MethodGet, "/tools/promote?from-sha="+sha[:8], nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	// Form still renders (so the operator sees what they're working
	// with) but submit is disabled and the error is visible.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (form should still render)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "does not compile") {
		t.Error("body should mention compile failure")
	}
	if !strings.Contains(body, "disabled") {
		t.Error("submit button should be disabled when script doesn't compile")
	}
}

func TestPromoteSubmitHappyPath(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	src := "package main\nfunc Run(p map[string]string) (any, error) { return \"hi \" + p[\"who\"], nil }"
	sha := seedPromotableLog(t, cfgPath, src, map[string]string{"who": "world"})

	form := url.Values{}
	form.Set("from-sha", sha)
	form.Set("name", "greet.someone")
	form.Set("confirm", "on")

	req := httptest.NewRequest(http.MethodPost, "/tools/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	// On success, HX-Redirect is set for htmx clients AND a 303 lands
	// for plain form posts. We accept either status code here.
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "/tools/greet.someone" {
		t.Errorf("HX-Redirect = %q, want /tools/greet.someone", rec.Header().Get("HX-Redirect"))
	}

	// Tool must now be in the config + reloaded.
	if _, ok := srv.cfg.Tools["greet.someone"]; !ok {
		t.Errorf("tool not registered after promote")
	}
}

func TestPromoteSubmitRequiresName(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	src := "package main\nfunc Run(p map[string]string) (any, error) { return nil, nil }"
	sha := seedPromotableLog(t, cfgPath, src, nil)

	form := url.Values{}
	form.Set("from-sha", sha)
	// no name
	req := httptest.NewRequest(http.MethodPost, "/tools/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestPromoteSubmitRefusesOverwrite(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	// Pre-populate a tool by the name we'll try to claim.
	srv.cfg.Tools["greet.someone"] = config.ToolConfig{Type: "cli", Command: "echo"}

	src := "package main\nfunc Run(p map[string]string) (any, error) { return nil, nil }"
	sha := seedPromotableLog(t, cfgPath, src, nil)

	form := url.Values{}
	form.Set("from-sha", sha)
	form.Set("name", "greet.someone")
	// No overwrite checkbox — should be rejected.

	req := httptest.NewRequest(http.MethodPost, "/tools/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestPromoteSubmitRefusesBrokenScript(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	badSrc := "package main\nfunc Run(p map[string]string) (any, error { return nil, nil }"
	sha := seedPromotableLog(t, cfgPath, badSrc, nil)

	form := url.Values{}
	form.Set("from-sha", sha)
	form.Set("name", "broken.tool")

	req := httptest.NewRequest(http.MethodPost, "/tools/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (broken script must not be written)", rec.Code)
	}
	if _, ok := srv.cfg.Tools["broken.tool"]; ok {
		t.Error("broken tool must not be registered")
	}
}
