// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

type GenerateOpts struct {
	Prefix  string // tool name prefix, defaults to slugified info.title
	BaseDir string // if set, local file paths are restricted to this directory
}

// Generate reads an OpenAPI 3.x spec (local file or URL) and returns Factorly tool definitions.
func Generate(specPath string, opts GenerateOpts) (map[string]config.ToolConfig, error) {
	data, err := readSpec(specPath, opts.BaseDir)
	if err != nil {
		return nil, err
	}

	// Parse as generic map (handles both YAML and JSON)
	var spec map[string]any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		// Try JSON
		if err2 := json.Unmarshal(data, &spec); err2 != nil {
			return nil, fmt.Errorf("parsing spec (tried YAML and JSON): %w", err)
		}
	}

	prefix := opts.Prefix
	if prefix == "" {
		prefix = extractPrefix(spec)
	}

	baseURL := extractBaseURL(spec)
	auth := extractAuth(spec, prefix)

	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("spec missing 'paths' or invalid format")
	}

	tools := make(map[string]config.ToolConfig)

	for path, pathItem := range paths {
		methods, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for method, operation := range methods {
			method = strings.ToUpper(method)
			if !isHTTPMethod(method) {
				continue
			}

			op, ok := operation.(map[string]any)
			if !ok {
				continue
			}

			toolName := buildToolName(prefix, method, path, op)
			description := extractDescription(op)
			params := extractParameters(op)

			// Merge path-level parameters (shared across methods)
			pathLevelParams := extractParameters(methods)
			params = mergeParams(pathLevelParams, params)

			// Ensure all path placeholders have a corresponding parameter
			params = ensurePathParams(path, params)

			tc := config.ToolConfig{
				Type:        "rest",
				Description: description,
				BaseURL:     baseURL,
				Method:      method,
				Path:        convertPathPlaceholders(path),
				Parameters:  params,
			}
			if auth != nil {
				authCopy := *auth
				tc.Auth = &authCopy
			}

			tools[toolName] = tc
		}
	}

	if len(tools) == 0 {
		return nil, fmt.Errorf("no operations found in spec")
	}

	return tools, nil
}

func extractPrefix(spec map[string]any) string {
	if info, ok := spec["info"].(map[string]any); ok {
		if title, ok := info["title"].(string); ok {
			return slugify(title)
		}
	}
	return "api"
}

func extractBaseURL(spec map[string]any) string {
	if servers, ok := spec["servers"].([]any); ok && len(servers) > 0 {
		if server, ok := servers[0].(map[string]any); ok {
			if url, ok := server["url"].(string); ok {
				return url
			}
		}
	}
	return "https://api.example.com"
}

func extractAuth(spec map[string]any, prefix string) *config.AuthConfig {
	components, ok := spec["components"].(map[string]any)
	if !ok {
		return nil
	}
	schemes, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		return nil
	}

	for _, scheme := range schemes {
		s, ok := scheme.(map[string]any)
		if !ok {
			continue
		}
		schemeType, _ := s["type"].(string)
		switch schemeType {
		case "http":
			if httpScheme, _ := s["scheme"].(string); strings.EqualFold(httpScheme, "bearer") {
				envVar := strings.ToUpper(prefix) + "_TOKEN"
				return &config.AuthConfig{
					Type:  "bearer",
					Token: "{{env:" + envVar + "}}",
				}
			}
		case "apiKey":
			headerName, _ := s["name"].(string)
			if headerName == "" {
				headerName = "X-Api-Key"
			}
			envVar := strings.ToUpper(prefix) + "_API_KEY"
			return &config.AuthConfig{
				Type:   "header",
				Header: headerName,
				Value:  "{{env:" + envVar + "}}",
			}
		}
	}
	return nil
}

func extractDescription(op map[string]any) string {
	if summary, ok := op["summary"].(string); ok && summary != "" {
		return summary
	}
	if desc, ok := op["description"].(string); ok && desc != "" {
		return desc
	}
	return ""
}

func extractParameters(op map[string]any) []config.ParamConfig {
	var params []config.ParamConfig

	if rawParams, ok := op["parameters"].([]any); ok {
		for _, rawParam := range rawParams {
			p, ok := rawParam.(map[string]any)
			if !ok {
				continue
			}
			name, _ := p["name"].(string)
			if name == "" {
				continue
			}
			desc, _ := p["description"].(string)
			required, _ := p["required"].(bool)
			in, _ := p["in"].(string)

			// Path params are always required
			if in == "path" {
				required = true
			}

			params = append(params, config.ParamConfig{
				Name:        name,
				Description: desc,
				Required:    required,
				In:          in,
			})
		}
	}

	// Check for request body
	if reqBody, ok := op["requestBody"].(map[string]any); ok {
		desc := "Request body (JSON)"
		if d, ok := reqBody["description"].(string); ok && d != "" {
			desc = d
		}
		required, _ := reqBody["required"].(bool)
		params = append(params, config.ParamConfig{
			Name:        "body",
			Description: desc,
			Required:    required,
			In:          "body",
		})
	}

	return params
}

