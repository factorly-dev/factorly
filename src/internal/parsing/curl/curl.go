package curl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
)

// ParsedCurl is the intermediate representation extracted from a curl command.
type ParsedCurl struct {
	Method     string
	URL        string
	Headers    map[string]string
	Body       string
	BasicAuth  string
	IsFormData bool
	FormFields map[string]string
}

// AuthDetection captures a detected auth pattern for vault storage.
type AuthDetection struct {
	Type       string // "bearer", "basic", "header"
	HeaderName string // for "header" type
	RawValue   string // the actual secret
	VaultKey   string // suggested vault key name
}

// Parse parses a curl command string into a ParsedCurl.
func Parse(input string) (*ParsedCurl, error) {
	// Normalize: join backslash continuations
	input = strings.ReplaceAll(input, "\\\n", " ")
	input = strings.ReplaceAll(input, "\\\r\n", " ")
	input = strings.TrimSpace(input)

	// Strip leading $ (common in docs)
	input = strings.TrimPrefix(input, "$ ")

	// Strip leading "curl " prefix
	if strings.HasPrefix(input, "curl ") {
		input = input[5:]
	} else if input == "curl" {
		return nil, fmt.Errorf("no URL provided")
	} else if !strings.HasPrefix(input, "-") && !strings.Contains(input, "://") {
		return nil, fmt.Errorf("input does not look like a curl command")
	}

	tokens := shellSplit(input)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty curl command")
	}

	// If first token is "curl", skip it
	if tokens[0] == "curl" {
		tokens = tokens[1:]
	}

	p := &ParsedCurl{
		Headers:    make(map[string]string),
		FormFields: make(map[string]string),
	}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		switch {
		// Method: -X POST or -XPOST
		case tok == "-X" || tok == "--request":
			if i+1 < len(tokens) {
				i++
				p.Method = strings.ToUpper(tokens[i])
			}
		case strings.HasPrefix(tok, "-X") && len(tok) > 2:
			p.Method = strings.ToUpper(tok[2:])

		// Header: -H "Name: Value"
		case tok == "-H" || tok == "--header":
			if i+1 < len(tokens) {
				i++
				name, value, ok := parseHeader(tokens[i])
				if ok {
					p.Headers[name] = value
				}
			}

		// Data: -d, --data, --data-raw, --data-binary
		case tok == "-d" || tok == "--data" || tok == "--data-raw" || tok == "--data-binary":
			if i+1 < len(tokens) {
				i++
				if p.Body != "" {
					p.Body += "&" + tokens[i]
				} else {
					p.Body = tokens[i]
				}
			}

		// Basic auth: -u user:pass
		case tok == "-u" || tok == "--user":
			if i+1 < len(tokens) {
				i++
				p.BasicAuth = tokens[i]
			}

		// Form data: -F "key=value"
		case tok == "-F" || tok == "--form":
			if i+1 < len(tokens) {
				i++
				p.IsFormData = true
				k, v := parseFormField(tokens[i])
				if k != "" {
					p.FormFields[k] = v
				}
			}

		// Ignored flags that consume next arg
		case tok == "-o" || tok == "--output":
			if i+1 < len(tokens) {
				i++
			}

		// Ignored no-arg flags
		case isIgnoredFlag(tok):
			continue

		// URL (bare positional arg)
		default:
			if strings.Contains(tok, "://") || strings.HasPrefix(tok, "http") {
				p.URL = tok
			}
			// else: ignore unknown flags/args
		}
	}

	if p.URL == "" {
		return nil, fmt.Errorf("no URL found in curl command")
	}

	// Default method
	if p.Method == "" {
		if p.Body != "" || p.IsFormData {
			p.Method = "POST"
		} else {
			p.Method = "GET"
		}
	}

	return p, nil
}

