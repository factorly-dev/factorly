// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/factorly-dev/factorly/internal/oauth"
)

type RESTParamDef struct {
	Name     string
	In       string // "query", "path", "header", "body"
	Required bool
	Type     string // "string" (default), "integer", "number", "boolean", "json"
}

// TokenStore reads and writes OAuth token bundles from the vault.
type TokenStore interface {
	GetTokenBundle(key string) (*oauth.TokenBundle, error)
	SetTokenBundle(key string, bundle *oauth.TokenBundle) error
}

type AuthDef struct {
	Type   string // "bearer", "basic", "header", "oauth"
	Token  string
	Header string
	Value  string

	// OAuth fields (only when Type == "oauth")
	OAuthProvider *oauth.ProviderConfig
	TokenKey      string
}

type RESTToolDef struct {
	Method  string
	BaseURL string
	Path    string
	Body    string // JSON body template with {{param}} placeholders
	Headers map[string]string
	Auth    *AuthDef
	Params  []RESTParamDef
	Timeout time.Duration
}

type RESTProvider struct {
	tools      map[string]RESTToolDef
	client     *http.Client
	tokenStore TokenStore // nil when no OAuth tools exist
	mu         sync.Mutex // protects concurrent token refresh
	Verbose    bool
}

func NewREST(tools map[string]RESTToolDef, tokenStore TokenStore) *RESTProvider {
	return &RESTProvider{tools: tools, tokenStore: tokenStore}
}

func (p *RESTProvider) Setup() error {
	p.client = &http.Client{}
	return nil
}

// AddTool registers a new tool definition at runtime.
func (p *RESTProvider) AddTool(name string, def RESTToolDef) {
	p.tools[name] = def
}

// RemoveTool removes a tool definition at runtime.
func (p *RESTProvider) RemoveTool(name string) {
	delete(p.tools, name)
}

func (p *RESTProvider) Teardown() error {
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	return nil
}

