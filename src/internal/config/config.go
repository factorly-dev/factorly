package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Tools map[string]ToolConfig `yaml:"tools"`
}

type ToolConfig struct {
	Type        string            `yaml:"type"`
	Description string            `yaml:"description"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Parameters  []ParamConfig     `yaml:"parameters"`
}

type ParamConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_-]*)\}`)

func Load(path string) (*Config, error) {
	if path == "" {
		path = "factorly.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	resolveEnvVars(&cfg)
	inferParameters(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func resolveEnvVars(cfg *Config) {
	for name, tool := range cfg.Tools {
		tool.Command = resolveString(tool.Command)
		tool.Description = resolveString(tool.Description)
		for i, arg := range tool.Args {
			tool.Args[i] = resolveString(arg)
		}
		for k, v := range tool.Env {
			tool.Env[k] = resolveString(v)
		}
		cfg.Tools[name] = tool
	}
}

func resolveString(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func inferParameters(cfg *Config) {
	for name, tool := range cfg.Tools {
		if len(tool.Parameters) > 0 {
			continue
		}
		seen := make(map[string]bool)
		for _, arg := range tool.Args {
			matches := placeholderPattern.FindAllStringSubmatch(arg, -1)
			for _, m := range matches {
				paramName := m[1]
				if !seen[paramName] {
					seen[paramName] = true
					tool.Parameters = append(tool.Parameters, ParamConfig{
						Name:     paramName,
						Required: true,
					})
				}
			}
		}
		cfg.Tools[name] = tool
	}
}

func validate(cfg *Config) error {
	if len(cfg.Tools) == 0 {
		return fmt.Errorf("config: no tools defined")
	}
	validTypes := map[string]bool{"cli": true, "mcp": true, "rest": true}
	for name, tool := range cfg.Tools {
		if tool.Type == "" {
			return fmt.Errorf("config: tool %q missing type", name)
		}
		if !validTypes[tool.Type] {
			return fmt.Errorf("config: tool %q has unknown type %q", name, tool.Type)
		}
		if tool.Type == "cli" && tool.Command == "" {
			return fmt.Errorf("config: cli tool %q missing command", name)
		}
	}
	return nil
}

func FindConfig() string {
	candidates := []string{
		"factorly.yaml",
		"factorly.yml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := home + "/.config/factorly/factorly.yaml"
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "factorly.yaml"
}

// ParamNames returns the list of parameter names for a tool config.
func (tc *ToolConfig) ParamNames() []string {
	names := make([]string, len(tc.Parameters))
	for i, p := range tc.Parameters {
		names[i] = p.Name
	}
	return names
}

// HasPlaceholder checks if any arg contains {name} style placeholders.
func HasPlaceholder(args []string, name string) bool {
	target := "{" + name + "}"
	for _, arg := range args {
		if strings.Contains(arg, target) {
			return true
		}
	}
	return false
}