// ToToolConfig converts a ParsedCurl into a Factorly ToolConfig and detects auth.
func ToToolConfig(parsed *ParsedCurl) (config.ToolConfig, *AuthDetection) {
	parsedURL, _ := url.Parse(parsed.URL)
	if parsedURL == nil {
		parsedURL = &url.URL{}
	}

	baseURL := parsedURL.Scheme + "://" + parsedURL.Host
	path := parsedURL.Path

	tool := config.ToolConfig{
		Type:    "rest",
		Method:  parsed.Method,
		BaseURL: baseURL,
		Path:    path,
	}

	// Detect and extract auth
	auth := DetectAuth(parsed.Headers, parsed.BasicAuth)

	// Set auth on tool (with raw value for now — caller replaces with vault ref)
	if auth != nil {
		switch auth.Type {
		case "bearer":
			tool.Auth = &config.AuthConfig{Type: "bearer", Token: auth.RawValue}
		case "basic":
			tool.Auth = &config.AuthConfig{Type: "basic", Token: auth.RawValue}
		case "header":
			tool.Auth = &config.AuthConfig{Type: "header", Header: auth.HeaderName, Value: auth.RawValue}
		}
	}

	// Remove auth headers from static headers
	staticHeaders := make(map[string]string)
	for k, v := range parsed.Headers {
		lower := strings.ToLower(k)
		if lower == "authorization" {
			continue
		}
		if auth != nil && auth.Type == "header" && k == auth.HeaderName {
			continue
		}
		// Skip default Content-Type (REST provider adds it)
		if lower == "content-type" && strings.HasPrefix(strings.ToLower(v), "application/json") {
			continue
		}
		staticHeaders[k] = v
	}
	if len(staticHeaders) > 0 {
		tool.Headers = staticHeaders
	}

	// Parameters from query string
	for key, values := range parsedURL.Query() {
		tool.Parameters = append(tool.Parameters, config.ParamConfig{
			Name: key,
			In:   "query",
		})
		_ = values // use first value as default (not stored in ParamConfig)
	}

	// Parameters from path
	paramPath, pathParams := ParameterizePath(path)
	if paramPath != path {
		tool.Path = paramPath
		tool.Parameters = append(tool.Parameters, pathParams...)
	}

	// Parameters from body
	if parsed.Body != "" {
		bodyParams := parseBodyParams(parsed.Body)
		tool.Parameters = append(tool.Parameters, bodyParams...)
	}

	// Parameters from form fields
	if parsed.IsFormData {
		for k := range parsed.FormFields {
			tool.Parameters = append(tool.Parameters, config.ParamConfig{
				Name:     k,
				In:       "body",
				Required: true,
			})
		}
	}

	return tool, auth
}

// DeriveToolName generates a tool name from the URL and method.
func DeriveToolName(rawURL, method string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "api.request"
	}

	// Extract hostname prefix (e.g., "api.stripe.com" → "stripe")
	host := parsedURL.Hostname()
	parts := strings.Split(host, ".")
	prefix := "api"
	if len(parts) >= 2 {
		// Use the second-to-last part (domain name)
		prefix = parts[len(parts)-2]
	}

	// Slugify the path
	pathSlug := slugify(parsedURL.Path)
	if pathSlug == "" {
		pathSlug = "root"
	}

	method = strings.ToLower(method)
	return prefix + "." + method + "_" + pathSlug
}

// DetectAuth identifies auth patterns in headers and basic auth.
func DetectAuth(headers map[string]string, basicAuth string) *AuthDetection {
	// Check Authorization header
	if authHeader, ok := headers["Authorization"]; ok {
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			return &AuthDetection{
				Type:     "bearer",
				RawValue: token,
				VaultKey: suggestVaultKey(headers, "TOKEN"),
			}
		}
		if strings.HasPrefix(authHeader, "Basic ") {
			value := strings.TrimPrefix(authHeader, "Basic ")
			return &AuthDetection{
				Type:     "basic",
				RawValue: value,
				VaultKey: suggestVaultKey(headers, "BASIC_AUTH"),
			}
		}
	}

	// Check -u basic auth
	if basicAuth != "" {
		return &AuthDetection{
			Type:     "basic",
			RawValue: basicAuth,
			VaultKey: suggestVaultKey(headers, "BASIC_AUTH"),
		}
	}

	// Check common API key headers
	apiKeyHeaders := []string{"X-Api-Key", "X-API-KEY", "Api-Key", "Apikey", "api-key", "x-api-key"}
	for _, name := range apiKeyHeaders {
		if val, ok := headers[name]; ok {
			return &AuthDetection{
				Type:       "header",
				HeaderName: name,
				RawValue:   val,
				VaultKey:   suggestVaultKey(headers, "API_KEY"),
			}
		}
	}

	return nil
}

