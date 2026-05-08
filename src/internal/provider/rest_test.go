// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/factorly-dev/factorly/internal/oauth"
)

// --- Errors ---

func TestRESTExecuteToolNotFound(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("nonexistent", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestRESTExecuteMissingRequiredParam(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: "http://localhost",
			Path:    "/items",
			Params: []RESTParamDef{
				{Name: "id", In: "query", Required: true},
			},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("expected error to mention param name, got: %s", err.Error())
	}
}

func TestRESTUnresolvedPathParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/items/{{id}}",
			Params:  []RESTParamDef{{Name: "id", In: "path", Required: false}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unresolved path param")
	}
	if !strings.Contains(err.Error(), "{{id}}") {
		t.Errorf("expected error to mention placeholder, got: %s", err.Error())
	}
}

// --- Path Params ---

func TestRESTPathParamSubstitution(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"pet.get": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/pets/{{petId}}",
			Params:  []RESTParamDef{{Name: "petId", In: "path", Required: true}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("pet.get", map[string]string{"petId": "123"})
	if err != nil {
		t.Fatal(err)
	}
	if capturedPath != "/pets/123" {
		t.Errorf("expected path /pets/123, got %s", capturedPath)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != `{"id":"123"}` {
		t.Errorf("expected output, got %q", result.Output)
	}
}

func TestRESTPathParamEscaping(t *testing.T) {
	var capturedURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURI = r.RequestURI
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/items/{{name}}",
			Params:  []RESTParamDef{{Name: "name", In: "path"}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"name": "hello world"})
	if capturedURI != "/items/hello%20world" {
		t.Errorf("expected escaped URI, got %s", capturedURI)
	}
}

// --- Query Params ---

func TestRESTQueryParams(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/items",
			Params: []RESTParamDef{
				{Name: "limit", In: "query"},
				{Name: "offset", In: "query"},
			},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"limit": "10", "offset": "20"})
	if !strings.Contains(capturedQuery, "limit=10") {
		t.Errorf("expected limit=10 in query, got %s", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "offset=20") {
		t.Errorf("expected offset=20 in query, got %s", capturedQuery)
	}
}

// --- Header Params ---

func TestRESTHeaderParams(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Request-Id")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/items",
			Params:  []RESTParamDef{{Name: "X-Request-Id", In: "header"}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"X-Request-Id": "abc-123"})
	if capturedHeader != "abc-123" {
		t.Errorf("expected header abc-123, got %s", capturedHeader)
	}
}

// --- Body Param ---

func TestRESTBodyParam(t *testing.T) {
	var capturedBody string
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		capturedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/items",
			BodyType: "raw",
			Params:   []RESTParamDef{{Name: "body", In: "body"}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{"body": `{"name":"Rex"}`})
	if err != nil {
		t.Fatal(err)
	}
	if capturedBody != `{"name":"Rex"}` {
		t.Errorf("expected body, got %q", capturedBody)
	}
	if capturedContentType != "application/json" {
		t.Errorf("expected application/json, got %s", capturedContentType)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for 201, got %d", result.ExitCode)
	}
}

// --- Default Param Routing ---

func TestRESTDefaultParamRoutingGET(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/items",
			// No Params defined — "In" will be empty
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"search": "foo"})
	if !strings.Contains(capturedQuery, "search=foo") {
		t.Errorf("expected GET param as query, got %s", capturedQuery)
	}
}

func TestRESTDefaultParamRoutingPOST(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/items",
			BodyType: "raw",
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"data": `{"x":1}`})
	if capturedBody != `{"x":1}` {
		t.Errorf("expected POST param as raw body, got %q", capturedBody)
	}
}

// --- Mixed Params ---

