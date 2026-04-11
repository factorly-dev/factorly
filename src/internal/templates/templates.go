package templates

import (
	"strings"

	"github.com/factorly-dev/factorly-cli/internal/config"
)

// Template defines a pre-built tool configuration for a service.
type Template struct {
	Name        string
	DisplayName string
	Description string
	Category    string       // "engineering" or "business"
	AuthType    string       // "api_key", "oauth", "bearer"
	AuthGuide   string       // Help text for getting credentials
	VaultKey    string       // Key name in vault (e.g. "LINEAR_API_KEY")
	BaseURL     string
	Headers     map[string]string
	Tools       []ToolDef
	OAuthConfig *OAuthConfig // Required when AuthType is "oauth"
}

// OAuthConfig holds OAuth provider details for templates that require OAuth.
type OAuthConfig struct {
	AuthURL  string
	TokenURL string
	Scopes   []string
}

// ToolDef defines a single tool within a template.
type ToolDef struct {
	Name        string
	Description string
	Method      string // GET, POST, PUT, PATCH, DELETE
	Path        string
	Parameters  []config.ParamConfig
	ActionType  string // "read", "write", "search", "delete"
	Essential   bool   // included in "essentials" selection
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

// ToToolConfigs converts a template's tools into Factorly config tool entries.
// selectedTools filters which tools to include; pass nil or empty to include all.
func (t *Template) ToToolConfigs(selectedTools []string) map[string]config.ToolConfig {
	tools := make(map[string]config.ToolConfig)
	selected := make(map[string]bool)
	for _, s := range selectedTools {
		selected[s] = true
	}

	for _, td := range t.Tools {
		fullName := t.Name + "." + td.Name
		if len(selectedTools) > 0 && !selected[td.Name] && !selected[fullName] {
			continue
		}

		tc := config.ToolConfig{
			Type:        "rest",
			Description: td.Description,
			BaseURL:     t.BaseURL,
			Method:      td.Method,
			Path:        td.Path,
			Headers:     t.Headers,
			Parameters:  td.Parameters,
		}

		// Auth
		switch t.AuthType {
		case "api_key":
			tc.Auth = &config.AuthConfig{
				Type:  "bearer",
				Token: "{{vault:" + t.VaultKey + "}}",
			}
		case "oauth":
			tc.Auth = &config.AuthConfig{
				Type:     "oauth",
				Provider: t.Name,
			}
		case "bearer":
			tc.Auth = &config.AuthConfig{
				Type:  "bearer",
				Token: "{{vault:" + t.VaultKey + "}}",
			}
		}

		// Shadow governance based on action type
		switch td.ActionType {
		case "write", "create":
			tc.Shadow = &config.ShadowConfig{Confirm: true}
		case "delete":
			tc.Shadow = &config.ShadowConfig{Confirm: true}
		}

		tools[fullName] = tc
	}
	return tools
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

// FullConfig generates a complete config.Config with tools and oauth_providers.
func (t *Template) FullConfig(selectedTools []string) *config.Config {
	cfg := &config.Config{
		Tools: t.ToToolConfigs(selectedTools),
	}
	if oauthProviders := t.ToOAuthProvider(); oauthProviders != nil {
		cfg.OAuthProviders = oauthProviders
	}
	return cfg
}

// EssentialTools returns the names of tools marked as essential.
func (t *Template) EssentialTools() []string {
	var names []string
	for _, td := range t.Tools {
		if td.Essential {
			names = append(names, td.Name)
		}
	}
	return names
}
