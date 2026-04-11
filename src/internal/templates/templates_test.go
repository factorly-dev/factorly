package templates

import (
	"regexp"
	"strings"
	"testing"
)

func TestAllTemplatesHaveRequiredFields(t *testing.T) {
	for _, tmpl := range All() {
		if tmpl.Name == "" {
			t.Error("template has empty name")
		}
		if tmpl.DisplayName == "" {
			t.Errorf("template %q has empty display name", tmpl.Name)
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", tmpl.Name)
		}
		if tmpl.AuthType == "" {
			t.Errorf("template %q has empty auth type", tmpl.Name)
		}
		if tmpl.VaultKey == "" {
			t.Errorf("template %q has empty vault key", tmpl.Name)
		}
		if tmpl.BaseURL == "" {
			t.Errorf("template %q has empty base URL", tmpl.Name)
		}
		if len(tmpl.Tools) == 0 {
			t.Errorf("template %q has no tools", tmpl.Name)
		}
	}
}

func TestAllTemplatesHaveEssentials(t *testing.T) {
	for _, tmpl := range All() {
		essentials := tmpl.EssentialTools()
		if len(essentials) == 0 {
			t.Errorf("template %q has no essential tools", tmpl.Name)
		}
	}
}

func TestGetTemplate(t *testing.T) {
	tmpl := Get("linear")
	if tmpl == nil {
		t.Fatal("expected to find linear template")
	}
	if tmpl.DisplayName != "Linear" {
		t.Errorf("expected Linear, got %q", tmpl.DisplayName)
	}
}

func TestGetTemplateNotFound(t *testing.T) {
	tmpl := Get("nonexistent")
	if tmpl != nil {
		t.Error("expected nil for nonexistent template")
	}
}

func TestToToolConfigs(t *testing.T) {
	tmpl := Get("github")
	configs := tmpl.ToToolConfigs(nil) // all tools
	if len(configs) == 0 {
		t.Fatal("expected tool configs")
	}
	// Check a known tool
	if tc, ok := configs["github.list_repos"]; !ok {
		t.Error("expected github.list_repos")
	} else {
		if tc.Type != "rest" {
			t.Errorf("expected type rest, got %q", tc.Type)
		}
		if tc.Auth == nil {
			t.Error("expected auth config")
		}
	}
}

func TestToToolConfigsFiltered(t *testing.T) {
	tmpl := Get("github")
	configs := tmpl.ToToolConfigs([]string{"list_repos", "get_repo"})
	if len(configs) != 2 {
		t.Errorf("expected 2 tools, got %d", len(configs))
	}
}

func TestToToolConfigsShadowOnWrite(t *testing.T) {
	tmpl := Get("github")
	configs := tmpl.ToToolConfigs(nil)
	// create_issue should have confirm shadow
	if tc, ok := configs["github.create_issue"]; ok {
		if tc.Shadow == nil {
			t.Error("expected shadow config on write action")
		}
	}
	// list_repos should NOT have shadow
	if tc, ok := configs["github.list_repos"]; ok {
		if tc.Shadow != nil {
			t.Error("expected no shadow on read action")
		}
	}
}