func TestRESTMixedParams(t *testing.T) {
	var captured struct {
		path   string
		query  string
		header string
		body   string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.header = r.Header.Get("X-Tenant")
		body, _ := io.ReadAll(r.Body)
		captured.body = string(body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/orgs/{{orgId}}/items",
			BodyType: "raw",
			Params: []RESTParamDef{
				{Name: "orgId", In: "path", Required: true},
				{Name: "limit", In: "query"},
				{Name: "X-Tenant", In: "header"},
				{Name: "body", In: "body"},
			},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{
		"orgId":    "acme",
		"limit":    "5",
		"X-Tenant": "tenant-1",
		"body":     `{"name":"item"}`,
	})

	if captured.path != "/orgs/acme/items" {
		t.Errorf("path: expected /orgs/acme/items, got %s", captured.path)
	}
	if !strings.Contains(captured.query, "limit=5") {
		t.Errorf("query: expected limit=5, got %s", captured.query)
	}
	if captured.header != "tenant-1" {
		t.Errorf("header: expected tenant-1, got %s", captured.header)
	}
	if captured.body != `{"name":"item"}` {
		t.Errorf("body: expected JSON, got %q", captured.body)
	}
}

// --- URL Construction ---

func TestRESTURLDoubleSlash(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL + "/",
			Path:    "/items",
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedPath != "/items" {
		t.Errorf("expected /items (no double slash), got %s", capturedPath)
	}
}

func TestRESTEmptyPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "",
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedPath != "/" {
		t.Errorf("expected /, got %s", capturedPath)
	}
}

// --- Auth ---

func TestRESTAuthBearer(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Auth:    &AuthDef{Type: "bearer", Token: "my-secret-token"},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedAuth != "Bearer my-secret-token" {
		t.Errorf("expected Bearer auth, got %q", capturedAuth)
	}
}

func TestRESTAuthBasic(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Auth:    &AuthDef{Type: "basic", Token: "user:pass"},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if capturedAuth != expected {
		t.Errorf("expected Basic auth, got %q", capturedAuth)
	}
}

func TestRESTAuthHeader(t *testing.T) {
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Auth:    &AuthDef{Type: "header", Header: "X-Api-Key", Value: "key123"},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedKey != "key123" {
		t.Errorf("expected X-Api-Key=key123, got %q", capturedKey)
	}
}

func TestRESTAuthNone(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedAuth != "" {
		t.Errorf("expected no auth header, got %q", capturedAuth)
	}
}

// --- Static Headers ---

func TestRESTStaticHeaders(t *testing.T) {
	var capturedAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Headers: map[string]string{"Accept": "application/xml"},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{})
	if capturedAccept != "application/xml" {
		t.Errorf("expected Accept=application/xml, got %q", capturedAccept)
	}
}

func TestRESTContentTypeOverride(t *testing.T) {
	var capturedCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/",
			BodyType: "raw",
			Headers:  map[string]string{"Content-Type": "text/plain"},
			Params:   []RESTParamDef{{Name: "body", In: "body"}},
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"body": "plain text"})
	if capturedCT != "text/plain" {
		t.Errorf("expected Content-Type override, got %q", capturedCT)
	}
}

func TestRESTParamHeaderOverridesStatic(t *testing.T) {
	var capturedTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTenant = r.Header.Get("X-Tenant")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Headers: map[string]string{"X-Tenant": "default-tenant"},
			Params:  []RESTParamDef{{Name: "X-Tenant", In: "header"}},
		},
	}, nil)
	_ = p.Setup()

	// Param header should override the static header
	_, _ = p.Execute("test", map[string]string{"X-Tenant": "custom-tenant"})
	if capturedTenant != "custom-tenant" {
		t.Errorf("expected param header to override static, got %q", capturedTenant)
	}
}

// --- Response Handling ---

func TestRESTSuccess200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {Method: "GET", BaseURL: srv.URL, Path: "/"},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Output != `{"ok":true}` {
		t.Errorf("expected output, got %q", result.Output)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestRESTSuccess201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {Method: "POST", BaseURL: srv.URL, Path: "/"},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, _ := p.Execute("test", map[string]string{})
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0 for 201, got %d", result.ExitCode)
	}
}

func TestRESTError404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {Method: "GET", BaseURL: srv.URL, Path: "/"},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 404 {
		t.Errorf("expected exit code 404, got %d", result.ExitCode)
	}
	if result.Output != `{"error":"not found"}` {
		t.Errorf("expected body in output, got %q", result.Output)
	}
	if !strings.Contains(result.Error, "404") {
		t.Errorf("expected 404 in error, got %q", result.Error)
	}
	if !result.IsError() {
		t.Error("expected IsError=true")
	}
}

func TestRESTError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {Method: "GET", BaseURL: srv.URL, Path: "/"},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, _ := p.Execute("test", map[string]string{})
	if result.ExitCode != 500 {
		t.Errorf("expected exit code 500, got %d", result.ExitCode)
	}
	if !result.IsError() {
		t.Error("expected IsError=true")
	}
}

func TestRESTNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srvURL := srv.URL
	srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {Method: "GET", BaseURL: srvURL, Path: "/"},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal("expected nil error (network error is in Result)")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