func (p *RESTProvider) Execute(toolName string, params map[string]string) (*Result, error) {
	def, ok := p.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("rest provider: tool %q not registered", toolName)
	}

	// Validate required parameters
	for _, pd := range def.Params {
		if pd.Required {
			if _, ok := params[pd.Name]; !ok {
				return nil, fmt.Errorf("rest provider: required parameter %q missing for tool %q", pd.Name, toolName)
			}
		}
	}

	// Build param index for routing and types
	paramIndex := make(map[string]string)
	paramType := make(map[string]string)
	for _, pd := range def.Params {
		paramIndex[pd.Name] = pd.In
		paramType[pd.Name] = pd.Type
	}

	// Route parameters
	pathParams := make(map[string]string)
	queryParams := make(map[string]string)
	headerParams := make(map[string]string)
	bodyParams := make(map[string]string)
	var fileParam string // path to file for in:file

	// Pre-scan path for {{param}} placeholders so we can auto-route
	// params that match, even if they lack an explicit "in: path".
	pathPlaceholders := make(map[string]bool)
	if strings.Contains(def.Path, "{{") {
		for _, m := range regexp.MustCompile(`\{\{(\w+)\}\}`).FindAllStringSubmatch(def.Path, -1) {
			pathPlaceholders[m[1]] = true
		}
	}

	for name, value := range params {
		in := paramIndex[name]
		if in == "" {
			if pathPlaceholders[name] {
				in = "path"
			} else {
				in = defaultParamLocation(def.Method)
			}
		}
		switch in {
		case "path":
			pathParams[name] = value
		case "query":
			queryParams[name] = value
		case "header":
			headerParams[name] = value
		case "body":
			bodyParams[name] = value
		case "file":
			fileParam = value
		default:
			queryParams[name] = value
		}
	}

	// Build body content
	var bodyContent string
	if def.Body != "" {
		// Body template: substitute {{param}} and {{param|default}} placeholders.
		// String values are JSON-escaped (newlines, quotes, etc.) since they're
		// inserted inside JSON string literals. Values with type "json" or numeric
		// types are inserted as-is.
		bodyContent = def.Body
		for k, v := range params {
			escaped := v
			switch paramType[k] {
			case "json", "integer", "number", "boolean":
				// Pass through as-is
			default:
				// JSON-escape: marshal produces "quoted string", strip the quotes
				b, err := json.Marshal(v)
				if err == nil {
					escaped = string(b[1 : len(b)-1]) // strip surrounding quotes
				}
			}
			bodyContent = strings.ReplaceAll(bodyContent, "{{"+k+"}}", escaped)
			// Also replace {{param|default}} variants
			for {
				prefix := "{{" + k + "|"
				idx := strings.Index(bodyContent, prefix)
				if idx < 0 {
					break
				}
				end := strings.Index(bodyContent[idx:], "}}")
				if end < 0 {
					break
				}
				bodyContent = bodyContent[:idx] + escaped + bodyContent[idx+end+2:]
			}
		}
		// Apply remaining defaults for unmatched {{param|default}} placeholders
		bodyContent = applyBodyDefaults(bodyContent)

	} else if len(bodyParams) == 1 {
		for _, v := range bodyParams {
			bodyContent = v
		}
	} else if len(bodyParams) > 1 {
		obj := make(map[string]json.RawMessage, len(bodyParams))
		for k, v := range bodyParams {
			switch paramType[k] {
			case "integer", "number":
				// Pass numeric values unquoted
				obj[k] = json.RawMessage(v)
			case "boolean":
				// Pass boolean values unquoted
				obj[k] = json.RawMessage(v)
			case "json":
				// Pass raw JSON through (arrays, objects)
				obj[k] = json.RawMessage(v)
			default:
				// "string" or unspecified — auto-detect:
				// if valid JSON (array, object, number, bool), pass through;
				// otherwise quote as string
				if json.Valid([]byte(v)) && (v[0] == '[' || v[0] == '{') {
					obj[k] = json.RawMessage(v)
				} else {
					quoted, _ := json.Marshal(v)
					obj[k] = json.RawMessage(quoted)
				}
			}
		}
		data, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("rest provider: building request body for tool %q: %w", toolName, err)
		}
		bodyContent = string(data)
	}

	// Substitute path parameters
	urlPath := def.Path
	for name, value := range pathParams {
		urlPath = strings.ReplaceAll(urlPath, "{{"+name+"}}", url.PathEscape(value))
	}

	// Check for unresolved path placeholders
	if idx := strings.Index(urlPath, "{{"); idx != -1 {
		end := strings.Index(urlPath[idx:], "}}")
		if end != -1 {
			placeholder := urlPath[idx+2 : idx+end]
			return nil, fmt.Errorf("rest provider: unresolved path parameter {{%s}} in tool %q", placeholder, toolName)
		}
	}

	// Build full URL
	fullURL := strings.TrimRight(def.BaseURL, "/")
	if urlPath != "" {
		fullURL += "/" + strings.TrimLeft(urlPath, "/")
	}

	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("rest provider: invalid URL for tool %q: %w", toolName, err)
	}

	q := parsedURL.Query()
	for name, value := range queryParams {
		q.Set(name, value)
	}
	parsedURL.RawQuery = q.Encode()

	// Build request body
	var reqBody io.Reader
	if fileParam != "" {
		// in:file — stream file as raw binary body
		// Resolve to absolute path to prevent ambiguity
		absFile, err := filepath.Abs(fileParam)
		if err != nil {
			return nil, fmt.Errorf("rest provider: invalid file path %q for tool %q: %w", fileParam, toolName, err)
		}
		f, err := os.Open(absFile) // #nosec G304 -- file path is an explicit tool parameter
		if err != nil {
			return nil, fmt.Errorf("rest provider: opening file %q for tool %q: %w", fileParam, toolName, err)
		}
		defer f.Close()
		reqBody = f
	} else if bodyContent != "" {
		reqBody = strings.NewReader(bodyContent)
	}

	// Create request with timeout
	timeout := def.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, def.Method, parsedURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("rest provider: creating request for tool %q: %w", toolName, err)
	}

	// Apply headers: static → auth → param headers → Content-Type default
	for k, v := range def.Headers {
		req.Header.Set(k, v)
	}
	if def.Auth != nil {
		if def.Auth.Type == "oauth" {
			token, err := p.ensureValidToken(def.Auth)
			if err != nil {
				return nil, fmt.Errorf("oauth token for tool %q: %w", toolName, err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			applyAuth(req, def.Auth)
		}
	}
	for name, value := range headerParams {
		req.Header.Set(name, value)
	}
	if bodyContent != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Verbose logging
	if p.Verbose {
		fmt.Fprintf(os.Stderr, "[rest] %s %s\n", req.Method, req.URL)
		if fileParam != "" {
			absFile, _ := filepath.Abs(fileParam)
			fi, _ := os.Stat(absFile) // #nosec G304 -- verbose logging for user-provided file param
			if fi != nil {
				fmt.Fprintf(os.Stderr, "[rest]   file: %s (%d bytes)\n", absFile, fi.Size())
			}
		}
		fmt.Fprintf(os.Stderr, "[rest]   timeout: %s\n", timeout)
	}

	// Execute request
	start := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		if p.Verbose {
			fmt.Fprintf(os.Stderr, "[rest]   error: %s (%s)\n", err, duration.Truncate(time.Millisecond))
		}
		return &Result{
			Error:    err.Error(),
			ExitCode: 1,
			Duration: duration,
		}, nil
	}
	defer resp.Body.Close()

	if p.Verbose {
		fmt.Fprintf(os.Stderr, "[rest]   %d %s (%s)\n", resp.StatusCode, resp.Status, duration.Truncate(time.Millisecond))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Result{
			Error:    fmt.Sprintf("reading response body: %v", err),
			ExitCode: 1,
			Duration: duration,
		}, nil
	}

	result := &Result{
		Output:   string(body),
		Duration: duration,
	}

	if resp.StatusCode >= 400 {
		result.Error = resp.Status
		result.ExitCode = resp.StatusCode
	}

	return result, nil
}

