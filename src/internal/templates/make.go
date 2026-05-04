// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/make.yaml
var makeYAML string

// Make returns the template for common make targets.
func Make() *Template {
	return &Template{
		Name:        "make",
		DisplayName: "Make",
		Description: "Common make targets with output filtering",
		Category:    "engineering",
		AuthType:    "none",
		VaultKey:    "",
		YAML:        makeYAML,
	}
}