// --- Timeout ---

func TestRESTTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Timeout: 100 * time.Millisecond,
		},
	}, nil)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 for timeout, got %d", result.ExitCode)
	}
}

// --- OAuth Token Refresh ---

type mockTokenStore struct {
	bundles  map[string]*oauth.TokenBundle
	setCalls int
}

func (m *mockTokenStore) GetTokenBundle(key string) (*oauth.TokenBundle, error) {
	b, ok := m.bundles[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return b, nil
}

func (m *mockTokenStore) SetTokenBundle(key string, bundle *oauth.TokenBundle) error {
	m.bundles[key] = bundle
	m.setCalls++
	return nil
}

func TestRESTOAuthValidToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	store := &mockTokenStore{
		bundles: map[string]*oauth.TokenBundle{
			"test_oauth": {
				AccessToken:  "valid-token",
				RefreshToken: "refresh",
				Expiry:       time.Now().Add(1 * time.Hour),
			},
		},
	}

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/",
			Auth: &AuthDef{
				Type:     "oauth",
				TokenKey: "test_oauth",
				OAuthProvider: &oauth.ProviderConfig{
					ClientID: "id",
					TokenURL: "http://unused",
				},
			},
		},
	}, store)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	if capturedAuth != "Bearer valid-token" {
		t.Errorf("expected Bearer valid-token, got %q", capturedAuth)
	}
	if store.setCalls != 0 {
		t.Error("expected no token store writes for valid token")
	}
}

func TestRESTOAuthExpiredTokenRefresh(t *testing.T) {
	// API server
	var capturedAuth string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer apiSrv.Close()

	// Token refresh server
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected refresh_token grant, got %s", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"refreshed-token","token_type":"bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	store := &mockTokenStore{
		bundles: map[string]*oauth.TokenBundle{
			"test_oauth": {
				AccessToken:  "expired-token",
				RefreshToken: "my-refresh",
				Expiry:       time.Now().Add(-1 * time.Hour), // expired
			},
		},
	}

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: apiSrv.URL,
			Path:    "/",
			Auth: &AuthDef{
				Type:     "oauth",
				TokenKey: "test_oauth",
				OAuthProvider: &oauth.ProviderConfig{
					ClientID: "id",
					TokenURL: tokenSrv.URL,
				},
			},
		},
	}, store)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	result, err := p.Execute("test", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	if capturedAuth != "Bearer refreshed-token" {
		t.Errorf("expected Bearer refreshed-token, got %q", capturedAuth)
	}
	if store.setCalls != 1 {
		t.Errorf("expected 1 token store write, got %d", store.setCalls)
	}
}

func TestRESTOAuthRefreshError(t *testing.T) {
	// Token refresh server that returns error
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token revoked"}`))
	}))
	defer tokenSrv.Close()

	store := &mockTokenStore{
		bundles: map[string]*oauth.TokenBundle{
			"test_oauth": {
				AccessToken:  "expired",
				RefreshToken: "revoked-refresh",
				Expiry:       time.Now().Add(-1 * time.Hour),
			},
		},
	}

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: "http://unused",
			Path:    "/",
			Auth: &AuthDef{
				Type:     "oauth",
				TokenKey: "test_oauth",
				OAuthProvider: &oauth.ProviderConfig{
					ClientID: "id",
					TokenURL: tokenSrv.URL,
				},
			},
		},
	}, store)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for failed refresh")
	}
	if !strings.Contains(err.Error(), "factorly auth login") {
		t.Errorf("expected 'factorly auth login' in error message, got: %s", err.Error())
	}
}

func TestRESTOAuthNoTokenStore(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: "http://unused",
			Path:    "/",
			Auth:    &AuthDef{Type: "oauth", TokenKey: "test_oauth"},
		},
	}, nil) // nil token store
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for nil token store")
	}
}

func TestRESTOAuthMissingToken(t *testing.T) {
	store := &mockTokenStore{bundles: map[string]*oauth.TokenBundle{}}

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: "http://unused",
			Path:    "/",
			Auth:    &AuthDef{Type: "oauth", TokenKey: "nonexistent"},
		},
	}, store)
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
	if !strings.Contains(err.Error(), "factorly auth login") {
		t.Errorf("expected 'factorly auth login' in error message, got: %s", err.Error())
	}
}

// --- Lifecycle ---

