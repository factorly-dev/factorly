package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/factorly-hq/factorly-cli/internal/oauth"
)

type RESTParamDef struct {
	Name     string
	In       string // "query", "path", "header", "body"
	Required bool
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
}

func NewREST(tools map[string]RESTToolDef, tokenStore TokenStore) *RESTProvider {
	return &RESTProvider{tools: tools, tokenStore: tokenStore}
}

func (p *RESTProvider) Setup() error {
	p.client = &http.Client{}
	return nil
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

	// Build param index for routing
	paramIndex := make(map[string]string)
	for _, pd := range def.Params {
		paramIndex[pd.Name] = pd.In
	}

	// Route parameters
	pathParams := make(map[string]string)
	queryParams := make(map[string]string)
	headerParams := make(map[string]string)
	var bodyContent string

	for name, value := range params {
		in := paramIndex[name]
		if in == "" {
			in = defaultParamLocation(def.Method)
		}
		switch in {
		case "path":
			pathParams[name] = value
		case "query":
			queryParams[name] = value
		case "header":
			headerParams[name] = value
		case "body":
			bodyContent = value
		default:
			queryParams[name] = value
		}
	}

	// Substitute path parameters
	urlPath := def.Path
	for name, value := range pathParams {
		urlPath = strings.ReplaceAll(urlPath, "{"+name+"}", url.PathEscape(value))
	}

	// Check for unresolved path placeholders
	if idx := strings.Index(urlPath, "{"); idx != -1 {
		end := strings.Index(urlPath[idx:], "}")
		if end != -1 {
			placeholder := urlPath[idx+1 : idx+end]
			return nil, fmt.Errorf("rest provider: unresolved path parameter {%s} in tool %q", placeholder, toolName)
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
	if bodyContent != "" {
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

	// Execute request
	start := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return &Result{
			Error:    err.Error(),
			ExitCode: 1,
			Duration: duration,
		}, nil
	}
	defer resp.Body.Close()

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

func defaultParamLocation(method string) string {
	switch method {
	case "POST", "PUT", "PATCH":
		return "body"
	default:
		return "query"
	}
}
