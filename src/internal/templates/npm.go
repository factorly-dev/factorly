// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/npm.yaml
var npmYAML string

// Npm returns the template for common npm commands.
func Npm() *Template {
	return &Template{
		Name:        "npm",
		DisplayName: "npm",
		Description: "Common npm commands with output filtering",
		Category:    "engineering",
		AuthType:    "none",
		VaultKey:    "",
		YAML:        npmYAML,
	}
}