// mergeParams merges path-level params with operation-level params.
// Operation-level params take precedence (override by name).
func mergeParams(pathLevel, opLevel []config.ParamConfig) []config.ParamConfig {
	if len(pathLevel) == 0 {
		return opLevel
	}
	seen := make(map[string]bool)
	for _, p := range opLevel {
		seen[p.Name] = true
	}
	var merged []config.ParamConfig
	for _, p := range pathLevel {
		if !seen[p.Name] {
			merged = append(merged, p)
		}
	}
	return append(merged, opLevel...)
}

// ensurePathParams checks for {param} placeholders in the path and adds
// any missing parameters as required path params.
func ensurePathParams(path string, params []config.ParamConfig) []config.ParamConfig {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return params
	}

	existing := make(map[string]bool)
	for _, p := range params {
		existing[p.Name] = true
	}

	for _, m := range matches {
		name := m[1]
		if !existing[name] {
			params = append(params, config.ParamConfig{
				Name:     name,
				Required: true,
				In:       "path",
			})
		}
	}
	return params
}

func buildToolName(prefix, method, path string, op map[string]any) string {
	if opID, ok := op["operationId"].(string); ok && opID != "" {
		// Sanitize: replace / and other non-safe chars with . for dot-separated naming
		safe := strings.ReplaceAll(opID, "/", ".")
		safe = strings.ReplaceAll(safe, " ", "_")
		return prefix + "." + safe
	}
	// Fallback: method_path_slug
	slug := slugify(path)
	return prefix + "." + strings.ToLower(method) + "_" + slug
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(s string) string {
	s = strings.TrimSpace(s)
	s = nonAlphanumeric.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	s = strings.ToLower(s)
	return s
}

// convertPathPlaceholders converts OpenAPI single-brace {param} to Factorly double-brace {{param}}.
func convertPathPlaceholders(path string) string {
	return regexp.MustCompile(`\{([^}]+)\}`).ReplaceAllString(path, "{{$1}}")
}

func isHTTPMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

func readSpec(specPath, baseDir string) ([]byte, error) {
	if strings.HasPrefix(specPath, "http://") || strings.HasPrefix(specPath, "https://") {
		parsed, err := url.Parse(specPath)
		if err != nil {
			return nil, fmt.Errorf("invalid spec URL: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("unsupported URL scheme %q (only http/https allowed)", parsed.Scheme)
		}
		if err := checkSpecURL(parsed); err != nil {
			return nil, err
		}
		resp, err := http.Get(parsed.String()) //nolint:gosec,noctx // user-provided URL is intentional for spec import
		if err != nil {
			return nil, fmt.Errorf("fetching spec from %s: %w", specPath, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching spec from %s: %s", specPath, resp.Status)
		}
		// Limit read to 10MB to prevent resource exhaustion
		data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			return nil, fmt.Errorf("reading spec from %s: %w", specPath, err)
		}
		return data, nil
	}
	// File path: resolve to absolute and verify it exists
	absPath, err := filepath.Abs(specPath)
	if err != nil {
		return nil, fmt.Errorf("resolving spec path: %w", err)
	}
	// When baseDir is set, restrict reads to that directory.
	if baseDir != "" {
		absBase, err := filepath.Abs(baseDir)
		if err != nil {
			return nil, fmt.Errorf("resolving base dir: %w", err)
		}
		// EvalSymlinks resolves symlink traversal before the prefix check.
		resolved, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("reading spec: %w", err)
		}
		if !strings.HasPrefix(resolved, absBase+string(filepath.Separator)) && resolved != absBase {
			return nil, fmt.Errorf("spec path %q is outside the project directory", specPath)
		}
	}
	data, err := os.ReadFile(absPath) // #nosec G304 -- validated against baseDir above when called from UI
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}
	return data, nil
}

// checkSpecURL blocks requests to internal/private network targets.
func checkSpecURL(u *url.URL) error {
	host := u.Hostname()

	// Block cloud metadata endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return fmt.Errorf("spec URL blocked: cloud metadata endpoint")
	}

	// Block localhost/loopback
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1" {
		return fmt.Errorf("spec URL blocked: localhost access denied")
	}

	// Block private/link-local IPs
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("spec URL blocked: private network access denied")
		}
	}

	return nil
}
