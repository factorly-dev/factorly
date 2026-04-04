package provider

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Errors ---

func TestRESTExecuteToolNotFound(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{})
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
	})
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
			Path:    "/items/{id}",
			Params:  []RESTParamDef{{Name: "id", In: "path", Required: false}},
		},
	})
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, err := p.Execute("test", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unresolved path param")
	}
	if !strings.Contains(err.Error(), "{id}") {
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
			Path:    "/pets/{petId}",
			Params:  []RESTParamDef{{Name: "petId", In: "path", Required: true}},
		},
	})
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
			Path:    "/items/{name}",
			Params:  []RESTParamDef{{Name: "name", In: "path"}},
		},
	})
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
	})
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
	})
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
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/items",
			Params:  []RESTParamDef{{Name: "body", In: "body"}},
		},
	})
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
	})
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
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/items",
		},
	})
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"data": `{"x":1}`})
	if capturedBody != `{"x":1}` {
		t.Errorf("expected POST param as body, got %q", capturedBody)
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
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/orgs/{orgId}/items",
			Params: []RESTParamDef{
				{Name: "orgId", In: "path", Required: true},
				{Name: "limit", In: "query"},
				{Name: "X-Tenant", In: "header"},
				{Name: "body", In: "body"},
			},
		},
	})
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
	})
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
	})
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
	})
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
	})
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
	})
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
	})
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
	})
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
			Method:  "POST",
			BaseURL: srv.URL,
			Path:    "/",
			Headers: map[string]string{"Content-Type": "text/plain"},
			Params:  []RESTParamDef{{Name: "body", In: "body"}},
		},
	})
	_ = p.Setup()
	defer func() { _ = p.Teardown() }()

	_, _ = p.Execute("test", map[string]string{"body": "plain text"})
	if capturedCT != "text/plain" {
		t.Errorf("expected Content-Type override, got %q", capturedCT)
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
	})
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
	})
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
	})
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
	})
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
	})
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
	})
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

// --- Lifecycle ---

func TestRESTSetupTeardown(t *testing.T) {
	p := NewREST(map[string]RESTToolDef{})
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
