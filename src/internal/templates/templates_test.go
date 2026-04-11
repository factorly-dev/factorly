package templates

import "testing"

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

func TestTemplateCount(t *testing.T) {
	all := All()
	if len(all) != 5 {
		t.Errorf("expected 5 templates, got %d", len(all))
	}
}
