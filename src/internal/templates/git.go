// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/git.yaml
var gitYAML string

// Git returns the template for common git CLI operations.
func Git() *Template {
	return &Template{
		Name:        "git",
		DisplayName: "Git",
		Description: "Common git operations with governance and output filtering",
		Category:    "engineering",
		AuthType:    "none",
		VaultKey:    "",
		YAML:        gitYAML,
	}
}
