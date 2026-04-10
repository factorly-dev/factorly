package registry

import (
	"fmt"
	"sort"
)

type Parameter struct {
	Name        string
	Description string
	Required    bool
}

type Tool struct {
	Name        string
	Type        string
	Description string
	Parameters  []Parameter
	ProviderKey string
	MaxOutput   int
	Compress    []string
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