// ParameterizePath detects ID-like segments and replaces them with {{param}} placeholders.
func ParameterizePath(path string) (string, []config.ParamConfig) {
	segments := strings.Split(path, "/")
	var params []config.ParamConfig
	changed := false

	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if looksLikeID(seg) {
			// Derive param name from previous segment
			paramName := "id"
			if i > 0 && segments[i-1] != "" {
				prev := segments[i-1]
				// Simple singularize: strip trailing "s"
				if strings.HasSuffix(prev, "s") && len(prev) > 1 {
					prev = prev[:len(prev)-1]
				}
				paramName = slugify(prev) + "_id"
			}
			segments[i] = "{{" + paramName + "}}"
			params = append(params, config.ParamConfig{
				Name:     paramName,
				In:       "path",
				Required: true,
			})
			changed = true
		}
	}

	if !changed {
		return path, nil
	}
	return strings.Join(segments, "/"), params
}

// --- Internal helpers ---

func parseHeader(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", "", false
	}
	name := strings.TrimSpace(s[:idx])
	value := strings.TrimSpace(s[idx+1:])
	return name, value, true
}

func parseFormField(s string) (string, string) {
	idx := strings.Index(s, "=")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

func isIgnoredFlag(tok string) bool {
	ignored := map[string]bool{
		"-s": true, "-S": true, "-L": true, "-k": true, "-v": true,
		"--compressed": true, "--silent": true, "--show-error": true,
		"--location": true, "--insecure": true, "--verbose": true,
		"-#": true, "--progress-bar": true, "-i": true, "--include": true,
	}
	return ignored[tok]
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var numericPattern = regexp.MustCompile(`^[0-9]+$`)

func looksLikeID(segment string) bool {
	if uuidPattern.MatchString(segment) {
		return true
	}
	if numericPattern.MatchString(segment) {
		return true
	}
	// Long alphanumeric strings that look like tokens/IDs
	if len(segment) > 20 && regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(segment) {
		return true
	}
	return false
}

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(s string) string {
	s = strings.TrimSpace(s)
	s = nonAlphanumeric.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	s = strings.ToLower(s)
	return s
}

func suggestVaultKey(headers map[string]string, suffix string) string {
	// Try to derive from host in headers or just use generic
	if host, ok := headers["Host"]; ok {
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			return strings.ToUpper(parts[len(parts)-2]) + "_" + suffix
		}
	}
	return suffix
}

func parseBodyParams(body string) []config.ParamConfig {
	// Try JSON
	var jsonMap map[string]any
	if err := json.Unmarshal([]byte(body), &jsonMap); err == nil {
		var params []config.ParamConfig
		for key := range jsonMap {
			params = append(params, config.ParamConfig{
				Name:     key,
				In:       "body",
				Required: true,
			})
		}
		return params
	}

	// Try form-urlencoded (key=value&key=value)
	if strings.Contains(body, "=") {
		values, err := url.ParseQuery(body)
		if err == nil && len(values) > 0 {
			var params []config.ParamConfig
			for key := range values {
				params = append(params, config.ParamConfig{
					Name:     key,
					In:       "body",
					Required: true,
				})
			}
			return params
		}
	}

	// Fallback: single body param
	return []config.ParamConfig{{Name: "body", In: "body", Required: true}}
}

// shellSplit splits a string into tokens respecting single and double quotes.
func shellSplit(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '\\' && inDouble && i+1 < len(s):
			i++
			current.WriteByte(s[i])
		case ch == '\\' && !inSingle && !inDouble && i+1 < len(s):
			i++
			current.WriteByte(s[i])
		case (ch == ' ' || ch == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