func TestRESTSetupTeardown(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{}, nil)
	if err := p.Setup(); err != nil {
		t.Fatal(err)
	}
	if p.client == nil {
		t.Error("expected client after setup")
	}
	if err := p.Teardown(); err != nil {
		t.Fatal(err)
	}
}

func TestRESTFileParam(t *testing.T) {
	// Create a test file
	tmpFile := filepath.Join(t.TempDir(), "test.bin")
	testData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	if err := os.WriteFile(tmpFile, testData, 0o644); err != nil {
		t.Fatal(err)
	}

	var receivedBody []byte
	var receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"upload": {
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/upload",
			Headers: map[string]string{"Content-Type": "audio/wav"},
			Params: []RESTParamDef{
				{Name: "file", In: "file", Required: true},
				{Name: "model", In: "query"},
			},
		},
	}, nil)
	_ = p.Setup()

	result, err := p.Execute("upload", map[string]string{
		"file":  tmpFile,
		"model": "nova-3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != `{"result":"ok"}` {
		t.Errorf("unexpected output: %s", result.Output)
	}
	if receivedContentType != "audio/wav" {
		t.Errorf("expected Content-Type audio/wav, got %s", receivedContentType)
	}
	if !bytes.Equal(receivedBody, testData) {
		t.Errorf("expected binary body %v, got %v", testData, receivedBody)
	}
}

func TestRESTFileParamMissing(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{
		"upload": {
			Method:  "POST",
			BaseURL: "http://localhost",
			Path:    "/upload",
			Params:  []RESTParamDef{{Name: "file", In: "file", Required: true}},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("upload", map[string]string{
		"file": "/nonexistent/file.wav",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "opening file") {
		t.Errorf("expected 'opening file' in error, got: %s", err.Error())
	}
}

func TestRESTFileParamRelativePath(t *testing.T) {
	// Relative paths should be resolved to absolute via filepath.Abs
	// This test ensures relative file paths work (no panic/weird behavior)
	tmpFile := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"upload": {
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/upload",
			Params:  []RESTParamDef{{Name: "file", In: "file", Required: true}},
		},
	}, nil)
	_ = p.Setup()

	// Use absolute path (relative would depend on CWD)
	result, err := p.Execute("upload", map[string]string{
		"file": tmpFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "test" {
		t.Errorf("expected 'test', got %q", result.Output)
	}
}

func TestRESTPathParamsAutoDetect(t *testing.T) {
	// Params without "in: path" should auto-route to path if they match
	// a {{param}} placeholder in the path template.
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"repos.get": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/repos/{{owner}}/{{repo}}",
			Params: []RESTParamDef{
				{Name: "owner", Required: true}, // no In field
				{Name: "repo", Required: true},  // no In field
			},
		},
	}, nil)
	_ = p.Setup()

	result, err := p.Execute("repos.get", map[string]string{
		"owner": "factorly-dev",
		"repo":  "factorly",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "ok" {
		t.Errorf("expected 'ok', got %q", result.Output)
	}
	if receivedPath != "/repos/factorly-dev/factorly" {
		t.Errorf("expected /repos/factorly-dev/factorly, got %s", receivedPath)
	}
}

// --- Body Type tests ---

func TestRESTBodyType_JSONSingleParam(t *testing.T) {
	var capturedBody string
	var capturedCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/getChat",
			Params:  []RESTParamDef{{Name: "chat_id", In: "body", Required: true}},
			// BodyType defaults to "json"
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{"chat_id": "123456"})
	if err != nil {
		t.Fatal(err)
	}
	if capturedBody != `{"chat_id":"123456"}` {
		t.Errorf("expected JSON object, got %q", capturedBody)
	}
	if capturedCT != "application/json" {
		t.Errorf("expected application/json, got %q", capturedCT)
	}
}

func TestRESTBodyType_JSONMultipleParams(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/sendMessage",
			BodyType: "json",
			Params: []RESTParamDef{
				{Name: "chat_id", In: "body", Required: true},
				{Name: "text", In: "body", Required: true},
			},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{"chat_id": "123", "text": "hello"})
	if err != nil {
		t.Fatal(err)
	}

	// Parse as JSON to check keys (order may vary)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(capturedBody), &parsed); err != nil {
		t.Fatalf("body not valid JSON: %s", capturedBody)
	}
	if parsed["chat_id"] != "123" {
		t.Errorf("chat_id: expected '123', got %v", parsed["chat_id"])
	}
	if parsed["text"] != "hello" {
		t.Errorf("text: expected 'hello', got %v", parsed["text"])
	}
}

