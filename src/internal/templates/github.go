package templates

import _ "embed"

//go:embed yaml/github.yaml
var githubYAML string

// GitHub returns the template for GitHub.
func GitHub() *Template {
	return &Template{
		Name:        "github",
		DisplayName: "GitHub",
		Description: "Code hosting, issues, pull requests, and repositories",
		Category:    "engineering",
		AuthType:    "bearer",
		AuthGuide:   "Create a token at https://github.com/settings/tokens",
		VaultKey:    "GITHUB_TOKEN",
		YAML:        githubYAML,
	}
}