func applyAuth(req *http.Request, auth *AuthDef) {
	switch auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.Token)
	case "basic":
		encoded := base64.StdEncoding.EncodeToString([]byte(auth.Token))
		req.Header.Set("Authorization", "Basic "+encoded)
	case "header":
		req.Header.Set(auth.Header, auth.Value)
	}
}

func (p *RESTProvider) ensureValidToken(auth *AuthDef) (string, error) {
	if p.tokenStore == nil {
		return "", fmt.Errorf("oauth configured but no token store available")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bundle, err := p.tokenStore.GetTokenBundle(auth.TokenKey)
	if err != nil {
		return "", fmt.Errorf("reading token %q: %w (run: factorly auth login)", auth.TokenKey, err)
	}

	if !bundle.IsExpired(30 * time.Second) {
		return bundle.AccessToken, nil
	}

	if bundle.RefreshToken == "" {
		return "", fmt.Errorf("token %q expired and no refresh token available (run: factorly auth login)", auth.TokenKey)
	}

	if auth.OAuthProvider == nil {
		return "", fmt.Errorf("token %q expired but no oauth provider config for refresh", auth.TokenKey)
	}

	newBundle, err := oauth.RefreshAccessToken(
		context.Background(), *auth.OAuthProvider, bundle.RefreshToken,
	)
	if err != nil {
		return "", fmt.Errorf("refreshing token %q: %w (run: factorly auth login)", auth.TokenKey, err)
	}

	if err := p.tokenStore.SetTokenBundle(auth.TokenKey, newBundle); err != nil {
		return "", fmt.Errorf("persisting refreshed token: %w", err)
	}

	return newBundle.AccessToken, nil
}

// applyBodyDefaults resolves remaining {{name|default}} placeholders in a body template.
func applyBodyDefaults(s string) string {
	for {
		start := strings.Index(s, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "}}")
		if end < 0 {
			break
		}
		inner := s[start+2 : start+end]
		pipe := strings.Index(inner, "|")
		if pipe < 0 {
			// No default — skip past this placeholder
			s = s[:start] + "\x00\x00" + s[start+2:]
			continue
		}
		defaultVal := inner[pipe+1:]
		// JSON-escape the default value
		b, err := json.Marshal(defaultVal)
		if err == nil {
			defaultVal = string(b[1 : len(b)-1])
		}
		s = s[:start] + defaultVal + s[start+end+2:]
	}
	s = strings.ReplaceAll(s, "\x00\x00", "{{")
	return s
}

func defaultParamLocation(method string) string {
	switch method {
	case "POST", "PUT", "PATCH":
		return "body"
	default:
		return "query"
	}
}
