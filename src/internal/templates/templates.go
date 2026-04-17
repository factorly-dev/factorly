// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import (
	"sort"
	"strings"

	"github.com/factorly-dev/factorly/internal/config"
	"gopkg.in/yaml.v3"
)

// Template defines a pre-built tool configuration for a service.
type Template struct {
	Name        string
	DisplayName string
	Description string
	Category    string       // "engineering" or "business"
	AuthType    string       // "api_key", "oauth", "bearer"
	AuthGuide   string       // Help text for getting credentials
	VaultKey    string       // Key name in vault (for api_key/bearer)
	OAuthConfig *OAuthConfig // OAuth provider details (for oauth)
	YAML        string       // Embedded tool definitions
}

// OAuthConfig holds OAuth provider details for templates that require OAuth.
type OAuthConfig struct {
	AuthURL  string
	TokenURL string
	Scopes   []string
}

// All returns all registered templates.
func All() []*Template {
	return []*Template{
		Linear(),
		GitHub(),
		Slack(),
		Stripe(),
		Notion(),
		GoogleSheets(),
		GoogleCalendar(),
		Gmail(),
		GoogleDrive(),
		GoogleDocs(),
		Airtable(),
		Discord(),
		Jira(),
		BigQuery(),
		Coda(),
		HubSpot(),
		Salesforce(),
		Calendly(),
		Intercom(),
		Zendesk(),
		Freshdesk(),
		Shopify(),
		SendGrid(),
		Asana(),
		ClickUp(),
		Trello(),
		Monday(),
		Dropbox(),
		DocuSign(),
		Apollo(),
		Attio(),
		MicrosoftTeams(),
		MicrosoftOutlook(),
		OneDrive(),
		SharePoint(),
		Telegram(),
	}
}

// Get returns a template by name, or nil if not found.
func Get(name string) *Template {
	for _, t := range All() {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// ToolCount returns the number of tools defined in the template's YAML.
func (t *Template) ToolCount() int {
	var tools map[string]any
	if err := yaml.Unmarshal([]byte(t.YAML), &tools); err != nil {
		return 0
	}
	return len(tools)
}

// ToolNames returns the tool names from the template's YAML.
func (t *Template) ToolNames() []string {
	var tools map[string]any
	if err := yaml.Unmarshal([]byte(t.YAML), &tools); err != nil {
		return nil
	}
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FilterYAML returns the YAML content with only the selected tools.
// If selectedTools is nil or empty, returns the full YAML.
func (t *Template) FilterYAML(selectedTools []string) string {
	if len(selectedTools) == 0 {
		return t.YAML
	}

	var allTools map[string]any
	if err := yaml.Unmarshal([]byte(t.YAML), &allTools); err != nil {
		return t.YAML
	}

	selected := make(map[string]bool)
	for _, s := range selectedTools {
		selected[s] = true
	}

	filtered := make(map[string]any)
	for name, def := range allTools {
		if selected[name] {
			filtered[name] = def
		}
	}

	data, err := yaml.Marshal(filtered)
	if err != nil {
		return t.YAML
	}
	return string(data)
}

// ToOAuthProvider generates the oauth_providers config entry for OAuth templates.
// Returns nil if the template does not use OAuth.
func (t *Template) ToOAuthProvider() map[string]config.OAuthProviderConfig {
	if t.AuthType != "oauth" || t.OAuthConfig == nil {
		return nil
	}
	return map[string]config.OAuthProviderConfig{
		t.Name: {
			ClientID:     "{{vault:" + strings.ToUpper(t.Name) + "_CLIENT_ID}}",
			ClientSecret: "{{vault:" + strings.ToUpper(t.Name) + "_CLIENT_SECRET}}",
			AuthURL:      t.OAuthConfig.AuthURL,
			TokenURL:     t.OAuthConfig.TokenURL,
			Scopes:       t.OAuthConfig.Scopes,
		},
	}
}
