package templates

import _ "embed"

//go:embed yaml/jira.yaml
var jiraYAML string

// Jira returns the template for Jira.
func Jira() *Template {
	return &Template{
		Name:        "jira",
		DisplayName: "Jira",
		Description: "Manage issues, projects, and workflows in Jira",
		Category:    "engineering",
		AuthType:    "api_key",
		AuthGuide:   "Create an API token at https://id.atlassian.com/manage-profile/security/api-tokens",
		VaultKey:    "JIRA_API_TOKEN",
		YAML:        jiraYAML,
	}
}
