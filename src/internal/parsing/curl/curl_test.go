// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package curl

import (
	"testing"
)

func TestParseBasicGET(t *testing.T) {
	p, err := Parse("curl https://api.example.com/users")
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "GET" {
		t.Errorf("expected GET, got %s", p.Method)
	}
	if p.URL != "https://api.example.com/users" {
		t.Errorf("expected URL, got %s", p.URL)
	}
}

func TestParseExplicitPOST(t *testing.T) {
	p, err := Parse(`curl -X POST https://api.stripe.com/v1/charges -H "Authorization: Bearer sk_test_xxx" -d '{"amount":2000}'`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "POST" {
		t.Errorf("expected POST, got %s", p.Method)
	}
	if p.Headers["Authorization"] != "Bearer sk_test_xxx" {
		t.Errorf("expected Bearer header, got %q", p.Headers["Authorization"])
	}
	if p.Body != `{"amount":2000}` {
		t.Errorf("expected JSON body, got %q", p.Body)
	}
}

func TestParseBasicAuth(t *testing.T) {
	p, err := Parse("curl -u user:password https://api.example.com/data")
	if err != nil {
		t.Fatal(err)
	}
	if p.BasicAuth != "user:password" {
		t.Errorf("expected basic auth, got %q", p.BasicAuth)
	}
}

func TestParseAPIKeyHeader(t *testing.T) {
	p, err := Parse(`curl -H "X-Api-Key: mykey123" https://api.example.com/items`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Headers["X-Api-Key"] != "mykey123" {
		t.Errorf("expected API key header, got %q", p.Headers["X-Api-Key"])
	}
}

func TestParseQueryParams(t *testing.T) {
	p, err := Parse(`curl "https://api.example.com/search?q=test&limit=10"`)
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://api.example.com/search?q=test&limit=10" {
		t.Errorf("expected URL with query, got %s", p.URL)
	}
}

func TestParseFormData(t *testing.T) {
	p, err := Parse(`curl -X POST -F "name=John" -F "email=john@test.com" https://api.example.com/users`)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsFormData {
		t.Error("expected IsFormData=true")
	}
	if p.FormFields["name"] != "John" {
		t.Errorf("expected name=John, got %q", p.FormFields["name"])
	}
	if p.FormFields["email"] != "john@test.com" {
		t.Errorf("expected email, got %q", p.FormFields["email"])
	}
}

func TestParseDataRaw(t *testing.T) {
	p, err := Parse(`curl -X PUT --data-raw '{"name":"updated"}' https://api.example.com/items/123`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "PUT" {
		t.Errorf("expected PUT, got %s", p.Method)
	}
	if p.Body != `{"name":"updated"}` {
		t.Errorf("expected body, got %q", p.Body)
	}
}

func TestParseShorthandMethod(t *testing.T) {
	p, err := Parse(`curl -XPOST -H "Authorization: Bearer tok" https://api.example.com/data`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "POST" {
		t.Errorf("expected POST from -XPOST, got %s", p.Method)
	}
}

func TestParseMultiLine(t *testing.T) {
	input := "curl -X POST https://api.example.com/data \\\n  -H \"Authorization: Bearer tok\" \\\n  -d '{\"key\":\"value\"}'"
	p, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != "POST" {
		t.Errorf("expected POST, got %s", p.Method)
	}
	if p.Body != `{"key":"value"}` {
		t.Errorf("expected body, got %q", p.Body)
	}
}

func TestParseMultipleDFlags(t *testing.T) {
	p, err := Parse(`curl -d "a=1" -d "b=2" https://api.example.com/data`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Body != "a=1&b=2" {
		t.Errorf("expected concatenated body, got %q", p.Body)
	}
	if p.Method != "POST" {
		t.Errorf("expected POST (body present), got %s", p.Method)
	}
}

func TestParseIgnoredFlags(t *testing.T) {
	p, err := Parse("curl -s -L -k --compressed https://api.example.com/data")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://api.example.com/data" {
		t.Errorf("expected URL, got %s", p.URL)
	}
}

func TestParseDollarPrefix(t *testing.T) {
	p, err := Parse("$ curl https://api.example.com/data")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://api.example.com/data" {
		t.Errorf("expected URL, got %s", p.URL)
	}
}

func TestParseMultipleHeaders(t *testing.T) {
	p, err := Parse(`curl -H "Accept: application/json" -H "X-Request-Id: abc" https://api.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Headers["Accept"] != "application/json" {
		t.Errorf("expected Accept header, got %q", p.Headers["Accept"])
	}
	if p.Headers["X-Request-Id"] != "abc" {
		t.Errorf("expected X-Request-Id header, got %q", p.Headers["X-Request-Id"])
	}
}

func TestParseNoURL(t *testing.T) {
	_, err := Parse("curl -X POST -H 'test: value'")
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestParseEmpty(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

// --- Auth Detection ---

func TestDetectAuthBearer(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer sk_test_xxx"}
	auth := DetectAuth(headers, "")
	if auth == nil {
		t.Fatal("expected auth detection")
	}
	if auth.Type != "bearer" {
		t.Errorf("expected bearer, got %s", auth.Type)
	}
	if auth.RawValue != "sk_test_xxx" {
		t.Errorf("expected token, got %s", auth.RawValue)
	}
}

func TestDetectAuthBasicHeader(t *testing.T) {
	headers := map[string]string{"Authorization": "Basic dXNlcjpwYXNz"}
	auth := DetectAuth(headers, "")
	if auth == nil {
		t.Fatal("expected auth detection")
	}
	if auth.Type != "basic" {
		t.Errorf("expected basic, got %s", auth.Type)
	}
}

func TestDetectAuthBasicFlag(t *testing.T) {
	auth := DetectAuth(map[string]string{}, "user:pass")
	if auth == nil {
		t.Fatal("expected auth detection")
	}
	if auth.Type != "basic" {
		t.Errorf("expected basic, got %s", auth.Type)
	}
	if auth.RawValue != "user:pass" {
		t.Errorf("expected user:pass, got %s", auth.RawValue)
	}
}

func TestDetectAuthAPIKey(t *testing.T) {
	headers := map[string]string{"X-Api-Key": "mykey123"}
	auth := DetectAuth(headers, "")
	if auth == nil {
		t.Fatal("expected auth detection")
	}
	if auth.Type != "header" {
		t.Errorf("expected header, got %s", auth.Type)
	}
	if auth.HeaderName != "X-Api-Key" {
		t.Errorf("expected X-Api-Key, got %s", auth.HeaderName)
	}
}

func TestDetectAuthNone(t *testing.T) {
	auth := DetectAuth(map[string]string{"Accept": "application/json"}, "")
	if auth != nil {
		t.Error("expected no auth detection")
	}
}

// --- Tool Naming ---

func TestDeriveToolName(t *testing.T) {
	tests := []struct {
		url    string
		method string
		want   string
	}{
		{"https://api.stripe.com/v1/charges", "POST", "stripe.post_v1_charges"},
		{"https://api.example.com/users", "GET", "example.get_users"},
		{"https://httpbin.org/get", "GET", "httpbin.get_get"},
		{"https://api.github.com/users/octocat/repos", "GET", "github.get_users_octocat_repos"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := DeriveToolName(tt.url, tt.method)
			if got != tt.want {
				t.Errorf("DeriveToolName(%q, %q) = %q, want %q", tt.url, tt.method, got, tt.want)
			}
		})
	}
}

// --- Path Parameterization ---

func TestParameterizePath(t *testing.T) {
	tests := []struct {
		path     string
		wantPath string
		wantLen  int
	}{
		{"/users/123", "/users/{{user_id}}", 1},
		{"/v1/charges", "/v1/charges", 0},
		{"/items/550e8400-e29b-41d4-a716-446655440000", "/items/{{item_id}}", 1},
		{"/api/v1/users", "/api/v1/users", 0},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotPath, gotParams := ParameterizePath(tt.path)
			if gotPath != tt.wantPath {
				t.Errorf("path: got %q, want %q", gotPath, tt.wantPath)
			}
			if len(gotParams) != tt.wantLen {
				t.Errorf("params: got %d, want %d", len(gotParams), tt.wantLen)
			}
		})
	}
}

// --- ToToolConfig ---

func TestToToolConfigBasic(t *testing.T) {
	parsed := &ParsedCurl{
		Method:  "GET",
		URL:     "https://api.github.com/users?per_page=10",
		Headers: map[string]string{},
	}
	tool, auth := ToToolConfig(parsed)
	if tool.Type != "rest" {
		t.Errorf("expected rest, got %s", tool.Type)
	}
	if tool.BaseURL != "https://api.github.com" {
		t.Errorf("expected base URL, got %s", tool.BaseURL)
	}
	if tool.Path != "/users" {
		t.Errorf("expected path, got %s", tool.Path)
	}
	if tool.Method != "GET" {
		t.Errorf("expected GET, got %s", tool.Method)
	}
	if auth != nil {
		t.Error("expected no auth")
	}
	// Should have per_page query param
	found := false
	for _, p := range tool.Parameters {
		if p.Name == "per_page" && p.In == "query" {
			found = true
		}
	}
	if !found {
		t.Error("expected per_page query param")
	}
}

func TestToToolConfigWithBearer(t *testing.T) {
	parsed := &ParsedCurl{
		Method:  "POST",
		URL:     "https://api.stripe.com/v1/charges",
		Headers: map[string]string{"Authorization": "Bearer sk_test_xxx"},
		Body:    `{"amount":2000,"currency":"usd"}`,
	}
	tool, auth := ToToolConfig(parsed)
	if auth == nil {
		t.Fatal("expected auth detection")
	}
	if auth.Type != "bearer" {
		t.Errorf("expected bearer, got %s", auth.Type)
	}
	if tool.Auth == nil {
		t.Fatal("expected auth on tool")
	}
	if tool.Auth.Type != "bearer" {
		t.Errorf("expected bearer auth, got %s", tool.Auth.Type)
	}
	// Should have body params
	if len(tool.Parameters) < 2 {
		t.Errorf("expected at least 2 body params, got %d", len(tool.Parameters))
	}
}

// --- Shell Split ---

func TestShellSplit(t *testing.T) {
	tests := []struct {
		input string
		want  int // number of tokens
	}{
		{`-H "Authorization: Bearer tok" https://api.com`, 3},
		{`-d '{"key":"value"}' https://api.com`, 3},
		{`-X POST -H "Content-Type: application/json"`, 4},
		{`simple tokens here`, 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := shellSplit(tt.input)
			if len(tokens) != tt.want {
				t.Errorf("shellSplit(%q) = %d tokens %v, want %d", tt.input, len(tokens), tokens, tt.want)
			}
		})
	}
}

// --- Body Params ---

func TestParseBodyParamsJSON(t *testing.T) {
	params := parseBodyParams(`{"name":"John","age":30}`)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
		if p.In != "body" {
			t.Errorf("expected in=body, got %s", p.In)
		}
	}
	if !names["name"] || !names["age"] {
		t.Error("expected name and age params")
	}
}

func TestParseBodyParamsForm(t *testing.T) {
	params := parseBodyParams("amount=2000&currency=usd")
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
}

func TestParseBodyParamsRaw(t *testing.T) {
	params := parseBodyParams("just some raw text")
	if len(params) != 1 {
		t.Fatalf("expected 1 fallback param, got %d", len(params))
	}
	if params[0].Name != "body" {
		t.Errorf("expected body param, got %s", params[0].Name)
	}
}