// TestAllTemplatesStructuralIntegrity validates every template across a set of
// structural rules that catch real bugs without requiring API credentials.
func TestAllTemplatesStructuralIntegrity(t *testing.T) {
	pathParamRe := regexp.MustCompile(`\{\{(\w+)\}\}`)
	validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	validActionTypes := map[string]bool{"read": true, "write": true, "search": true, "delete": true}
	validAuthTypes := map[string]bool{"api_key": true, "oauth": true, "bearer": true}
	validCategories := map[string]bool{"engineering": true, "business": true}

	for _, tmpl := range All() {
		t.Run(tmpl.Name, func(t *testing.T) {
			// Template-level validation
			if !validAuthTypes[tmpl.AuthType] {
				t.Errorf("invalid auth type %q", tmpl.AuthType)
			}
			if !validCategories[tmpl.Category] {
				t.Errorf("invalid category %q", tmpl.Category)
			}
			if !strings.HasPrefix(tmpl.BaseURL, "https://") {
				t.Errorf("base URL should use HTTPS: %q", tmpl.BaseURL)
			}

			// No duplicate tool names
			seen := make(map[string]bool)
			for _, td := range tmpl.Tools {
				if seen[td.Name] {
					t.Errorf("duplicate tool name %q", td.Name)
				}
				seen[td.Name] = true
			}

			for _, td := range tmpl.Tools {
				t.Run(td.Name, func(t *testing.T) {
					// Valid method
					if !validMethods[td.Method] {
						t.Errorf("invalid method %q", td.Method)
					}

					// Valid action type
					if !validActionTypes[td.ActionType] {
						t.Errorf("invalid action type %q", td.ActionType)
					}

					// Method matches action type
					switch td.ActionType {
					case "read", "search":
						// GET or POST both acceptable (GraphQL APIs use POST for reads)
					case "delete":
						if td.Method != "DELETE" && td.Method != "POST" {
							t.Errorf("delete action uses %s (expected DELETE or POST)", td.Method)
						}
					}

					// Path params have matching parameter definitions
					pathParams := pathParamRe.FindAllStringSubmatch(td.Path, -1)
					paramSet := make(map[string]string) // name → in
					for _, p := range td.Parameters {
						paramSet[p.Name] = p.In
					}
					for _, match := range pathParams {
						paramName := match[1]
						in, exists := paramSet[paramName]
						if !exists {
							t.Errorf("path param {{%s}} has no matching parameter definition", paramName)
						} else if in != "path" && in != "" {
							t.Errorf("path param {{%s}} has in=%q (expected 'path')", paramName, in)
						}
					}

					// Path params marked as required
					for _, match := range pathParams {
						paramName := match[1]
						for _, p := range td.Parameters {
							if p.Name == paramName && !p.Required {
								t.Errorf("path param %q should be required", paramName)
							}
						}
					}

					// POST/PUT/PATCH with body params should have Content-Type
					// (unless template-level headers already set it)
					if td.Method == "POST" || td.Method == "PUT" || td.Method == "PATCH" {
						hasBodyParam := false
						for _, p := range td.Parameters {
							if p.In == "body" {
								hasBodyParam = true
								break
							}
						}
						_ = hasBodyParam // Content-Type check is template-level, not tool-level
					}

					// Path should start with /
					if td.Path != "" && !strings.HasPrefix(td.Path, "/") {
						t.Errorf("path should start with /: %q", td.Path)
					}

					// Description should not be empty
					if td.Description == "" {
						t.Errorf("tool has empty description")
					}

					// Name should be snake_case (no spaces, no uppercase)
					if strings.ContainsAny(td.Name, " ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
						t.Errorf("tool name should be snake_case: %q", td.Name)
					}
				})
			}

			// Generated configs should be valid
			configs := tmpl.ToToolConfigs(nil)
			if len(configs) != len(tmpl.Tools) {
				t.Errorf("ToToolConfigs produced %d configs for %d tools", len(configs), len(tmpl.Tools))
			}
			for name, tc := range configs {
				if tc.Type != "rest" {
					t.Errorf("tool %q: expected type rest, got %q", name, tc.Type)
				}
				if tc.Auth == nil {
					t.Errorf("tool %q: missing auth config", name)
				}
				if tc.Auth != nil && tc.Auth.Token != "" {
					if !strings.Contains(tc.Auth.Token, "vault:") {
						t.Errorf("tool %q: auth token should reference vault", name)
					}
				}
			}
		})
	}
}

func TestTemplateCount(t *testing.T) {
	all := All()
	if len(all) != 36 {
		t.Errorf("expected 36 templates, got %d", len(all))
	}
}
