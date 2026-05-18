// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedWorkspaces writes workspace YAML files alongside the test
// server's cfgPath. cfgPath is <tmp>/factorly.yaml from
// testServerWithProxy; workspace files land at
// <tmp>/.factorly/workspaces/<name>.yaml.
func seedWorkspaces(t *testing.T, cfgPath string, workspaces map[string]string) {
	t.Helper()
	dir := filepath.Join(filepath.Dir(cfgPath), ".factorly", "workspaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range workspaces {
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHandleWorkspacesListEmpty(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "No workspaces defined yet") {
		t.Errorf("expected empty-state message; body=%s", rec.Body.String())
	}
}

func TestHandleWorkspacesListPopulated(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"staging": "description: stage\nvars: {API: https://staging}\n",
		"prod":    "description: live\nvars: {}\n",
	})

	req := httptest.NewRequest(http.MethodGet, "/workspaces", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"staging", "prod", "stage", "live"} {
		if !strings.Contains(body, want) {
			t.Errorf("list page missing %q", want)
		}
	}
}

func TestHandleWorkspaceCreateRoundTrip(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)

	form := url.Values{}
	form.Set("name", "qa")
	form.Set("description", "QA environment")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/_new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/workspaces/qa" {
		t.Errorf("HX-Redirect = %q, want /workspaces/qa", got)
	}
	path := filepath.Join(filepath.Dir(cfgPath), ".factorly", "workspaces", "qa.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("workspace file not created: %v", err)
	}
	if !strings.Contains(string(data), "QA environment") {
		t.Errorf("description not persisted: %s", data)
	}
}

func TestHandleWorkspaceCreateRejectsInvalidName(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	for _, bad := range []string{"foo/bar", "a.b", `back\slash`, ""} {
		form := url.Values{}
		form.Set("name", bad)
		req := httptest.NewRequest(http.MethodPost, "/workspaces/_new", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusNoContent {
			t.Errorf("expected error for name %q, got 204", bad)
		}
	}
}

func TestHandleWorkspaceSaveUpdatesVars(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"staging": "vars: {API: old}\n",
	})

	form := url.Values{}
	form.Set("description", "staging API")
	form.Set("var_key_0", "API")
	form.Set("var_value_0", "https://api.staging.example.com")
	form.Set("var_key_1", "REGION")
	form.Set("var_value_1", "us-west")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/staging", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "staging")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("save status = %d, body=%s", rec.Code, rec.Body.String())
	}
	path := filepath.Join(filepath.Dir(cfgPath), ".factorly", "workspaces", "staging.yaml")
	data, _ := os.ReadFile(path)
	body := string(data)
	for _, want := range []string{"API:", "https://api.staging.example.com", "REGION:", "us-west", "staging API"} {
		if !strings.Contains(body, want) {
			t.Errorf("save missed %q in %s", want, body)
		}
	}
}

func TestHandleWorkspaceSwitchSetsCookieAndRedirects(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"staging": "vars: {}\n",
	})

	form := url.Values{}
	form.Set("name", "staging")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("switch status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("HX-Refresh = %q, want true", got)
	}
	// Cookie should be set to staging
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == workspaceCookieName && c.Value == "staging" {
			found = true
		}
	}
	if !found {
		t.Errorf("factorly_workspace=staging cookie not set; got cookies %+v", cookies)
	}
	// Server state should reflect the switch.
	if srv.requestWorkspaceFromState() != "staging" {
		t.Errorf("activeWorkspace = %q, want staging", srv.requestWorkspaceFromState())
	}
}

func TestHandleWorkspaceSwitchUnknownReturns404(t *testing.T) {
	srv, _ := testServerWithProxy(t, nil)
	form := url.Values{}
	form.Set("name", "ghost")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/switch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleWorkspaceSwitcherRendersList(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"staging": "description: stage\nvars: {}\n",
		"prod":    "description: live\nvars: {}\n",
	})

	req := httptest.NewRequest(http.MethodGet, "/workspaces/switcher", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"staging", "prod", "Manage workspaces"} {
		if !strings.Contains(body, want) {
			t.Errorf("switcher missing %q; body=%s", want, body)
		}
	}
}

func TestHandleWorkspaceDeleteRemovesFile(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"toremove": "vars: {}\n",
	})

	req := httptest.NewRequest(http.MethodDelete, "/workspaces/toremove", nil)
	req.SetPathValue("name", "toremove")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	path := filepath.Join(filepath.Dir(cfgPath), ".factorly", "workspaces", "toremove.yaml")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("workspace file not removed: %v", err)
	}
}

func TestHandleWorkspaceUnlockWithoutPasswordOpener(t *testing.T) {
	srv, cfgPath := testServerWithProxy(t, nil)
	seedWorkspaces(t, cfgPath, map[string]string{
		"locked": "vars: {}\n",
	})

	form := url.Values{}
	form.Set("name", "locked")
	form.Set("password", "secret")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/unlock", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	// No password opener configured → returns the unlock partial with an error.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unlock is not configured") {
		t.Errorf("expected helpful error; body=%s", rec.Body.String())
	}
}
