// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package registry

import (
	"fmt"
	"sort"

	"github.com/factorly-dev/factorly/internal/output"
)

type Parameter struct {
	Name        string
	Description string
	Required    bool
	Type        string // "string" (default), "integer", "number", "boolean", "json"
	Default     string
	// Validation rules
	Min       *float64
	Max       *float64
	MinLength *int
	MaxLength *int
	Pattern   string
	Enum      []string
}

type Tool struct {
	Name           string
	Type           string
	Description    string
	Parameters     []Parameter
	ProviderKey    string
	MaxOutput      int
	Compress       []string
	AllowOverrides []string       // allow overrides for built-in tool guards
	Filter         *output.Filter // per-tool output filter
	VaultKeys      []string       // vault keys this tool accesses (e.g., "vault:GITHUB_TOKEN")
}

// ValidateParams checks that all required parameters (without defaults) are present.
func (t *Tool) ValidateParams(params map[string]string) error {
	for _, pd := range t.Parameters {
		if pd.Required && pd.Default == "" {
			if _, ok := params[pd.Name]; !ok {
				return fmt.Errorf("required parameter %q missing for tool %q", pd.Name, t.Name)
			}
		}
	}
	return nil
}

type Registry struct {
	tools map[string]*Tool
}

func New() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

func (r *Registry) Register(tool *Tool) {
	r.tools[tool.Name] = tool
}

func (r *Registry) Get(name string) (*Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

func (r *Registry) List() []*Tool {
	tools := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}
