// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

type GenerateOpts struct {
	Prefix string // tool name prefix, defaults to slugified info.title
}

// Generate reads an OpenAPI 3.x spec (local file or URL) and returns Factorly tool definitions.
func Generate(specPath string, opts GenerateOpts) (map[string]config.ToolConfig, error) {
	data, err := readSpec(specPath)
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

func buildToolName(prefix, method, path string, op map[string]any) string {
	if opID, ok := op["operationId"].(string); ok && opID != "" {
		return prefix + "." + opID
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

func readSpec(specPath string) ([]byte, error) {
	if strings.HasPrefix(specPath, "http://") || strings.HasPrefix(specPath, "https://") {
		resp, err := http.Get(specPath) //nolint:gosec // user-provided URL is intentional
		if err != nil {
			return nil, fmt.Errorf("fetching spec from %s: %w", specPath, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching spec from %s: %s", specPath, resp.Status)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading spec from %s: %w", specPath, err)
		}
		return data, nil
	}
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}
	return data, nil
}