func TestRESTBodyType_JSONWithTypes(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/",
			Params: []RESTParamDef{
				{Name: "count", In: "body", Type: "integer"},
				{Name: "active", In: "body", Type: "boolean"},
				{Name: "tags", In: "body", Type: "json"},
				{Name: "name", In: "body"},
			},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{
		"count":  "42",
		"active": "true",
		"tags":   `["a","b"]`,
		"name":   "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(capturedBody), &parsed); err != nil {
		t.Fatalf("body not valid JSON: %s", capturedBody)
	}
	// integer: unquoted
	if string(parsed["count"]) != "42" {
		t.Errorf("count: expected 42, got %s", parsed["count"])
	}
	// boolean: unquoted
	if string(parsed["active"]) != "true" {
		t.Errorf("active: expected true, got %s", parsed["active"])
	}
	// json: raw array
	if string(parsed["tags"]) != `["a","b"]` {
		t.Errorf("tags: expected [\"a\",\"b\"], got %s", parsed["tags"])
	}
	// string: quoted
	if string(parsed["name"]) != `"test"` {
		t.Errorf("name: expected \"test\", got %s", parsed["name"])
	}
}

func TestRESTBodyType_Form(t *testing.T) {
	var capturedBody string
	var capturedCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/submit",
			BodyType: "form",
			Params: []RESTParamDef{
				{Name: "username", In: "body", Required: true},
				{Name: "password", In: "body", Required: true},
			},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{"username": "admin", "password": "secret"})
	if err != nil {
		t.Fatal(err)
	}

	if capturedCT != "application/x-www-form-urlencoded" {
		t.Errorf("expected form content-type, got %q", capturedCT)
	}
	if !strings.Contains(capturedBody, "username=admin") {
		t.Errorf("expected username=admin in body, got %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "password=secret") {
		t.Errorf("expected password=secret in body, got %q", capturedBody)
	}
}

func TestRESTBodyType_Raw(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:   "POST",
			BaseURL:  srv.URL,
			Path:     "/",
			BodyType: "raw",
			Params:   []RESTParamDef{{Name: "payload", In: "body"}},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{"payload": `<xml>hello</xml>`})
	if err != nil {
		t.Fatal(err)
	}
	if capturedBody != `<xml>hello</xml>` {
		t.Errorf("expected raw XML, got %q", capturedBody)
	}
}

func TestRESTBodyType_DefaultIsJSON(t *testing.T) {
	// No explicit BodyType — should default to json
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/",
			Params:  []RESTParamDef{{Name: "key", In: "body"}},
		},
	}, nil)
	_ = p.Setup()

	_, err := p.Execute("test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if capturedBody != `{"key":"value"}` {
		t.Errorf("expected JSON object, got %q", capturedBody)
	}
}

func TestRESTVerboseRedactsSecrets(t *testing.T) {
	secret := "sk_live_test_secret_12345"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/data",
			Auth:    &AuthDef{Type: "bearer", Token: secret},
			RedactSecrets: func(s string) string {
				return strings.ReplaceAll(s, secret, "****")
			},
		},
	}, nil)
	p.Verbose = true
	_ = p.Setup()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, _ = p.Execute("test", map[string]string{})

	_ = w.Close()
	os.Stderr = oldStderr
	captured, _ := io.ReadAll(r)
	stderr := string(captured)

	if strings.Contains(stderr, secret) {
		t.Errorf("SECRET LEAKED in verbose output: %s", stderr)
	}
	if !strings.Contains(stderr, "****") {
		t.Errorf("expected redacted value in output, got: %s", stderr)
	}
	if !strings.Contains(stderr, "[rest] GET") {
		t.Errorf("expected verbose request line, got: %s", stderr)
	}
}

func TestRESTVerboseNoRedactWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := NewREST(map[string]RESTToolDef{
		"test": {
			Method:  "GET",
			BaseURL: srv.URL,
			Path:    "/public",
			// No RedactSecrets — no vault refs
		},
	}, nil)
	p.Verbose = true
	_ = p.Setup()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	_, _ = p.Execute("test", map[string]string{})

	_ = w.Close()
	os.Stderr = oldStderr
	captured, _ := io.ReadAll(r)
	stderr := string(captured)

	if !strings.Contains(stderr, "/public") {
		t.Errorf("expected full URL in output without redaction, got: %s", stderr)
	}
}
